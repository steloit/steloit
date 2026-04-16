CREATE TABLE contact_submissions (
    id           UUID PRIMARY KEY,
    name         VARCHAR(255) NOT NULL,
    email        VARCHAR(255) NOT NULL,
    company      VARCHAR(255),
    subject      VARCHAR(255) NOT NULL,
    message      TEXT NOT NULL,
    inquiry_type VARCHAR(50),
    ip_address   VARCHAR(45),
    user_agent   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_contact_submissions_email ON contact_submissions(email);
