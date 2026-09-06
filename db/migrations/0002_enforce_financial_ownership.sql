BEGIN;

ALTER TABLE bank_connections
    ADD CONSTRAINT bank_connections_id_user_key UNIQUE (id, user_id);

ALTER TABLE accounts
    ADD CONSTRAINT accounts_id_connection_user_key
        UNIQUE (id, bank_connection_id, user_id),
    ADD CONSTRAINT accounts_connection_user_fk
        FOREIGN KEY (bank_connection_id, user_id)
        REFERENCES bank_connections (id, user_id)
        ON DELETE CASCADE;

ALTER TABLE transactions
    ADD CONSTRAINT transactions_account_connection_user_fk
        FOREIGN KEY (account_id, bank_connection_id, user_id)
        REFERENCES accounts (id, bank_connection_id, user_id)
        ON DELETE CASCADE;

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
