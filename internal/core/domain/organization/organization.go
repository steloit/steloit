// Package organization provides the organizational structure domain model.
//
// The organization domain handles multi-tenancy, organizational hierarchy,
// project management, and team membership across the platform.
package organization

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"brokle/pkg/uid"

	"gorm.io/gorm"
)

// Organization represents a tenant organization with full multi-tenancy.
type Organization struct {
	UpdatedAt          time.Time              `json:"updated_at"`
	CreatedAt          time.Time              `json:"created_at"`
	TrialEndsAt        *time.Time             `json:"trial_ends_at,omitempty"`
	DeletedAt          gorm.DeletedAt         `json:"deleted_at,omitempty" gorm:"index"`
	Plan               string                 `json:"plan" gorm:"size:50;default:'free'"`
	SubscriptionStatus string                 `json:"subscription_status" gorm:"size:50;default:'active'"`
	BillingEmail       string                 `json:"billing_email,omitempty" gorm:"size:255"`
	Name               string                 `json:"name" gorm:"size:255;not null"`
	Projects           []Project              `json:"projects,omitempty" gorm:"foreignKey:OrganizationID"`
	Members            []Member               `json:"members,omitempty" gorm:"foreignKey:OrganizationID"`
	Invitations        []Invitation           `json:"invitations,omitempty" gorm:"foreignKey:OrganizationID"`
	Settings           []OrganizationSettings `json:"settings,omitempty" gorm:"foreignKey:OrganizationID"`
	ID                 uuid.UUID              `json:"id" gorm:"type:uuid;primaryKey"`
}

// Member represents the many-to-many relationship between users and organizations.
type Member struct {
	JoinedAt       time.Time      `json:"joined_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	InvitedBy      *uuid.UUID     `json:"invited_by,omitempty" gorm:"type:uuid"`
	DeletedAt      gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
	Status         string         `json:"status" gorm:"size:20;default:'active'"`
	Organization   Organization   `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
	OrganizationID uuid.UUID      `json:"organization_id" gorm:"type:uuid;not null;primaryKey;priority:1"`
	UserID         uuid.UUID      `json:"user_id" gorm:"type:uuid;not null;primaryKey;priority:2"`
	RoleID         uuid.UUID      `json:"role_id" gorm:"type:uuid;not null"`
}

// Project represents a project within an organization.
type Project struct {
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"deleted_at,omitempty" gorm:"index"`
	Name           string         `json:"name" gorm:"size:255;not null"`
	Description    string         `json:"description,omitempty" gorm:"text"`
	Status         string         `json:"status" gorm:"size:20;not null;default:active;check:status IN ('active','archived')"`
	Organization   Organization   `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
	ID             uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID      `json:"organization_id" gorm:"type:uuid;not null"`
}

// Invitation represents an invitation to join an organization.
type Invitation struct {
	ExpiresAt      time.Time        `json:"expires_at"`
	UpdatedAt      time.Time        `json:"updated_at"`
	CreatedAt      time.Time        `json:"created_at"`
	AcceptedAt     *time.Time       `json:"accepted_at,omitempty"`
	RevokedAt      *time.Time       `json:"revoked_at,omitempty"`
	ResentAt       *time.Time       `json:"resent_at,omitempty"`
	DeletedAt      gorm.DeletedAt   `json:"deleted_at,omitempty" gorm:"index"`
	Status         InvitationStatus `json:"status" gorm:"size:50;default:'pending'"`
	TokenHash      string           `json:"-" gorm:"size:64;not null"`              // SHA-256 hash for secure storage
	TokenPreview   string           `json:"token_preview,omitempty" gorm:"size:16"` // First 12 chars for display: "inv_AbCd..."
	Email          string           `json:"email" gorm:"size:255;not null"`
	Message        *string          `json:"message,omitempty" gorm:"type:text"` // Personal message from inviter
	Organization   Organization     `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
	ID             uuid.UUID        `json:"id" gorm:"type:uuid;primaryKey"`
	InvitedByID    uuid.UUID        `json:"invited_by_id" gorm:"type:uuid;not null"`
	RoleID         uuid.UUID        `json:"role_id" gorm:"type:uuid;not null"`
	OrganizationID uuid.UUID        `json:"organization_id" gorm:"type:uuid;not null"`
	AcceptedByID   *uuid.UUID       `json:"accepted_by_id,omitempty" gorm:"type:uuid"` // User who accepted (for audit)
	RevokedByID    *uuid.UUID       `json:"revoked_by_id,omitempty" gorm:"type:uuid"`  // User who revoked (for audit)
	ResentCount    int              `json:"resent_count" gorm:"default:0"`             // Track resend attempts
}

// Request/Response DTOs
type CreateOrganizationRequest struct {
	Name         string `json:"name" validate:"required,min=1,max=100"`
	BillingEmail string `json:"billing_email" validate:"email"`
}

type UpdateOrganizationRequest struct {
	Name         *string `json:"name,omitempty"`
	BillingEmail *string `json:"billing_email,omitempty"`
	Plan         *string `json:"plan,omitempty"`
}

type CreateProjectRequest struct {
	Name        string `json:"name" validate:"required,min=1,max=100"`
	Description string `json:"description"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

type InviteUserRequest struct {
	Email   string    `json:"email" validate:"required,email"`
	RoleID  uuid.UUID `json:"role_id" validate:"required"`
	Message *string   `json:"message,omitempty" validate:"omitempty,max=500"` // Personal message for the invitee
}

// InvitationStatus represents the status of an organization invitation
type InvitationStatus string

// Invitation statuses
const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusExpired  InvitationStatus = "expired"
	InvitationStatusRevoked  InvitationStatus = "revoked"
)

