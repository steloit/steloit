-- Drop the orphaned per-request billing tables.
--
-- Context: the usage_records / usage_quotas tables were scaffolded for
-- a per-request billing model (UsageRecord / CostMetric / RecordUsage),
-- which was superseded by the ClickHouse-backed BillableUsage pipeline
-- (billable_usage_hourly / billable_usage_daily) before it was wired.
-- An exhaustive audit confirmed zero writers, zero readers, no FKs from
-- other tables, no frontend references, no worker references. Per
-- CLAUDE.md policy ("scaffolded-but-unreachable code must be deleted,
-- not kept for later"), the tables go alongside the Go code that
-- defined them.
--
-- No data loss concern: neither table has ever been written to in
-- production. Existing rows in dev only, none elsewhere.

DROP TABLE IF EXISTS usage_records;
DROP TABLE IF EXISTS usage_quotas;
