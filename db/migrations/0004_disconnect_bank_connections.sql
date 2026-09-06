BEGIN;

ALTER TABLE bank_connections
    ALTER COLUMN access_token_ciphertext DROP NOT NULL,
    ADD COLUMN disconnected_at TIMESTAMPTZ;

ALTER TABLE bank_connections
    DROP CONSTRAINT bank_connections_status_check,
    ADD CONSTRAINT bank_connections_status_check
        CHECK (status IN ('SYNC_PENDING', 'SYNCING', 'READY', 'ERROR', 'DISCONNECTED')),
    ADD CONSTRAINT bank_connections_disconnected_token_check
        CHECK ((status = 'DISCONNECTED') = (access_token_ciphertext IS NULL));

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
