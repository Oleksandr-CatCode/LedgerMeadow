package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"ledgermeadow/src/entities"
	categoryrepo "ledgermeadow/src/modules/categories/repository"
	"ledgermeadow/src/modules/transactionanalysis/models"
	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrInputBound = errors.New("transaction analysis input exceeds bound")

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Load(ctx context.Context, userID shared.UserID, asOf time.Time) (models.Input, error) {
	var timezone string
	if err := r.db.QueryRow(ctx, `SELECT timezone FROM users WHERE id=$1`, string(userID)).Scan(&timezone); err != nil {
		return models.Input{}, fmt.Errorf("load transaction analysis timezone: %w", err)
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return models.Input{}, errors.New("stored user timezone is invalid")
	}
	asOf = asOf.In(location)
	rows, err := r.db.Query(ctx, `
		SELECT t.id::text, t.account_id::text, t.name, t.merchant_name,
		       t.original_description, t.amount_minor, t.currency,
		       t.transaction_date, t.provider_category_primary,
		       t.provider_category_detailed,t.categorization_learning_key,
		       t.category_id::text,category.category_type,t.category_source,
		       CASE WHEN account.account_type IN ('credit', 'loan') THEN 'LIABILITY' ELSE 'ASSET' END
		FROM transactions t
		JOIN accounts account ON account.id = t.account_id AND account.user_id = t.user_id
		LEFT JOIN categories category ON category.id = t.category_id
		WHERE t.user_id = $1 AND t.removed_at IS NULL AND NOT t.is_pending
		  AND t.transaction_date >= $2::date - interval '24 months'
		  AND t.transaction_date <= $2::date
		ORDER BY t.transaction_date DESC, t.id DESC
		LIMIT $3
	`, string(userID), asOf, models.MaxTransactions+1)
	if err != nil {
		return models.Input{}, fmt.Errorf("load transaction analysis history: %w", err)
	}
	defer rows.Close()

	transactions := make([]models.Transaction, 0, models.MaxTransactions)
	for rows.Next() {
		var transaction models.Transaction
		if err := rows.Scan(
			&transaction.ID, &transaction.AccountID, &transaction.Name,
			&transaction.MerchantName, &transaction.OriginalDescription,
			&transaction.AmountMinor, &transaction.Currency, &transaction.Date,
			&transaction.ProviderCategoryPrimary, &transaction.ProviderCategoryDetailed,
			&transaction.LearningKey, &transaction.CategoryID, &transaction.CategoryType,
			&transaction.CategorySource, &transaction.AccountKind,
		); err != nil {
			return models.Input{}, fmt.Errorf("scan transaction analysis history: %w", err)
		}
		transactions = append(transactions, transaction)
	}
	if err := rows.Err(); err != nil {
		return models.Input{}, fmt.Errorf("iterate transaction analysis history: %w", err)
	}
	if len(transactions) > models.MaxTransactions {
		return models.Input{}, ErrInputBound
	}
	categories, err := r.loadCategories(ctx, userID)
	if err != nil {
		return models.Input{}, err
	}
	evidence, prevailingFrequency, err := r.loadRecurringEvidence(ctx, userID)
	if err != nil {
		return models.Input{}, err
	}
	return models.Input{
		AsOf: asOf, Transactions: transactions, Categories: categories,
		CategoryEvidence: evidence, PrevailingFrequency: prevailingFrequency,
	}, nil
}

func (r *Repository) loadCategories(ctx context.Context, userID shared.UserID) ([]models.Category, error) {
	rows, err := r.db.Query(ctx, `
		SELECT category.id::text,category.name,category.category_type,parent.name,
		       category.recurring_kind_hint
		FROM categories category
		LEFT JOIN categories parent ON parent.id=category.parent_id
		 AND (parent.user_id IS NULL OR parent.user_id=$1)
		WHERE category.user_id IS NULL OR category.user_id=$1
		ORDER BY category.name,category.id
		LIMIT 201
	`, string(userID))
	if err != nil {
		return nil, fmt.Errorf("load transaction analysis categories: %w", err)
	}
	defer rows.Close()
	categories := make([]models.Category, 0, 200)
	for rows.Next() {
		var category models.Category
		if err := rows.Scan(
			&category.ID, &category.Name, &category.Type, &category.ParentName,
			&category.RecurringKindHint,
		); err != nil {
			return nil, fmt.Errorf("scan transaction analysis category: %w", err)
		}
		categories = append(categories, category)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transaction analysis categories: %w", err)
	}
	if len(categories) > 200 {
		return nil, ErrInputBound
	}
	return categories, nil
}

