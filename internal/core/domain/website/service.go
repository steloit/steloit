package website

import "context"

// WebsiteService defines business logic for website form submissions.
type WebsiteService interface {
	SubmitContactForm(ctx context.Context, req *CreateContactSubmissionRequest, ipAddress, userAgent string) error
}
