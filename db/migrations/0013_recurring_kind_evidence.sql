BEGIN;

ALTER TABLE subscriptions
    ADD COLUMN kind_confirmed_at TIMESTAMPTZ;

ALTER TABLE bills
    ADD COLUMN kind_confirmed_at TIMESTAMPTZ;

UPDATE subscriptions
SET kind_confirmed_at = created_at
WHERE source = 'MANUAL';

UPDATE bills
SET kind_confirmed_at = created_at
WHERE source = 'MANUAL';

ALTER TABLE subscriptions
    DROP CONSTRAINT subscriptions_occurrence_count_check,
    ADD CONSTRAINT subscriptions_occurrence_count_check CHECK (
        occurrence_count IS NULL OR occurrence_count >= 1
    );

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
