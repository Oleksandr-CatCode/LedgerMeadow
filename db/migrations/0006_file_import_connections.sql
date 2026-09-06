BEGIN;

ALTER TABLE bank_connections
    DROP CONSTRAINT bank_connections_provider_check,
    DROP CONSTRAINT bank_connections_disconnected_token_check,
    ADD CONSTRAINT bank_connections_provider_check
        CHECK (provider IN ('PLAID', 'FILE_IMPORT')),
    ADD CONSTRAINT bank_connections_disconnected_token_check
        CHECK (
            (provider = 'PLAID' AND ((status = 'DISCONNECTED') = (access_token_ciphertext IS NULL)))
            OR
            (provider = 'FILE_IMPORT' AND access_token_ciphertext IS NULL
                AND status IN ('READY', 'DISCONNECTED'))
        );

ALTER TABLE accounts
    DROP CONSTRAINT accounts_provider_check,
    ADD CONSTRAINT accounts_provider_check
        CHECK (provider IN ('PLAID', 'FILE_IMPORT'));

ALTER TABLE transactions
    DROP CONSTRAINT transactions_provider_check,
    ADD CONSTRAINT transactions_provider_check
        CHECK (provider IN ('PLAID', 'FILE_IMPORT'));

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
