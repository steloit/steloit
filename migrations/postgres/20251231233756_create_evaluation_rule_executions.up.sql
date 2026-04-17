-- Evaluation Rule Executions: Track execution history for evaluation rules
-- Design patterns from Langfuse (JobExecution) and Opik (AutomationRuleEvaluatorLogs)

CREATE TABLE evaluation_rule_executions (
    id              UUID PRIMARY KEY,
    rule_id         UUID NOT NULL REFERENCES evaluation_rules(id) ON DELETE CASCADE,
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status          VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    trigger_type    VARCHAR(20) NOT NULL DEFAULT 'automatic' CHECK (trigger_type IN ('automatic', 'manual')),
    spans_matched   INTEGER NOT NULL DEFAULT 0,
    spans_scored    INTEGER NOT NULL DEFAULT 0,
    errors_count    INTEGER NOT NULL DEFAULT 0,
    error_message   TEXT,
    started_at      TIMESTAMP WITH TIME ZONE,
    completed_at    TIMESTAMP WITH TIME ZONE,
    duration_ms     INTEGER,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Index for listing executions by rule (primary use case)
CREATE INDEX idx_rule_executions_rule ON evaluation_rule_executions(rule_id);

-- Index for project-level queries
CREATE INDEX idx_rule_executions_project ON evaluation_rule_executions(project_id);

-- Index for filtering by status (e.g., find running executions)
CREATE INDEX idx_rule_executions_status ON evaluation_rule_executions(status);

-- Index for chronological listing (most recent first)
CREATE INDEX idx_rule_executions_created ON evaluation_rule_executions(created_at DESC);

-- Composite index for common query pattern: list executions for a rule, ordered by time
CREATE INDEX idx_rule_executions_rule_created ON evaluation_rule_executions(rule_id, created_at DESC);

COMMENT ON TABLE evaluation_rule_executions IS 'Tracks execution history for evaluation rules including status, counts, and timing';
COMMENT ON COLUMN evaluation_rule_executions.status IS 'pending=queued, running=in progress, completed=success, failed=error, cancelled=aborted';
COMMENT ON COLUMN evaluation_rule_executions.trigger_type IS 'automatic=triggered by span matching, manual=triggered via API/UI';
COMMENT ON COLUMN evaluation_rule_executions.metadata IS 'Additional context: trace_ids, span_ids processed, etc.';
