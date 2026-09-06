BEGIN;

ALTER TABLE users
    ADD COLUMN display_name TEXT,
    ADD COLUMN email TEXT,
    ADD COLUMN timezone TEXT NOT NULL DEFAULT 'UTC',
	ADD COLUMN open_inbox_count INTEGER NOT NULL DEFAULT 0 CHECK (open_inbox_count >= 0);

CREATE TABLE households (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE household_members (
    household_id UUID NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('OWNER', 'MEMBER')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id)
);
CREATE UNIQUE INDEX household_owner_idx ON household_members (household_id) WHERE role = 'OWNER';
CREATE INDEX household_members_user_idx ON household_members (user_id, household_id);
CREATE UNIQUE INDEX household_members_one_household_idx ON household_members (user_id);

CREATE TABLE household_member_summaries (
	household_id UUID NOT NULL REFERENCES households(id) ON DELETE CASCADE,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
	paid_minor BIGINT NOT NULL DEFAULT 0 CHECK (paid_minor >= 0),
	owed_minor BIGINT NOT NULL DEFAULT 0 CHECK (owed_minor >= 0),
	difference_minor BIGINT NOT NULL DEFAULT 0,
	settlement_minor BIGINT,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (household_id, user_id, currency),
	FOREIGN KEY (household_id, user_id) REFERENCES household_members(household_id, user_id) ON DELETE CASCADE
);

CREATE TABLE household_summaries (
	household_id UUID NOT NULL REFERENCES households(id) ON DELETE CASCADE,
	currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
	total_spending_minor BIGINT NOT NULL DEFAULT 0 CHECK (total_spending_minor >= 0),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (household_id, currency)
);

CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    category_type TEXT NOT NULL CHECK (category_type IN ('INCOME', 'EXPENSE', 'TRANSFER')),
    parent_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX categories_user_idx ON categories (user_id, name, id);

CREATE TABLE tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, name)
);

CREATE TABLE spaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    household_id UUID REFERENCES households(id) ON DELETE SET NULL,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    space_type TEXT NOT NULL CHECK (space_type IN ('DAILY', 'BILLS', 'HOUSEHOLD', 'SAVINGS', 'GOAL', 'CUSTOM')),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    monthly_allocation_minor BIGINT NOT NULL DEFAULT 0 CHECK (monthly_allocation_minor >= 0),
	balance_minor BIGINT NOT NULL DEFAULT 0,
	balance_period_start DATE NOT NULL DEFAULT date_trunc('month', CURRENT_DATE)::date,
    protected BOOLEAN NOT NULL DEFAULT false,
    visibility TEXT NOT NULL DEFAULT 'PRIVATE' CHECK (visibility IN ('PRIVATE', 'HOUSEHOLD')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, name)
);
CREATE INDEX spaces_user_idx ON spaces (user_id, created_at, id);
CREATE INDEX spaces_user_name_prefix_idx ON spaces (user_id, lower(name) text_pattern_ops, id);
CREATE INDEX spaces_household_idx ON spaces (household_id, id) WHERE household_id IS NOT NULL;

ALTER TABLE transactions
    ADD COLUMN original_description TEXT,
    ADD COLUMN category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    ADD COLUMN review_status TEXT NOT NULL DEFAULT 'NEEDS_REVIEW'
        CHECK (review_status IN ('NEEDS_REVIEW', 'REVIEWED', 'IGNORED')),
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'PRIVATE'
        CHECK (visibility IN ('PRIVATE', 'HOUSEHOLD'));
CREATE INDEX transactions_user_category_idx
    ON transactions (user_id, category_id, transaction_date DESC, id DESC)
    WHERE removed_at IS NULL;
CREATE INDEX transactions_user_review_idx
    ON transactions (user_id, review_status, transaction_date DESC, id DESC)
    WHERE removed_at IS NULL;
CREATE INDEX transactions_user_account_idx
    ON transactions (user_id, account_id, transaction_date DESC, id DESC)
    WHERE removed_at IS NULL;
CREATE INDEX transactions_user_name_prefix_idx
    ON transactions (user_id, lower(name) text_pattern_ops, id)
    WHERE removed_at IS NULL;
CREATE INDEX accounts_user_name_prefix_idx ON accounts (user_id, lower(name) text_pattern_ops, id);

CREATE TABLE transaction_tags (
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (transaction_id, tag_id)
);
CREATE INDEX transaction_tags_tag_idx ON transaction_tags (tag_id, transaction_id);

CREATE TABLE transaction_allocations (
    transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
    space_id UUID NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    transaction_date DATE NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor <> 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (transaction_id, space_id)
);
CREATE INDEX transaction_allocations_space_idx ON transaction_allocations (space_id, transaction_id);
CREATE INDEX transaction_allocations_space_page_idx
    ON transaction_allocations (space_id, transaction_date DESC, transaction_id DESC);

