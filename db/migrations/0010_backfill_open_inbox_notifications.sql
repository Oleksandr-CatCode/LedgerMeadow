BEGIN;

WITH candidates AS MATERIALIZED (
    SELECT item.id, item.user_id, item.item_type, item.entity_type, item.entity_id,
           item.payload, item.created_at
    FROM inbox_items item
    WHERE item.status = 'OPEN'
    ORDER BY item.user_id, item.priority DESC, item.created_at DESC, item.id DESC
    LIMIT 10000
), mapped AS (
    SELECT candidate.*,
           CASE candidate.item_type
               WHEN 'POSSIBLE_SUBSCRIPTION' THEN 'NEW_RECURRING'
               WHEN 'POSSIBLE_RECURRING_PAYMENT' THEN 'NEW_RECURRING'
               WHEN 'POSSIBLE_RECURRING_INCOME' THEN 'NEW_RECURRING'
               WHEN 'PRICE_CHANGE' THEN 'SUBSCRIPTION_PRICE_CHANGE'
               WHEN 'UNUSUAL_TRANSACTION' THEN 'UNUSUAL_CHARGE'
               ELSE candidate.item_type
           END AS notification_type,
           CASE candidate.item_type
               WHEN 'TRANSACTION_REVIEW' THEN 'Category needs review'
               WHEN 'POSSIBLE_SUBSCRIPTION' THEN 'Possible subscription detected'
               WHEN 'POSSIBLE_RECURRING_PAYMENT' THEN 'Possible recurring payment detected'
               WHEN 'POSSIBLE_RECURRING_INCOME' THEN 'Possible recurring income detected'
               WHEN 'PRICE_CHANGE' THEN 'Price change detected'
               WHEN 'UNUSUAL_TRANSACTION' THEN 'Unusual transaction detected'
               WHEN 'POSSIBLE_SHARED_EXPENSE' THEN 'Possible shared expense'
               WHEN 'BANK_CONNECTION_ERROR' THEN 'Bank connection needs attention'
               WHEN 'BUDGET_WARNING' THEN 'Budget needs attention'
               WHEN 'UPCOMING_SHORTFALL' THEN 'Upcoming shortfall'
               ELSE initcap(replace(lower(candidate.item_type), '_', ' '))
           END AS title
    FROM candidates candidate
)
INSERT INTO notifications (
    user_id, notification_type, title, body, entity_type, entity_id,
    dedupe_key, created_at
)
SELECT mapped.user_id, mapped.notification_type, mapped.title,
       left(COALESCE(NULLIF(mapped.payload->>'entity_name', ''),
                     NULLIF(mapped.payload->>'transaction_name', ''), mapped.title), 1000),
       mapped.entity_type, mapped.entity_id, 'inbox:' || mapped.id::text, mapped.created_at
FROM mapped
WHERE NOT EXISTS (
    SELECT 1 FROM notification_preferences preference
    WHERE preference.user_id = mapped.user_id
      AND preference.notification_type = mapped.notification_type
      AND NOT preference.in_app_enabled
)
ON CONFLICT (user_id, dedupe_key) WHERE dedupe_key IS NOT NULL DO NOTHING;

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
