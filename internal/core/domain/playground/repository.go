package playground

import (
	"context"

	"github.com/google/uuid"
)

// SessionRepository defines the repository interface for playground sessions.
type SessionRepository interface {
	// Create creates a new playground session.
	Create(ctx context.Context, session *Session) error

	// GetByID retrieves a session by its ID.
	// Returns ErrSessionNotFound if not found.
	GetByID(ctx context.Context, id uuid.UUID) (*Session, error)

	// List retrieves sessions for a project (for sidebar).
	// Ordered by last_used_at DESC.
	List(ctx context.Context, projectID uuid.UUID, limit int) ([]*Session, error)

	// ListByTags retrieves sessions filtered by tags.
	// Returns sessions where tags contains any of the provided tags.
	ListByTags(ctx context.Context, projectID uuid.UUID, tags []string, limit int) ([]*Session, error)

	// Update updates an existing session.
	// Updates updated_at automatically.
	Update(ctx context.Context, session *Session) error

	// UpdateLastRun updates only the last_run and last_used_at fields.
	UpdateLastRun(ctx context.Context, id uuid.UUID, lastRun JSON) error

	// UpdateWindows updates only the windows JSONB field.
	UpdateWindows(ctx context.Context, id uuid.UUID, windows JSON) error

	// Delete removes a session by ID.
	Delete(ctx context.Context, id uuid.UUID) error

	// Exists checks if a session exists.
	Exists(ctx context.Context, id uuid.UUID) (bool, error)

	// ExistsByProjectID checks if a session exists for a specific project.
	ExistsByProjectID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (bool, error)
}
