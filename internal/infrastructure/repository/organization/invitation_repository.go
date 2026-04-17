package organization

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"time"

	"github.com/google/uuid"

	orgDomain "brokle/internal/core/domain/organization"
	"brokle/internal/infrastructure/db"
	"brokle/internal/infrastructure/db/gen"
	"brokle/pkg/token"
)

// invitationRepository is the pgx+sqlc implementation of
// orgDomain.InvitationRepository. Soft-delete via deleted_at;
// GetByToken is kept for the deprecated raw-token path in the
// registration service — it hashes the token before lookup.
type invitationRepository struct {
	tm *db.TxManager
}

// NewInvitationRepository returns the pgx-backed repository.
func NewInvitationRepository(tm *db.TxManager) orgDomain.InvitationRepository {
	return &invitationRepository{tm: tm}
}

// ----- CRUD ----------------------------------------------------------

func (r *invitationRepository) Create(ctx context.Context, inv *orgDomain.Invitation) error {
	now := time.Now()
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = now
	}
	if inv.UpdatedAt.IsZero() {
		inv.UpdatedAt = now
	}
	if inv.Status == "" {
		inv.Status = orgDomain.InvitationStatusPending
	}
	invitedBy := inv.InvitedByID
	if err := r.tm.Queries(ctx).CreateInvitation(ctx, gen.CreateInvitationParams{
		ID:             inv.ID,
		OrganizationID: inv.OrganizationID,
		RoleID:         inv.RoleID,
		Email:          inv.Email,
		Status:         string(inv.Status),
		ExpiresAt:      inv.ExpiresAt,
		CreatedAt:      inv.CreatedAt,
		UpdatedAt:      inv.UpdatedAt,
		InvitedByID:    &invitedBy,
		TokenHash:      inv.TokenHash,
		TokenPreview:   emptyToNilString(inv.TokenPreview),
		Message:        inv.Message,
		ResentCount:    int32(inv.ResentCount),
	}); err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}
	return nil
}

func (r *invitationRepository) GetByID(ctx context.Context, id uuid.UUID) (*orgDomain.Invitation, error) {
	row, err := r.tm.Queries(ctx).GetInvitationByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get invitation %s: %w", id, orgDomain.ErrInvitationNotFound)
		}
		return nil, fmt.Errorf("get invitation %s: %w", id, err)
	}
	return invitationFromRow(&row), nil
}

// GetByToken is retained for the deprecated raw-token path (registration
// service). It hashes the token and delegates to GetByTokenHash.
func (r *invitationRepository) GetByToken(ctx context.Context, rawToken string) (*orgDomain.Invitation, error) {
	return r.GetByTokenHash(ctx, token.HashToken(rawToken))
}

func (r *invitationRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*orgDomain.Invitation, error) {
	row, err := r.tm.Queries(ctx).GetInvitationByTokenHash(ctx, tokenHash)
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get invitation by token hash: %w", orgDomain.ErrInvitationNotFound)
		}
		return nil, fmt.Errorf("get invitation by token hash: %w", err)
	}
	return invitationFromRow(&row), nil
}

func (r *invitationRepository) Update(ctx context.Context, inv *orgDomain.Invitation) error {
	if err := r.tm.Queries(ctx).UpdateInvitation(ctx, gen.UpdateInvitationParams{
		ID:             inv.ID,
		OrganizationID: inv.OrganizationID,
		RoleID:         inv.RoleID,
		Email:          inv.Email,
		Status:         string(inv.Status),
		ExpiresAt:      inv.ExpiresAt,
		AcceptedAt:     inv.AcceptedAt,
		RevokedAt:      inv.RevokedAt,
		ResentAt:       inv.ResentAt,
		AcceptedByID:   inv.AcceptedByID,
		RevokedByID:    inv.RevokedByID,
		ResentCount:    int32(inv.ResentCount),
		Message:        inv.Message,
		TokenHash:      inv.TokenHash,
		TokenPreview:   emptyToNilString(inv.TokenPreview),
	}); err != nil {
		return fmt.Errorf("update invitation %s: %w", inv.ID, err)
	}
	return nil
}

func (r *invitationRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).SoftDeleteInvitation(ctx, id); err != nil {
		return fmt.Errorf("soft-delete invitation %s: %w", id, err)
	}
	return nil
}

// ----- Listings ------------------------------------------------------

func (r *invitationRepository) GetByOrganizationID(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Invitation, error) {
	rows, err := r.tm.Queries(ctx).ListInvitationsByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list invitations for org %s: %w", orgID, err)
	}
	return invitationsFromRows(rows), nil
}

func (r *invitationRepository) GetByEmail(ctx context.Context, email string) ([]*orgDomain.Invitation, error) {
	rows, err := r.tm.Queries(ctx).ListInvitationsByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("list invitations for email %s: %w", email, err)
	}
	return invitationsFromRows(rows), nil
}

