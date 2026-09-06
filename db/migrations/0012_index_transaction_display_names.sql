BEGIN;

CREATE INDEX transactions_user_display_name_prefix_idx
    ON transactions (user_id, lower(COALESCE(merchant_name, name)) text_pattern_ops, id)
    WHERE removed_at IS NULL;

INSERT INTO schema_migrations (version) VALUES (:'migration_version');

COMMIT;
