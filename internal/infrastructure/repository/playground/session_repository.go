// Package playground provides repository implementations for playground session storage.
package playground

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/google/uuid"

	playgroundDomain "brokle/internal/core/domain/playground"
)

// sessionRepository implements playgroundDomain.SessionRepository using GORM
type sessionRepository struct {
	db *gorm.DB
}

// NewSessionRepository creates a new repository instance
func NewSessionRepository(db *gorm.DB) playgroundDomain.SessionRepository {
	return &sessionRepository{
		db: db,
	}
}

// Create creates a new playground session.
func (r *sessionRepository) Create(ctx context.Context, session *playgroundDomain.Session) error {
	now := time.Now()
	session.CreatedAt = now
	session.UpdatedAt = now
	session.LastUsedAt = now

	err := r.db.WithContext(ctx).Create(session).Error
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetByID retrieves a session by its ID.
// Returns ErrSessionNotFound if not found.
func (r *sessionRepository) GetByID(ctx context.Context, id uuid.UUID) (*playgroundDomain.Session, error) {
	var session playgroundDomain.Session
	err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("get session %s: %w", id, playgroundDomain.ErrSessionNotFound)
		}
		return nil, fmt.Errorf("get session by ID: %w", err)
	}

	return &session, nil
}

// List retrieves sessions for a project (for sidebar).
// Ordered by last_used_at DESC.
func (r *sessionRepository) List(ctx context.Context, projectID uuid.UUID, limit int) ([]*playgroundDomain.Session, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var sessions []*playgroundDomain.Session
	err := r.db.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("last_used_at DESC").
		Limit(limit).
		Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("list sessions for project %s: %w", projectID, err)
	}
	return sessions, nil
}

// ListByTags retrieves sessions filtered by tags.
// Returns sessions where tags contains any of the provided tags.
func (r *sessionRepository) ListByTags(ctx context.Context, projectID uuid.UUID, tags []string, limit int) ([]*playgroundDomain.Session, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var sessions []*playgroundDomain.Session
	err := r.db.WithContext(ctx).
		Where("project_id = ? AND tags && ?", projectID, pq.Array(tags)).
		Order("last_used_at DESC").
		Limit(limit).
		Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("list sessions by tags for project %s: %w", projectID, err)
	}
	return sessions, nil
}

// Update updates an existing session.
func (r *sessionRepository) Update(ctx context.Context, session *playgroundDomain.Session) error {
	session.UpdatedAt = time.Now()
	result := r.db.WithContext(ctx).Save(session)
	if result.Error != nil {
		return fmt.Errorf("update session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update session: %w", playgroundDomain.ErrSessionNotFound)
	}
	return nil
}

// UpdateLastRun updates only the last_run and last_used_at fields.
func (r *sessionRepository) UpdateLastRun(ctx context.Context, id uuid.UUID, lastRun playgroundDomain.JSON) error {
	now := time.Now()

	result := r.db.WithContext(ctx).
		Model(&playgroundDomain.Session{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_run":     lastRun,
			"last_used_at": now,
			"updated_at":   now,
		})

	if result.Error != nil {
		return fmt.Errorf("update last run: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update last run for session %s: %w", id, playgroundDomain.ErrSessionNotFound)
	}

	return nil
}

// UpdateWindows updates only the windows JSONB field.
func (r *sessionRepository) UpdateWindows(ctx context.Context, id uuid.UUID, windows playgroundDomain.JSON) error {
	now := time.Now()
	result := r.db.WithContext(ctx).
		Model(&playgroundDomain.Session{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"windows":    windows,
			"updated_at": now,
		})
	if result.Error != nil {
		return fmt.Errorf("update windows: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update windows for session %s: %w", id, playgroundDomain.ErrSessionNotFound)
	}
	return nil
}

// Delete removes a session by ID.
func (r *sessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).
		Where("id = ?", id).
		Delete(&playgroundDomain.Session{})
	if result.Error != nil {
		return fmt.Errorf("delete session: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("delete session %s: %w", id, playgroundDomain.ErrSessionNotFound)
	}
	return nil
}

// Exists checks if a session exists.
func (r *sessionRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&playgroundDomain.Session{}).
		Where("id = ?", id).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check session exists: %w", err)
	}
	return count > 0, nil
}

// ExistsByProjectID checks if a session exists for a specific project.
func (r *sessionRepository) ExistsByProjectID(ctx context.Context, id uuid.UUID, projectID uuid.UUID) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&playgroundDomain.Session{}).
		Where("id = ? AND project_id = ?", id, projectID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check session exists in project: %w", err)
	}
	return count > 0, nil
}
