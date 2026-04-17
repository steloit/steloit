// Package playground provides repository implementations for playground session storage.
package playground

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	playgroundDomain "brokle/internal/core/domain/playground"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// sessionRepository is the pgx+sqlc implementation of
// playgroundDomain.SessionRepository. Hybrid saved/unsaved UX: unsaved
// sessions have NULL name and expire after 30 days of inactivity.
type sessionRepository struct {
	tm *db.TxManager
}

// NewSessionRepository returns the pgx-backed repository.
func NewSessionRepository(tm *db.TxManager) playgroundDomain.SessionRepository {
	return &sessionRepository{tm: tm}
}

func (r *sessionRepository) Create(ctx context.Context, s *playgroundDomain.Session) error {
	now := time.Now()
	s.CreatedAt = now
	s.UpdatedAt = now
	s.LastUsedAt = now
	if err := r.tm.Queries(ctx).CreatePlaygroundSession(ctx, gen.CreatePlaygroundSessionParams{
		ID:          s.ID,
		ProjectID:   s.ProjectID,
		Name:        s.Name,
		Description: s.Description,
		Tags:        []string(s.Tags),
		Variables:   jsonFromPlayground(s.Variables, true),
		Config:      jsonFromPlayground(s.Config, false),
		Windows:     jsonFromPlayground(s.Windows, false),
		LastRun:     jsonFromPlayground(s.LastRun, false),
		CreatedBy:   s.CreatedBy,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
		LastUsedAt:  s.LastUsedAt,
	}); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (r *sessionRepository) GetByID(ctx context.Context, id uuid.UUID) (*playgroundDomain.Session, error) {
	row, err := r.tm.Queries(ctx).GetPlaygroundSessionByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get session %s: %w", id, playgroundDomain.ErrSessionNotFound)
		}
		return nil, fmt.Errorf("get session %s: %w", id, err)
	}
	return sessionFromRow(&row), nil
}

func (r *sessionRepository) List(ctx context.Context, projectID uuid.UUID, limit int) ([]*playgroundDomain.Session, error) {
	limit = clampLimit(limit)
	rows, err := r.tm.Queries(ctx).ListPlaygroundSessionsByProject(ctx, gen.ListPlaygroundSessionsByProjectParams{
		ProjectID: projectID,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list sessions for project %s: %w", projectID, err)
	}
	return sessionsFromRows(rows), nil
}

func (r *sessionRepository) ListByTags(ctx context.Context, projectID uuid.UUID, tags []string, limit int) ([]*playgroundDomain.Session, error) {
	limit = clampLimit(limit)
	rows, err := r.tm.Queries(ctx).ListPlaygroundSessionsByProjectAndTags(ctx, gen.ListPlaygroundSessionsByProjectAndTagsParams{
		ProjectID: projectID,
		Column2:   tags,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list sessions by tags for project %s: %w", projectID, err)
	}
	return sessionsFromRows(rows), nil
}

func (r *sessionRepository) Update(ctx context.Context, s *playgroundDomain.Session) error {
	s.UpdatedAt = time.Now()
	n, err := r.tm.Queries(ctx).UpdatePlaygroundSession(ctx, gen.UpdatePlaygroundSessionParams{
		ID:          s.ID,
		Name:        s.Name,
		Description: s.Description,
		Tags:        []string(s.Tags),
		Variables:   jsonFromPlayground(s.Variables, true),
		Config:      jsonFromPlayground(s.Config, false),
		Windows:     jsonFromPlayground(s.Windows, false),
		LastRun:     jsonFromPlayground(s.LastRun, false),
		LastUsedAt:  s.LastUsedAt,
	})
	if err != nil {
		return fmt.Errorf("update session %s: %w", s.ID, err)
	}
	if n == 0 {
		return fmt.Errorf("update session %s: %w", s.ID, playgroundDomain.ErrSessionNotFound)
	}
	return nil
}

func (r *sessionRepository) UpdateLastRun(ctx context.Context, id uuid.UUID, lastRun playgroundDomain.JSON) error {
	n, err := r.tm.Queries(ctx).UpdatePlaygroundSessionLastRun(ctx, gen.UpdatePlaygroundSessionLastRunParams{
		ID:      id,
		LastRun: jsonFromPlayground(lastRun, false),
	})
	if err != nil {
		return fmt.Errorf("update last run for session %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("update last run for session %s: %w", id, playgroundDomain.ErrSessionNotFound)
	}
	return nil
}

func (r *sessionRepository) UpdateWindows(ctx context.Context, id uuid.UUID, windows playgroundDomain.JSON) error {
	n, err := r.tm.Queries(ctx).UpdatePlaygroundSessionWindows(ctx, gen.UpdatePlaygroundSessionWindowsParams{
		ID:      id,
		Windows: jsonFromPlayground(windows, false),
	})
	if err != nil {
		return fmt.Errorf("update windows for session %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("update windows for session %s: %w", id, playgroundDomain.ErrSessionNotFound)
	}
	return nil
}

func (r *sessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	n, err := r.tm.Queries(ctx).DeletePlaygroundSession(ctx, id)
	if err != nil {
		return fmt.Errorf("delete session %s: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete session %s: %w", id, playgroundDomain.ErrSessionNotFound)
	}
	return nil
}

func (r *sessionRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	ok, err := r.tm.Queries(ctx).PlaygroundSessionExists(ctx, id)
	if err != nil {
		return false, fmt.Errorf("check session exists: %w", err)
	}
	return ok, nil
}

func (r *sessionRepository) ExistsByProjectID(ctx context.Context, id, projectID uuid.UUID) (bool, error) {
	ok, err := r.tm.Queries(ctx).PlaygroundSessionExistsInProject(ctx, gen.PlaygroundSessionExistsInProjectParams{
		ID:        id,
		ProjectID: projectID,
	})
	if err != nil {
		return false, fmt.Errorf("check session exists in project: %w", err)
	}
	return ok, nil
}

// ----- helpers --------------------------------------------------------

func clampLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// jsonFromPlayground converts the domain JSON alias to json.RawMessage.
// `variables` is NOT NULL in schema (DEFAULT '{}'); empty becomes `{}`.
// Other JSONB columns are nullable; empty stays empty and pgx writes NULL.
func jsonFromPlayground(j playgroundDomain.JSON, defaultEmpty bool) json.RawMessage {
	if len(j) == 0 {
		if defaultEmpty {
			return json.RawMessage(`{}`)
		}
		return nil
	}
	return json.RawMessage(j)
}

// ----- gen ↔ domain boundary -----------------------------------------

func sessionFromRow(row *gen.PlaygroundSession) *playgroundDomain.Session {
	return &playgroundDomain.Session{
		ID:          row.ID,
		ProjectID:   row.ProjectID,
		Name:        row.Name,
		Description: row.Description,
		Tags:        row.Tags,
		Variables:   playgroundDomain.JSON(row.Variables),
		Config:      playgroundDomain.JSON(row.Config),
		Windows:     playgroundDomain.JSON(row.Windows),
		LastRun:     playgroundDomain.JSON(row.LastRun),
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
		LastUsedAt:  row.LastUsedAt,
	}
}

func sessionsFromRows(rows []gen.PlaygroundSession) []*playgroundDomain.Session {
	out := make([]*playgroundDomain.Session, 0, len(rows))
	for i := range rows {
		out = append(out, sessionFromRow(&rows[i]))
	}
	return out
}
