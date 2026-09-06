package repository

import (
	"context"
	"fmt"

	shared "ledgermeadow/src/shared/types"
	"github.com/jackc/pgx/v5"
)

func syncRecurringAmountChanges(
	ctx context.Context,
	tx pgx.Tx,
	userID shared.UserID,
	payload []byte,
) error {
	if _, err := tx.Exec(ctx, `
		WITH candidates AS (
			SELECT value.*,
			       COALESCE(value.observed_amount_minor,value.expected_amount_minor) proposed_amount_minor,
			       CASE WHEN value.observed_amount_minor IS NULL
			            THEN value.supporting_ids ELSE value.amount_observation_ids END observation_ids
			FROM jsonb_to_recordset($2::jsonb) AS value(
				detection_key text,kind text,currency text,frequency text,
				expected_amount_minor bigint,next_expected_at date,supporting_ids jsonb,
				observed_amount_minor bigint,amount_observation_ids jsonb
			)
		), eligible AS (
			SELECT subscription.id, candidate.kind, subscription.merchant_name entity_name,
			       subscription.expected_amount_minor previous_amount_minor,
			       candidate.proposed_amount_minor, candidate.currency, candidate.frequency,
			       candidate.next_expected_at, candidate.observation_ids
			FROM candidates candidate
			JOIN subscriptions subscription ON subscription.user_id=$1
			 AND subscription.detection_key=candidate.detection_key
			 AND subscription.status='ACTIVE'
			WHERE candidate.kind='SUBSCRIPTION'
			  AND candidate.proposed_amount_minor>0
			  AND subscription.expected_amount_minor<>candidate.proposed_amount_minor
			  AND jsonb_array_length(candidate.observation_ids)>0
			UNION ALL
			SELECT bill.id, candidate.kind, bill.name,
			       bill.expected_amount_minor, candidate.proposed_amount_minor,
			       candidate.currency, candidate.frequency, candidate.next_expected_at,
			       candidate.observation_ids
			FROM candidates candidate
			JOIN bills bill ON bill.user_id=$1 AND bill.detection_key=candidate.detection_key
			 AND bill.status='ACTIVE'
			WHERE candidate.kind='BILL'
			  AND candidate.proposed_amount_minor>0
			  AND bill.expected_amount_minor<>candidate.proposed_amount_minor
			  AND jsonb_array_length(candidate.observation_ids)>0
		), inserted AS (
			INSERT INTO inbox_items(user_id,item_type,priority,entity_type,entity_id,payload)
			SELECT $1,'PRICE_CHANGE','NORMAL',eligible.kind,eligible.id,
			       jsonb_build_object(
			           'entity_kind',eligible.kind,
			           'entity_name',eligible.entity_name,
			           'previous_amount_minor',eligible.previous_amount_minor::text,
			           'latest_amount_minor',eligible.proposed_amount_minor::text,
			           'currency',eligible.currency,
			           'frequency',eligible.frequency,
			           'next_expected_at',eligible.next_expected_at,
			           'amount_observation_ids',eligible.observation_ids,
			           'explanation',jsonb_array_length(eligible.observation_ids)::text
			               || ' charge(s) matched the latest expected cadence slot.'
			       )
			FROM eligible
			WHERE NOT EXISTS (
				SELECT 1 FROM inbox_items item
				WHERE item.user_id=$1 AND item.item_type='PRICE_CHANGE'
				  AND item.entity_type=eligible.kind AND item.entity_id=eligible.id
				  AND item.status='OPEN'
			)
			AND NOT EXISTS (
				SELECT 1 FROM inbox_items item
				WHERE item.user_id=$1 AND item.item_type='PRICE_CHANGE'
				  AND item.entity_type=eligible.kind AND item.entity_id=eligible.id
				  AND item.payload->'amount_observation_ids'=eligible.observation_ids
			)
			RETURNING id,entity_type,entity_id,payload
		), created_notifications AS (
			INSERT INTO notifications(
				user_id,notification_type,title,body,entity_type,entity_id,dedupe_key
			)
			SELECT $1,
			       CASE inserted.entity_type WHEN 'SUBSCRIPTION' THEN 'SUBSCRIPTION_PRICE_CHANGE'
			            ELSE 'BILL_AMOUNT_CHANGED' END,
			       CASE inserted.entity_type WHEN 'SUBSCRIPTION' THEN 'Subscription amount changed'
			            ELSE 'Bill amount changed' END,
			       left(inserted.payload->>'entity_name' || ' · confirm the new amount',1000),
			       inserted.entity_type,inserted.entity_id,
			       left('recurring:amount:' || inserted.entity_type || ':' || inserted.id::text,300)
			FROM inserted
			WHERE NOT EXISTS (
				SELECT 1 FROM notification_preferences preference
				WHERE preference.user_id=$1
				  AND preference.notification_type=CASE inserted.entity_type
				      WHEN 'SUBSCRIPTION' THEN 'SUBSCRIPTION_PRICE_CHANGE'
				      ELSE 'BILL_AMOUNT_CHANGED' END
				  AND NOT preference.in_app_enabled
			)
			ON CONFLICT (user_id,dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING
			RETURNING 1
		)
		UPDATE users SET open_inbox_count=open_inbox_count+(SELECT count(*) FROM inserted),
		                 updated_at=now()
		WHERE id=$1 AND EXISTS(SELECT 1 FROM inserted)
	`, string(userID), payload); err != nil {
		return fmt.Errorf("synchronize recurring amount changes: %w", err)
	}
	return nil
}