CREATE TABLE expense_splits (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	transaction_id UUID NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
	payer_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	participant_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
	status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'ACCEPTED', 'SETTLED', 'DECLINED')),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (transaction_id, participant_user_id),
	CHECK (payer_user_id <> participant_user_id)
);
CREATE INDEX expense_splits_participant_idx ON expense_splits (participant_user_id, status, created_at DESC, id DESC);

CREATE TABLE bills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    amount_type TEXT NOT NULL CHECK (amount_type IN ('FIXED', 'VARIABLE')),
    expected_amount_minor BIGINT NOT NULL CHECK (expected_amount_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    frequency TEXT NOT NULL CHECK (frequency IN ('WEEKLY', 'BIWEEKLY', 'MONTHLY', 'QUARTERLY', 'ANNUALLY')),
    next_due_at DATE NOT NULL,
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    space_id UUID REFERENCES spaces(id) ON DELETE SET NULL,
    source TEXT NOT NULL CHECK (source IN ('MANUAL', 'DETECTED')),
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'PAUSED', 'CANCELLED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX bills_user_due_idx ON bills (user_id, next_due_at, id) WHERE status = 'ACTIVE';
CREATE INDEX bills_user_name_prefix_idx ON bills (user_id, lower(name) text_pattern_ops, id);

CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    merchant_name TEXT NOT NULL CHECK (char_length(merchant_name) BETWEEN 1 AND 160),
    expected_amount_minor BIGINT NOT NULL CHECK (expected_amount_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    frequency TEXT NOT NULL CHECK (frequency IN ('WEEKLY', 'MONTHLY', 'QUARTERLY', 'ANNUALLY')),
    next_expected_at DATE NOT NULL,
    category_id UUID REFERENCES categories(id) ON DELETE SET NULL,
    space_id UUID REFERENCES spaces(id) ON DELETE SET NULL,
    payment_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'PAUSED', 'CANCELLED', 'UNKNOWN')),
    detected_from_transaction_id UUID REFERENCES transactions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX subscriptions_user_due_idx ON subscriptions (user_id, next_expected_at, id) WHERE status = 'ACTIVE';
CREATE INDEX subscriptions_user_name_prefix_idx ON subscriptions (user_id, lower(merchant_name) text_pattern_ops, id);

CREATE TABLE budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    category_id UUID REFERENCES categories(id) ON DELETE CASCADE,
    space_id UUID REFERENCES spaces(id) ON DELETE CASCADE,
    period TEXT NOT NULL CHECK (period IN ('WEEKLY', 'MONTHLY', 'ANNUAL')),
    limit_minor BIGINT NOT NULL CHECK (limit_minor > 0),
	spent_minor BIGINT NOT NULL DEFAULT 0 CHECK (spent_minor >= 0),
	period_start DATE NOT NULL DEFAULT CURRENT_DATE,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    warning_threshold INTEGER NOT NULL CHECK (warning_threshold BETWEEN 1 AND 100),
    critical_threshold INTEGER NOT NULL CHECK (critical_threshold BETWEEN warning_threshold AND 100),
    carryover_enabled BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((category_id IS NOT NULL)::integer + (space_id IS NOT NULL)::integer = 1)
);
CREATE INDEX budgets_user_idx ON budgets (user_id, created_at, id);

CREATE TABLE goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    household_id UUID REFERENCES households(id) ON DELETE SET NULL,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    target_minor BIGINT NOT NULL CHECK (target_minor > 0),
    current_minor BIGINT NOT NULL DEFAULT 0 CHECK (current_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    target_date DATE,
    space_id UUID REFERENCES spaces(id) ON DELETE SET NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'COMPLETED', 'PAUSED', 'CANCELLED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX goals_user_idx ON goals (user_id, status, target_date, id);
CREATE INDEX goals_user_name_prefix_idx ON goals (user_id, lower(name) text_pattern_ops, id);

CREATE TABLE rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    priority INTEGER NOT NULL CHECK (priority > 0),
    enabled BOOLEAN NOT NULL DEFAULT true,
    conditions JSONB NOT NULL,
    actions JSONB NOT NULL,
    match_count BIGINT NOT NULL DEFAULT 0 CHECK (match_count >= 0),
    last_run_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, priority)
);
CREATE INDEX rules_user_idx ON rules (user_id, priority, id);
CREATE INDEX rules_user_name_prefix_idx ON rules (user_id, lower(name) text_pattern_ops, id);