func (r *invitationRepository) GetPendingByEmail(ctx context.Context, orgID uuid.UUID, email string) (*orgDomain.Invitation, error) {
	row, err := r.tm.Queries(ctx).GetPendingInvitationByOrgAndEmail(ctx, gen.GetPendingInvitationByOrgAndEmailParams{
		OrganizationID: orgID,
		Email:          email,
	})
	if err != nil {
		if db.IsNoRows(err) {
			return nil, fmt.Errorf("get pending invitation (org=%s email=%s): %w", orgID, email, orgDomain.ErrInvitationNotFound)
		}
		return nil, fmt.Errorf("get pending invitation (org=%s email=%s): %w", orgID, email, err)
	}
	return invitationFromRow(&row), nil
}

func (r *invitationRepository) GetPendingInvitations(ctx context.Context, orgID uuid.UUID) ([]*orgDomain.Invitation, error) {
	rows, err := r.tm.Queries(ctx).ListPendingInvitationsByOrganization(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list pending invitations for org %s: %w", orgID, err)
	}
	return invitationsFromRows(rows), nil
}

// ----- Status transitions --------------------------------------------

func (r *invitationRepository) MarkAccepted(ctx context.Context, id uuid.UUID, acceptedByID uuid.UUID) error {
	if err := r.tm.Queries(ctx).MarkInvitationAccepted(ctx, gen.MarkInvitationAcceptedParams{
		ID:           id,
		AcceptedByID: &acceptedByID,
	}); err != nil {
		return fmt.Errorf("mark invitation %s accepted: %w", id, err)
	}
	return nil
}

func (r *invitationRepository) MarkExpired(ctx context.Context, id uuid.UUID) error {
	if err := r.tm.Queries(ctx).MarkInvitationExpired(ctx, id); err != nil {
		return fmt.Errorf("mark invitation %s expired: %w", id, err)
	}
	return nil
}

func (r *invitationRepository) RevokeInvitation(ctx context.Context, id uuid.UUID, revokedByID uuid.UUID) error {
	if err := r.tm.Queries(ctx).RevokeInvitation(ctx, gen.RevokeInvitationParams{
		ID:          id,
		RevokedByID: &revokedByID,
	}); err != nil {
		return fmt.Errorf("revoke invitation %s: %w", id, err)
	}
	return nil
}

// MarkResent atomically increments resent_count + refreshes expires_at
// when the invitation is still within the resend limit and cooldown.
// Zero rows affected ⇒ re-read the row to disambiguate limit vs cooldown.
func (r *invitationRepository) MarkResent(
	ctx context.Context,
	id uuid.UUID,
	newExpiresAt time.Time,
	maxAttempts int,
	cooldown time.Duration,
) error {
	cooldownThreshold := time.Now().Add(-cooldown)
	n, err := r.tm.Queries(ctx).MarkInvitationResentIfAllowed(ctx, gen.MarkInvitationResentIfAllowedParams{
		ID:         id,
		ExpiresAt:  newExpiresAt,
		Column3:    int32(maxAttempts),
		Column4:    cooldownThreshold,
	})
	if err != nil {
		return fmt.Errorf("mark invitation %s resent: %w", id, err)
	}
	if n > 0 {
		return nil
	}
	row, err := r.tm.Queries(ctx).GetInvitationByID(ctx, id)
	if err != nil {
		if db.IsNoRows(err) {
			return fmt.Errorf("mark invitation %s resent: %w", id, orgDomain.ErrInvitationNotFound)
		}
		return fmt.Errorf("mark invitation %s resent: %w", id, err)
	}
	if int(row.ResentCount) >= maxAttempts {
		return ErrResendLimitReached
	}
	return ErrResendCooldown
}

func (r *invitationRepository) CleanupExpiredInvitations(ctx context.Context) error {
	if err := r.tm.Queries(ctx).CleanupExpiredPendingInvitations(ctx); err != nil {
		return fmt.Errorf("cleanup expired invitations: %w", err)
	}
	return nil
}

func (r *invitationRepository) UpdateTokenHash(ctx context.Context, id uuid.UUID, tokenHash, tokenPreview string) error {
	if err := r.tm.Queries(ctx).UpdateInvitationTokenHash(ctx, gen.UpdateInvitationTokenHashParams{
		ID:           id,
		TokenHash:    tokenHash,
		TokenPreview: emptyToNilString(tokenPreview),
	}); err != nil {
		return fmt.Errorf("update invitation token hash %s: %w", id, err)
	}
	return nil
}

// ----- Validation ----------------------------------------------------

