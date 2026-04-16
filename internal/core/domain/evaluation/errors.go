package evaluation

import "errors"

var (
	ErrScoreConfigNotFound   = errors.New("score config not found")
	ErrScoreConfigExists     = errors.New("score config with this name already exists")
	ErrInvalidScoreConfigID  = errors.New("invalid score config ID")
	ErrScoreConfigValidation = errors.New("score config validation failed")

	ErrScoreValueOutOfRange = errors.New("score value out of configured range")
	ErrInvalidScoreCategory = errors.New("invalid category for categorical score")
	ErrScoreTypeMismatch    = errors.New("score type does not match config")

	ErrDatasetNotFound = errors.New("dataset not found")
	ErrDatasetExists   = errors.New("dataset with this name already exists")

	ErrDatasetVersionNotFound = errors.New("dataset version not found")
	ErrDatasetVersionExists   = errors.New("dataset version already exists")

	ErrDatasetItemNotFound = errors.New("dataset item not found")

	ErrExperimentNotFound       = errors.New("experiment not found")
	ErrExperimentItemNotFound   = errors.New("experiment item not found")
	ErrExperimentConfigNotFound = errors.New("experiment config not found")

	ErrEvaluatorNotFound   = errors.New("evaluator not found")
	ErrEvaluatorExists     = errors.New("evaluator with this name already exists")
	ErrInvalidEvaluatorID  = errors.New("invalid evaluator ID")
	ErrEvaluatorValidation = errors.New("evaluator validation failed")
	ErrInvalidScorerConfig = errors.New("invalid scorer configuration")

	ErrExecutionNotFound = errors.New("evaluator execution not found")
	ErrExecutionTerminal = errors.New("execution is already in a terminal state")
)
