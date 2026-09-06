BEGIN;

ALTER TABLE categories
    ADD COLUMN system_key TEXT,
    ADD CONSTRAINT categories_system_key_format CHECK (
        system_key IS NULL OR system_key ~ '^[a-z]+([._-][a-z]+)*$'
    );
CREATE UNIQUE INDEX categories_system_key_idx
    ON categories (system_key) WHERE system_key IS NOT NULL;

INSERT INTO categories (id, user_id, name, category_type, system_key) VALUES
    ('00000000-0000-4000-8000-000000000001', NULL, 'Salary & Wages', 'INCOME', 'income.salary'),
    ('00000000-0000-4000-8000-000000000002', NULL, 'Other Income', 'INCOME', 'income.other'),
    ('00000000-0000-4000-8000-000000000003', NULL, 'Transfers', 'TRANSFER', 'transfer'),
    ('00000000-0000-4000-8000-000000000004', NULL, 'Housing', 'EXPENSE', 'housing'),
    ('00000000-0000-4000-8000-000000000005', NULL, 'Utilities', 'EXPENSE', 'utilities'),
    ('00000000-0000-4000-8000-000000000006', NULL, 'Groceries', 'EXPENSE', 'groceries'),
    ('00000000-0000-4000-8000-000000000007', NULL, 'Dining', 'EXPENSE', 'dining'),
    ('00000000-0000-4000-8000-000000000008', NULL, 'Transportation', 'EXPENSE', 'transportation'),
    ('00000000-0000-4000-8000-000000000009', NULL, 'Insurance', 'EXPENSE', 'insurance'),
    ('00000000-0000-4000-8000-000000000010', NULL, 'Debt Payments', 'EXPENSE', 'debt'),
    ('00000000-0000-4000-8000-000000000011', NULL, 'Subscriptions', 'EXPENSE', 'subscriptions'),
    ('00000000-0000-4000-8000-000000000012', NULL, 'Shopping', 'EXPENSE', 'shopping'),
    ('00000000-0000-4000-8000-000000000013', NULL, 'Health', 'EXPENSE', 'health'),
    ('00000000-0000-4000-8000-000000000014', NULL, 'Entertainment', 'EXPENSE', 'entertainment'),
    ('00000000-0000-4000-8000-000000000015', NULL, 'Education', 'EXPENSE', 'education'),
    ('00000000-0000-4000-8000-000000000016', NULL, 'Personal Care', 'EXPENSE', 'personal'),
    ('00000000-0000-4000-8000-000000000017', NULL, 'Bank Fees', 'EXPENSE', 'fees'),
    ('00000000-0000-4000-8000-000000000018', NULL, 'Other', 'EXPENSE', 'other');

ALTER TABLE transactions
    ADD COLUMN provider_category_primary TEXT,
    ADD COLUMN provider_category_detailed TEXT,
    ADD COLUMN category_source TEXT NOT NULL DEFAULT 'UNASSIGNED'
        CHECK (category_source IN ('UNASSIGNED', 'AUTOMATIC', 'USER')),
    ADD COLUMN category_confidence_basis_points INTEGER
        CHECK (category_confidence_basis_points BETWEEN 0 AND 10000),
    ADD COLUMN recurring_detection_key TEXT
        CHECK (recurring_detection_key IS NULL OR char_length(recurring_detection_key) BETWEEN 1 AND 200);
UPDATE transactions SET category_source = 'USER' WHERE category_id IS NOT NULL;
CREATE INDEX transactions_user_recurring_idx
    ON transactions (user_id, recurring_detection_key, transaction_date DESC, id DESC)
    WHERE removed_at IS NULL AND recurring_detection_key IS NOT NULL;

