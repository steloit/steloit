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
		Company:     emptyToNilString(s.Company),
		Subject:     s.Subject,
		Message:     s.Message,
		InquiryType: emptyToNilString(s.InquiryType),
		IpAddress:   emptyToNilString(s.IPAddress),
		UserAgent:   emptyToNilString(s.UserAgent),
		CreatedAt:   s.CreatedAt,
	}); err != nil {
		return fmt.Errorf("create contact submission: %w", err)
	}
	return nil
}

func emptyToNilString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
