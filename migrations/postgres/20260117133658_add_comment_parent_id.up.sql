-- Migration: add_comment_parent_id
-- Created: 2026-01-17T13:36:58+05:30

-- Add parent_id column to support one-level-deep reply threading
-- Only top-level comments (parent_id IS NULL) can have replies

ALTER TABLE trace_comments
ADD COLUMN parent_id UUID;

-- Foreign key constraint (replies reference parent comment)
ALTER TABLE trace_comments
ADD CONSTRAINT fk_trace_comments_parent FOREIGN KEY (parent_id)
    REFERENCES trace_comments(id) ON DELETE CASCADE;

-- Partial index for efficient fetching of replies (only non-deleted comments)
CREATE INDEX idx_trace_comments_parent ON trace_comments(parent_id)
    WHERE deleted_at IS NULL;
