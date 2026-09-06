package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/rules/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const listLimit = 100

var ErrTooMany = errors.New("rule response exceeds bound")
var ErrNotFound = errors.New("rule not found")
var ErrInvalidOrder = errors.New("rule reorder must include every owned rule exactly once")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) List(ctx context.Context, userID shared.UserID) ([]models.Rule, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, priority, enabled, conditions, actions, match_count, last_run_at
		FROM rules WHERE user_id = $1 ORDER BY priority, id LIMIT $2
	`, string(userID), listLimit+1)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	items := make([]models.Rule, 0, listLimit)
	for rows.Next() {
		var item models.Rule
		if err := rows.Scan(&item.ID, &item.Name, &item.Priority, &item.Enabled, &item.Conditions, &item.Actions, &item.MatchCount, &item.LastRunAt); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	if len(items) > listLimit {
		return nil, ErrTooMany
	}
	return items, nil
}

func (r *Repository) Create(ctx context.Context, userID shared.UserID, command models.Create) (string, error) {
	conditions, err := json.Marshal(command.Conditions)
	if err != nil {
		return "", fmt.Errorf("encode rule conditions: %w", err)
	}
	actions, err := json.Marshal(command.Actions)
	if err != nil {
		return "", fmt.Errorf("encode rule actions: %w", err)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin rule create: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, string(userID)); err != nil {
		return "", fmt.Errorf("lock rule owner: %w", err)
	}
	var priority int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(priority), 0) + 1 FROM rules WHERE user_id = $1`, string(userID)).Scan(&priority); err != nil {
		return "", fmt.Errorf("allocate rule priority: %w", err)
	}
	if priority > listLimit {
		return "", ErrTooMany
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO rules (user_id, name, priority, enabled, conditions, actions)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id::text
	`, string(userID), strings.TrimSpace(command.Name), priority, command.Enabled, conditions, actions).Scan(&id); err != nil {
		return "", fmt.Errorf("create rule: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id) VALUES ($1, 'RULE_CREATED', $2, $3)`, string(userID), string(entities.Rule), id); err != nil {
		return "", fmt.Errorf("audit rule create: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit rule create: %w", err)
	}
	return id, nil
}

func (r *Repository) Update(ctx context.Context, userID shared.UserID, id string, command models.Update) error {
	var conditions any
	if command.Conditions != nil {
		encoded, err := json.Marshal(*command.Conditions)
		if err != nil {
			return fmt.Errorf("encode updated rule conditions: %w", err)
		}
		conditions = encoded
	}
	var actions any
	if command.Actions != nil {
		encoded, err := json.Marshal(*command.Actions)
		if err != nil {
			return fmt.Errorf("encode updated rule actions: %w", err)
		}
		actions = encoded
	}
	var name any
	if command.Name != nil {
		name = strings.TrimSpace(*command.Name)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rule update: %w", err)
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE rules SET name=COALESCE($3,name),enabled=COALESCE($4,enabled),conditions=COALESCE($5,conditions),actions=COALESCE($6,actions),updated_at=now() WHERE user_id=$1 AND id=$2`, string(userID), id, name, command.Enabled, conditions, actions)
	if err != nil {
		return fmt.Errorf("update rule: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrNotFound
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id) VALUES($1,'RULE_UPDATED',$2,$3)`, string(userID), string(entities.Rule), id); err != nil {
		return fmt.Errorf("audit rule update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rule update: %w", err)
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, userID shared.UserID, id string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rule delete: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, string(userID)); err != nil {
		return fmt.Errorf("lock rule owner: %w", err)
	}
	var priority int
	if err := tx.QueryRow(ctx, `DELETE FROM rules WHERE user_id=$1 AND id=$2 RETURNING priority`, string(userID), id).Scan(&priority); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	var maximumPriority int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(priority),0) FROM rules WHERE user_id=$1`, string(userID)).Scan(&maximumPriority); err != nil {
		return fmt.Errorf("load maximum rule priority: %w", err)
	}
	if maximumPriority > 1000000 {
		return ErrInvalidOrder
	}
	offset := maximumPriority + 1
	if _, err := tx.Exec(ctx, `UPDATE rules SET priority=priority+$3,updated_at=now() WHERE user_id=$1 AND priority>$2`, string(userID), priority, offset); err != nil {
		return fmt.Errorf("stage rule priority compaction: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE rules SET priority=priority-$2-1 WHERE user_id=$1 AND priority>$2`, string(userID), offset); err != nil {
		return fmt.Errorf("compact rule priorities: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id) VALUES($1,'RULE_DELETED',$2,$3)`, string(userID), string(entities.Rule), id); err != nil {
		return fmt.Errorf("audit rule delete: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rule delete: %w", err)
	}
	return nil
}

