-- Static queries for the evaluation aggregate: score_configs, datasets,
-- dataset_items, dataset_versions, dataset_item_versions, experiments,
-- experiment_configs, experiment_items, evaluators, evaluator_executions.
-- Dynamic filters (search, multi-criteria) live in the per-repo squirrel code.

-- ----- score_configs ------------------------------------------------

-- name: CreateScoreConfig :exec
INSERT INTO score_configs (
    id, project_id, name, description, type,
    min_value, max_value, categories, metadata,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    NOW(), NOW()
);

-- name: GetScoreConfigByID :one
SELECT * FROM score_configs
WHERE id = $1 AND project_id = $2
LIMIT 1;

-- name: GetScoreConfigByName :one
SELECT * FROM score_configs
WHERE project_id = $1 AND name = $2
LIMIT 1;

-- name: ListScoreConfigsByProject :many
SELECT * FROM score_configs
WHERE project_id = $1
ORDER BY created_at DESC
OFFSET $2 LIMIT $3;

-- name: CountScoreConfigsByProject :one
SELECT COUNT(*)::bigint FROM score_configs WHERE project_id = $1;

-- name: UpdateScoreConfig :execrows
UPDATE score_configs
SET name        = $3,
    description = $4,
    type        = $5,
    min_value   = $6,
    max_value   = $7,
    categories  = $8,
    metadata    = $9,
    updated_at  = NOW()
WHERE id = $1 AND project_id = $2;

-- name: DeleteScoreConfig :execrows
DELETE FROM score_configs
WHERE id = $1 AND project_id = $2;

-- name: ScoreConfigExistsByName :one
SELECT EXISTS (
    SELECT 1 FROM score_configs
    WHERE project_id = $1 AND name = $2
);

-- ----- datasets -----------------------------------------------------

-- name: CreateDataset :exec
INSERT INTO datasets (
    id, project_id, name, description, metadata,
    created_at, updated_at, current_version_id
) VALUES (
    $1, $2, $3, $4, $5,
    NOW(), NOW(), $6
);

-- name: GetDatasetByID :one
SELECT * FROM datasets
WHERE id = $1 AND project_id = $2
LIMIT 1;

-- name: GetDatasetByName :one
SELECT * FROM datasets
WHERE project_id = $1 AND name = $2
LIMIT 1;

-- name: UpdateDataset :execrows
UPDATE datasets
SET name               = $3,
    description        = $4,
    metadata           = $5,
    current_version_id = $6,
    updated_at         = NOW()
WHERE id = $1 AND project_id = $2;

-- name: DeleteDataset :execrows
DELETE FROM datasets
WHERE id = $1 AND project_id = $2;

-- name: DatasetExistsByName :one
SELECT EXISTS (
    SELECT 1 FROM datasets
    WHERE project_id = $1 AND name = $2
);

-- ----- dataset_items ------------------------------------------------

-- name: CreateDatasetItem :exec
INSERT INTO dataset_items (
    id, dataset_id, input, expected, metadata,
    source, source_trace_id, source_span_id, content_hash,
    created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    NOW()
);

-- name: GetDatasetItemByID :one
SELECT * FROM dataset_items
WHERE id = $1 AND dataset_id = $2
LIMIT 1;

-- name: GetDatasetItemByIDForProject :one
SELECT di.* FROM dataset_items di
JOIN datasets d ON d.id = di.dataset_id
WHERE di.id = $1 AND d.project_id = $2
LIMIT 1;

-- name: ListDatasetItems :many
SELECT * FROM dataset_items
WHERE dataset_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDatasetItems :one
SELECT COUNT(*)::bigint FROM dataset_items WHERE dataset_id = $1;

-- name: ListAllDatasetItems :many
SELECT * FROM dataset_items
WHERE dataset_id = $1
ORDER BY created_at ASC;

-- name: DeleteDatasetItem :execrows
DELETE FROM dataset_items
WHERE id = $1 AND dataset_id = $2;

-- name: FindDatasetItemByContentHash :one
SELECT * FROM dataset_items
WHERE dataset_id = $1 AND content_hash = $2
LIMIT 1;

-- name: ListDatasetItemContentHashes :many
SELECT content_hash FROM dataset_items
WHERE dataset_id = $1 AND content_hash = ANY($2::text[]);

-- ----- dataset_versions ---------------------------------------------

