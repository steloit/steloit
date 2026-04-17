package evaluation

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	sq "github.com/Masterminds/squirrel"

	evalDomain "brokle/internal/core/domain/evaluation"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/pagination"
)

// parseCreatedByUUID converts the domain's legacy *string created_by
// (UUID-string) to the gen layer's *uuid.UUID. Returns nil on empty or
// malformed input rather than surfacing an error — CreatedBy is audit
// metadata that shouldn't fail writes.
func parseCreatedByUUID(s *string) *uuid.UUID {
	if s == nil || *s == "" {
		return nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil
	}
	return &id
}

func formatCreatedByString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

type EvaluatorRepository struct {
	tm *db.TxManager
}

func NewEvaluatorRepository(tm *db.TxManager) *EvaluatorRepository {
	return &EvaluatorRepository{tm: tm}
}

func (r *EvaluatorRepository) Create(ctx context.Context, e *evalDomain.Evaluator) error {
	filter, err := marshalEvalJSON(e.Filter)
	if err != nil {
		return err
	}
	scorerConfig, err := marshalEvalJSON(e.ScorerConfig)
	if err != nil {
		return err
	}
	variableMapping, err := marshalEvalJSON(e.VariableMapping)
	if err != nil {
		return err
	}
	if err := r.tm.Queries(ctx).CreateEvaluator(ctx, gen.CreateEvaluatorParams{
		ID:              e.ID,
		ProjectID:       e.ProjectID,
		Name:            e.Name,
		Description:     e.Description,
		Status:          string(e.Status),
		TriggerType:     string(e.TriggerType),
		TargetScope:     string(e.TargetScope),
		Filter:          filter,
		SpanNames:       []string(e.SpanNames),
		SamplingRate:    decimal.NewFromFloat(e.SamplingRate),
		ScorerType:      string(e.ScorerType),
		ScorerConfig:    scorerConfig,
		VariableMapping: variableMapping,
		CreatedBy:       parseCreatedByUUID(e.CreatedBy),
	}); err != nil {
		if appErrors.IsUniqueViolation(err) {
			return evalDomain.ErrEvaluatorExists
		}
		return err
	}
	return nil
}

func (r *EvaluatorRepository) GetByID(ctx context.Context, id, projectID uuid.UUID) (*evalDomain.Evaluator, error) {
	row, err := r.tm.Queries(ctx).GetEvaluatorByID(ctx, gen.GetEvaluatorByIDParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, evalDomain.ErrEvaluatorNotFound
		}
		return nil, err
	}
	return evaluatorFromRow(&row)
}