func (r *invitationRepository) IsEmailAlreadyInvited(ctx context.Context, email string, orgID uuid.UUID) (bool, error) {
	ok, err := r.tm.Queries(ctx).IsEmailAlreadyInvited(ctx, gen.IsEmailAlreadyInvitedParams{
		OrganizationID: orgID,
		Email:          email,
	})
	if err != nil {
		return false, fmt.Errorf("check invitation exists (org=%s email=%s): %w", orgID, email, err)
	}
	return ok, nil
}

// ----- Audit events --------------------------------------------------

func (r *invitationRepository) CreateAuditEvent(ctx context.Context, event *orgDomain.InvitationAuditEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	ipAddr, err := parseIPAddr(event.IPAddress)
	if err != nil {
		return fmt.Errorf("create invitation audit event: %w", err)
	}
	if err := r.tm.Queries(ctx).CreateInvitationAuditEvent(ctx, gen.CreateInvitationAuditEventParams{
		ID:           event.ID,
		InvitationID: event.InvitationID,
		EventType:    string(event.EventType),
		ActorID:      event.ActorID,
		ActorType:    string(event.ActorType),
		Metadata:     metadataFromString(event.Metadata),
		IpAddress:    ipAddr,
		UserAgent:    event.UserAgent,
		CreatedAt:    event.CreatedAt,
	}); err != nil {
		return fmt.Errorf("create invitation audit event: %w", err)
	}
	return nil
}

func (r *invitationRepository) GetAuditEventsByInvitationID(ctx context.Context, invitationID uuid.UUID) ([]*orgDomain.InvitationAuditEvent, error) {
	rows, err := r.tm.Queries(ctx).ListInvitationAuditEventsByInvitation(ctx, invitationID)
	if err != nil {
		return nil, fmt.Errorf("list invitation audit events %s: %w", invitationID, err)
	}
	out := make([]*orgDomain.InvitationAuditEvent, 0, len(rows))
	for i := range rows {
		out = append(out, auditEventFromRow(&rows[i]))
	}
	return out, nil
}

// ----- gen ↔ domain boundary ----------------------------------------

func invitationFromRow(row *gen.UserInvitation) *orgDomain.Invitation {
	return &orgDomain.Invitation{
		ID:             row.ID,
		OrganizationID: row.OrganizationID,
		RoleID:         row.RoleID,
		Email:          row.Email,
		Status:         orgDomain.InvitationStatus(row.Status),
		ExpiresAt:      row.ExpiresAt,
		AcceptedAt:     row.AcceptedAt,
		RevokedAt:      row.RevokedAt,
		ResentAt:       row.ResentAt,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		DeletedAt:      row.DeletedAt,
		InvitedByID:    derefUUID(row.InvitedByID),
		AcceptedByID:   row.AcceptedByID,
		RevokedByID:    row.RevokedByID,
		TokenHash:      row.TokenHash,
		TokenPreview:   derefString(row.TokenPreview),
		Message:        row.Message,
		ResentCount:    int(row.ResentCount),
	}
}

func invitationsFromRows(rows []gen.UserInvitation) []*orgDomain.Invitation {
	out := make([]*orgDomain.Invitation, 0, len(rows))
	for i := range rows {
		out = append(out, invitationFromRow(&rows[i]))
	}
	return out
}

func auditEventFromRow(row *gen.InvitationAuditEvent) *orgDomain.InvitationAuditEvent {
	return &orgDomain.InvitationAuditEvent{
		ID:           row.ID,
		InvitationID: row.InvitationID,
		EventType:    orgDomain.InvitationAuditEventType(row.EventType),
		ActorID:      row.ActorID,
		ActorType:    orgDomain.InvitationAuditActorType(row.ActorType),
		Metadata:     metadataToString(row.Metadata),
		IPAddress:    ipAddrToString(row.IpAddress),
		UserAgent:    row.UserAgent,
		CreatedAt:    row.CreatedAt,
	}
}

// parseIPAddr parses a domain IP string into pgx's netip.Addr; nil IP ⇒ NULL.
func parseIPAddr(ip *string) (*netip.Addr, error) {
	if ip == nil || *ip == "" {
		return nil, nil
	}
	addr, err := netip.ParseAddr(*ip)
	if err != nil {
		return nil, fmt.Errorf("invalid IP %q: %w", *ip, err)
	}
	return &addr, nil
}

func ipAddrToString(addr *netip.Addr) *string {
	if addr == nil || !addr.IsValid() {
		return nil
	}
	s := addr.String()
	return &s
}

// Domain stores metadata as *string containing JSON; sqlc emits
// json.RawMessage. Convert at the boundary to keep domain stable.
func metadataFromString(s *string) json.RawMessage {
	if s == nil || *s == "" {
		return nil
	}
	return json.RawMessage(*s)
}

func metadataToString(m json.RawMessage) *string {
	if len(m) == 0 {
		return nil
	}
	s := string(m)
	return &s
}
