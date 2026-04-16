-- Create onboarding questions table
CREATE TABLE onboarding_questions (
    id UUID PRIMARY KEY,
    step INTEGER NOT NULL,
    question_type VARCHAR(50) NOT NULL CHECK (question_type IN ('single_choice', 'multiple_choice', 'text', 'skip_optional')),
    title TEXT NOT NULL,
    description TEXT,
    is_required BOOLEAN DEFAULT true,
    options JSONB, -- For choice-based questions: ["Option 1", "Option 2", ...]
    display_order INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Create user onboarding responses table
CREATE TABLE user_onboarding_responses (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    question_id UUID NOT NULL REFERENCES onboarding_questions(id) ON DELETE CASCADE,
    response_value JSONB NOT NULL, -- Flexible answer storage: "string", ["array"], {"object": "value"}
    skipped BOOLEAN DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id, question_id)
);

-- Create indexes for better performance
CREATE INDEX idx_onboarding_questions_step_active ON onboarding_questions(step, is_active);
CREATE INDEX idx_onboarding_questions_display_order ON onboarding_questions(display_order);
CREATE INDEX idx_user_onboarding_responses_user_id ON user_onboarding_responses(user_id);
CREATE INDEX idx_user_onboarding_responses_question_id ON user_onboarding_responses(question_id);

-- Create updated_at trigger for onboarding_questions
CREATE TRIGGER update_onboarding_questions_updated_at 
    BEFORE UPDATE ON onboarding_questions 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();