-- name: CreateDatasetVersion :exec
INSERT INTO dataset_versions (
    id, dataset_id, version, item_count, description,
    metadata, created_by, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, NOW()
);

-- name: GetDatasetVersionByID :one
SELECT * FROM dataset_versions
WHERE id = $1 AND dataset_id = $2
LIMIT 1;

-- name: GetDatasetVersionByNumber :one
SELECT * FROM dataset_versions
WHERE dataset_id = $1 AND version = $2
LIMIT 1;

-- name: GetLatestDatasetVersion :one
SELECT * FROM dataset_versions
WHERE dataset_id = $1
ORDER BY version DESC
LIMIT 1;

-- name: ListDatasetVersions :many
SELECT * FROM dataset_versions
WHERE dataset_id = $1
ORDER BY version DESC;

-- name: GetNextDatasetVersionNumber :one
SELECT COALESCE(MAX(version), 0)::int + 1 FROM dataset_versions
WHERE dataset_id = $1;

-- name: InsertDatasetItemVersions :exec
-- Bulk insert of (version, item) pairs. UNNEST parallel arrays.
INSERT INTO dataset_item_versions (dataset_version_id, dataset_item_id)
SELECT UNNEST($1::uuid[]), UNNEST($2::uuid[]);

-- name: ListDatasetItemIDsForVersion :many
SELECT dataset_item_id FROM dataset_item_versions
WHERE dataset_version_id = $1;

-- name: ListDatasetItemsForVersion :many
SELECT di.* FROM dataset_items di
JOIN dataset_item_versions div ON div.dataset_item_id = di.id
WHERE div.dataset_version_id = $1
ORDER BY di.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountDatasetItemsForVersion :one
SELECT COUNT(*)::bigint FROM dataset_item_versions
WHERE dataset_version_id = $1;

-- ----- experiments --------------------------------------------------

-- name: CreateExperiment :exec
INSERT INTO experiments (
    id, project_id, dataset_id, name, description,
    status, metadata, started_at, completed_at,
    config_id, source, total_items, completed_items, failed_items,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9,
    $10, $11, $12, $13, $14,
    NOW(), NOW()
);

-- name: GetExperimentByID :one
SELECT * FROM experiments
WHERE id = $1 AND project_id = $2
LIMIT 1;

-- name: UpdateExperiment :execrows
UPDATE experiments
SET dataset_id      = $3,
    name            = $4,
    description     = $5,
    status          = $6,
    metadata        = $7,
    started_at      = $8,
    completed_at    = $9,
    config_id       = $10,
    source          = $11,
    total_items     = $12,
    completed_items = $13,
    failed_items    = $14,
    updated_at      = NOW()
WHERE id = $1 AND project_id = $2;

-- name: DeleteExperiment :execrows
DELETE FROM experiments
WHERE id = $1 AND project_id = $2;

-- name: SetExperimentTotalItems :execrows
UPDATE experiments
SET total_items = $3,
    updated_at  = NOW()
WHERE id = $1 AND project_id = $2;

-- name: IncrementExperimentCounters :execrows
UPDATE experiments
SET completed_items = completed_items + $3,
    failed_items    = failed_items + $4,
    updated_at      = NOW()
WHERE id = $1 AND project_id = $2;

-- name: LockExperimentForUpdate :one
SELECT * FROM experiments
WHERE id = $1 AND project_id = $2
FOR UPDATE;

-- name: GetExperimentProgress :one
SELECT id, status, total_items, completed_items, failed_items,
       started_at, completed_at
FROM experiments
WHERE id = $1 AND project_id = $2
LIMIT 1;

-- ----- experiment_configs -------------------------------------------

-- name: CreateExperimentConfig :exec
INSERT INTO experiment_configs (
    id, experiment_id, prompt_id, prompt_version_id,
    model_config, dataset_id, dataset_version_id,
    variable_mapping, evaluators,
    created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7,
    $8, $9,
    NOW(), NOW()
);

-- name: GetExperimentConfigByID :one
SELECT * FROM experiment_configs WHERE id = $1 LIMIT 1;

-- name: GetExperimentConfigByExperimentID :one
SELECT * FROM experiment_configs WHERE experiment_id = $1 LIMIT 1;

