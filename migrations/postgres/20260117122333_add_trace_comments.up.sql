-- Entity type enum for future extensibility (span comments, etc.)
CREATE TYPE comment_entity_type AS ENUM ('trace', 'span');

CREATE TABLE trace_comments (
    id UUID PRIMARY KEY,
    entity_type comment_entity_type NOT NULL DEFAULT 'trace',
    entity_id VARCHAR(64) NOT NULL,  -- trace_id or span_id
    project_id UUID NOT NULL,
    content TEXT NOT NULL,

    -- Audit trail (following Opik/Langfuse patterns)
    created_by UUID,
    updated_by UUID,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE,

    CONSTRAINT fk_trace_comments_project FOREIGN KEY (project_id)
        REFERENCES projects(id) ON DELETE CASCADE,
    CONSTRAINT fk_trace_comments_created_by FOREIGN KEY (created_by)
        REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_trace_comments_updated_by FOREIGN KEY (updated_by)
        REFERENCES users(id) ON DELETE SET NULL
);

-- Optimized for common queries: list comments by entity
CREATE INDEX idx_trace_comments_entity ON trace_comments(entity_type, entity_id, project_id)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_trace_comments_project ON trace_comments(project_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_trace_comments_user ON trace_comments(created_by)
    WHERE deleted_at IS NULL;
