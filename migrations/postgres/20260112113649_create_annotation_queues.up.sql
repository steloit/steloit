-- PostgreSQL Migration: create_annotation_queues
-- Created: 2026-01-12
--
-- Annotation Queues for Human-in-the-Loop (HITL) evaluation workflows
-- Design informed by competitive analysis:
-- - Langfuse: 5-minute lock lease, dual-user tracking (locker vs completer)
-- - Opik: instructions field for annotation guidelines

-- ============================================================================
-- Annotation Queues Table
-- ============================================================================
-- Queue configuration for organizing annotation tasks

CREATE TABLE annotation_queues (
    id                  UUID PRIMARY KEY,           -- ULID
    project_id          UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    description         TEXT,
    instructions        TEXT,                           -- Annotation guidelines (from Opik pattern)
    score_config_ids    JSONB DEFAULT '[]'::jsonb,      -- Array of score config IDs to collect
    status              VARCHAR(20) NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'paused', 'archived')),
    settings            JSONB DEFAULT '{"lock_timeout_seconds": 300, "auto_assignment": false}'::jsonb,
    created_by          UUID REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, name)
);

CREATE INDEX idx_annotation_queues_project_status ON annotation_queues(project_id, status);
CREATE INDEX idx_annotation_queues_project_created ON annotation_queues(project_id, created_at DESC);

-- ============================================================================
-- Annotation Queue Items Table
-- ============================================================================
-- Items pending human review with locking mechanism
-- Design: Dual-user tracking from Langfuse (locked_by vs annotator_user_id)

CREATE TABLE annotation_queue_items (
    id                  UUID PRIMARY KEY,           -- ULID
    queue_id            UUID NOT NULL REFERENCES annotation_queues(id) ON DELETE CASCADE,
    object_id           VARCHAR(32) NOT NULL,           -- trace_id or span_id
    object_type         VARCHAR(20) NOT NULL CHECK (object_type IN ('trace', 'span')),
    status              VARCHAR(20) NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'completed', 'skipped')),
    priority            INTEGER NOT NULL DEFAULT 0,     -- Higher = more urgent
    -- Locking (who claimed it)
    locked_at           TIMESTAMPTZ,                    -- When item was locked for review
    locked_by_user_id   UUID REFERENCES users(id),  -- Who currently holds the lock
    -- Completion (who finished it) - Langfuse pattern: separate from locker
    annotator_user_id   UUID REFERENCES users(id),  -- Who completed the annotation
    completed_at        TIMESTAMPTZ,
    metadata            JSONB DEFAULT '{}'::jsonb,      -- Source info, sampling reason, etc.
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(queue_id, object_id, object_type)            -- No duplicate items
);

-- Index for listing items by queue and status
CREATE INDEX idx_annotation_queue_items_queue_status ON annotation_queue_items(queue_id, status);

-- Index for finding locked items (for lock expiry worker)
CREATE INDEX idx_annotation_queue_items_locked ON annotation_queue_items(queue_id, locked_by_user_id)
    WHERE locked_by_user_id IS NOT NULL;

-- Index for claiming next item (priority DESC, FIFO within same priority)
CREATE INDEX idx_annotation_queue_items_priority ON annotation_queue_items(queue_id, priority DESC, created_at ASC)
    WHERE status = 'pending';

-- Index for finding items by object (trace/span)
CREATE INDEX idx_annotation_queue_items_object ON annotation_queue_items(object_id, object_type);

-- ============================================================================
-- Annotation Queue Assignments Table
-- ============================================================================
-- User assignments to queues (who can annotate what)

CREATE TABLE annotation_queue_assignments (
    id              UUID PRIMARY KEY,           -- ULID
    queue_id        UUID NOT NULL REFERENCES annotation_queues(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            VARCHAR(20) NOT NULL DEFAULT 'annotator'
                    CHECK (role IN ('annotator', 'reviewer', 'admin')),
    assigned_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    assigned_by     UUID REFERENCES users(id),
    UNIQUE(queue_id, user_id)
);

-- Index for finding queues assigned to a user
CREATE INDEX idx_annotation_queue_assignments_user ON annotation_queue_assignments(user_id);

-- Index for listing assignments by queue
CREATE INDEX idx_annotation_queue_assignments_queue ON annotation_queue_assignments(queue_id);
