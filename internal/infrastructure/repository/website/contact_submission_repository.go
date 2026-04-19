package website

import (
	"context"
	"fmt"
	"time"

	websiteDomain "brokle/internal/core/domain/website"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

type contactSubmissionRepository struct {
	tm *db.TxManager
}

func NewContactSubmissionRepository(tm *db.TxManager) websiteDomain.ContactSubmissionRepository {
	return &contactSubmissionRepository{tm: tm}
}

func (r *contactSubmissionRepository) Create(ctx context.Context, s *websiteDomain.ContactSubmission) error {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	if err := r.tm.Queries(ctx).CreateContactSubmission(ctx, gen.CreateContactSubmissionParams{
		ID:          s.ID,
		Name:        s.Name,
		Email:       s.Email,
		Company:     s.Company,
		Subject:     s.Subject,
		Message:     s.Message,
		InquiryType: s.InquiryType,
		IpAddress:   s.IPAddress,
		UserAgent:   s.UserAgent,
		CreatedAt:   s.CreatedAt,
	}); err != nil {
		return fmt.Errorf("create contact submission: %w", err)
	}
	return nil
}
