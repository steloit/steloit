package annotation

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	annotationDomain "brokle/internal/core/domain/annotation"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
	appErrors "brokle/pkg/errors"
)

type AssignmentRepository struct {
	tm *db.TxManager
}

func NewAssignmentRepository(tm *db.TxManager) *AssignmentRepository {
	return &AssignmentRepository{tm: tm}
}

func (r *AssignmentRepository) Create(ctx context.Context, a *annotationDomain.QueueAssignment) error {
	if err := r.tm.Queries(ctx).CreateAnnotationQueueAssignment(ctx, gen.CreateAnnotationQueueAssignmentParams{
		ID:         a.ID,
		QueueID:    a.QueueID,
		UserID:     a.UserID,
		Role:       string(a.Role),
		AssignedBy: a.AssignedBy,
	}); err != nil {
		if appErrors.IsUniqueViolation(err) {
			return annotationDomain.ErrAssignmentExists
		}
		return fmt.Errorf("create assignment: %w", err)
	}
	return nil
}

func (r *AssignmentRepository) Delete(ctx context.Context, queueID, userID uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeleteAnnotationQueueAssignment(ctx, gen.DeleteAnnotationQueueAssignmentParams{
		QueueID: queueID,
		UserID:  userID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return annotationDomain.ErrAssignmentNotFound
	}
	return nil
}

func (r *AssignmentRepository) GetByQueueAndUser(ctx context.Context, queueID, userID uuid.UUID) (*annotationDomain.QueueAssignment, error) {
	row, err := r.tm.Queries(ctx).GetAnnotationQueueAssignmentByQueueAndUser(ctx, gen.GetAnnotationQueueAssignmentByQueueAndUserParams{
		QueueID: queueID,
		UserID:  userID,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, annotationDomain.ErrAssignmentNotFound
		}
		return nil, err
	}
	return assignmentFromRow(&row), nil
}

func (r *AssignmentRepository) List(ctx context.Context, queueID uuid.UUID) ([]*annotationDomain.QueueAssignment, error) {
	rows, err := r.tm.Queries(ctx).ListAnnotationQueueAssignmentsByQueue(ctx, queueID)
	if err != nil {
		return nil, err
	}
	out := make([]*annotationDomain.QueueAssignment, 0, len(rows))
	for i := range rows {
		out = append(out, assignmentFromRow(&rows[i]))
	}
	return out, nil
}

func (r *AssignmentRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]*annotationDomain.QueueAssignment, error) {
	rows, err := r.tm.Queries(ctx).ListAnnotationQueueAssignmentsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*annotationDomain.QueueAssignment, 0, len(rows))
	for i := range rows {
		out = append(out, assignmentFromRow(&rows[i]))
	}
	return out, nil
}

func (r *AssignmentRepository) IsAssigned(ctx context.Context, queueID, userID uuid.UUID) (bool, error) {
	return r.tm.Queries(ctx).AnnotationQueueAssignmentExists(ctx, gen.AnnotationQueueAssignmentExistsParams{
		QueueID: queueID,
		UserID:  userID,
	})
}

// HasRole checks whether the user's assigned role meets or exceeds the
// minimum. Role hierarchy: admin > reviewer > annotator.
func (r *AssignmentRepository) HasRole(ctx context.Context, queueID, userID uuid.UUID, minRole annotationDomain.AssignmentRole) (bool, error) {
	a, err := r.GetByQueueAndUser(ctx, queueID, userID)
	if err != nil {
		if err == annotationDomain.ErrAssignmentNotFound {
			return false, nil
		}
		return false, err
	}
	return roleAtLeast(a.Role, minRole), nil
}

func roleAtLeast(actual, minimum annotationDomain.AssignmentRole) bool {
	level := map[annotationDomain.AssignmentRole]int{
		annotationDomain.RoleAnnotator: 1,
		annotationDomain.RoleReviewer:  2,
		annotationDomain.RoleAdmin:     3,
	}
	a, ok := level[actual]
	if !ok {
		return false
	}
	m, ok := level[minimum]
	if !ok {
		return false
	}
	return a >= m
}

func assignmentFromRow(row *gen.AnnotationQueueAssignment) *annotationDomain.QueueAssignment {
	return &annotationDomain.QueueAssignment{
		ID:         row.ID,
		QueueID:    row.QueueID,
		UserID:     row.UserID,
		Role:       annotationDomain.AssignmentRole(row.Role),
		AssignedAt: row.AssignedAt,
		AssignedBy: row.AssignedBy,
	}
}