func (r *EvaluatorRepository) GetByProjectID(
	ctx context.Context,
	projectID uuid.UUID,
	filter *evalDomain.EvaluatorFilter,
	params pagination.Params,
) ([]*evalDomain.Evaluator, int64, error) {
	base := sq.Select().From("evaluators").Where(sq.Eq{"project_id": projectID})
	if filter != nil {
		if filter.Status != nil {
			base = base.Where(sq.Eq{"status": string(*filter.Status)})
		}
		if filter.ScorerType != nil {
			base = base.Where(sq.Eq{"scorer_type": string(*filter.ScorerType)})
		}
		if filter.Search != nil && *filter.Search != "" {
			base = base.Where(sq.Expr("name ILIKE ?", "%"+*filter.Search+"%"))
		}
	}

	cntSQL, cntArgs, err := base.Columns("COUNT(*)").PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := r.tm.DB(ctx).QueryRow(ctx, cntSQL, cntArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	selSQL, selArgs, err := base.Columns(evaluatorColumns...).
		OrderBy(fmt.Sprintf("%s %s", params.SortBy, params.SortDir)).
		Offset(uint64(params.GetOffset())).Limit(uint64(params.Limit)).
		PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.tm.DB(ctx).Query(ctx, selSQL, selArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]*evalDomain.Evaluator, 0)
	for rows.Next() {
		e, err := scanEvaluator(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

func (r *EvaluatorRepository) GetActiveByProjectID(ctx context.Context, projectID uuid.UUID) ([]*evalDomain.Evaluator, error) {
	rows, err := r.tm.Queries(ctx).ListActiveEvaluatorsByProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]*evalDomain.Evaluator, 0, len(rows))
	for i := range rows {
		e, err := evaluatorFromRow(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func (r *EvaluatorRepository) Update(ctx context.Context, e *evalDomain.Evaluator) error {
	filter, err := marshalEvalJSON(e.Filter)
	if err != nil {
		return err
	}
	scorerConfig, err := marshalEvalJSON(e.ScorerConfig)
	if err != nil {
		return err
	}
	variableMapping, err := marshalEvalJSON(e.VariableMapping)
	if err != nil {
		return err
	}
	n, err := r.tm.Queries(ctx).UpdateEvaluator(ctx, gen.UpdateEvaluatorParams{
		ID:              e.ID,
		ProjectID:       e.ProjectID,
		Name:            e.Name,
		Description:     e.Description,
		Status:          string(e.Status),
		TriggerType:     string(e.TriggerType),
		TargetScope:     string(e.TargetScope),
		Filter:          filter,
		SpanNames:       []string(e.SpanNames),
		SamplingRate:    decimal.NewFromFloat(e.SamplingRate),
		ScorerType:      string(e.ScorerType),
		ScorerConfig:    scorerConfig,
		VariableMapping: variableMapping,
	})
	if err != nil {
		if appErrors.IsUniqueViolation(err) {
			return evalDomain.ErrEvaluatorExists
		}
		return err
	}
	if n == 0 {
		return evalDomain.ErrEvaluatorNotFound
	}
	return nil
}

func (r *EvaluatorRepository) Delete(ctx context.Context, id, projectID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteEvaluator(ctx, gen.DeleteEvaluatorParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return evalDomain.ErrEvaluatorNotFound
	}
	return nil
}

func (r *EvaluatorRepository) ExistsByName(ctx context.Context, projectID uuid.UUID, name string) (bool, error) {
	return r.tm.Queries(ctx).EvaluatorExistsByName(ctx, gen.EvaluatorExistsByNameParams{
		ProjectID: projectID,
		Name:      name,
	})
}

var evaluatorColumns = []string{
	"id", "project_id", "name", "description",
	"status", "trigger_type", "target_scope", "filter",
	"span_names", "sampling_rate",
	"scorer_type", "scorer_config", "variable_mapping",
	"created_by", "created_at", "updated_at",
}

func scanEvaluator(row interface {
	Scan(dest ...any) error
}) (*evalDomain.Evaluator, error) {
	var r2 gen.Evaluator
	if err := row.Scan(
		&r2.ID, &r2.ProjectID, &r2.Name, &r2.Description,
		&r2.Status, &r2.TriggerType, &r2.TargetScope, &r2.Filter,
		&r2.SpanNames, &r2.SamplingRate,
		&r2.ScorerType, &r2.ScorerConfig, &r2.VariableMapping,
		&r2.CreatedBy, &r2.CreatedAt, &r2.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return evaluatorFromRow(&r2)
}

func evaluatorFromRow(row *gen.Evaluator) (*evalDomain.Evaluator, error) {
	e := &evalDomain.Evaluator{
		ID:           row.ID,
		ProjectID:    row.ProjectID,
		Name:         row.Name,
		Description:  row.Description,
		Status:       evalDomain.EvaluatorStatus(row.Status),
		TriggerType:  evalDomain.EvaluatorTrigger(row.TriggerType),
		TargetScope:  evalDomain.TargetScope(row.TargetScope),
		SpanNames:    row.SpanNames,
		SamplingRate: row.SamplingRate.InexactFloat64(),
		ScorerType:   evalDomain.ScorerType(row.ScorerType),
		CreatedBy:    formatCreatedByString(row.CreatedBy),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
	if err := unmarshalEvalJSON(row.Filter, &e.Filter); err != nil {
		return nil, err
	}
	if err := unmarshalEvalJSON(row.ScorerConfig, &e.ScorerConfig); err != nil {
		return nil, err
	}
	if err := unmarshalEvalJSON(row.VariableMapping, &e.VariableMapping); err != nil {
		return nil, err
	}
	return e, nil
}