func (r *Repository) Duplicate(ctx context.Context, userID shared.UserID, id string) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin rule duplicate: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, string(userID)); err != nil {
		return "", fmt.Errorf("lock rule owner: %w", err)
	}
	var name string
	var enabled bool
	var conditions, actions []byte
	if err := tx.QueryRow(ctx, `SELECT name,enabled,conditions,actions FROM rules WHERE user_id=$1 AND id=$2`, string(userID), id).Scan(&name, &enabled, &conditions, &actions); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	} else if err != nil {
		return "", fmt.Errorf("load rule to duplicate: %w", err)
	}
	var priority int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(priority),0)+1 FROM rules WHERE user_id=$1`, string(userID)).Scan(&priority); err != nil {
		return "", fmt.Errorf("allocate duplicate rule priority: %w", err)
	}
	if priority > listLimit {
		return "", ErrTooMany
	}
	duplicateNameRunes := []rune(strings.TrimSpace(name) + " copy")
	if len(duplicateNameRunes) > 120 {
		duplicateNameRunes = duplicateNameRunes[:120]
	}
	duplicateName := string(duplicateNameRunes)
	var duplicateID string
	if err := tx.QueryRow(ctx, `INSERT INTO rules(user_id,name,priority,enabled,conditions,actions) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, string(userID), duplicateName, priority, enabled, conditions, actions).Scan(&duplicateID); err != nil {
		return "", fmt.Errorf("duplicate rule: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id) VALUES($1,'RULE_DUPLICATED',$2,$3)`, string(userID), string(entities.Rule), duplicateID); err != nil {
		return "", fmt.Errorf("audit rule duplicate: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit rule duplicate: %w", err)
	}
	return duplicateID, nil
}

func (r *Repository) Reorder(ctx context.Context, userID shared.UserID, orderedIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin rule reorder: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, string(userID)); err != nil {
		return fmt.Errorf("lock rule owner: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT id::text FROM rules WHERE user_id=$1 ORDER BY priority,id LIMIT $2`, string(userID), listLimit+1)
	if err != nil {
		return fmt.Errorf("load rule reorder set: %w", err)
	}
	current := make(map[string]struct{}, listLimit)
	for rows.Next() {
		var currentID string
		if err := rows.Scan(&currentID); err != nil {
			rows.Close()
			return fmt.Errorf("scan rule reorder set: %w", err)
		}
		current[currentID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate rule reorder set: %w", err)
	}
	rows.Close()
	if len(current) != len(orderedIDs) || len(current) > listLimit {
		return ErrInvalidOrder
	}
	seen := make(map[string]struct{}, len(orderedIDs))
	for _, orderedID := range orderedIDs {
		if _, exists := current[orderedID]; !exists {
			return ErrInvalidOrder
		}
		if _, duplicate := seen[orderedID]; duplicate {
			return ErrInvalidOrder
		}
		seen[orderedID] = struct{}{}
	}
	var maximumPriority int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(priority),0) FROM rules WHERE user_id=$1`, string(userID)).Scan(&maximumPriority); err != nil {
		return fmt.Errorf("load maximum rule priority: %w", err)
	}
	if maximumPriority > 1000000 {
		return ErrInvalidOrder
	}
	offset := maximumPriority + len(orderedIDs) + 1
	if _, err := tx.Exec(ctx, `UPDATE rules SET priority=priority+$2,updated_at=now() WHERE user_id=$1`, string(userID), offset); err != nil {
		return fmt.Errorf("stage rule reorder: %w", err)
	}
	for index, orderedID := range orderedIDs {
		result, err := tx.Exec(ctx, `UPDATE rules SET priority=$3 WHERE user_id=$1 AND id=$2`, string(userID), orderedID, index+1)
		if err != nil {
			return fmt.Errorf("set rule priority: %w", err)
		}
		if result.RowsAffected() != 1 {
			return ErrInvalidOrder
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id) VALUES($1,'RULES_REORDERED',$2,NULL)`, string(userID), string(entities.Rule)); err != nil {
		return fmt.Errorf("audit rule reorder: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit rule reorder: %w", err)
	}
	return nil
}
