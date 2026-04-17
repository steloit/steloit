-- Playground Sessions: Hybrid "Ephemeral by Default" Model
-- Stores configuration + last run, not full history
-- Auto-created on visit, optionally saved for sidebar

CREATE TABLE playground_sessions (
    -- Primary key (shareable URL ID)
    id              UUID PRIMARY KEY,

    -- Project scope
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Configuration (what persists across refreshes)
    name            VARCHAR(200),                   -- NULL = unsaved, "My Test" = saved
    description     TEXT,
    template        JSONB NOT NULL,                 -- Chat or text template
    template_type   VARCHAR(10) NOT NULL,           -- 'chat' or 'text'
    variables       JSONB DEFAULT '{}'::jsonb,      -- Variable values
    config          JSONB,                          -- Model, temperature, top_p, etc.

    -- Multi-window comparison state (embedded, not separate table)
    windows         JSONB,                          -- Array: [{template, vars, config, last_run}]

    -- Last execution (overwrites on each execute, not appends)
    last_run        JSONB,                          -- {content, metrics, timestamp, model}

    -- Saved vs unsaved (controls UI visibility)
    is_saved        BOOLEAN DEFAULT FALSE,          -- FALSE = ephemeral UX, TRUE = shows in sidebar
    tags            TEXT[] DEFAULT '{}',            -- For saved playgrounds only

    -- Audit fields
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    last_used_at    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Auto-cleanup for unsaved sessions
    expires_at      TIMESTAMP WITH TIME ZONE,                    -- NULL if saved, NOW()+30d if unsaved

    -- Constraints
    CONSTRAINT chk_template_type CHECK (template_type IN ('chat', 'text'))
);

-- Index for listing saved sessions (sidebar query)
CREATE INDEX idx_playground_sessions_project_saved
    ON playground_sessions(project_id, is_saved, last_used_at DESC);

-- Index for cleanup of expired sessions (background job)
CREATE INDEX idx_playground_sessions_expires
    ON playground_sessions(expires_at)
    WHERE expires_at IS NOT NULL;

-- Index for tag-based search (saved sessions only)
CREATE INDEX idx_playground_sessions_tags
    ON playground_sessions USING GIN(tags)
    WHERE is_saved = TRUE;

-- Comments for documentation
COMMENT ON TABLE playground_sessions IS 'Hybrid playground storage: auto-created sessions with ephemeral UX but persistent backing. Unsaved sessions expire after 30 days of inactivity.';
COMMENT ON COLUMN playground_sessions.name IS 'NULL = unsaved (ephemeral UX), non-NULL = saved (shows in sidebar)';
COMMENT ON COLUMN playground_sessions.is_saved IS 'FALSE = auto-created, not in sidebar, auto-expires. TRUE = user saved, in sidebar, kept forever.';
COMMENT ON COLUMN playground_sessions.last_run IS 'Most recent execution result. Overwrites on each execute (not append). JSONB: {content, metrics: {tokens, cost, latency, ttft_ms}, timestamp, model}';
COMMENT ON COLUMN playground_sessions.windows IS 'Multi-window comparison state. JSONB array: [{template, variables, config, last_run}, ...]. Solves comparison restoration without separate table.';
COMMENT ON COLUMN playground_sessions.expires_at IS 'NULL if is_saved=TRUE (kept forever). NOW()+30 days if is_saved=FALSE (auto-cleanup). Updated on each use.';