CREATE TABLE inbox_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_type TEXT NOT NULL CHECK (item_type IN (
        'TRANSACTION_REVIEW', 'POSSIBLE_SUBSCRIPTION', 'PRICE_CHANGE', 'UNUSUAL_TRANSACTION',
        'POSSIBLE_SHARED_EXPENSE', 'BANK_CONNECTION_ERROR', 'BUDGET_WARNING', 'UPCOMING_SHORTFALL'
    )),
    priority TEXT NOT NULL CHECK (priority IN ('LOW', 'NORMAL', 'HIGH', 'URGENT')),
    entity_type TEXT NOT NULL,
    entity_id UUID,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    status TEXT NOT NULL DEFAULT 'OPEN' CHECK (status IN ('OPEN', 'RESOLVED', 'DISMISSED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);
CREATE INDEX inbox_items_user_open_idx ON inbox_items (user_id, priority DESC, created_at DESC, id DESC) WHERE status = 'OPEN';

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type TEXT NOT NULL,
    title TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 200),
    body TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 1000),
    entity_type TEXT,
    entity_id UUID,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC, id DESC);

CREATE TABLE notification_preferences (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    notification_type TEXT NOT NULL,
    in_app_enabled BOOLEAN NOT NULL DEFAULT true,
    email_enabled BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, notification_type)
);

CREATE TABLE manual_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    asset_type TEXT NOT NULL CHECK (asset_type IN ('PROPERTY', 'VEHICLE', 'INVESTMENT', 'CASH', 'OTHER')),
    value_minor BIGINT NOT NULL CHECK (value_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    include_in_net_worth BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX manual_assets_user_idx ON manual_assets (user_id, created_at, id);

CREATE TABLE manual_liabilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    liability_type TEXT NOT NULL CHECK (liability_type IN ('LOAN', 'MORTGAGE', 'CREDIT_CARD', 'OTHER')),
    balance_minor BIGINT NOT NULL CHECK (balance_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    include_in_net_worth BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX manual_liabilities_user_idx ON manual_liabilities (user_id, created_at, id);

CREATE TABLE loans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    principal_remaining_minor BIGINT NOT NULL CHECK (principal_remaining_minor >= 0),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    interest_rate_basis_points INTEGER NOT NULL CHECK (interest_rate_basis_points >= 0),
    monthly_payment_minor BIGINT NOT NULL CHECK (monthly_payment_minor > 0),
    next_payment_at DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX loans_user_idx ON loans (user_id, next_payment_at, id);

CREATE TABLE loan_payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    loan_id UUID NOT NULL REFERENCES loans(id) ON DELETE CASCADE,
    paid_at DATE NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    principal_minor BIGINT NOT NULL CHECK (principal_minor >= 0),
    interest_minor BIGINT NOT NULL CHECK (interest_minor >= 0),
    CHECK (amount_minor = principal_minor + interest_minor)
);
CREATE INDEX loan_payments_loan_idx ON loan_payments (loan_id, paid_at DESC, id DESC);

CREATE TABLE net_worth_snapshots (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    snapshot_date DATE NOT NULL,
    value_minor BIGINT NOT NULL,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    PRIMARY KEY (user_id, snapshot_date, currency)
);

CREATE TABLE financial_projections (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    total_minor BIGINT NOT NULL,
    available_minor BIGINT NOT NULL,
    protected_minor BIGINT NOT NULL,
    projected_month_end_minor BIGINT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ON_TRACK', 'WATCH', 'AT_RISK')),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, currency)
);

CREATE TABLE analytics_snapshots (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	view_type TEXT NOT NULL CHECK (view_type IN ('SPENDING', 'INCOME', 'CASH_FLOW', 'CATEGORIES', 'MERCHANTS', 'RECURRING')),
	currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
	period_start DATE NOT NULL,
	period_end DATE NOT NULL,
	total_minor BIGINT NOT NULL,
	breakdown JSONB NOT NULL CHECK (jsonb_typeof(breakdown) = 'array' AND jsonb_array_length(breakdown) <= 512),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, view_type, currency, period_start, period_end),
	CHECK (period_end >= period_start)
);
CREATE INDEX analytics_snapshots_user_idx ON analytics_snapshots (user_id, view_type, currency, period_end DESC, period_start DESC);

CREATE TABLE cash_flow_snapshots (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
	period_start DATE NOT NULL,
	period_end DATE NOT NULL,
	actual JSONB NOT NULL CHECK (jsonb_typeof(actual) = 'object'),
	projected JSONB NOT NULL CHECK (jsonb_typeof(projected) = 'object'),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, currency, period_start, period_end),
	CHECK (period_end >= period_start)
);
CREATE INDEX cash_flow_snapshots_user_idx ON cash_flow_snapshots (user_id, currency, period_end DESC, period_start DESC);

CREATE TABLE timeline_snapshots (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
	as_of_date DATE NOT NULL,
	points JSONB NOT NULL CHECK (jsonb_typeof(points) = 'array' AND jsonb_array_length(points) <= 512),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, currency, as_of_date)
);
CREATE INDEX timeline_snapshots_user_idx ON timeline_snapshots (user_id, currency, as_of_date DESC);

CREATE TABLE audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_user_id, created_at DESC, id DESC);

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
