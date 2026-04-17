package website

import (
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"
)

// ContactSubmission represents a contact form submission from the marketing website.
type ContactSubmission struct {
	ID          uuid.UUID
	Name        string   
	Email       string   
	Company     string   
	Subject     string   
	Message     string   
	InquiryType string   
	IPAddress   string   
	UserAgent   string   
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

// NewContactSubmission creates a new ContactSubmission from the request.
func NewContactSubmission(req *CreateContactSubmissionRequest, ipAddress, userAgent string) *ContactSubmission {
	return &ContactSubmission{
		ID:          uid.New(),
		Name:        req.Name,
		Email:       req.Email,
		Company:     req.Company,
		Subject:     req.Subject,
		Message:     req.Message,
		InquiryType: req.InquiryType,
		IPAddress:   ipAddress,
		UserAgent:   userAgent,
		CreatedAt:   time.Now(),
	}
}
