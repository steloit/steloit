-- ClickHouse Migration: add_scores_materialized_views
-- Created: 2026-01-11T23:22:12+05:30
--
-- Materialized views for scores analytics optimization
-- These views pre-aggregate score data for faster dashboard queries
-- Uses AggregatingMergeTree with -State/-Merge combinators for correct min/max aggregation

-- ============================================================================
-- Daily Score Summary View
-- ============================================================================
-- Pre-aggregates score metrics by project, name, and day for time-series charts
-- Uses AggregatingMergeTree with -State combinators for correct aggregation during merges

CREATE MATERIALIZED VIEW IF NOT EXISTS scores_daily_summary
ENGINE = AggregatingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (project_id, name, day)
SETTINGS index_granularity = 8192
POPULATE
AS SELECT
    project_id,
    name,
    toDate(timestamp) AS day,
    countState() AS count_state,
    sumState(value) AS sum_state,
    minState(value) AS min_state,
    maxState(value) AS max_state
FROM scores
WHERE value IS NOT NULL
GROUP BY project_id, name, day;

-- ============================================================================
-- Scores by Experiment View
-- ============================================================================
-- Pre-aggregates score metrics by project, experiment, and name for experiment comparisons
-- Uses AggregatingMergeTree with -State combinators for correct aggregation during merges
-- Note: allow_nullable_key=1 required because experiment_id is Nullable(UUID) in source

CREATE MATERIALIZED VIEW IF NOT EXISTS scores_by_experiment
ENGINE = AggregatingMergeTree()
PARTITION BY project_id
ORDER BY (project_id, experiment_id, name)
SETTINGS index_granularity = 8192, allow_nullable_key = 1
POPULATE
AS SELECT
    project_id,
    experiment_id,
    name,
    countState() AS count_state,
    sumState(value) AS sum_state,
    minState(value) AS min_state,
    maxState(value) AS max_state
FROM scores
WHERE experiment_id IS NOT NULL AND experiment_id != '' AND value IS NOT NULL
GROUP BY project_id, experiment_id, name;

-- ============================================================================
-- Scores by Trace View
-- ============================================================================
-- Pre-aggregates score metrics by project, trace, and name for span-level analysis
-- Useful for getting all scores associated with a trace quickly

CREATE MATERIALIZED VIEW IF NOT EXISTS scores_by_trace
ENGINE = AggregatingMergeTree()
PARTITION BY project_id
ORDER BY (project_id, trace_id, name)
SETTINGS index_granularity = 8192
POPULATE
AS SELECT
    project_id,
    trace_id,
    name,
    countState() AS count_state,
    sumState(value) AS sum_state,
    minState(value) AS min_state,
    maxState(value) AS max_state
FROM scores
WHERE value IS NOT NULL
GROUP BY project_id, trace_id, name;

-- ============================================================================
-- Score Source Distribution View
-- ============================================================================
-- Pre-aggregates score counts by source (code, llm, human) for analytics
-- Useful for understanding evaluation coverage and source breakdown
-- Note: SummingMergeTree is correct here - only has count column (no min/max)

CREATE MATERIALIZED VIEW IF NOT EXISTS scores_source_distribution
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(day)
ORDER BY (project_id, source, day)
SETTINGS index_granularity = 8192
POPULATE
AS SELECT
    project_id,
    source,
    toDate(timestamp) AS day,
    count() AS count
FROM scores
GROUP BY project_id, source, day;