func (r *Repository) loadRecurringEvidence(
	ctx context.Context,
	userID shared.UserID,
) ([]models.CategoryRecurrenceEvidence, string, error) {
	rows, err := r.db.Query(ctx, `
		WITH recurring_entities AS (
			SELECT category_id,frequency,'SUBSCRIPTION'::text AS kind,kind_confirmed_at
			FROM subscriptions
			WHERE user_id=$1 AND status='ACTIVE'
			UNION ALL
			SELECT category_id,frequency,'BILL'::text,kind_confirmed_at
			FROM bills
			WHERE user_id=$1 AND status='ACTIVE'
		), bounded AS (
			SELECT category_id,frequency,kind,kind_confirmed_at
			FROM recurring_entities
			LIMIT $2
		), summary AS (
			SELECT count(*) AS entity_count FROM bounded
		), modal AS (
			SELECT frequency
			FROM bounded
			GROUP BY frequency
			ORDER BY count(*) DESC,frequency
			LIMIT 1
		), evidence AS (
			SELECT category_id,
			       count(*) FILTER (WHERE kind='SUBSCRIPTION') AS subscription_count,
			       count(*) FILTER (WHERE kind='BILL') AS bill_count,
			       count(*) FILTER (WHERE kind='SUBSCRIPTION' AND kind_confirmed_at IS NOT NULL)
			           AS confirmed_subscription_count,
			       count(*) FILTER (WHERE kind='BILL' AND kind_confirmed_at IS NOT NULL)
			           AS confirmed_bill_count
			FROM bounded
			WHERE category_id IS NOT NULL
			GROUP BY category_id
		)
		SELECT evidence.category_id::text,COALESCE(evidence.subscription_count,0),
		       COALESCE(evidence.bill_count,0),
		       COALESCE(evidence.confirmed_subscription_count,0),
		       COALESCE(evidence.confirmed_bill_count,0),
		       modal.frequency,summary.entity_count
		FROM summary
		LEFT JOIN modal ON true
		LEFT JOIN evidence ON true
		ORDER BY evidence.category_id NULLS LAST
		LIMIT $3
	`, string(userID), models.MaxTransactions+1, 201)
	if err != nil {
		return nil, "", fmt.Errorf("load recurring category evidence: %w", err)
	}
	defer rows.Close()
	evidence := make([]models.CategoryRecurrenceEvidence, 0, 200)
	var prevailingFrequency string
	for rows.Next() {
		var categoryID *string
		var frequency *string
		var subscriptionCount, billCount int64
		var confirmedSubscriptionCount, confirmedBillCount int64
		var entityCount int64
		if err := rows.Scan(
			&categoryID, &subscriptionCount, &billCount,
			&confirmedSubscriptionCount, &confirmedBillCount,
			&frequency, &entityCount,
		); err != nil {
			return nil, "", fmt.Errorf("scan recurring category evidence: %w", err)
		}
		if entityCount > models.MaxTransactions {
			return nil, "", ErrInputBound
		}
		if frequency != nil {
			prevailingFrequency = *frequency
		}
		if categoryID == nil {
			continue
		}
		evidence = append(evidence, models.CategoryRecurrenceEvidence{
			CategoryID:        *categoryID,
			SubscriptionCount: uint32(subscriptionCount), BillCount: uint32(billCount),
			ConfirmedSubscriptionCount: uint32(confirmedSubscriptionCount),
			ConfirmedBillCount:         uint32(confirmedBillCount),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate recurring category evidence: %w", err)
	}
	if len(evidence) > 200 {
		return nil, "", ErrInputBound
	}
	return evidence, prevailingFrequency, nil
}

func (r *Repository) LoadLearningSignals(
	ctx context.Context,
	userID shared.UserID,
	learningKeys []string,
) ([]models.LearningSignal, error) {
	if len(learningKeys) == 0 {
		return []models.LearningSignal{}, nil
	}
	if len(learningKeys) > models.MaxTransactions {
		return nil, ErrInputBound
	}
	uniqueKeys := make([]string, 0, len(learningKeys))
	seen := make(map[string]struct{}, len(learningKeys))
	for _, key := range learningKeys {
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		uniqueKeys = append(uniqueKeys, key)
	}
	rows, err := r.db.Query(ctx, `
		SELECT signal.learning_key,signal.category_id::text,
		       signal.personal_observation_count,signal.global_user_count,
		       signal.global_total_contributor_count
		FROM (
			SELECT preference.learning_key,preference.category_id,
			       1::integer AS personal_observation_count,0::integer AS global_user_count,
			       0::integer AS global_total_contributor_count
			FROM category_pattern_preferences preference
			JOIN categories category ON category.id=preference.category_id
			 AND (category.user_id IS NULL OR category.user_id=$1)
			WHERE preference.user_id=$1 AND preference.learning_key=ANY($2::text[])
			UNION ALL
			SELECT global.learning_key,global.category_id,0,
			       global.global_user_count,global.global_total_contributor_count
			FROM categorization_global_signals global
			JOIN categories category ON category.id=global.category_id AND category.user_id IS NULL
			WHERE global.learning_key=ANY($2::text[])
			  AND global.global_user_count>=3
		) signal
		ORDER BY signal.learning_key,signal.personal_observation_count DESC,
		         signal.global_user_count DESC,signal.category_id
		LIMIT 8193
	`, string(userID), uniqueKeys)
	if err != nil {
		return nil, fmt.Errorf("load transaction analysis learning signals: %w", err)
	}
	defer rows.Close()
	signals := make([]models.LearningSignal, 0, len(uniqueKeys)*2)
	for rows.Next() {
		var signal models.LearningSignal
		if err := rows.Scan(
			&signal.LearningKey, &signal.CategoryID, &signal.PersonalObservationCount,
			&signal.GlobalUserCount, &signal.GlobalTotalContributorCount,
		); err != nil {
			return nil, fmt.Errorf("scan transaction analysis learning signal: %w", err)
		}
		signals = append(signals, signal)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transaction analysis learning signals: %w", err)
	}
	if len(signals) > 8192 {
		return nil, ErrInputBound
	}
	return signals, nil
}

func (r *Repository) Apply(ctx context.Context, userID shared.UserID, result models.Result) error {
	learningJSON, err := marshalJSONArray(result.LearningKeys)
	if err != nil {
		return fmt.Errorf("encode transaction learning keys: %w", err)
	}
	categoryJSON, err := marshalJSONArray(result.Categories)
	if err != nil {
		return fmt.Errorf("encode automatic categories: %w", err)
	}
	recurringJSON, err := marshalJSONArray(result.Recurring)
	if err != nil {
		return fmt.Errorf("encode recurring candidates: %w", err)
	}
	reviewJSON, err := marshalJSONArray(result.CategoryReviews)
	if err != nil {
		return fmt.Errorf("encode category reviews: %w", err)
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction analysis persistence: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		WITH learning AS (
			SELECT * FROM jsonb_to_recordset($2::jsonb) AS value(
				transaction_id uuid,learning_key text
			)
		)
		UPDATE transactions transaction
		SET categorization_learning_key=learning.learning_key
		FROM learning
		WHERE transaction.user_id=$1 AND transaction.id=learning.transaction_id
		  AND transaction.removed_at IS NULL
	`, string(userID), learningJSON); err != nil {
		return fmt.Errorf("store transaction category learning keys: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		WITH assignments AS (
			SELECT * FROM jsonb_to_recordset($2::jsonb) AS value(
				transaction_id uuid,category_id uuid,confidence_basis_points integer
			)
		)
		UPDATE transactions transaction
		SET category_id = category.id,
		    category_source = 'AUTOMATIC',
		    category_confidence_basis_points = assignment.confidence_basis_points,
		    updated_at = now()
		FROM assignments assignment
		JOIN categories category ON category.id=assignment.category_id
		 AND (category.user_id IS NULL OR category.user_id=$1)
		WHERE transaction.user_id = $1
		  AND transaction.id = assignment.transaction_id
		  AND transaction.removed_at IS NULL
		  AND transaction.category_source <> 'USER'
	`, string(userID), categoryJSON); err != nil {
		return fmt.Errorf("apply automatic categories: %w", err)
	}
	if err := syncCategoryReviews(ctx, tx, userID, reviewJSON); err != nil {
		return err
	}

	if err := upsertRecurring(ctx, tx, userID, recurringJSON); err != nil {
		return err
	}
	if err := syncUserLearningPreferences(ctx, tx, userID, result.LearningKeys); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction analysis persistence: %w", err)
	}
	return nil
}

func marshalJSONArray[T any](items []T) ([]byte, error) {
	if items == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(items)
}

func syncCategoryReviews(ctx context.Context, tx pgx.Tx, userID shared.UserID, payload []byte) error {
	if _, err := tx.Exec(ctx, `
		WITH reviews AS (
			SELECT * FROM jsonb_to_recordset($2::jsonb) AS value(
				transaction_id uuid,suggested_category_id uuid,
				confidence_basis_points integer, explanation text
			)
		), resolved AS (
			UPDATE inbox_items item SET status='RESOLVED',resolved_at=now()
			WHERE item.user_id=$1 AND item.item_type='TRANSACTION_REVIEW'
			  AND item.status='OPEN' AND item.payload->>'reason'='CATEGORY_UNCERTAIN'
			  AND EXISTS (
			      SELECT 1 FROM transactions transaction
			      WHERE transaction.id=item.entity_id AND transaction.user_id=$1
			        AND transaction.category_id IS NOT NULL
			  )
			RETURNING 1
		), eligible AS (
			SELECT transaction.id, transaction.name, transaction.transaction_date,
			       transaction.amount_minor, transaction.currency,
			       review.confidence_basis_points, review.explanation,
			       category.id AS suggested_category_id, category.name AS suggested_category_name
			FROM reviews review
			JOIN transactions transaction ON transaction.id=review.transaction_id
			 AND transaction.user_id=$1 AND transaction.removed_at IS NULL
			 AND transaction.category_id IS NULL AND transaction.category_source='UNASSIGNED'
			LEFT JOIN categories category ON category.id=review.suggested_category_id
			 AND (category.user_id IS NULL OR category.user_id=$1)
		), inserted AS (
			INSERT INTO inbox_items(user_id,item_type,priority,entity_type,entity_id,payload)
			SELECT $1,'TRANSACTION_REVIEW','NORMAL',$3,eligible.id,
			       jsonb_strip_nulls(jsonb_build_object(
			           'reason','CATEGORY_UNCERTAIN',
			           'transaction_name',left(eligible.name,160),
			           'transaction_date',eligible.transaction_date,
			           'amount_minor',eligible.amount_minor::text,
			           'currency',eligible.currency,
			           'suggested_category_id',eligible.suggested_category_id,
			           'suggested_category_name',eligible.suggested_category_name,
			           'confidence_basis_points',NULLIF(eligible.confidence_basis_points,0),
			           'explanation',NULLIF(eligible.explanation,'')
			       ))
			FROM eligible
			WHERE NOT EXISTS (
				SELECT 1 FROM inbox_items item
				WHERE item.user_id=$1 AND item.entity_id=eligible.id
				  AND item.item_type='TRANSACTION_REVIEW' AND item.status='OPEN'
				  AND item.payload->>'reason'='CATEGORY_UNCERTAIN'
			)
			RETURNING 1
		)
		UPDATE users
		SET open_inbox_count=GREATEST(
		        open_inbox_count-(SELECT count(*) FROM resolved)+(SELECT count(*) FROM inserted),0
		    ),updated_at=now()
		WHERE id=$1 AND (EXISTS(SELECT 1 FROM resolved) OR EXISTS(SELECT 1 FROM inserted))
	`, string(userID), payload, string(entities.Transaction)); err != nil {
		return fmt.Errorf("synchronize category review items: %w", err)
	}
	return nil
}

func syncUserLearningPreferences(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	learning []models.LearningKeyAssignment,
) error {
	if len(learning) == 0 {
		return nil
	}
	transactionIDs := make([]string, len(learning))
	for index, item := range learning {
		transactionIDs[index] = item.TransactionID
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (categorization_learning_key)
		       categorization_learning_key,category_id::text
		FROM transactions
		WHERE user_id=$1 AND id=ANY($2::uuid[]) AND removed_at IS NULL
		  AND category_source='USER' AND category_id IS NOT NULL
		  AND categorization_learning_key IS NOT NULL
		ORDER BY categorization_learning_key,updated_at DESC,id DESC
		LIMIT 4097
	`, string(userID), transactionIDs)
	if err != nil {
		return fmt.Errorf("load analyzed user category preferences: %w", err)
	}
	defer rows.Close()
	updates := make([]categoryrepo.LearningPreferenceUpdate, 0, len(learning))
	for rows.Next() {
		var update categoryrepo.LearningPreferenceUpdate
		if err := rows.Scan(&update.LearningKey, &update.CategoryID); err != nil {
			return fmt.Errorf("scan analyzed user category preference: %w", err)
		}
		updates = append(updates, update)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate analyzed user category preferences: %w", err)
	}
	if len(updates) > models.MaxTransactions {
		return ErrInputBound
	}
	if err := categoryrepo.SyncLearningPreferences(ctx, tx, userID, updates); err != nil {
		return fmt.Errorf("synchronize analyzed user category preferences: %w", err)
	}
	return nil
}

func upsertRecurring(ctx context.Context, tx pgx.Tx, userID shared.UserID, payload []byte) error {
	if _, err := tx.Exec(ctx, `
			WITH candidates AS (
				SELECT * FROM jsonb_to_recordset($2::jsonb) AS value(
				detection_key text, name text, kind text, currency text,
				frequency text, expected_amount_minor bigint, next_expected_at date,
				confidence_basis_points integer, occurrence_count integer,
				account_id text,category_id uuid,supporting_ids jsonb,
					explanation text, status text
				)
			), stale_bills AS (
				DELETE FROM bills bill
				USING candidates candidate
				WHERE bill.user_id=$1 AND bill.source='DETECTED'
				  AND bill.user_modified_at IS NULL AND candidate.kind<>'BILL'
				  AND bill.detected_from_transaction_id=
				      (candidate.supporting_ids ->> (jsonb_array_length(candidate.supporting_ids) - 1))::uuid
				RETURNING bill.id
			), stale_subscriptions AS (
				DELETE FROM subscriptions subscription
				USING candidates candidate
				WHERE subscription.user_id=$1 AND subscription.source='DETECTED'
				  AND subscription.user_modified_at IS NULL AND candidate.kind<>'SUBSCRIPTION'
				  AND subscription.detected_from_transaction_id=
				      (candidate.supporting_ids ->> (jsonb_array_length(candidate.supporting_ids) - 1))::uuid
				RETURNING subscription.id
			), stale_income AS (
				DELETE FROM recurring_income_sources income
				USING candidates candidate
				WHERE income.user_id=$1 AND income.user_modified_at IS NULL AND candidate.kind<>'INCOME'
				  AND income.detected_from_transaction_id=
				      (candidate.supporting_ids ->> (jsonb_array_length(candidate.supporting_ids) - 1))::uuid
				RETURNING income.id
			), previous AS (
				SELECT candidate.kind, candidate.detection_key, income.expected_amount_minor
				FROM candidates candidate
				JOIN recurring_income_sources income ON income.user_id=$1
				 AND income.detection_key=candidate.detection_key AND income.user_modified_at IS NULL
				WHERE candidate.kind='INCOME'
				UNION ALL
				SELECT candidate.kind, candidate.detection_key, bill.expected_amount_minor
				FROM candidates candidate
				JOIN bills bill ON bill.user_id=$1
				 AND bill.detection_key=candidate.detection_key AND bill.user_modified_at IS NULL
				WHERE candidate.kind='BILL'
				UNION ALL
				SELECT candidate.kind, candidate.detection_key, subscription.expected_amount_minor
				FROM candidates candidate
				JOIN subscriptions subscription ON subscription.user_id=$1
				 AND subscription.detection_key=candidate.detection_key AND subscription.user_modified_at IS NULL
				WHERE candidate.kind='SUBSCRIPTION'
			), income AS (
			INSERT INTO recurring_income_sources (
				user_id, name, expected_amount_minor, currency, frequency,
				next_expected_at, category_id, payment_account_id, status,
				detection_key, confidence_basis_points, occurrence_count,
				detected_from_transaction_id
			)
			SELECT $1, candidate.name, candidate.expected_amount_minor,
			       candidate.currency, candidate.frequency, candidate.next_expected_at,
			       category.id, NULLIF(candidate.account_id, '')::uuid, candidate.status,
			       candidate.detection_key, candidate.confidence_basis_points,
			       candidate.occurrence_count,
			       (candidate.supporting_ids ->> (jsonb_array_length(candidate.supporting_ids) - 1))::uuid
			FROM candidates candidate
			LEFT JOIN categories category ON category.id=candidate.category_id
			 AND (category.user_id IS NULL OR category.user_id=$1)
			WHERE candidate.kind = 'INCOME'
			ON CONFLICT (user_id, detection_key) DO UPDATE SET
				name = EXCLUDED.name, expected_amount_minor = EXCLUDED.expected_amount_minor,
				frequency = EXCLUDED.frequency, next_expected_at = EXCLUDED.next_expected_at,
				category_id = EXCLUDED.category_id, payment_account_id = EXCLUDED.payment_account_id,
				status = EXCLUDED.status, confidence_basis_points = EXCLUDED.confidence_basis_points,
				occurrence_count = EXCLUDED.occurrence_count,
				detected_from_transaction_id = EXCLUDED.detected_from_transaction_id,
				updated_at = now()
				WHERE recurring_income_sources.user_modified_at IS NULL
				RETURNING id, 'INCOME'::text kind, detection_key
		), bills_upsert AS (
			INSERT INTO bills (
				user_id, name, amount_type, expected_amount_minor, currency,
				frequency, next_due_at, category_id, source, status, detection_key,
				confidence_basis_points, occurrence_count, detected_from_transaction_id
			)
			SELECT $1, candidate.name, 'VARIABLE', candidate.expected_amount_minor,
			       candidate.currency, candidate.frequency, candidate.next_expected_at,
			       category.id, 'DETECTED', candidate.status, candidate.detection_key,
			       candidate.confidence_basis_points, candidate.occurrence_count,
			       (candidate.supporting_ids ->> (jsonb_array_length(candidate.supporting_ids) - 1))::uuid
			FROM candidates candidate
			LEFT JOIN categories category ON category.id=candidate.category_id
				 AND (category.user_id IS NULL OR category.user_id=$1)
			WHERE candidate.kind = 'BILL'
			  AND NOT EXISTS (
			      SELECT 1 FROM subscriptions subscription
			      WHERE subscription.user_id=$1
			        AND subscription.detection_key=candidate.detection_key
			        AND subscription.user_modified_at IS NOT NULL
			  )
			ON CONFLICT (user_id, detection_key) WHERE detection_key IS NOT NULL DO UPDATE SET
				name = EXCLUDED.name,
				expected_amount_minor = CASE WHEN bills.status='ACTIVE'
					THEN bills.expected_amount_minor ELSE EXCLUDED.expected_amount_minor END,
				frequency = EXCLUDED.frequency, next_due_at = EXCLUDED.next_due_at,
				category_id = EXCLUDED.category_id, status = EXCLUDED.status,
				confidence_basis_points = EXCLUDED.confidence_basis_points,
				occurrence_count = EXCLUDED.occurrence_count,
				detected_from_transaction_id = EXCLUDED.detected_from_transaction_id,
				updated_at = now()
				WHERE bills.user_modified_at IS NULL
				RETURNING id, 'BILL'::text kind, detection_key
		), subscriptions_upsert AS (
			INSERT INTO subscriptions (
				user_id, merchant_name, expected_amount_minor, currency, frequency,
				next_expected_at, category_id, payment_account_id, status,
				detected_from_transaction_id, source, detection_key,
				confidence_basis_points, occurrence_count
			)
			SELECT $1, candidate.name, candidate.expected_amount_minor,
			       candidate.currency, candidate.frequency, candidate.next_expected_at,
			       category.id, NULLIF(candidate.account_id, '')::uuid, candidate.status,
			       (candidate.supporting_ids ->> (jsonb_array_length(candidate.supporting_ids) - 1))::uuid,
			       'DETECTED', candidate.detection_key,
			       candidate.confidence_basis_points, candidate.occurrence_count
			FROM candidates candidate
			LEFT JOIN categories category ON category.id=candidate.category_id
				 AND (category.user_id IS NULL OR category.user_id=$1)
			WHERE candidate.kind = 'SUBSCRIPTION'
			  AND NOT EXISTS (
			      SELECT 1 FROM bills bill
			      WHERE bill.user_id=$1 AND bill.detection_key=candidate.detection_key
			        AND bill.user_modified_at IS NOT NULL
			  )
			ON CONFLICT (user_id, detection_key) WHERE detection_key IS NOT NULL DO UPDATE SET
				merchant_name = EXCLUDED.merchant_name,
				expected_amount_minor = CASE WHEN subscriptions.status='ACTIVE'
					THEN subscriptions.expected_amount_minor ELSE EXCLUDED.expected_amount_minor END,
				frequency = EXCLUDED.frequency, next_expected_at = EXCLUDED.next_expected_at,
				category_id = EXCLUDED.category_id, payment_account_id = EXCLUDED.payment_account_id,
				status = EXCLUDED.status, confidence_basis_points = EXCLUDED.confidence_basis_points,
				occurrence_count = EXCLUDED.occurrence_count,
				detected_from_transaction_id = EXCLUDED.detected_from_transaction_id,
				updated_at = now()
				WHERE subscriptions.user_modified_at IS NULL
				RETURNING id, 'SUBSCRIPTION'::text kind, detection_key
			), recurring_entities AS (
				SELECT * FROM income
				UNION ALL SELECT * FROM bills_upsert
				UNION ALL SELECT * FROM subscriptions_upsert
			), created_notifications AS (
				INSERT INTO notifications(
					user_id,notification_type,title,body,entity_type,entity_id,dedupe_key
				)
				SELECT $1,
				       CASE
					           WHEN previous.expected_amount_minor IS NOT NULL
					                AND previous.expected_amount_minor<>candidate.expected_amount_minor
					                AND candidate.kind='SUBSCRIPTION'
					               THEN 'SUBSCRIPTION_PRICE_CHANGE'
					           WHEN previous.expected_amount_minor IS NOT NULL
					                AND previous.expected_amount_minor<>candidate.expected_amount_minor
					                AND candidate.kind='BILL'
				               THEN 'BILL_AMOUNT_CHANGED'
				           ELSE 'NEW_RECURRING'
				       END,
				       CASE
					           WHEN previous.expected_amount_minor IS NOT NULL
					                AND previous.expected_amount_minor<>candidate.expected_amount_minor
					                AND candidate.kind='SUBSCRIPTION'
				               THEN 'Subscription amount changed'
					           WHEN previous.expected_amount_minor IS NOT NULL
					                AND previous.expected_amount_minor<>candidate.expected_amount_minor
					                AND candidate.kind='BILL'
				               THEN 'Bill amount changed'
				           WHEN candidate.status='UNKNOWN' AND candidate.kind='SUBSCRIPTION'
				               THEN 'Possible subscription detected'
				           WHEN candidate.status='UNKNOWN' AND candidate.kind='BILL'
				               THEN 'Possible recurring payment detected'
				           WHEN candidate.status='UNKNOWN' THEN 'Possible recurring income detected'
				           WHEN candidate.kind='SUBSCRIPTION' THEN 'Subscription detected'
				           WHEN candidate.kind='BILL' THEN 'Recurring payment detected'
				           ELSE 'Recurring income detected'
				       END,
				       left(candidate.name || ' · ' || replace(lower(candidate.frequency),'_',' '),1000),
				       entity.kind,entity.id,
				       left(
					           CASE WHEN previous.expected_amount_minor IS NULL
					                     OR previous.expected_amount_minor=candidate.expected_amount_minor
					                THEN 'recurring:new:' ELSE 'recurring:amount:' END
				           || candidate.kind || ':' || candidate.detection_key || ':' || candidate.expected_amount_minor::text,
				           300
				       )
				FROM candidates candidate
				JOIN recurring_entities entity ON entity.kind=candidate.kind
				 AND entity.detection_key=candidate.detection_key
				LEFT JOIN previous ON previous.kind=candidate.kind
				 AND previous.detection_key=candidate.detection_key
				WHERE candidate.status IN ('ACTIVE','UNKNOWN')
				  AND (
					      previous.expected_amount_minor IS NULL
					      OR (candidate.kind IN ('BILL','SUBSCRIPTION')
					          AND previous.expected_amount_minor<>candidate.expected_amount_minor)
					      OR NOT EXISTS (
					          SELECT 1 FROM notifications existing
					          WHERE existing.user_id=$1
					            AND (
					                existing.dedupe_key=left(
					                    'recurring:new:' || candidate.kind || ':' || candidate.detection_key
					                    || ':' || candidate.expected_amount_minor::text,300
					                )
					                OR (existing.notification_type='NEW_RECURRING'
					                    AND existing.entity_id=entity.id)
					            )
					      )
				  )
				  AND NOT EXISTS (
				      SELECT 1 FROM notification_preferences preference
				      WHERE preference.user_id=$1
				        AND preference.notification_type=CASE
					            WHEN previous.expected_amount_minor IS NOT NULL
					                 AND previous.expected_amount_minor<>candidate.expected_amount_minor
					                 AND candidate.kind='SUBSCRIPTION'
				                THEN 'SUBSCRIPTION_PRICE_CHANGE'
					            WHEN previous.expected_amount_minor IS NOT NULL
					                 AND previous.expected_amount_minor<>candidate.expected_amount_minor
					                 AND candidate.kind='BILL'
				                THEN 'BILL_AMOUNT_CHANGED'
				            ELSE 'NEW_RECURRING'
				        END
				        AND NOT preference.in_app_enabled
				  )
				ON CONFLICT (user_id,dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING
				RETURNING 1
			)
		UPDATE transactions transaction
		SET recurring_detection_key = candidate.detection_key, updated_at = now()
		FROM candidates candidate,
		     LATERAL jsonb_array_elements_text(candidate.supporting_ids) supporting(transaction_id)
		WHERE transaction.id = supporting.transaction_id::uuid
		  AND transaction.user_id = $1 AND transaction.removed_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM subscriptions subscription
		      WHERE candidate.kind = 'SUBSCRIPTION' AND subscription.user_id = $1
		        AND subscription.detection_key = candidate.detection_key
		        AND subscription.status = 'CANCELLED' AND subscription.user_modified_at IS NOT NULL
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM bills bill
		      WHERE candidate.kind = 'BILL' AND bill.user_id = $1
		        AND bill.detection_key = candidate.detection_key
		        AND bill.status = 'CANCELLED' AND bill.user_modified_at IS NOT NULL
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM recurring_income_sources income
		      WHERE candidate.kind = 'INCOME' AND income.user_id = $1
		        AND income.detection_key = candidate.detection_key
		        AND income.status = 'CANCELLED' AND income.user_modified_at IS NOT NULL
		  )
	`, string(userID), payload); err != nil {
		return fmt.Errorf("persist detected recurring patterns: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		WITH candidates AS (
			SELECT * FROM jsonb_to_recordset($2::jsonb) AS value(
				detection_key text, kind text, confidence_basis_points integer,
				expected_amount_minor bigint, currency text, frequency text,
				next_expected_at date, explanation text, status text
			)
		), detected AS (
			SELECT subscription.id, candidate.kind, candidate.confidence_basis_points,
			       candidate.expected_amount_minor, candidate.currency, candidate.frequency,
			       candidate.next_expected_at, candidate.explanation
			FROM candidates candidate
			JOIN subscriptions subscription ON subscription.user_id = $1
			 AND subscription.detection_key = candidate.detection_key
			 AND subscription.status = 'UNKNOWN' AND subscription.user_modified_at IS NULL
			WHERE candidate.kind = 'SUBSCRIPTION' AND candidate.status = 'UNKNOWN'
			UNION ALL
			SELECT bill.id, candidate.kind, candidate.confidence_basis_points,
			       candidate.expected_amount_minor, candidate.currency, candidate.frequency,
			       candidate.next_expected_at, candidate.explanation
			FROM candidates candidate
			JOIN bills bill ON bill.user_id = $1 AND bill.detection_key = candidate.detection_key
			 AND bill.status = 'UNKNOWN' AND bill.user_modified_at IS NULL
			WHERE candidate.kind = 'BILL' AND candidate.status = 'UNKNOWN'
			UNION ALL
			SELECT income.id, candidate.kind, candidate.confidence_basis_points,
			       candidate.expected_amount_minor, candidate.currency, candidate.frequency,
			       candidate.next_expected_at, candidate.explanation
			FROM candidates candidate
			JOIN recurring_income_sources income ON income.user_id = $1
			 AND income.detection_key = candidate.detection_key
			 AND income.status = 'UNKNOWN' AND income.user_modified_at IS NULL
			WHERE candidate.kind = 'INCOME' AND candidate.status = 'UNKNOWN'
		), inserted AS (
			INSERT INTO inbox_items (user_id, item_type, priority, entity_type, entity_id, payload)
			SELECT $1,
			       CASE detected.kind
			           WHEN 'SUBSCRIPTION' THEN 'POSSIBLE_SUBSCRIPTION'
			           WHEN 'BILL' THEN 'POSSIBLE_RECURRING_PAYMENT'
			           ELSE 'POSSIBLE_RECURRING_INCOME'
			       END,
			       'NORMAL', detected.kind, detected.id,
			       jsonb_build_object(
			           'confidence_basis_points', detected.confidence_basis_points,
			           'expected_amount_minor', detected.expected_amount_minor,
			           'currency', detected.currency, 'frequency', detected.frequency,
			           'next_expected_at', detected.next_expected_at,
			           'explanation', detected.explanation
			       )
			FROM detected
			WHERE NOT EXISTS (
				SELECT 1 FROM inbox_items item
				WHERE item.user_id = $1 AND item.entity_id = detected.id
				  AND item.status = 'OPEN'
			)
			RETURNING 1
		)
		UPDATE users SET open_inbox_count = open_inbox_count + (SELECT count(*) FROM inserted),
		                 updated_at = now()
		WHERE id = $1 AND EXISTS (SELECT 1 FROM inserted)
	`, string(userID), payload); err != nil {
		return fmt.Errorf("create recurring review items: %w", err)
	}
	if err := syncRecurringAmountChanges(ctx, tx, userID, payload); err != nil {
		return err
	}
	return nil
}
