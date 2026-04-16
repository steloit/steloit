-- Migration: create_datasets_experiments
-- Created: 2025-12-20T23:18:17+05:30

-- Datasets: collections of test cases for evaluation
CREATE TABLE datasets (
    id              UUID PRIMARY KEY,
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT datasets_project_name_unique UNIQUE(project_id, name)
);
CREATE INDEX idx_datasets_project ON datasets(project_id);

-- Dataset Items: individual test cases within a dataset
CREATE TABLE dataset_items (
    id              UUID PRIMARY KEY,
    dataset_id      UUID NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
    input           JSONB NOT NULL,
    expected        JSONB,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_dataset_items_dataset ON dataset_items(dataset_id);

-- Experiments: batch evaluation runs against datasets
CREATE TABLE experiments (
    id              UUID PRIMARY KEY,
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    dataset_id      UUID REFERENCES datasets(id) ON DELETE SET NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    metadata        JSONB DEFAULT '{}',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_experiments_project ON experiments(project_id);
CREATE INDEX idx_experiments_dataset ON experiments(dataset_id);
CREATE INDEX idx_experiments_status ON experiments(status);

-- Experiment Items: individual results from experiment runs
CREATE TABLE experiment_items (
    id              UUID PRIMARY KEY,
    experiment_id   UUID NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    dataset_item_id UUID REFERENCES dataset_items(id) ON DELETE SET NULL,
    trace_id        VARCHAR(32),
    input           JSONB NOT NULL,
    output          JSONB,
    expected        JSONB,
    trial_number    INTEGER NOT NULL DEFAULT 1,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_experiment_items_experiment ON experiment_items(experiment_id);
CREATE INDEX idx_experiment_items_trace ON experiment_items(trace_id);
