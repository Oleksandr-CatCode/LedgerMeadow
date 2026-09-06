package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/household/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrAlreadyMember  = errors.New("user already belongs to household")
	ErrTooManyMembers = errors.New("household member response exceeds bound")
	ErrResponseBound  = errors.New("household aggregate response exceeds bound")
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Get(ctx context.Context, userID shared.UserID) (*models.Household, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin household read: %w", err)
	}
	defer tx.Rollback(ctx)

	var household models.Household
	err = tx.QueryRow(ctx, `
		SELECT h.id::text, h.name
		FROM household_members hm
		JOIN households h ON h.id = hm.household_id
		WHERE hm.user_id = $1
		LIMIT 1
	`, string(userID)).Scan(&household.ID, &household.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load household: %w", err)
	}
	if err := loadMembers(ctx, tx, &household); err != nil {
		return nil, err
	}
	if err := loadMemberSummaries(ctx, tx, &household); err != nil {
		return nil, err
	}
	if err := loadSpendingSummaries(ctx, tx, &household); err != nil {
		return nil, err
	}
	if err := loadSharedSpaces(ctx, tx, &household); err != nil {
		return nil, err
	}
	if err := loadSharedTransactions(ctx, tx, &household); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit household read: %w", err)
	}
	return &household, nil
}