// Constructor functions
func NewOrganization(name string) *Organization {
	return &Organization{
		ID:                 uid.New(),
		Name:               name,
		Plan:               "free",
		SubscriptionStatus: "active",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
}

func NewProject(orgID uuid.UUID, name, description string) *Project {
	return &Project{
		ID:             uid.New(),
		OrganizationID: orgID,
		Name:           name,
		Description:    description,
		Status:         "active",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

func NewMember(orgID, userID, roleID uuid.UUID) *Member {
	now := time.Now()
	return &Member{
		OrganizationID: orgID,
		UserID:         userID,
		RoleID:         roleID,
		Status:         "active",
		JoinedAt:       now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// NewInvitation creates a new invitation with secure token handling
// The tokenHash is the SHA-256 hash of the plaintext token
// The tokenPreview is the first 12 characters of the plaintext token for display
func NewInvitation(orgID, roleID, invitedByID uuid.UUID, email, tokenHash, tokenPreview string, expiresAt time.Time) *Invitation {
	return &Invitation{
		ID:             uid.New(),
		OrganizationID: orgID,
		RoleID:         roleID,
		InvitedByID:    invitedByID,
		Email:          email,
		TokenHash:      tokenHash,
		TokenPreview:   tokenPreview,
		Status:         InvitationStatusPending,
		ExpiresAt:      expiresAt,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		ResentCount:    0,
	}
}

// NewInvitationWithMessage creates a new invitation with an optional personal message
func NewInvitationWithMessage(orgID, roleID, invitedByID uuid.UUID, email, tokenHash, tokenPreview string, message *string, expiresAt time.Time) *Invitation {
	inv := NewInvitation(orgID, roleID, invitedByID, email, tokenHash, tokenPreview, expiresAt)
	inv.Message = message
	return inv
}

// Utility methods
func (i *Invitation) IsExpired() bool {
	return time.Now().After(i.ExpiresAt)
}

func (i *Invitation) IsValid() bool {
	return i.Status == InvitationStatusPending && !i.IsExpired()
}

// Accept marks the invitation as accepted by the given user
func (i *Invitation) Accept(acceptedByID uuid.UUID) {
	now := time.Now()
	i.Status = InvitationStatusAccepted
	i.AcceptedAt = &now
	i.AcceptedByID = &acceptedByID
	i.UpdatedAt = now
}

// Revoke marks the invitation as revoked by the given user
func (i *Invitation) Revoke(revokedByID uuid.UUID) {
	now := time.Now()
	i.Status = InvitationStatusRevoked
	i.RevokedAt = &now
	i.RevokedByID = &revokedByID
	i.UpdatedAt = now
}

// MarkResent updates the invitation after a resend
func (i *Invitation) MarkResent(newExpiresAt time.Time) {
	now := time.Now()
	i.ResentAt = &now
	i.ResentCount++
	i.ExpiresAt = newExpiresAt
	i.UpdatedAt = now
}

// CanResend checks if the invitation can be resent (max 5 resends, 1 hour cooldown)
func (i *Invitation) CanResend() bool {
	if i.ResentCount >= 5 {
		return false
	}
	if i.ResentAt != nil && time.Since(*i.ResentAt) < time.Hour {
		return false
	}
	return true
}

// Project utility methods
func (p *Project) IsActive() bool {
	return p.Status == "active"
}

func (p *Project) IsArchived() bool {
	return p.Status == "archived"
}

func (p *Project) Archive() {
	p.Status = "archived"
	p.UpdatedAt = time.Now()
}

func (p *Project) Unarchive() {
	p.Status = "active"
	p.UpdatedAt = time.Now()
}

// OrganizationSettings represents key-value settings for an organization.
type OrganizationSettings struct {
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
	Key            string       `json:"key" gorm:"size:255;not null"`
	Value          string       `json:"value" gorm:"type:jsonb;not null"`
	Organization   Organization `json:"organization,omitempty" gorm:"foreignKey:OrganizationID"`
	ID             uuid.UUID    `json:"id" gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID    `json:"organization_id" gorm:"type:uuid;not null"`
}

// Settings-related DTOs
type CreateOrganizationSettingRequest struct {
	Value interface{} `json:"value" validate:"required"`
	Key   string      `json:"key" validate:"required,min=1,max=255"`
}

type UpdateOrganizationSettingRequest struct {
	Value interface{} `json:"value" validate:"required"`
}

type GetOrganizationSettingsResponse struct {
	Settings map[string]interface{} `json:"settings"`
}

// OrganizationSetting utility methods
func NewOrganizationSettings(orgID uuid.UUID, key string, value interface{}) (*OrganizationSettings, error) {
	// Convert value to JSON string for storage
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	return &OrganizationSettings{
		ID:             uid.New(),
		OrganizationID: orgID,
		Key:            key,
		Value:          string(valueBytes),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}, nil
}

func (os *OrganizationSettings) GetValue() (interface{}, error) {
	var value interface{}
	err := json.Unmarshal([]byte(os.Value), &value)
	return value, err
}

func (os *OrganizationSettings) SetValue(value interface{}) error {
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return err
	}
	os.Value = string(valueBytes)
	os.UpdatedAt = time.Now()
	return nil
}

// OrganizationWithProjectsAndRole represents an organization with its projects and the user's role
type OrganizationWithProjectsAndRole struct {
	Organization *Organization
	RoleName     string
	Projects     []*Project
}

// InvitationAuditEventType represents the type of audit event
type InvitationAuditEventType string

// Audit event types
const (
	AuditEventCreated  InvitationAuditEventType = "created"
	AuditEventResent   InvitationAuditEventType = "resent"
	AuditEventAccepted InvitationAuditEventType = "accepted"
	AuditEventRevoked  InvitationAuditEventType = "revoked"
	AuditEventExpired  InvitationAuditEventType = "expired"
	AuditEventDeclined InvitationAuditEventType = "declined"
)

// InvitationAuditActorType represents who performed the action
type InvitationAuditActorType string

// Actor types
const (
	ActorTypeUser   InvitationAuditActorType = "user"
	ActorTypeSystem InvitationAuditActorType = "system"
)

// InvitationAuditEvent represents an audit log entry for invitation lifecycle events
type InvitationAuditEvent struct {
	CreatedAt    time.Time                `json:"created_at"`
	EventType    InvitationAuditEventType `json:"event_type" gorm:"size:50;not null"`
	ActorType    InvitationAuditActorType `json:"actor_type" gorm:"size:20;not null;default:'user'"`
	Metadata     *string                  `json:"metadata,omitempty" gorm:"type:jsonb"` // JSON metadata
	IPAddress    *string                  `json:"ip_address,omitempty" gorm:"type:inet"`
	UserAgent    *string                  `json:"user_agent,omitempty" gorm:"type:text"`
	ID           uuid.UUID                `json:"id" gorm:"type:uuid;primaryKey"`
	InvitationID uuid.UUID                `json:"invitation_id" gorm:"type:uuid;not null"`
	ActorID      *uuid.UUID               `json:"actor_id,omitempty" gorm:"type:uuid"` // NULL for system events
}

// NewInvitationAuditEvent creates a new audit event
func NewInvitationAuditEvent(invitationID uuid.UUID, eventType InvitationAuditEventType, actorID *uuid.UUID, actorType InvitationAuditActorType) *InvitationAuditEvent {
	return &InvitationAuditEvent{
		ID:           uid.New(),
		InvitationID: invitationID,
		EventType:    eventType,
		ActorID:      actorID,
		ActorType:    actorType,
		CreatedAt:    time.Now(),
	}
}

// WithMetadata adds metadata to the audit event
func (e *InvitationAuditEvent) WithMetadata(metadata map[string]interface{}) *InvitationAuditEvent {
	if metadata != nil {
		bytes, err := json.Marshal(metadata)
		if err == nil {
			str := string(bytes)
			e.Metadata = &str
		}
	}
	return e
}

// WithRequestInfo adds IP address and user agent to the audit event
func (e *InvitationAuditEvent) WithRequestInfo(ipAddress, userAgent string) *InvitationAuditEvent {
	if ipAddress != "" {
		e.IPAddress = &ipAddress
	}
	if userAgent != "" {
		e.UserAgent = &userAgent
	}
	return e
}

// Table name methods for GORM
func (Organization) TableName() string         { return "organizations" }
func (Member) TableName() string               { return "organization_members" }
func (Project) TableName() string              { return "projects" }
func (Invitation) TableName() string           { return "user_invitations" }
func (OrganizationSettings) TableName() string { return "organization_settings" }
func (InvitationAuditEvent) TableName() string { return "invitation_audit_events" }
