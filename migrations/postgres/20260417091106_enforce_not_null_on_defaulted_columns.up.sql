-- Migration: enforce_not_null_on_defaulted_columns
-- Created: 2026-04-17
--
-- Background: dozens of columns were declared with a DEFAULT clause but no
-- NOT NULL constraint — a classic ORM-era habit. GORM populated the
-- fields in Go so the missing constraint never showed up at runtime.
-- Moving to sqlc + pgx surfaces the lie: sqlc emits pointer types for
-- nullable columns, forcing every repository to deref/wrap at the
-- boundary even though the columns are semantically never-null.
--
-- Fix: add NOT NULL to every column that is never null in practice.
-- Uses plain DDL (no DO blocks) so sqlc's static analyzer can pick up
-- the new constraints and stop emitting *time.Time / *string / etc.
-- Every UPDATE is no-op for a freshly-bootstrapped DB; it is the
-- belt-and-suspenders path for replays against a clone that already
-- contains legitimate rows.
--
-- Columns referenced below are confirmed present in the HEAD schema as
-- of this migration. Columns dropped by earlier migrations (e.g.
-- api_keys.rate_limit_rpm, api_keys.is_active, playground_sessions.is_saved,
-- users.onboarding_completed, roles.is_system_role) are intentionally
-- omitted.
--
-- Left nullable on purpose (domain semantics require it):
--   users.email_verified_at, users.last_login_at, users.deleted_at
--   user_sessions.last_used_at
--   *_tokens.used_at, *_tokens.revoked_at (where legitimately optional)
--   invitations.accepted_at / revoked_at / used_at
--   contracts.ended_at, contracts.cancelled_at

BEGIN;

-- =====================================================================
-- created_at / updated_at timestamps
-- Every row always has them; DEFAULT NOW() already ensures it.
-- =====================================================================

UPDATE users                     SET created_at = NOW() WHERE created_at IS NULL;
UPDATE users                     SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE users                ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE users                ALTER COLUMN updated_at SET NOT NULL;

UPDATE organizations             SET created_at = NOW() WHERE created_at IS NULL;
UPDATE organizations             SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE organizations        ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE organizations        ALTER COLUMN updated_at SET NOT NULL;

UPDATE projects                  SET created_at = NOW() WHERE created_at IS NULL;
UPDATE projects                  SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE projects             ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE projects             ALTER COLUMN updated_at SET NOT NULL;

UPDATE api_keys                  SET created_at = NOW() WHERE created_at IS NULL;
UPDATE api_keys                  SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE api_keys             ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE api_keys             ALTER COLUMN updated_at SET NOT NULL;

UPDATE roles                     SET created_at = NOW() WHERE created_at IS NULL;
UPDATE roles                     SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE roles                ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE roles                ALTER COLUMN updated_at SET NOT NULL;

UPDATE permissions               SET created_at = NOW() WHERE created_at IS NULL;
ALTER TABLE permissions          ALTER COLUMN created_at SET NOT NULL;

-- organization_members / project_members use joined_at as their creation
-- timestamp; they have no separate created_at/updated_at columns. The
-- joined_at NOT NULL constraint is applied further below.

UPDATE audit_logs                SET created_at = NOW() WHERE created_at IS NULL;
ALTER TABLE audit_logs           ALTER COLUMN created_at SET NOT NULL;

-- The drop migration 20251101020000 referenced "onboarding_responses"
-- (a no-op: the table is actually called user_onboarding_responses and
-- still exists), so tighten its nullable fields here.
UPDATE user_onboarding_responses SET created_at = NOW()  WHERE created_at IS NULL;
UPDATE user_onboarding_responses SET skipped    = FALSE  WHERE skipped    IS NULL;
ALTER TABLE user_onboarding_responses ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE user_onboarding_responses ALTER COLUMN skipped    SET NOT NULL;

UPDATE password_reset_tokens     SET created_at = NOW() WHERE created_at IS NULL;
UPDATE password_reset_tokens     SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE password_reset_tokens ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE password_reset_tokens ALTER COLUMN updated_at SET NOT NULL;

