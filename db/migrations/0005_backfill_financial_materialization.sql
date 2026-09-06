BEGIN;

INSERT INTO outbox_events (
    user_id,
    aggregate_type,
    aggregate_id,
    event_type,
    dedupe_key,
    payload
)
SELECT DISTINCT
    account.user_id,
    'USER',
    account.user_id,
    'FINANCIAL_MATERIALIZATION_REQUESTED',
    'financial-materialization:backfill:0005:' || account.user_id::text,
    '{}'::jsonb
FROM accounts account
JOIN bank_connections connection
    ON connection.id = account.bank_connection_id
   AND connection.user_id = account.user_id
WHERE connection.status <> 'DISCONNECTED'
ON CONFLICT (dedupe_key) DO NOTHING;

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