func loadMembers(ctx context.Context, tx pgx.Tx, household *models.Household) error {
	rows, err := tx.Query(ctx, `
		SELECT hm.user_id::text, COALESCE(u.display_name, u.email, 'Member'), hm.role
		FROM household_members hm
		JOIN users u ON u.id = hm.user_id
		WHERE hm.household_id = $1
		ORDER BY hm.joined_at, hm.user_id
		LIMIT 101
	`, household.ID)
	if err != nil {
		return fmt.Errorf("load household members: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var member models.Member
		if err := rows.Scan(&member.UserID, &member.DisplayName, &member.Role); err != nil {
			return fmt.Errorf("scan household member: %w", err)
		}
		member.Summaries = make([]models.MemberSummary, 0)
		household.Members = append(household.Members, member)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate household members: %w", err)
	}
	if len(household.Members) > 100 {
		return ErrTooManyMembers
	}
	return nil
}

func loadMemberSummaries(ctx context.Context, tx pgx.Tx, household *models.Household) error {
	memberIndexes := make(map[string]int, len(household.Members))
	for index, member := range household.Members {
		memberIndexes[member.UserID] = index
	}
	rows, err := tx.Query(ctx, `
		SELECT hms.user_id::text, hms.currency, hms.paid_minor, hms.owed_minor,
		       hms.difference_minor, hms.settlement_minor
		FROM household_member_summaries hms
		JOIN household_members hm
		  ON hm.household_id = hms.household_id AND hm.user_id = hms.user_id
		WHERE hms.household_id = $1
		ORDER BY hms.user_id, hms.currency
		LIMIT 201
	`, household.ID)
	if err != nil {
		return fmt.Errorf("load household member summaries: %w", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var summary models.MemberSummary
		var summaryUserID string
		if err := rows.Scan(&summaryUserID, &summary.Currency, &summary.PaidMinor, &summary.OwedMinor,
			&summary.DifferenceMinor, &summary.SettlementMinor); err != nil {
			return fmt.Errorf("scan household member summary: %w", err)
		}
		if index, exists := memberIndexes[summaryUserID]; exists {
			household.Members[index].Summaries = append(household.Members[index].Summaries, summary)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate household member summaries: %w", err)
	}
	if count > 200 {
		return ErrResponseBound
	}
	return nil
}

func loadSpendingSummaries(ctx context.Context, tx pgx.Tx, household *models.Household) error {
	rows, err := tx.Query(ctx, `
		SELECT currency, total_spending_minor
		FROM household_summaries
		WHERE household_id = $1
		ORDER BY currency
		LIMIT 3
	`, household.ID)
	if err != nil {
		return fmt.Errorf("load household spending summaries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var summary models.SpendingSummary
		if err := rows.Scan(&summary.Currency, &summary.TotalSpendingMinor); err != nil {
			return fmt.Errorf("scan household spending summary: %w", err)
		}
		household.SpendingSummaries = append(household.SpendingSummaries, summary)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate household spending summaries: %w", err)
	}
	if len(household.SpendingSummaries) > 2 {
		return ErrResponseBound
	}
	return nil
}

func loadSharedSpaces(ctx context.Context, tx pgx.Tx, household *models.Household) error {
	rows, err := tx.Query(ctx, `
		SELECT id::text, name, space_type, currency,
		       CASE
		           WHEN balance_period_start = date_trunc('month', CURRENT_DATE)::date THEN balance_minor
		           ELSE monthly_allocation_minor
		       END
		FROM spaces
		WHERE household_id = $1 AND visibility = 'HOUSEHOLD'
		ORDER BY created_at, id
		LIMIT 101
	`, household.ID)
	if err != nil {
		return fmt.Errorf("load shared Spaces: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var space models.SharedSpace
		if err := rows.Scan(&space.ID, &space.Name, &space.Type, &space.Currency, &space.BalanceMinor); err != nil {
			return fmt.Errorf("scan shared Space: %w", err)
		}
		household.SharedSpaces = append(household.SharedSpaces, space)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate shared Spaces: %w", err)
	}
	if len(household.SharedSpaces) > 100 {
		return ErrResponseBound
	}
	return nil
}

func loadSharedTransactions(ctx context.Context, tx pgx.Tx, household *models.Household) error {
	rows, err := tx.Query(ctx, `
		SELECT t.id::text, t.name, t.merchant_name, t.amount_minor, t.currency,
		       t.transaction_date, t.user_id::text,
		       COALESCE(payer.display_name, payer.email, 'Member')
		FROM transactions t
		JOIN household_members payer_member
		  ON payer_member.user_id = t.user_id AND payer_member.household_id = $1
		JOIN users payer ON payer.id = t.user_id
		WHERE t.visibility = 'HOUSEHOLD' AND t.removed_at IS NULL
		ORDER BY t.transaction_date DESC, t.id DESC
		LIMIT 51
	`, household.ID)
	if err != nil {
		return fmt.Errorf("load shared transactions: %w", err)
	}
	transactionIndexes := make(map[string]int, 50)
	transactionIDs := make([]string, 0, 50)
	for rows.Next() {
		var transaction models.SharedTransaction
		var date time.Time
		if err := rows.Scan(&transaction.ID, &transaction.Name, &transaction.MerchantName,
			&transaction.AmountMinor, &transaction.Currency, &date, &transaction.PayerUserID,
			&transaction.PayerDisplayName); err != nil {
			rows.Close()
			return fmt.Errorf("scan shared transaction: %w", err)
		}
		transaction.Date = date.Format("2006-01-02")
		transaction.Participants = make([]models.SplitParticipant, 0)
		if int64(transaction.AmountMinor) < 0 && int64(transaction.AmountMinor) != math.MinInt64 {
			transaction.PayerAmountMinor = shared.MinorUnits(-int64(transaction.AmountMinor))
		}
		transactionIndexes[transaction.ID] = len(household.SharedTransactions)
		transactionIDs = append(transactionIDs, transaction.ID)
		household.SharedTransactions = append(household.SharedTransactions, transaction)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate shared transactions: %w", err)
	}
	rows.Close()
	if len(household.SharedTransactions) > 50 {
		return ErrResponseBound
	}
	if len(transactionIDs) == 0 {
		return nil
	}

	rows, err = tx.Query(ctx, `
		SELECT es.transaction_id::text, es.participant_user_id::text,
		       COALESCE(participant.display_name, participant.email, 'Member'),
		       es.amount_minor, es.status
		FROM expense_splits es
		JOIN household_members participant_member
		  ON participant_member.user_id = es.participant_user_id
		 AND participant_member.household_id = $1
		JOIN users participant ON participant.id = es.participant_user_id
		WHERE es.transaction_id = ANY($2::uuid[])
		ORDER BY es.transaction_id, es.participant_user_id
		LIMIT 1001
	`, household.ID, transactionIDs)
	if err != nil {
		return fmt.Errorf("load shared transaction splits: %w", err)
	}
	defer rows.Close()
	splitCount := 0
	for rows.Next() {
		var participant models.SplitParticipant
		var transactionID string
		if err := rows.Scan(&transactionID, &participant.UserID, &participant.DisplayName,
			&participant.AmountMinor, &participant.Status); err != nil {
			return fmt.Errorf("scan shared transaction split: %w", err)
		}
		if index, exists := transactionIndexes[transactionID]; exists {
			household.SharedTransactions[index].Participants = append(
				household.SharedTransactions[index].Participants, participant,
			)
			household.SharedTransactions[index].PayerAmountMinor -= participant.AmountMinor
		}
		splitCount++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate shared transaction splits: %w", err)
	}
	if splitCount > 1000 {
		return ErrResponseBound
	}
	return nil
}

func (r *Repository) Create(ctx context.Context, userID shared.UserID, name string) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin household create: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, string(userID)); err != nil {
		return "", fmt.Errorf("lock household creator: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM household_members WHERE user_id = $1)
	`, string(userID)).Scan(&exists); err != nil {
		return "", fmt.Errorf("check household membership: %w", err)
	}
	if exists {
		return "", ErrAlreadyMember
	}
	var id string
	if err := tx.QueryRow(ctx, `
		INSERT INTO households (name, created_by) VALUES ($1, $2) RETURNING id::text
	`, strings.TrimSpace(name), string(userID)).Scan(&id); err != nil {
		return "", fmt.Errorf("create household: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO household_members (household_id, user_id, role) VALUES ($1, $2, 'OWNER')
	`, id, string(userID)); err != nil {
		return "", fmt.Errorf("add household owner: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id)
		VALUES ($1, 'HOUSEHOLD_CREATED', $2, $3)
	`, string(userID), string(entities.Household), id); err != nil {
		return "", fmt.Errorf("audit household create: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit household create: %w", err)
	}
	return id, nil
}