UPDATE email_verification_tokens SET created_at = NOW() WHERE created_at IS NULL;
UPDATE email_verification_tokens SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE email_verification_tokens ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE email_verification_tokens ALTER COLUMN updated_at SET NOT NULL;

UPDATE user_profiles             SET created_at = NOW() WHERE created_at IS NULL;
UPDATE user_profiles             SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE user_profiles        ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE user_profiles        ALTER COLUMN updated_at SET NOT NULL;

UPDATE user_invitations          SET created_at = NOW() WHERE created_at IS NULL;
UPDATE user_invitations          SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE user_invitations     ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE user_invitations     ALTER COLUMN updated_at SET NOT NULL;

UPDATE organization_settings     SET created_at = NOW() WHERE created_at IS NULL;
UPDATE organization_settings     SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE organization_settings ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE organization_settings ALTER COLUMN updated_at SET NOT NULL;

UPDATE contracts                 SET created_at = NOW() WHERE created_at IS NULL;
UPDATE contracts                 SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE contracts            ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE contracts            ALTER COLUMN updated_at SET NOT NULL;

UPDATE volume_discount_tiers     SET created_at = NOW() WHERE created_at IS NULL;
ALTER TABLE volume_discount_tiers ALTER COLUMN created_at SET NOT NULL;

-- pricing_configs was renamed to plans by migration 20260106153135.
UPDATE plans                     SET created_at = NOW() WHERE created_at IS NULL;
UPDATE plans                     SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE plans                ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE plans                ALTER COLUMN updated_at SET NOT NULL;

UPDATE usage_budgets             SET created_at = NOW() WHERE created_at IS NULL;
UPDATE usage_budgets             SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE usage_budgets        ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE usage_budgets        ALTER COLUMN updated_at SET NOT NULL;

UPDATE organization_billings     SET created_at = NOW() WHERE created_at IS NULL;
UPDATE organization_billings     SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE organization_billings ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE organization_billings ALTER COLUMN updated_at SET NOT NULL;

UPDATE prompts                   SET created_at = NOW() WHERE created_at IS NULL;
UPDATE prompts                   SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE prompts              ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE prompts              ALTER COLUMN updated_at SET NOT NULL;

UPDATE prompt_versions           SET created_at = NOW() WHERE created_at IS NULL;
ALTER TABLE prompt_versions      ALTER COLUMN created_at SET NOT NULL;

UPDATE prompt_labels             SET created_at = NOW() WHERE created_at IS NULL;
UPDATE prompt_labels             SET updated_at = NOW() WHERE updated_at IS NULL;
ALTER TABLE prompt_labels        ALTER COLUMN created_at SET NOT NULL;
ALTER TABLE prompt_labels        ALTER COLUMN updated_at SET NOT NULL;

UPDATE prompt_protected_labels   SET created_at = NOW() WHERE created_at IS NULL;
ALTER TABLE prompt_protected_labels ALTER COLUMN created_at SET NOT NULL;

-- =====================================================================
-- Domain timestamps other than created_at/updated_at
-- =====================================================================

UPDATE role_permissions          SET granted_at = NOW() WHERE granted_at IS NULL;
ALTER TABLE role_permissions     ALTER COLUMN granted_at SET NOT NULL;

UPDATE organization_members      SET joined_at = NOW() WHERE joined_at IS NULL;
ALTER TABLE organization_members ALTER COLUMN joined_at SET NOT NULL;

UPDATE project_members           SET joined_at = NOW() WHERE joined_at IS NULL;
ALTER TABLE project_members      ALTER COLUMN joined_at SET NOT NULL;

UPDATE contract_history          SET changed_at = NOW() WHERE changed_at IS NULL;
ALTER TABLE contract_history     ALTER COLUMN changed_at SET NOT NULL;

UPDATE usage_alerts              SET triggered_at = NOW() WHERE triggered_at IS NULL;
ALTER TABLE usage_alerts         ALTER COLUMN triggered_at SET NOT NULL;

UPDATE organization_billings     SET last_synced_at = NOW() WHERE last_synced_at IS NULL;
ALTER TABLE organization_billings ALTER COLUMN last_synced_at SET NOT NULL;

-- =====================================================================
-- Booleans: flags with meaningful defaults are never semantically null.
-- =====================================================================

