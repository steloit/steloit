-- name: CreateContactSubmission :exec
INSERT INTO contact_submissions (
    id, name, email, company, subject, message,
    inquiry_type, ip_address, user_agent, created_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10
);
