package repository

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
)

const maxLearningPreferenceUpdates = 4096

var ErrLearningCategoryNotFound = errors.New("category learning category not found")

type LearningPreferenceUpdate struct {
	LearningKey string
	CategoryID  *string
}

type learningCategory struct {
	ID        string
	UserID    *string
	SystemKey *string
}

type storedPreference struct {
	LearningKey string
	CategoryID  string
	SystemKey   *string
}

type preferenceRecord struct {
	LearningKey       string  `json:"learning_key"`
	CategoryID        string  `json:"category_id"`
	CategoryUserID    *string `json:"category_user_id"`
	CategorySystemKey *string `json:"category_system_key"`
}

type signalDelta struct {
	LearningKey       string `json:"learning_key"`
	CategoryID        string `json:"category_id"`
	CategorySystemKey string `json:"category_system_key"`
	Delta             int    `json:"delta"`
}

func SyncLearningPreferences(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	updates []LearningPreferenceUpdate,
) error {
	if len(updates) == 0 {
		return nil
	}
	if len(updates) > maxLearningPreferenceUpdates {
		return errors.New("category learning preference update exceeds bound")
	}

	byKey := make(map[string]LearningPreferenceUpdate, len(updates))
	for _, update := range updates {
		decoded, err := hex.DecodeString(update.LearningKey)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != update.LearningKey {
			return errors.New("category learning key is invalid")
		}
		byKey[update.LearningKey] = update
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if _, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended(value, 0))
		FROM unnest($1::text[]) value
		ORDER BY value
	`, keys); err != nil {
		return fmt.Errorf("lock category learning preferences: %w", err)
	}

	categories, err := loadLearningCategories(ctx, tx, userID, byKey)
	if err != nil {
		return err
	}
	oldPreferences, err := loadStoredPreferences(ctx, tx, userID, keys)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `
		DELETE FROM category_pattern_preferences
		WHERE user_id=$1 AND learning_key=ANY($2::text[])
	`, string(userID), keys); err != nil {
		return fmt.Errorf("clear category learning preferences: %w", err)
	}

	records := make([]preferenceRecord, 0, len(byKey))
	for _, key := range keys {
		update := byKey[key]
		if update.CategoryID == nil {
			continue
		}
		category := categories[*update.CategoryID]
		records = append(records, preferenceRecord{
			LearningKey: key, CategoryID: category.ID,
			CategoryUserID: category.UserID, CategorySystemKey: category.SystemKey,
		})
	}
	if len(records) > 0 {
		payload, err := json.Marshal(records)
		if err != nil {
			return fmt.Errorf("encode category learning preferences: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO category_pattern_preferences(
				user_id,learning_key,category_id,category_user_id,category_system_key
			)
			SELECT $1,value.learning_key,value.category_id,value.category_user_id,
			       value.category_system_key
			FROM jsonb_to_recordset($2::jsonb) AS value(
				learning_key text,category_id uuid,category_user_id uuid,category_system_key text
			)
		`, string(userID), payload); err != nil {
			return fmt.Errorf("store category learning preferences: %w", err)
		}
	}

	deltas := learningSignalDeltas(oldPreferences, records)
	if err := applyGlobalSignalDeltas(ctx, tx, keys, deltas); err != nil {
		return err
	}
	return nil
}

