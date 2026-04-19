package billing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	billingDomain "brokle/internal/core/domain/billing"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
)

// contractHistoryRepository is the pgx+sqlc implementation of
// billingDomain.ContractHistoryRepository. Append-only audit log.
//
// Boundary note: the domain still represents the actor as a plain
// string (UUID-string) rather than *uuid.UUID. The repo converts at
// the boundary: empty string ⇒ nil actor (system event), and
// invalid UUIDs surface an error instead of silently dropping.
type contractHistoryRepository struct {
	tm *db.TxManager
}

// NewContractHistoryRepository returns the pgx-backed repository.
func NewContractHistoryRepository(tm *db.TxManager) billingDomain.ContractHistoryRepository {
	return &contractHistoryRepository{tm: tm}
}

func (r *contractHistoryRepository) Log(ctx context.Context, h *billingDomain.ContractHistory) error {
	if h.ChangedAt.IsZero() {
		h.ChangedAt = time.Now()
	}
	actor, err := parseActorID(h.ChangedBy)
	if err != nil {
		return fmt.Errorf("log contract history (contract=%s): %w", h.ContractID, err)
	}
	if err := r.tm.Queries(ctx).CreateContractHistory(ctx, gen.CreateContractHistoryParams{
		ID:             h.ID,
		ContractID:     h.ContractID,
		Action:         string(h.Action),
		ChangedBy:      actor,
		ChangedByEmail: h.ChangedByEmail,
		ChangedAt:      h.ChangedAt,
		Changes:        h.Changes,
		Reason:         h.Reason,
	}); err != nil {
		return fmt.Errorf("log contract history (contract=%s): %w", h.ContractID, err)
	}
	return nil
}

func (r *contractHistoryRepository) GetByContractID(ctx context.Context, contractID uuid.UUID) ([]*billingDomain.ContractHistory, error) {
	rows, err := r.tm.Queries(ctx).ListContractHistoryByContract(ctx, contractID)
	if err != nil {
		return nil, fmt.Errorf("get contract history for %s: %w", contractID, err)
	}
	out := make([]*billingDomain.ContractHistory, 0, len(rows))
	for i := range rows {
		out = append(out, contractHistoryFromRow(&rows[i]))
	}
	return out, nil
}

// ----- gen ↔ domain boundary -----------------------------------------

func contractHistoryFromRow(row *gen.ContractHistory) *billingDomain.ContractHistory {
	var changedBy string
	if row.ChangedBy != nil {
		changedBy = row.ChangedBy.String()
	}
	return &billingDomain.ContractHistory{
		ID:             row.ID,
		ContractID:     row.ContractID,
		Action:         billingDomain.ContractAction(row.Action),
		ChangedBy:      changedBy,
		ChangedByEmail: row.ChangedByEmail,
		ChangedAt:      row.ChangedAt,
		Changes:        row.Changes,
		Reason:         row.Reason,
	}
}

// parseActorID maps the legacy ChangedBy string to a nullable UUID.
// Empty string means "system event" (actor_id NULL in the DB). A
// non-empty string must parse as UUID or the caller gets an error —
// we don't silently discard malformed IDs because that would leak
// bad audit entries.
func parseActorID(s string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parse actor id %q: %w", s, err)
	}
	return &id, nil
}

