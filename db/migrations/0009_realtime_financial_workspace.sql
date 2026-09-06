BEGIN;

ALTER TABLE notifications
    ADD COLUMN dedupe_key TEXT CHECK (dedupe_key IS NULL OR char_length(dedupe_key) BETWEEN 1 AND 300);

CREATE UNIQUE INDEX notifications_user_dedupe_idx
    ON notifications (user_id, dedupe_key)
    WHERE dedupe_key IS NOT NULL;

CREATE INDEX notifications_user_unread_idx
    ON notifications (user_id, created_at DESC, id DESC)
    WHERE read_at IS NULL;

ALTER TABLE financial_projections
    ADD COLUMN breakdown JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT financial_projections_breakdown_array_check
        CHECK (jsonb_typeof(breakdown) = 'array' AND jsonb_array_length(breakdown) <= 515);

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
