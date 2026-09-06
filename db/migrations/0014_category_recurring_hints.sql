BEGIN;

ALTER TABLE categories
    ADD COLUMN recurring_kind_hint TEXT,
    ADD CONSTRAINT categories_recurring_kind_hint_check CHECK (
        recurring_kind_hint IS NULL OR recurring_kind_hint IN ('BILL', 'SUBSCRIPTION')
    );

UPDATE categories
SET recurring_kind_hint = CASE system_key
        WHEN 'subscriptions' THEN 'SUBSCRIPTION'
        WHEN 'housing' THEN 'BILL'
        WHEN 'utilities' THEN 'BILL'
        WHEN 'insurance' THEN 'BILL'
        WHEN 'debt' THEN 'BILL'
    END,
    name = CASE system_key
        WHEN 'debt' THEN 'Loan & Debt Payments'
        ELSE name
    END,
    updated_at = now()
WHERE system_key IN ('subscriptions', 'housing', 'utilities', 'insurance', 'debt');

INSERT INTO categories (
    id, user_id, name, category_type, system_key, recurring_kind_hint
) VALUES (
    '00000000-0000-4000-8000-000000000019', NULL, 'Bills', 'EXPENSE', 'bills', 'BILL'
);

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
