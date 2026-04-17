-- Rollback: enforce_not_null_on_defaulted_columns
-- Created: 2026-04-17
--
-- Drops the NOT NULL constraint from every column the up migration
-- tightened. Plain DDL, symmetric with the up migration.

BEGIN;

-- created_at / updated_at
ALTER TABLE users                     ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE users                     ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE organizations             ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE organizations             ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE projects                  ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE projects                  ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE api_keys                  ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE api_keys                  ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE roles                     ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE roles                     ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE permissions               ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE audit_logs                ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE user_onboarding_responses ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE user_onboarding_responses ALTER COLUMN skipped      DROP NOT NULL;
ALTER TABLE password_reset_tokens     ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE password_reset_tokens     ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE email_verification_tokens ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE email_verification_tokens ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE user_invitations          ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE user_invitations          ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE organization_settings     ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE organization_settings     ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE contracts                 ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE contracts                 ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE volume_discount_tiers     ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE plans                     ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE plans                     ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE usage_budgets             ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE usage_budgets             ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE organization_billings     ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE organization_billings     ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE prompts                   ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE prompts                   ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE prompt_versions           ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE prompt_labels             ALTER COLUMN created_at  DROP NOT NULL;
ALTER TABLE prompt_labels             ALTER COLUMN updated_at  DROP NOT NULL;
ALTER TABLE prompt_protected_labels   ALTER COLUMN created_at  DROP NOT NULL;

-- Domain timestamps
ALTER TABLE role_permissions          ALTER COLUMN granted_at     DROP NOT NULL;
ALTER TABLE organization_members      ALTER COLUMN joined_at      DROP NOT NULL;
ALTER TABLE project_members           ALTER COLUMN joined_at      DROP NOT NULL;
ALTER TABLE contract_history          ALTER COLUMN changed_at     DROP NOT NULL;
ALTER TABLE usage_alerts              ALTER COLUMN triggered_at   DROP NOT NULL;
ALTER TABLE organization_billings     ALTER COLUMN last_synced_at DROP NOT NULL;

-- Booleans
ALTER TABLE users                     ALTER COLUMN is_active             DROP NOT NULL;
ALTER TABLE users                     ALTER COLUMN is_email_verified     DROP NOT NULL;
ALTER TABLE usage_alerts              ALTER COLUMN notification_sent     DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN email_notifications   DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN push_notifications    DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN weekly_reports        DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN monthly_reports       DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN security_alerts       DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN billing_alerts        DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN marketing_emails      DROP NOT NULL;
ALTER TABLE email_verification_tokens ALTER COLUMN used                  DROP NOT NULL;
ALTER TABLE plans                     ALTER COLUMN is_active             DROP NOT NULL;
ALTER TABLE plans                     ALTER COLUMN is_default            DROP NOT NULL;
ALTER TABLE usage_budgets             ALTER COLUMN is_active             DROP NOT NULL;

-- Integers
ALTER TABLE users                     ALTER COLUMN login_count              DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN usage_threshold_percent  DROP NOT NULL;
ALTER TABLE organization_billings     ALTER COLUMN billing_cycle_anchor_day DROP NOT NULL;
ALTER TABLE organization_billings     ALTER COLUMN current_period_cost      DROP NOT NULL;
ALTER TABLE organization_billings     ALTER COLUMN free_spans_remaining     DROP NOT NULL;
ALTER TABLE organization_billings     ALTER COLUMN free_bytes_remaining     DROP NOT NULL;
ALTER TABLE organization_billings     ALTER COLUMN free_scores_remaining    DROP NOT NULL;

-- Strings
ALTER TABLE users                     ALTER COLUMN timezone            DROP NOT NULL;
ALTER TABLE users                     ALTER COLUMN language            DROP NOT NULL;
ALTER TABLE users                     ALTER COLUMN auth_method         DROP NOT NULL;
ALTER TABLE organizations             ALTER COLUMN plan                DROP NOT NULL;
ALTER TABLE organizations             ALTER COLUMN subscription_status DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN timezone            DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN language            DROP NOT NULL;
ALTER TABLE user_profiles             ALTER COLUMN theme               DROP NOT NULL;
ALTER TABLE organization_members      ALTER COLUMN status              DROP NOT NULL;
ALTER TABLE project_members           ALTER COLUMN status              DROP NOT NULL;
ALTER TABLE usage_alerts              ALTER COLUMN status              DROP NOT NULL;

-- JSONB / arrays
ALTER TABLE prompts                   ALTER COLUMN tags               DROP NOT NULL;
ALTER TABLE prompt_versions           ALTER COLUMN variables          DROP NOT NULL;
ALTER TABLE annotation_queues         ALTER COLUMN settings           DROP NOT NULL;
ALTER TABLE annotation_queues         ALTER COLUMN score_config_ids   DROP NOT NULL;
ALTER TABLE annotation_queue_items    ALTER COLUMN metadata           DROP NOT NULL;
ALTER TABLE filter_presets            ALTER COLUMN column_order       DROP NOT NULL;
ALTER TABLE filter_presets            ALTER COLUMN column_visibility  DROP NOT NULL;
ALTER TABLE playground_sessions       ALTER COLUMN variables          DROP NOT NULL;

COMMIT;