ALTER TABLE bills
    DROP CONSTRAINT bills_status_check,
    ADD CONSTRAINT bills_status_check CHECK (status IN ('ACTIVE', 'PAUSED', 'CANCELLED', 'UNKNOWN')),
    ADD COLUMN detection_key TEXT,
    ADD COLUMN confidence_basis_points INTEGER CHECK (confidence_basis_points BETWEEN 0 AND 10000),
    ADD COLUMN occurrence_count INTEGER CHECK (occurrence_count IS NULL OR occurrence_count >= 3),
    ADD COLUMN detected_from_transaction_id UUID REFERENCES transactions(id) ON DELETE SET NULL,
    ADD COLUMN user_modified_at TIMESTAMPTZ,
    ADD CONSTRAINT bills_detection_key_length CHECK (
        detection_key IS NULL OR char_length(detection_key) BETWEEN 1 AND 200
    );
CREATE UNIQUE INDEX bills_user_detection_idx
    ON bills (user_id, detection_key) WHERE detection_key IS NOT NULL;

ALTER TABLE subscriptions
    DROP CONSTRAINT subscriptions_frequency_check,
    ADD CONSTRAINT subscriptions_frequency_check CHECK (frequency IN ('WEEKLY', 'BIWEEKLY', 'MONTHLY', 'QUARTERLY', 'ANNUALLY')),
    ADD COLUMN source TEXT NOT NULL DEFAULT 'MANUAL' CHECK (source IN ('MANUAL', 'DETECTED')),
    ADD COLUMN detection_key TEXT,
    ADD COLUMN confidence_basis_points INTEGER CHECK (confidence_basis_points BETWEEN 0 AND 10000),
    ADD COLUMN occurrence_count INTEGER CHECK (occurrence_count IS NULL OR occurrence_count >= 3),
    ADD COLUMN user_modified_at TIMESTAMPTZ,
    ADD CONSTRAINT subscriptions_detection_key_length CHECK (
        detection_key IS NULL OR char_length(detection_key) BETWEEN 1 AND 200
    );
CREATE UNIQUE INDEX subscriptions_user_detection_idx
    ON subscriptions (user_id, detection_key) WHERE detection_key IS NOT NULL;

CREATE TABLE recurring_income_sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 160),
    expected_amount_minor BIGINT NOT NULL CHECK (expected_amount_minor > 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    frequency TEXT NOT NULL CHECK (frequency IN ('WEEKLY', 'BIWEEKLY', 'MONTHLY', 'QUARTERLY', 'ANNUALLY')),
    next_expected_at DATE NOT NULL,
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    payment_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'PAUSED', 'CANCELLED', 'UNKNOWN')),
    source TEXT NOT NULL DEFAULT 'DETECTED' CHECK (source = 'DETECTED'),
    detection_key TEXT NOT NULL CHECK (char_length(detection_key) BETWEEN 1 AND 200),
    confidence_basis_points INTEGER NOT NULL CHECK (confidence_basis_points BETWEEN 0 AND 10000),
    occurrence_count INTEGER NOT NULL CHECK (occurrence_count >= 3),
    detected_from_transaction_id UUID REFERENCES transactions(id) ON DELETE SET NULL,
    user_modified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, detection_key)
);
CREATE INDEX recurring_income_sources_user_due_idx
    ON recurring_income_sources (user_id, next_expected_at, id) WHERE status = 'ACTIVE';

ALTER TABLE inbox_items
    DROP CONSTRAINT inbox_items_item_type_check,
    ADD CONSTRAINT inbox_items_item_type_check CHECK (item_type IN (
        'TRANSACTION_REVIEW', 'POSSIBLE_SUBSCRIPTION', 'POSSIBLE_RECURRING_PAYMENT',
        'POSSIBLE_RECURRING_INCOME', 'PRICE_CHANGE', 'UNUSUAL_TRANSACTION',
        'POSSIBLE_SHARED_EXPENSE', 'BANK_CONNECTION_ERROR', 'BUDGET_WARNING',
        'UPCOMING_SHORTFALL'
    ));

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
