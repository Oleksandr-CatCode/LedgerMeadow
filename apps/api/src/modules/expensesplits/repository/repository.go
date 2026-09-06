package repository

import (
	"context"
	"errors"
	"fmt"
	"math"

	"ledgermeadow/src/entities"
	"ledgermeadow/src/modules/expensesplits/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound            = errors.New("transaction not found")
	ErrNotExpense          = errors.New("only expense transactions can be split")
	ErrHouseholdRequired   = errors.New("an active household is required")
	ErrInvalidParticipants = errors.New("split participants must belong to the payer household")
	ErrAmountsMismatch     = errors.New("split shares do not equal the transaction amount")
	ErrExistingBound       = errors.New("existing expense split exceeds the configured bound")
)

type Repository struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Replace(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
	command models.Replace,
) (models.Split, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return models.Split{}, fmt.Errorf("begin expense split replacement: %w", err)
	}
	defer tx.Rollback(ctx)

	var amountMinor int64
	var currency, visibility string
	err = tx.QueryRow(ctx, `
		SELECT amount_minor, currency, visibility
		FROM transactions
		WHERE id = $1 AND user_id = $2 AND removed_at IS NULL
		FOR UPDATE
	`, string(transactionID), string(userID)).Scan(&amountMinor, &currency, &visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Split{}, ErrNotFound
	}
	if err != nil {
		return models.Split{}, fmt.Errorf("lock split transaction: %w", err)
	}
	if amountMinor >= 0 || amountMinor == math.MinInt64 {
		return models.Split{}, ErrNotExpense
	}
	totalMinor := -amountMinor
	participantTotal := int64(0)
	participantIDs := make([]string, 0, len(command.Participants))
	for _, participant := range command.Participants {
		if participant.UserID == string(userID) {
			return models.Split{}, ErrInvalidParticipants
		}
		participantAmount := int64(participant.AmountMinor)
		if participantAmount <= 0 || participantTotal > math.MaxInt64-participantAmount {
			return models.Split{}, ErrAmountsMismatch
		}
		participantTotal += participantAmount
		participantIDs = append(participantIDs, participant.UserID)
	}
	if command.PayerAmountMinor > totalMinor || participantTotal != totalMinor-command.PayerAmountMinor {
		return models.Split{}, ErrAmountsMismatch
	}

	var householdID string
	err = tx.QueryRow(ctx, `
		SELECT household_id::text
		FROM household_members
		WHERE user_id = $1
		FOR SHARE
	`, string(userID)).Scan(&householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Split{}, ErrHouseholdRequired
	}
	if err != nil {
		return models.Split{}, fmt.Errorf("load payer household: %w", err)
	}
	var participantCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM household_members
		WHERE household_id = $1 AND user_id = ANY($2::uuid[])
	`, householdID, participantIDs).Scan(&participantCount); err != nil {
		return models.Split{}, fmt.Errorf("validate split participants: %w", err)
	}
	if participantCount != len(command.Participants) {
		return models.Split{}, ErrInvalidParticipants
	}

	oldRows, err := tx.Query(ctx, `
		SELECT participant_user_id::text, amount_minor
		FROM expense_splits
		WHERE transaction_id = $1 AND payer_user_id = $2
		ORDER BY participant_user_id
		FOR UPDATE
		LIMIT 21
	`, string(transactionID), string(userID))
	if err != nil {
		return models.Split{}, fmt.Errorf("load existing expense split: %w", err)
	}
	type oldParticipant struct {
		userID      string
		amountMinor int64
	}
	oldParticipants := make([]oldParticipant, 0, 20)
	for oldRows.Next() {
		var participant oldParticipant
		if err := oldRows.Scan(&participant.userID, &participant.amountMinor); err != nil {
			oldRows.Close()
			return models.Split{}, fmt.Errorf("scan existing expense split: %w", err)
		}
		oldParticipants = append(oldParticipants, participant)
	}
	if err := oldRows.Err(); err != nil {
		oldRows.Close()
		return models.Split{}, fmt.Errorf("iterate existing expense split: %w", err)
	}
	oldRows.Close()
	if len(oldParticipants) > 20 {
		return models.Split{}, ErrExistingBound
	}

	if visibility != "HOUSEHOLD" {
		if err := activateHouseholdExpense(ctx, tx, householdID, string(userID), currency, totalMinor); err != nil {
			return models.Split{}, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE transactions SET visibility = 'HOUSEHOLD', updated_at = now()
			WHERE id = $1 AND user_id = $2
		`, string(transactionID), string(userID)); err != nil {
			return models.Split{}, fmt.Errorf("share split transaction: %w", err)
		}
	}

	oldParticipantTotal := int64(0)
	for _, participant := range oldParticipants {
		if participant.amountMinor <= 0 || oldParticipantTotal > math.MaxInt64-participant.amountMinor {
			return models.Split{}, errors.New("existing expense split amount is invalid")
		}
		oldParticipantTotal += participant.amountMinor
		if err := changeOwed(ctx, tx, householdID, participant.userID, currency, -participant.amountMinor); err != nil {
			return models.Split{}, err
		}
	}
	if oldParticipantTotal > 0 {
		if err := changeOwed(ctx, tx, householdID, string(userID), currency, oldParticipantTotal); err != nil {
			return models.Split{}, err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM expense_splits WHERE transaction_id = $1`, string(transactionID)); err != nil {
		return models.Split{}, fmt.Errorf("clear existing expense split: %w", err)
	}

	if participantTotal > 0 {
		if err := changeOwed(ctx, tx, householdID, string(userID), currency, -participantTotal); err != nil {
			return models.Split{}, err
		}
	}
	for _, participant := range command.Participants {
		if _, err := tx.Exec(ctx, `
			INSERT INTO expense_splits (
				transaction_id, payer_user_id, participant_user_id, amount_minor, status
			) VALUES ($1, $2, $3, $4, 'PENDING')
		`, string(transactionID), string(userID), participant.UserID, int64(participant.AmountMinor)); err != nil {
			return models.Split{}, fmt.Errorf("create expense split participant: %w", err)
		}
		if err := changeOwed(ctx, tx, householdID, participant.UserID, currency, int64(participant.AmountMinor)); err != nil {
			return models.Split{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE household_member_summaries
		SET settlement_minor = NULL, updated_at = now()
		WHERE household_id = $1 AND currency = $2
	`, householdID, currency); err != nil {
		return models.Split{}, fmt.Errorf("invalidate household settlement: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id, metadata)
		VALUES ($1, 'EXPENSE_SPLIT_REPLACED', $2, $3,
		        jsonb_build_object('participant_count', $4::integer))
	`, string(userID), string(entities.Transaction), string(transactionID), len(command.Participants)); err != nil {
		return models.Split{}, fmt.Errorf("audit expense split replacement: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Split{}, fmt.Errorf("commit expense split replacement: %w", err)
	}

	participants := make([]models.Participant, 0, len(command.Participants))
	for _, participant := range command.Participants {
		participant.Status = "PENDING"
		participants = append(participants, participant)
	}
	return models.Split{
		TransactionID: string(transactionID), PayerUserID: string(userID), Currency: currency,
		TotalMinor: shared.MinorUnits(totalMinor), PayerAmountMinor: shared.MinorUnits(command.PayerAmountMinor),
		Participants: participants,
	}, nil
}

func (r *Repository) Get(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) (models.Split, error) {
	var amountMinor int64
	var currency string
	err := r.db.QueryRow(ctx, `
		SELECT amount_minor, currency
		FROM transactions
		WHERE id = $1 AND user_id = $2 AND removed_at IS NULL
	`, string(transactionID), string(userID)).Scan(&amountMinor, &currency)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Split{}, ErrNotFound
	}
	if err != nil {
		return models.Split{}, fmt.Errorf("load split transaction: %w", err)
	}
	if amountMinor >= 0 || amountMinor == math.MinInt64 {
		return models.Split{}, ErrNotExpense
	}
	totalMinor := -amountMinor
	split := models.Split{
		TransactionID: string(transactionID), PayerUserID: string(userID), Currency: currency,
		TotalMinor: shared.MinorUnits(totalMinor), PayerAmountMinor: shared.MinorUnits(totalMinor),
		Participants: make([]models.Participant, 0),
	}
	rows, err := r.db.Query(ctx, `
		SELECT participant_user_id::text, amount_minor, status
		FROM expense_splits
		WHERE transaction_id = $1 AND payer_user_id = $2
		ORDER BY participant_user_id
		LIMIT 21
	`, string(transactionID), string(userID))
	if err != nil {
		return models.Split{}, fmt.Errorf("load expense split: %w", err)
	}
	defer rows.Close()
	participantTotal := int64(0)
	for rows.Next() {
		var participant models.Participant
		if err := rows.Scan(&participant.UserID, &participant.AmountMinor, &participant.Status); err != nil {
			return models.Split{}, fmt.Errorf("scan expense split: %w", err)
		}
		participantAmount := int64(participant.AmountMinor)
		if participantAmount <= 0 || participantTotal > math.MaxInt64-participantAmount {
			return models.Split{}, errors.New("stored expense split amount is invalid")
		}
		participantTotal += participantAmount
		split.Participants = append(split.Participants, participant)
	}
	if err := rows.Err(); err != nil {
		return models.Split{}, fmt.Errorf("iterate expense split: %w", err)
	}
	if len(split.Participants) > 20 {
		return models.Split{}, ErrExistingBound
	}
	if participantTotal > totalMinor {
		return models.Split{}, errors.New("stored expense split exceeds transaction amount")
	}
	split.PayerAmountMinor = shared.MinorUnits(totalMinor - participantTotal)
	return split, nil
}

func (r *Repository) Clear(
	ctx context.Context,
	userID shared.UserID,
	transactionID shared.TransactionID,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin expense split clear: %w", err)
	}
	defer tx.Rollback(ctx)
	var amountMinor int64
	var currency, visibility string
	err = tx.QueryRow(ctx, `
		SELECT amount_minor, currency, visibility
		FROM transactions
		WHERE id = $1 AND user_id = $2 AND removed_at IS NULL
		FOR UPDATE
	`, string(transactionID), string(userID)).Scan(&amountMinor, &currency, &visibility)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock split transaction: %w", err)
	}
	if amountMinor >= 0 || amountMinor == math.MinInt64 {
		return ErrNotExpense
	}
	if visibility != "HOUSEHOLD" {
		return nil
	}
	var householdID string
	err = tx.QueryRow(ctx, `
		SELECT household_id::text FROM household_members WHERE user_id = $1 FOR SHARE
	`, string(userID)).Scan(&householdID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrHouseholdRequired
	}
	if err != nil {
		return fmt.Errorf("load payer household: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT participant_user_id::text, amount_minor
		FROM expense_splits
		WHERE transaction_id = $1 AND payer_user_id = $2
		ORDER BY participant_user_id
		FOR UPDATE
		LIMIT 21
	`, string(transactionID), string(userID))
	if err != nil {
		return fmt.Errorf("load expense split to clear: %w", err)
	}
	type participantShare struct {
		userID      string
		amountMinor int64
	}
	participants := make([]participantShare, 0, 20)
	participantTotal := int64(0)
	for rows.Next() {
		var participant participantShare
		if err := rows.Scan(&participant.userID, &participant.amountMinor); err != nil {
			rows.Close()
			return fmt.Errorf("scan expense split to clear: %w", err)
		}
		if participant.amountMinor <= 0 || participantTotal > math.MaxInt64-participant.amountMinor {
			rows.Close()
			return errors.New("stored expense split amount is invalid")
		}
		participantTotal += participant.amountMinor
		participants = append(participants, participant)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate expense split to clear: %w", err)
	}
	rows.Close()
	if len(participants) > 20 {
		return ErrExistingBound
	}
	for _, participant := range participants {
		if err := changeOwed(ctx, tx, householdID, participant.userID, currency, -participant.amountMinor); err != nil {
			return err
		}
	}
	if participantTotal > 0 {
		if err := changeOwed(ctx, tx, householdID, string(userID), currency, participantTotal); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM expense_splits WHERE transaction_id = $1`, string(transactionID)); err != nil {
		return fmt.Errorf("clear expense split: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE household_member_summaries
		SET settlement_minor = NULL, updated_at = now()
		WHERE household_id = $1 AND currency = $2
	`, householdID, currency); err != nil {
		return fmt.Errorf("invalidate household settlement: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (actor_user_id, event_type, entity_type, entity_id)
		VALUES ($1, 'EXPENSE_SPLIT_CLEARED', $2, $3)
	`, string(userID), string(entities.Transaction), string(transactionID)); err != nil {
		return fmt.Errorf("audit expense split clear: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit expense split clear: %w", err)
	}
	return nil
}

func activateHouseholdExpense(
	ctx context.Context,
	tx pgx.Tx,
	householdID string,
	userID string,
	currency string,
	amountMinor int64,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO household_summaries (household_id, currency, total_spending_minor)
		VALUES ($1, $2, $3)
		ON CONFLICT (household_id, currency) DO UPDATE
		SET total_spending_minor = household_summaries.total_spending_minor + EXCLUDED.total_spending_minor,
		    updated_at = now()
	`, householdID, currency, amountMinor); err != nil {
		return fmt.Errorf("update household spending summary: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO household_member_summaries (
			household_id, user_id, currency, paid_minor, owed_minor, difference_minor
		) VALUES ($1, $2, $3, $4, $4, 0)
		ON CONFLICT (household_id, user_id, currency) DO UPDATE
		SET paid_minor = household_member_summaries.paid_minor + EXCLUDED.paid_minor,
		    owed_minor = household_member_summaries.owed_minor + EXCLUDED.owed_minor,
		    difference_minor = (household_member_summaries.paid_minor + EXCLUDED.paid_minor)
		                     - (household_member_summaries.owed_minor + EXCLUDED.owed_minor),
		    settlement_minor = NULL,
		    updated_at = now()
	`, householdID, userID, currency, amountMinor); err != nil {
		return fmt.Errorf("update household payer summary: %w", err)
	}
	return nil
}

func changeOwed(
	ctx context.Context,
	tx pgx.Tx,
	householdID string,
	userID string,
	currency string,
	delta int64,
) error {
	if delta >= 0 {
		if _, err := tx.Exec(ctx, `
			INSERT INTO household_member_summaries (
				household_id, user_id, currency, owed_minor, difference_minor
			) VALUES ($1, $2, $3, $4::bigint, -($4::bigint))
			ON CONFLICT (household_id, user_id, currency) DO UPDATE
			SET owed_minor = household_member_summaries.owed_minor + EXCLUDED.owed_minor,
			    difference_minor = household_member_summaries.paid_minor
			                     - (household_member_summaries.owed_minor + EXCLUDED.owed_minor),
			    settlement_minor = NULL,
			    updated_at = now()
		`, householdID, userID, currency, delta); err != nil {
			return fmt.Errorf("increase household member owed amount: %w", err)
		}
		return nil
	}
	result, err := tx.Exec(ctx, `
		UPDATE household_member_summaries
		SET owed_minor = owed_minor + $4,
		    difference_minor = paid_minor - (owed_minor + $4),
		    settlement_minor = NULL,
		    updated_at = now()
		WHERE household_id = $1 AND user_id = $2 AND currency = $3
		  AND owed_minor >= -($4::bigint)
	`, householdID, userID, currency, delta)
	if err != nil {
		return fmt.Errorf("decrease household member owed amount: %w", err)
	}
	if result.RowsAffected() != 1 {
		return errors.New("household owed summary is inconsistent")
	}
	return nil
}
