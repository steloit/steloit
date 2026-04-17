-- Rollback: drop_api_keys_metadata_column
-- Created: 2026-04-17T16:11:50+05:30
--
-- Restore the column in its original shape (nullable JSONB, no default)
-- for rollback parity with the pre-migration schema. Data that was in the
-- column before the up migration is not recoverable by rollback.

ALTER TABLE api_keys ADD COLUMN metadata JSONB;