func loadLearningCategories(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	updates map[string]LearningPreferenceUpdate,
) (map[string]learningCategory, error) {
	ids := make([]string, 0, len(updates))
	seen := make(map[string]struct{}, len(updates))
	for _, update := range updates {
		if update.CategoryID == nil {
			continue
		}
		if _, exists := seen[*update.CategoryID]; !exists {
			seen[*update.CategoryID] = struct{}{}
			ids = append(ids, *update.CategoryID)
		}
	}
	if len(ids) == 0 {
		return map[string]learningCategory{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text,user_id::text,system_key
		FROM categories
		WHERE id=ANY($1::uuid[]) AND (user_id IS NULL OR user_id=$2)
		ORDER BY id
		LIMIT 201
	`, ids, string(userID))
	if err != nil {
		return nil, fmt.Errorf("load category learning categories: %w", err)
	}
	defer rows.Close()
	categories := make(map[string]learningCategory, len(ids))
	for rows.Next() {
		var category learningCategory
		if err := rows.Scan(&category.ID, &category.UserID, &category.SystemKey); err != nil {
			return nil, fmt.Errorf("scan category learning category: %w", err)
		}
		categories[category.ID] = category
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate category learning categories: %w", err)
	}
	if len(categories) != len(ids) {
		return nil, ErrLearningCategoryNotFound
	}
	return categories, nil
}

func loadStoredPreferences(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	keys []string,
) ([]storedPreference, error) {
	rows, err := tx.Query(ctx, `
		SELECT learning_key,category_id::text,category_system_key
		FROM category_pattern_preferences
		WHERE user_id=$1 AND learning_key=ANY($2::text[])
		ORDER BY learning_key
	`, string(userID), keys)
	if err != nil {
		return nil, fmt.Errorf("load stored category learning preferences: %w", err)
	}
	defer rows.Close()
	preferences := make([]storedPreference, 0, len(keys))
	for rows.Next() {
		var preference storedPreference
		if err := rows.Scan(&preference.LearningKey, &preference.CategoryID, &preference.SystemKey); err != nil {
			return nil, fmt.Errorf("scan stored category learning preference: %w", err)
		}
		preferences = append(preferences, preference)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stored category learning preferences: %w", err)
	}
	return preferences, nil
}

func learningSignalDeltas(old []storedPreference, next []preferenceRecord) []signalDelta {
	type signalKey struct{ learningKey, categoryID, systemKey string }
	values := make(map[signalKey]int, len(old)+len(next))
	for _, preference := range old {
		if preference.SystemKey != nil {
			values[signalKey{preference.LearningKey, preference.CategoryID, *preference.SystemKey}]--
		}
	}
	for _, preference := range next {
		if preference.CategorySystemKey != nil {
			values[signalKey{preference.LearningKey, preference.CategoryID, *preference.CategorySystemKey}]++
		}
	}
	deltas := make([]signalDelta, 0, len(values))
	for key, delta := range values {
		if delta != 0 {
			deltas = append(deltas, signalDelta{key.learningKey, key.categoryID, key.systemKey, delta})
		}
	}
	return deltas
}

func applyGlobalSignalDeltas(ctx context.Context, tx pgx.Tx, keys []string, deltas []signalDelta) error {
	if len(deltas) > 0 {
		payload, err := json.Marshal(deltas)
		if err != nil {
			return fmt.Errorf("encode category learning signal deltas: %w", err)
		}
		decreased, err := tx.Exec(ctx, `
			WITH delta AS (
				SELECT learning_key,category_id,category_system_key,sum(delta)::integer AS amount
				FROM jsonb_to_recordset($1::jsonb) AS value(
					learning_key text,category_id uuid,category_system_key text,delta integer
				)
				GROUP BY learning_key,category_id,category_system_key
			)
			UPDATE categorization_global_signal_counts count
			SET distinct_user_count=count.distinct_user_count+delta.amount,updated_at=now()
			FROM delta
			WHERE delta.amount<0 AND count.learning_key=delta.learning_key
			  AND count.category_id=delta.category_id
			  AND count.distinct_user_count+delta.amount>0
		`, payload)
		if err != nil {
			return fmt.Errorf("decrease category learning signal counts: %w", err)
		}
		removed, err := tx.Exec(ctx, `
			WITH delta AS (
				SELECT learning_key,category_id,sum(delta)::integer AS amount
				FROM jsonb_to_recordset($1::jsonb) AS value(
					learning_key text,category_id uuid,category_system_key text,delta integer
				)
				GROUP BY learning_key,category_id
			)
			DELETE FROM categorization_global_signal_counts count
			USING delta
			WHERE delta.amount<0 AND count.learning_key=delta.learning_key
			  AND count.category_id=delta.category_id
			  AND count.distinct_user_count=-delta.amount
		`, payload)
		if err != nil {
			return fmt.Errorf("remove empty category learning signal counts: %w", err)
		}
		negativeCount := int64(0)
		for _, delta := range deltas {
			if delta.Delta < 0 {
				negativeCount++
			}
		}
		if decreased.RowsAffected()+removed.RowsAffected() != negativeCount {
			return errors.New("category learning signal counts are inconsistent")
		}
		if _, err := tx.Exec(ctx, `
			WITH delta AS (
				SELECT learning_key,category_id,category_system_key,sum(delta)::integer AS amount
				FROM jsonb_to_recordset($1::jsonb) AS value(
					learning_key text,category_id uuid,category_system_key text,delta integer
				)
				WHERE delta>0
				GROUP BY learning_key,category_id,category_system_key
			)
			INSERT INTO categorization_global_signal_counts(
				learning_key,category_id,category_system_key,distinct_user_count
			)
			SELECT learning_key,category_id,category_system_key,amount FROM delta
			ON CONFLICT(learning_key,category_id) DO UPDATE
			SET distinct_user_count=categorization_global_signal_counts.distinct_user_count+EXCLUDED.distinct_user_count,
			    updated_at=now()
		`, payload); err != nil {
			return fmt.Errorf("increase category learning signal counts: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM categorization_global_signals WHERE learning_key=ANY($1::text[])`, keys); err != nil {
		return fmt.Errorf("clear materialized global category signals: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO categorization_global_signals(
			learning_key,category_id,category_system_key,global_user_count,
			global_total_contributor_count
		)
		SELECT learning_key,category_id,category_system_key,distinct_user_count,total_count
		FROM (
			SELECT count.learning_key,count.category_id,count.category_system_key,
			       count.distinct_user_count,
			       sum(count.distinct_user_count) OVER(PARTITION BY count.learning_key) AS total_count,
			       row_number() OVER(
			           PARTITION BY count.learning_key
			           ORDER BY count.distinct_user_count DESC,count.category_id
			       ) AS rank
			FROM categorization_global_signal_counts count
			WHERE count.learning_key=ANY($1::text[])
		) ranked
		WHERE rank=1
	`, keys); err != nil {
		return fmt.Errorf("materialize global category signals: %w", err)
	}
	return nil
}