UPDATE users                     SET is_active         = TRUE  WHERE is_active         IS NULL;
UPDATE users                     SET is_email_verified = FALSE WHERE is_email_verified IS NULL;
ALTER TABLE users                ALTER COLUMN is_active         SET NOT NULL;
ALTER TABLE users                ALTER COLUMN is_email_verified SET NOT NULL;

UPDATE usage_alerts              SET notification_sent = FALSE WHERE notification_sent IS NULL;
ALTER TABLE usage_alerts         ALTER COLUMN notification_sent SET NOT NULL;

UPDATE user_profiles             SET email_notifications = TRUE  WHERE email_notifications IS NULL;
UPDATE user_profiles             SET push_notifications  = TRUE  WHERE push_notifications  IS NULL;
UPDATE user_profiles             SET weekly_reports      = TRUE  WHERE weekly_reports      IS NULL;
UPDATE user_profiles             SET monthly_reports     = TRUE  WHERE monthly_reports     IS NULL;
UPDATE user_profiles             SET security_alerts     = TRUE  WHERE security_alerts     IS NULL;
UPDATE user_profiles             SET billing_alerts      = TRUE  WHERE billing_alerts      IS NULL;
UPDATE user_profiles             SET marketing_emails    = FALSE WHERE marketing_emails    IS NULL;
ALTER TABLE user_profiles        ALTER COLUMN email_notifications SET NOT NULL;
ALTER TABLE user_profiles        ALTER COLUMN push_notifications  SET NOT NULL;
ALTER TABLE user_profiles        ALTER COLUMN weekly_reports      SET NOT NULL;
ALTER TABLE user_profiles        ALTER COLUMN monthly_reports     SET NOT NULL;
ALTER TABLE user_profiles        ALTER COLUMN security_alerts     SET NOT NULL;
ALTER TABLE user_profiles        ALTER COLUMN billing_alerts      SET NOT NULL;
ALTER TABLE user_profiles        ALTER COLUMN marketing_emails    SET NOT NULL;

UPDATE email_verification_tokens SET used = FALSE WHERE used IS NULL;
ALTER TABLE email_verification_tokens ALTER COLUMN used SET NOT NULL;

-- =====================================================================
-- Integers / numerics — counters and quotas always seeded with defaults
-- =====================================================================

UPDATE users                     SET login_count             = 0  WHERE login_count             IS NULL;
ALTER TABLE users                ALTER COLUMN login_count             SET NOT NULL;

UPDATE user_profiles             SET usage_threshold_percent = 80 WHERE usage_threshold_percent IS NULL;
ALTER TABLE user_profiles        ALTER COLUMN usage_threshold_percent SET NOT NULL;

UPDATE organization_billings     SET billing_cycle_anchor_day = 1         WHERE billing_cycle_anchor_day IS NULL;
UPDATE organization_billings     SET current_period_cost      = 0         WHERE current_period_cost      IS NULL;
UPDATE organization_billings     SET free_spans_remaining     = 1000000   WHERE free_spans_remaining     IS NULL;
UPDATE organization_billings     SET free_bytes_remaining     = 1073741824 WHERE free_bytes_remaining    IS NULL;
UPDATE organization_billings     SET free_scores_remaining    = 10000     WHERE free_scores_remaining    IS NULL;
ALTER TABLE organization_billings ALTER COLUMN billing_cycle_anchor_day SET NOT NULL;
ALTER TABLE organization_billings ALTER COLUMN current_period_cost      SET NOT NULL;
ALTER TABLE organization_billings ALTER COLUMN free_spans_remaining     SET NOT NULL;
ALTER TABLE organization_billings ALTER COLUMN free_bytes_remaining     SET NOT NULL;
ALTER TABLE organization_billings ALTER COLUMN free_scores_remaining    SET NOT NULL;

-- =====================================================================
-- Strings — locales, plans, themes, statuses always have a default
-- =====================================================================

UPDATE users                     SET timezone    = 'UTC'      WHERE timezone    IS NULL;
UPDATE users                     SET language    = 'en'       WHERE language    IS NULL;
UPDATE users                     SET auth_method = 'password' WHERE auth_method IS NULL;
ALTER TABLE users                ALTER COLUMN timezone    SET NOT NULL;
ALTER TABLE users                ALTER COLUMN language    SET NOT NULL;
ALTER TABLE users                ALTER COLUMN auth_method SET NOT NULL;

