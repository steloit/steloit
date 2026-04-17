-- Migration: drop_api_keys_metadata_column
-- Created: 2026-04-17T16:11:50+05:30
--
-- The column was added 2025-09-06 (commit 0ddaea2) but never wired into any
-- read or write path. The domain APIKey struct has no Metadata field, and
-- grep confirms no other Go/SQL/docs/SDK/ee code references it. Removing it
-- is the root-cause fix for a regression where the sqlc UpdateAPIKey path
-- was overwriting the column with '{}' on every update.
--
-- DROP COLUMN on a single JSONB is catalog-only in Postgres (no table
-- rewrite). Data that existed in the column is not recoverable — none does,
-- by the audit above. Git history preserves the original migration if a
-- future real requirement reintroduces metadata.

ALTER TABLE api_keys DROP COLUMN metadata;
