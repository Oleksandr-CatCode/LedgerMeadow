package repository

import (
	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/inbox/models"
	"ledgermeadow/src/modules/platform/changes"
	shared "ledgermeadow/src/shared/types"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"strconv"
	"time"
)

var ErrInvalidCursor = errors.New("invalid inbox cursor")
var ErrNotFound = errors.New("inbox item not found")
var ErrUnsupportedResolution = errors.New("unsupported inbox resolution")

type cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}
type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }
func (r *Repository) List(ctx context.Context, u shared.UserID, encoded string, limit int) (models.Page, error) {
	if limit < 1 || limit > 100 {
		return models.Page{}, ErrInvalidCursor
	}
	var c *cursor
	if encoded != "" {
		raw, e := base64.RawURLEncoding.DecodeString(encoded)
		if e != nil {
			return models.Page{}, ErrInvalidCursor
		}
		var v cursor
		if e := json.Unmarshal(raw, &v); e != nil || v.ID == "" {
			return models.Page{}, ErrInvalidCursor
		}
		c = &v
	}
	var created any
	var id any
	if c != nil {
		created = c.CreatedAt
		id = c.ID
	}
	rows, e := r.db.Query(ctx, `
		SELECT item.id::text,item.item_type,item.priority,item.entity_type,item.entity_id::text,
		       (item.payload-'confidence_basis_points')
		       || CASE WHEN item.payload ? 'expected_amount_minor'
		               THEN jsonb_build_object('expected_amount_minor',item.payload->>'expected_amount_minor')
		               ELSE '{}'::jsonb END
		       || jsonb_strip_nulls(jsonb_build_object(
		           'entity_name',COALESCE(subscription.merchant_name,bill.name,income.name,
		                                tx_item.merchant_name,tx_item.name,connection.institution_name),
		           'transaction_date',tx_item.transaction_date,
		           'amount_minor',tx_item.amount_minor::text,
		           'currency',tx_item.currency
		       )),item.status,item.created_at
		FROM inbox_items item
		LEFT JOIN subscriptions subscription ON item.item_type='POSSIBLE_SUBSCRIPTION'
		 AND subscription.id=item.entity_id AND subscription.user_id=item.user_id
		LEFT JOIN bills bill ON item.item_type='POSSIBLE_RECURRING_PAYMENT'
		 AND bill.id=item.entity_id AND bill.user_id=item.user_id
		LEFT JOIN recurring_income_sources income ON item.item_type='POSSIBLE_RECURRING_INCOME'
		 AND income.id=item.entity_id AND income.user_id=item.user_id
		LEFT JOIN transactions tx_item ON item.entity_type='TRANSACTION'
		 AND tx_item.id=item.entity_id AND tx_item.user_id=item.user_id AND tx_item.removed_at IS NULL
		LEFT JOIN bank_connections connection ON item.entity_type='BANK_CONNECTION'
		 AND connection.id=item.entity_id AND connection.user_id=item.user_id
		WHERE item.user_id=$1 AND item.status='OPEN'
		  AND ($2::timestamptz IS NULL OR(item.created_at,item.id)<($2::timestamptz,$3::uuid))
		ORDER BY item.created_at DESC,item.id DESC LIMIT $4
	`, string(u), created, id, limit+1)
	if e != nil {
		return models.Page{}, fmt.Errorf("list inbox: %w", e)
	}
	defer rows.Close()
	items := make([]models.Item, 0, limit+1)
	for rows.Next() {
		var i models.Item
		if e := rows.Scan(&i.ID, &i.Type, &i.Priority, &i.EntityType, &i.EntityID, &i.Payload, &i.Status, &i.CreatedAt); e != nil {
			return models.Page{}, fmt.Errorf("scan inbox item: %w", e)
		}
		items = append(items, i)
	}
	if e := rows.Err(); e != nil {
		return models.Page{}, fmt.Errorf("iterate inbox: %w", e)
	}
	p := models.Page{Items: items}
	if len(items) > limit {
		p.Items = items[:limit]
		last := p.Items[len(p.Items)-1]
		raw, _ := json.Marshal(cursor{CreatedAt: last.CreatedAt, ID: last.ID})
		p.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return p, nil
}
func (r *Repository) Resolve(ctx context.Context, u shared.UserID, id string, resolution string) error {
	tx, e := r.db.Begin(ctx)
	if e != nil {
		return fmt.Errorf("begin inbox resolve: %w", e)
	}
	defer tx.Rollback(ctx)
	var itemType string
	var entityType string
	var entityID *string
	var payload json.RawMessage
	e = tx.QueryRow(ctx, `SELECT item_type,entity_type,entity_id::text,payload FROM inbox_items WHERE id=$1 AND user_id=$2 AND status='OPEN' FOR UPDATE`, id, string(u)).Scan(&itemType, &entityType, &entityID, &payload)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if e != nil {
		return fmt.Errorf("lock inbox item: %w", e)
	}
	status, eventType, e := applyResolution(ctx, tx, u, id, itemType, entityType, entityID, payload, resolution)
	if e != nil {
		return e
	}
	if _, e := tx.Exec(ctx, `UPDATE inbox_items SET status=$3,resolved_at=now() WHERE id=$1 AND user_id=$2`, id, string(u), status); e != nil {
		return fmt.Errorf("resolve inbox item: %w", e)
	}
	if _, e := tx.Exec(ctx, `UPDATE users SET open_inbox_count=GREATEST(open_inbox_count-1,0),updated_at=now() WHERE id=$1`, string(u)); e != nil {
		return fmt.Errorf("update inbox summary: %w", e)
	}
	if _, e := tx.Exec(ctx, `INSERT INTO audit_events(actor_user_id,event_type,entity_type,entity_id,metadata)VALUES($1,$2,$3,$4,jsonb_build_object('resolution',$5::text,'item_type',$6::text))`, string(u), eventType, string(entities.InboxItem), id, resolution, itemType); e != nil {
		return fmt.Errorf("audit inbox resolve: %w", e)
	}
	if e := changes.Notify(ctx, tx, u, changes.ResourceInbox, changes.ResourceActivity); e != nil {
		return fmt.Errorf("notify inbox resolve: %w", e)
	}
	if e := tx.Commit(ctx); e != nil {
		return fmt.Errorf("commit inbox resolve: %w", e)
	}
	return nil
}

func applyResolution(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	inboxID string,
	itemType string,
	entityType string,
	entityID *string,
	payload json.RawMessage,
	resolution string,
) (string, string, error) {
	switch {
	case itemType == "TRANSACTION_REVIEW" && resolution == "CONFIRMED",
		itemType == "UNUSUAL_TRANSACTION" && resolution == "LOOKS_CORRECT",
		itemType == "POSSIBLE_SHARED_EXPENSE" && resolution == "LOOKS_CORRECT":
		if entityID == nil {
			return "", "", ErrUnsupportedResolution
		}
		result, err := tx.Exec(ctx, `UPDATE transactions SET review_status='REVIEWED',updated_at=now() WHERE id=$1 AND user_id=$2 AND removed_at IS NULL`, *entityID, string(userID))
		if err != nil {
			return "", "", fmt.Errorf("apply inbox transaction review: %w", err)
		}
		if result.RowsAffected() != 1 {
			return "", "", ErrUnsupportedResolution
		}
		return "RESOLVED", "TRANSACTION_REVIEWED", nil
	case (itemType == "POSSIBLE_SUBSCRIPTION" || itemType == "POSSIBLE_RECURRING_PAYMENT" || itemType == "POSSIBLE_RECURRING_INCOME") &&
		(resolution == "CONFIRMED" || resolution == "NOT_RECURRING"):
		if entityID == nil {
			return "", "", ErrUnsupportedResolution
		}
		table := "subscriptions"
		if itemType == "POSSIBLE_RECURRING_PAYMENT" {
			table = "bills"
		} else if itemType == "POSSIBLE_RECURRING_INCOME" {
			table = "recurring_income_sources"
		}
		nextStatus := "ACTIVE"
		inboxStatus := "RESOLVED"
		eventType := "RECURRING_CANDIDATE_CONFIRMED"
		if resolution == "NOT_RECURRING" {
			nextStatus = "CANCELLED"
			inboxStatus = "DISMISSED"
			eventType = "RECURRING_CANDIDATE_REJECTED"
		}
		query := fmt.Sprintf("UPDATE %s SET status=$3,user_modified_at=now(),updated_at=now() WHERE id=$1 AND user_id=$2 RETURNING detection_key", table)
		var detectionKey *string
		err := tx.QueryRow(ctx, query, *entityID, string(userID), nextStatus).Scan(&detectionKey)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrUnsupportedResolution
		}
		if err != nil {
			return "", "", fmt.Errorf("apply recurring candidate resolution: %w", err)
		}
		if resolution == "NOT_RECURRING" && detectionKey != nil {
			if _, err := tx.Exec(ctx, `
				UPDATE transactions SET recurring_detection_key=NULL,updated_at=now()
				WHERE user_id=$1 AND recurring_detection_key=$2 AND removed_at IS NULL
			`, string(userID), *detectionKey); err != nil {
				return "", "", fmt.Errorf("clear rejected recurring transaction links: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)`, string(userID), string(entities.User)); err != nil {
			return "", "", fmt.Errorf("enqueue recurring candidate materialization: %w", err)
		}
		return inboxStatus, eventType, nil
	case itemType == "PRICE_CHANGE" && (resolution == "CONFIRMED" || resolution == "DISMISSED"):
		if resolution == "DISMISSED" {
			return "DISMISSED", "PRICE_CHANGE_ACKNOWLEDGED", nil
		}
		if entityID == nil {
			return "", "", ErrUnsupportedResolution
		}
		var change struct {
			EntityKind          string   `json:"entity_kind"`
			PreviousAmountMinor string   `json:"previous_amount_minor"`
			LatestAmountMinor   string   `json:"latest_amount_minor"`
			ObservationIDs      []string `json:"amount_observation_ids"`
		}
		if err := json.Unmarshal(payload, &change); err != nil || change.EntityKind != entityType || len(change.ObservationIDs) == 0 {
			return "", "", ErrUnsupportedResolution
		}
		previousAmount, err := strconv.ParseInt(change.PreviousAmountMinor, 10, 64)
		if err != nil || previousAmount <= 0 {
			return "", "", ErrUnsupportedResolution
		}
		latestAmount, err := strconv.ParseInt(change.LatestAmountMinor, 10, 64)
		if err != nil || latestAmount <= 0 || latestAmount == previousAmount {
			return "", "", ErrUnsupportedResolution
		}
		var result pgconn.CommandTag
		switch entityType {
		case "SUBSCRIPTION":
			result, err = tx.Exec(ctx, `UPDATE subscriptions SET expected_amount_minor=$3,user_modified_at=now(),updated_at=now() WHERE id=$1 AND user_id=$2 AND status='ACTIVE' AND expected_amount_minor=$4`, *entityID, string(userID), latestAmount, previousAmount)
		case "BILL":
			result, err = tx.Exec(ctx, `UPDATE bills SET expected_amount_minor=$3,user_modified_at=now(),updated_at=now() WHERE id=$1 AND user_id=$2 AND status='ACTIVE' AND expected_amount_minor=$4`, *entityID, string(userID), latestAmount, previousAmount)
		default:
			return "", "", ErrUnsupportedResolution
		}
		if err != nil {
			return "", "", fmt.Errorf("apply recurring amount change: %w", err)
		}
		if result.RowsAffected() != 1 {
			return "", "", ErrUnsupportedResolution
		}
		if _, err := tx.Exec(ctx, `
			UPDATE transactions SET recurring_detection_key=entity.detection_key,updated_at=now()
			FROM (
				SELECT detection_key FROM subscriptions WHERE $3='SUBSCRIPTION' AND id=$1 AND user_id=$2
				UNION ALL
				SELECT detection_key FROM bills WHERE $3='BILL' AND id=$1 AND user_id=$2
			) entity
			WHERE transactions.user_id=$2 AND transactions.removed_at IS NULL
			  AND transactions.id=ANY($4::uuid[])
		`, *entityID, string(userID), entityType, change.ObservationIDs); err != nil {
			return "", "", fmt.Errorf("link recurring amount observations: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload)VALUES($1,$2,$1,'FINANCIAL_MATERIALIZATION_REQUESTED',gen_random_uuid()::text,'{}'::jsonb)`, string(userID), string(entities.User)); err != nil {
			return "", "", fmt.Errorf("enqueue recurring amount materialization: %w", err)
		}
		return "RESOLVED", "RECURRING_AMOUNT_CHANGE_CONFIRMED", nil
	case itemType == "BUDGET_WARNING" && resolution == "DISMISSED",
		itemType == "UPCOMING_SHORTFALL" && resolution == "DISMISSED":
		return "DISMISSED", "FINANCIAL_WARNING_ACKNOWLEDGED", nil
	case itemType == "BANK_CONNECTION_ERROR" && resolution == "CONFIRMED":
		if entityID == nil {
			return "", "", ErrUnsupportedResolution
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bank_connections WHERE id=$1 AND user_id=$2)`, *entityID, string(userID)).Scan(&exists); err != nil {
			return "", "", fmt.Errorf("validate inbox bank connection: %w", err)
		}
		if !exists {
			return "", "", ErrUnsupportedResolution
		}
		if _, err := tx.Exec(ctx, `INSERT INTO outbox_events(user_id,aggregate_type,aggregate_id,event_type,dedupe_key,payload) VALUES($1,'BANK_CONNECTION',$2,'BANK_CONNECTION_SYNC_REQUESTED',$3,jsonb_build_object('connection_id',$2::text)) ON CONFLICT(dedupe_key)DO NOTHING`, string(userID), *entityID, "inbox-reconnect:"+inboxID); err != nil {
			return "", "", fmt.Errorf("queue inbox bank reconnect: %w", err)
		}
		return "RESOLVED", "BANK_RECONNECT_REQUESTED", nil
	default:
		return "", "", ErrUnsupportedResolution
	}
}
