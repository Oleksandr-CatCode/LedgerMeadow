BEGIN;

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    clerk_user_id TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE bank_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider = 'PLAID'),
    provider_item_id TEXT NOT NULL,
    institution_id TEXT,
    institution_name TEXT,
    access_token_ciphertext BYTEA NOT NULL,
    token_key_version INTEGER NOT NULL DEFAULT 1 CHECK (token_key_version > 0),
    status TEXT NOT NULL DEFAULT 'SYNC_PENDING'
        CHECK (status IN ('SYNC_PENDING', 'SYNCING', 'READY', 'ERROR')),
    last_sync_at TIMESTAMPTZ,
    last_sync_error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_item_id)
);

CREATE INDEX bank_connections_user_id_idx ON bank_connections (user_id, created_at DESC);

CREATE TABLE accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bank_connection_id UUID NOT NULL REFERENCES bank_connections(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider = 'PLAID'),
    provider_account_id TEXT NOT NULL,
    name TEXT NOT NULL,
    official_name TEXT,
    mask TEXT,
    account_type TEXT NOT NULL,
    account_subtype TEXT,
    balance_minor BIGINT NOT NULL,
    available_balance_minor BIGINT,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (bank_connection_id, provider_account_id)
);

CREATE INDEX accounts_user_id_idx ON accounts (user_id, created_at, id);

CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bank_connection_id UUID NOT NULL REFERENCES bank_connections(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider = 'PLAID'),
    provider_transaction_id TEXT NOT NULL,
    pending_provider_transaction_id TEXT,
    name TEXT NOT NULL,
    merchant_name TEXT,
    amount_minor BIGINT NOT NULL,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    transaction_date DATE NOT NULL,
    authorized_date DATE,
    is_pending BOOLEAN NOT NULL,
    removed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_transaction_id)
);

CREATE INDEX transactions_user_page_idx
    ON transactions (user_id, transaction_date DESC, id DESC)
    WHERE removed_at IS NULL;
CREATE INDEX transactions_connection_idx
    ON transactions (bank_connection_id, provider_transaction_id);

CREATE TABLE sync_cursors (
    bank_connection_id UUID PRIMARY KEY REFERENCES bank_connections(id) ON DELETE CASCADE,
    cursor TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID,
    event_type TEXT NOT NULL,
    dedupe_key TEXT NOT NULL UNIQUE,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    locked_at TIMESTAMPTZ,
    processed_at TIMESTAMPTZ,
    last_error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX outbox_events_claim_idx ON outbox_events (status, available_at, created_at);

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
