BEGIN;

ALTER TABLE categories
    ADD CONSTRAINT categories_scope_check CHECK (
        (user_id IS NULL AND system_key IS NOT NULL)
        OR (user_id IS NOT NULL AND system_key IS NULL)
    ),
    ADD CONSTRAINT categories_id_user_key UNIQUE (id, user_id),
    ADD CONSTRAINT categories_id_system_key_key UNIQUE (id, system_key);

ALTER TABLE transactions
    ADD COLUMN categorization_learning_key TEXT,
    ADD CONSTRAINT transactions_categorization_learning_key_format CHECK (
        categorization_learning_key IS NULL
        OR categorization_learning_key ~ '^[0-9a-f]{64}$'
    ),
    ADD CONSTRAINT transactions_id_user_key UNIQUE (id, user_id);

CREATE TABLE category_pattern_preferences (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    learning_key TEXT NOT NULL CHECK (learning_key ~ '^[0-9a-f]{64}$'),
    category_id UUID NOT NULL,
    category_user_id UUID,
    category_system_key TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, learning_key),
    CHECK (
        (category_user_id IS NOT NULL AND category_user_id = user_id AND category_system_key IS NULL)
        OR (category_user_id IS NULL AND category_system_key IS NOT NULL)
    ),
    FOREIGN KEY (category_id, category_user_id)
        REFERENCES categories(id, user_id) ON DELETE CASCADE,
    FOREIGN KEY (category_id, category_system_key)
        REFERENCES categories(id, system_key) ON DELETE CASCADE
);

CREATE TABLE categorization_global_signal_counts (
    learning_key TEXT NOT NULL CHECK (learning_key ~ '^[0-9a-f]{64}$'),
    category_id UUID NOT NULL,
    category_system_key TEXT NOT NULL,
    distinct_user_count INTEGER NOT NULL CHECK (distinct_user_count > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (learning_key, category_id),
    FOREIGN KEY (category_id, category_system_key)
        REFERENCES categories(id, system_key) ON DELETE CASCADE
);

CREATE TABLE categorization_global_signals (
    learning_key TEXT PRIMARY KEY CHECK (learning_key ~ '^[0-9a-f]{64}$'),
    category_id UUID NOT NULL,
    category_system_key TEXT NOT NULL,
    global_user_count INTEGER NOT NULL CHECK (global_user_count > 0),
    global_total_contributor_count INTEGER NOT NULL CHECK (
        global_total_contributor_count >= global_user_count
    ),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (category_id, category_system_key)
        REFERENCES categories(id, system_key) ON DELETE CASCADE
);

CREATE INDEX categorization_global_signals_eligible_idx
    ON categorization_global_signals (learning_key, category_id)
    WHERE global_user_count >= 3;

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
