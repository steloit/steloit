package website

import (
	"context"

	"gorm.io/gorm"

	"brokle/internal/core/domain/website"
	"brokle/internal/infrastructure/shared"
)

type contactSubmissionRepository struct {
	db *gorm.DB
}

func NewContactSubmissionRepository(db *gorm.DB) website.ContactSubmissionRepository {
	return &contactSubmissionRepository{db: db}
}

func (r *contactSubmissionRepository) getDB(ctx context.Context) *gorm.DB {
	return shared.GetDB(ctx, r.db)
}

func (r *contactSubmissionRepository) Create(ctx context.Context, submission *website.ContactSubmission) error {
	return r.getDB(ctx).WithContext(ctx).Create(submission).Error
}
