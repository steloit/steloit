-- Comment reactions table for emoji reactions on comments
-- Users can toggle reactions (add/remove), with max 6 different emojis per comment

CREATE TABLE comment_reactions (
    id UUID PRIMARY KEY,
    comment_id UUID NOT NULL,
    user_id UUID NOT NULL,
    emoji VARCHAR(8) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    -- Foreign keys with cascade delete
    CONSTRAINT fk_comment_reactions_comment FOREIGN KEY (comment_id)
        REFERENCES trace_comments(id) ON DELETE CASCADE,
    CONSTRAINT fk_comment_reactions_user FOREIGN KEY (user_id)
        REFERENCES users(id) ON DELETE CASCADE,

    -- Ensure one reaction per user per emoji per comment
    CONSTRAINT uq_comment_reactions_user_emoji UNIQUE (comment_id, user_id, emoji)
);

-- Index for efficient lookups by comment
CREATE INDEX idx_comment_reactions_comment ON comment_reactions(comment_id);

-- Index for finding user's reactions across comments
CREATE INDEX idx_comment_reactions_user ON comment_reactions(user_id, comment_id);