-- name: UpdateExperimentConfig :execrows
UPDATE experiment_configs
SET prompt_id          = $2,
    prompt_version_id  = $3,
    model_config       = $4,
    dataset_id         = $5,
    dataset_version_id = $6,
    variable_mapping   = $7,
    evaluators         = $8,
    updated_at         = NOW()
WHERE id = $1;

-- name: DeleteExperimentConfig :execrows
DELETE FROM experiment_configs WHERE id = $1;

-- ----- experiment_items ---------------------------------------------

-- name: CreateExperimentItem :exec
INSERT INTO experiment_items (
    id, experiment_id, dataset_item_id, trace_id,
    input, output, expected, trial_number,
    metadata, error, created_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10, NOW()
);

-- name: ListExperimentItems :many
SELECT * FROM experiment_items
WHERE experiment_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountExperimentItems :one
SELECT COUNT(*)::bigint FROM experiment_items WHERE experiment_id = $1;

-- ----- evaluators ---------------------------------------------------

-- name: CreateEvaluator :exec
INSERT INTO evaluators (
    id, project_id, name, description,
    status, trigger_type, target_scope, filter,
    span_names, sampling_rate,
    scorer_type, scorer_config, variable_mapping,
    created_by, created_at, updated_at
) VALUES (
    $1, $2, $3, $4,
    $5, $6, $7, $8,
    $9, $10,
    $11, $12, $13,
    $14, NOW(), NOW()
);

-- name: GetEvaluatorByID :one
SELECT * FROM evaluators
WHERE id = $1 AND project_id = $2
LIMIT 1;

-- name: ListActiveEvaluatorsByProject :many
SELECT * FROM evaluators
WHERE project_id = $1 AND status = 'active'
ORDER BY created_at DESC;

-- name: UpdateEvaluator :execrows
UPDATE evaluators
SET name             = $3,
    description      = $4,
    status           = $5,
    trigger_type     = $6,
    target_scope     = $7,
    filter           = $8,
    span_names       = $9,
    sampling_rate    = $10,
    scorer_type      = $11,
    scorer_config    = $12,
    variable_mapping = $13,
    updated_at       = NOW()
WHERE id = $1 AND project_id = $2;

-- name: DeleteEvaluator :execrows
DELETE FROM evaluators
WHERE id = $1 AND project_id = $2;

-- name: EvaluatorExistsByName :one
SELECT EXISTS (
    SELECT 1 FROM evaluators
    WHERE project_id = $1 AND name = $2
);

-- ----- evaluator_executions ------------------------------------------

-- name: CreateEvaluatorExecution :exec
INSERT INTO evaluator_executions (
    id, evaluator_id, project_id, status, trigger_type,
    spans_matched, spans_scored, errors_count,
    error_message, started_at, completed_at, duration_ms,
    metadata, created_at
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12,
    $13, NOW()
);

-- name: UpdateEvaluatorExecution :execrows
UPDATE evaluator_executions
SET status        = $3,
    spans_matched = $4,
    spans_scored  = $5,
    errors_count  = $6,
    error_message = $7,
    started_at    = $8,
    completed_at  = $9,
    duration_ms   = $10,
    metadata      = $11
WHERE id = $1 AND project_id = $2;

-- name: GetEvaluatorExecutionByID :one
SELECT * FROM evaluator_executions
WHERE id = $1 AND project_id = $2
LIMIT 1;

-- name: GetLatestEvaluatorExecution :one
SELECT * FROM evaluator_executions
WHERE evaluator_id = $1 AND project_id = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: IncrementEvaluatorExecutionCounters :execrows
UPDATE evaluator_executions
SET spans_scored = spans_scored + $3,
    errors_count = errors_count + $4
WHERE id = $1 AND project_id = $2;

-- name: UpdateEvaluatorExecutionSpansMatched :execrows
UPDATE evaluator_executions
SET spans_matched = $3
WHERE id = $1 AND project_id = $2;

-- name: LockEvaluatorExecutionForUpdate :one
SELECT * FROM evaluator_executions
WHERE id = $1 AND project_id = $2
FOR UPDATE;

-- name: ApplyEvaluatorExecutionCompletion :execrows
UPDATE evaluator_executions
SET spans_scored = $3,
    errors_count = $4,
    status       = $5,
    completed_at = $6,
    duration_ms  = $7
WHERE id = $1 AND project_id = $2;
