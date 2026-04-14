package website

import "context"

// ContactSubmissionRepository defines data access for contact form submissions.
type ContactSubmissionRepository interface {
	Create(ctx context.Context, submission *ContactSubmission) error
}