UPDATE organizations             SET plan                = 'free'   WHERE plan                IS NULL;
UPDATE organizations             SET subscription_status = 'active' WHERE subscription_status IS NULL;
ALTER TABLE organizations        ALTER COLUMN plan                SET NOT NULL;
ALTER TABLE organizations        ALTER COLUMN subscription_status SET NOT NULL;

UPDATE user_profiles             SET timezone = 'UTC'   WHERE timezone IS NULL;
UPDATE user_profiles             SET language = 'en'    WHERE language IS NULL;
UPDATE user_profiles             SET theme    = 'light' WHERE theme    IS NULL;
ALTER TABLE user_profiles        ALTER COLUMN timezone SET NOT NULL;
ALTER TABLE user_profiles        ALTER COLUMN language SET NOT NULL;
ALTER TABLE user_profiles        ALTER COLUMN theme    SET NOT NULL;

UPDATE organization_members      SET status = 'active'    WHERE status IS NULL;
UPDATE project_members           SET status = 'active'    WHERE status IS NULL;
UPDATE usage_alerts              SET status = 'triggered' WHERE status IS NULL;
ALTER TABLE organization_members ALTER COLUMN status SET NOT NULL;
ALTER TABLE project_members      ALTER COLUMN status SET NOT NULL;
ALTER TABLE usage_alerts         ALTER COLUMN status SET NOT NULL;

-- =====================================================================
-- JSONB / array defaults — empty object/array is the valid "no data" form
-- =====================================================================

UPDATE prompts                   SET tags      = '{}'::text[] WHERE tags      IS NULL;
ALTER TABLE prompts              ALTER COLUMN tags      SET NOT NULL;

UPDATE prompt_versions           SET variables = '{}'::text[] WHERE variables IS NULL;
ALTER TABLE prompt_versions      ALTER COLUMN variables SET NOT NULL;

UPDATE annotation_queues         SET settings         = '{}'::jsonb WHERE settings         IS NULL;
UPDATE annotation_queues         SET score_config_ids = '[]'::jsonb WHERE score_config_ids IS NULL;
ALTER TABLE annotation_queues    ALTER COLUMN settings         SET NOT NULL;
ALTER TABLE annotation_queues    ALTER COLUMN score_config_ids SET NOT NULL;

UPDATE annotation_queue_items    SET metadata = '{}'::jsonb WHERE metadata IS NULL;
ALTER TABLE annotation_queue_items ALTER COLUMN metadata SET NOT NULL;

UPDATE filter_presets            SET column_order      = '[]'::jsonb WHERE column_order      IS NULL;
UPDATE filter_presets            SET column_visibility = '{}'::jsonb WHERE column_visibility IS NULL;
ALTER TABLE filter_presets       ALTER COLUMN column_order      SET NOT NULL;
ALTER TABLE filter_presets       ALTER COLUMN column_visibility SET NOT NULL;

UPDATE playground_sessions       SET variables = '{}'::jsonb WHERE variables IS NULL;
ALTER TABLE playground_sessions  ALTER COLUMN variables SET NOT NULL;

-- Billing boolean schema-lies: these columns have DEFAULT but no NOT NULL,
-- which sqlc correctly flags as *bool. Every seed and INSERT sets a value,
-- so runtime leakage of nil would silently mark plans inactive / budgets
-- disabled / no default plan — none of which produce a stack trace.
-- Fixed at the schema rather than deref'd at the boundary.
UPDATE plans                     SET is_active  = TRUE  WHERE is_active  IS NULL;
UPDATE plans                     SET is_default = FALSE WHERE is_default IS NULL;
ALTER TABLE plans                ALTER COLUMN is_active  SET NOT NULL;
ALTER TABLE plans                ALTER COLUMN is_default SET NOT NULL;

UPDATE usage_budgets             SET is_active = TRUE WHERE is_active IS NULL;
ALTER TABLE usage_budgets        ALTER COLUMN is_active SET NOT NULL;

COMMIT;
