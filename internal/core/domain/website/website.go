package website

import (
	"time"

	"github.com/google/uuid"

	"brokle/internal/core/domain/shared"
	"brokle/pkg/uid"
)

// ContactSubmission represents a contact form submission from the
// marketing website. Optional fields are pointer-typed to mirror the
// nullable columns in Postgres — nil means "not provided".
type ContactSubmission struct {
	ID          uuid.UUID
	Name        string
	Email       string
	Subject     string
	Message     string
	Company     *string
	InquiryType *string
	IPAddress   *string
	UserAgent   *string
	CreatedAt   time.Time
}

// CreateContactSubmissionRequest is the DTO for creating a contact submission.
type CreateContactSubmissionRequest struct {
	Name        string `json:"name" binding:"required,min=2,max=255"`
	Email       string `json:"email" binding:"required,email,max=255"`
	Company     string `json:"company" binding:"max=255"`
	Subject     string `json:"subject" binding:"required,min=5,max=255"`
	Message     string `json:"message" binding:"required,min=10,max=5000"`
	InquiryType string `json:"inquiry_type" binding:"max=50"`
}

// NewContactSubmission builds a ContactSubmission from the ingress
// request. Empty optional values are normalised to nil so the domain
// struct never carries a non-nil pointer to "".
func NewContactSubmission(req *CreateContactSubmissionRequest, ipAddress, userAgent string) *ContactSubmission {
	return &ContactSubmission{
		ID:          uid.New(),
		Name:        req.Name,
		Email:       req.Email,
		Subject:     req.Subject,
		Message:     req.Message,
		Company:     shared.NilIfEmpty(req.Company),
		InquiryType: shared.NilIfEmpty(req.InquiryType),
		IPAddress:   shared.NilIfEmpty(ipAddress),
		UserAgent:   shared.NilIfEmpty(userAgent),
		CreatedAt:   time.Now(),
	}
}
