BEGIN;

ALTER TABLE net_worth_snapshots
    ADD COLUMN total_assets_minor BIGINT CHECK (total_assets_minor IS NULL OR total_assets_minor >= 0),
    ADD COLUMN total_liabilities_minor BIGINT CHECK (total_liabilities_minor IS NULL OR total_liabilities_minor >= 0);

ALTER TABLE timeline_snapshots
    ADD COLUMN projection_end_date DATE,
    ADD COLUMN starting_balance_minor BIGINT,
    ADD COLUMN ending_balance_minor BIGINT,
    ADD COLUMN minimum_balance_minor BIGINT,
    ADD COLUMN minimum_balance_date DATE,
    ADD CONSTRAINT timeline_snapshots_projection_range_check
        CHECK (projection_end_date IS NULL OR projection_end_date >= as_of_date),
    ADD CONSTRAINT timeline_snapshots_minimum_date_check
        CHECK (
            minimum_balance_date IS NULL OR
            (minimum_balance_date >= as_of_date AND minimum_balance_date <= projection_end_date)
        );

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
