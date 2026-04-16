-- Migration rollback: Restore onboarding tables and slugs

-- Recreate onboarding tables (reverse order)
CREATE TABLE IF NOT EXISTS onboarding_questions (
    id UUID PRIMARY KEY,
    question_text TEXT NOT NULL,
    question_type VARCHAR(50) NOT NULL,
    options JSONB,
    is_required BOOLEAN DEFAULT true,
    display_order INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS onboarding_responses (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question_id UUID NOT NULL REFERENCES onboarding_questions(id) ON DELETE CASCADE,
    response_value TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id, question_id)
);

-- Re-add slug columns (will be NULL for data created without slugs)
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS slug VARCHAR(255);
ALTER TABLE projects ADD COLUMN IF NOT EXISTS slug VARCHAR(255);

-- Make role nullable again
ALTER TABLE users ALTER COLUMN role DROP NOT NULL;

-- Drop new columns
ALTER TABLE users DROP COLUMN IF EXISTS referral_source;
ALTER TABLE users DROP COLUMN IF EXISTS role;
