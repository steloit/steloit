package organization

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	authDomain "brokle/internal/core/domain/auth"
	billingDomain "brokle/internal/core/domain/billing"
	orgDomain "brokle/internal/core/domain/organization"
	userDomain "brokle/internal/core/domain/user"
	appErrors "brokle/pkg/errors"
)

// organizationService implements the orgDomain.OrganizationService interface
type organizationService struct {
	orgRepo     orgDomain.OrganizationRepository
	userRepo    userDomain.Repository
	memberSvc   orgDomain.MemberService
	projectSvc  orgDomain.ProjectService
	roleService authDomain.RoleService
	billingRepo billingDomain.OrganizationBillingRepository
	planRepo    billingDomain.PlanRepository
	logger      *slog.Logger
}

func NewOrganizationService(
	orgRepo orgDomain.OrganizationRepository,
	userRepo userDomain.Repository,
	memberSvc orgDomain.MemberService,
	projectSvc orgDomain.ProjectService,
	roleService authDomain.RoleService,
	billingRepo billingDomain.OrganizationBillingRepository,
	planRepo billingDomain.PlanRepository,
	logger *slog.Logger,
) orgDomain.OrganizationService {
	return &organizationService{
		orgRepo:     orgRepo,
		userRepo:    userRepo,
		memberSvc:   memberSvc,
		projectSvc:  projectSvc,
		roleService: roleService,
		billingRepo: billingRepo,
		planRepo:    planRepo,
		logger:      logger,
	}
}

func (s *organizationService) CreateOrganization(ctx context.Context, userID uuid.UUID, req *orgDomain.CreateOrganizationRequest) (*orgDomain.Organization, error) {
	// Create organization (no slug - use UUID only)
	org := orgDomain.NewOrganization(req.Name)
	if req.BillingEmail != "" {
		org.BillingEmail = req.BillingEmail
	}

	err := s.orgRepo.Create(ctx, org)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to create organization", err)
	}

	// Provision billing with Free plan
	if err := s.provisionBilling(ctx, org.ID); err != nil {
		s.logger.Error("failed to provision billing for organization",
			"error", err,
			"organization_id", org.ID,
		)
		// Don't fail org creation if billing provisioning fails - it can be retried
	}

	ownerRole, err := s.roleService.GetRoleByNameAndScope(ctx, "owner", authDomain.ScopeOrganization)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to get owner role", err)
	}

	err = s.memberSvc.AddMember(ctx, org.ID, userID, ownerRole.ID, userID)
	if err != nil {
		return nil, appErrors.NewInternalError("Failed to add user as organization owner", err)
	}

	// Set as user's default organization if they don't have one
	user, _ := s.userRepo.GetByID(ctx, userID)
	if user != nil && user.DefaultOrganizationID == nil {
		err = s.userRepo.SetDefaultOrganization(ctx, userID, org.ID)
		if err != nil {
			s.logger.Warn("failed to set default organization",
				"error", err,
				"user_id", userID,
				"organization_id", org.ID,
			)
		}
	}

	return org, nil
}

// provisionBilling creates a billing record for a new organization with the default pricing plan
func (s *organizationService) provisionBilling(ctx context.Context, orgID uuid.UUID) error {
	// Look up the default pricing plan (dynamically, not hardcoded)
	defaultPlan, err := s.planRepo.GetDefault(ctx)
	if err != nil {
		return fmt.Errorf("get default pricing plan: %w", err)
	}

	now := time.Now()
	billingRecord := &billingDomain.OrganizationBilling{
		OrganizationID:        orgID,
		PlanID:                defaultPlan.ID,
		BillingCycleStart:     now,
		BillingCycleAnchorDay: 1,
		// Free tier remaining (from default plan)
		FreeSpansRemaining:  defaultPlan.FreeSpans,
		FreeBytesRemaining:  defaultPlan.FreeGB.Mul(decimal.NewFromInt(1024 * 1024 * 1024)).IntPart(), // Convert GB to bytes
		FreeScoresRemaining: defaultPlan.FreeScores,
		CurrentPeriodSpans:  0,
		CurrentPeriodBytes:  0,
		CurrentPeriodScores: 0,
		CurrentPeriodCost:   decimal.Zero,
		LastSyncedAt:        now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	if err := s.billingRepo.Create(ctx, billingRecord); err != nil {
		return fmt.Errorf("create billing record: %w", err)
	}

	s.logger.Info("provisioned billing for organization",
		"organization_id", orgID,
		"pricing_plan", defaultPlan.Name,
	)

	return nil
}

func (s *organizationService) GetOrganization(ctx context.Context, orgID uuid.UUID) (*orgDomain.Organization, error) {
	return s.orgRepo.GetByID(ctx, orgID)
}

func (s *organizationService) GetOrganizationBySlug(ctx context.Context, slug string) (*orgDomain.Organization, error) {
	return s.orgRepo.GetBySlug(ctx, slug)
}

func (s *organizationService) UpdateOrganization(ctx context.Context, orgID uuid.UUID, req *orgDomain.UpdateOrganizationRequest) error {
	org, err := s.orgRepo.GetByID(ctx, orgID)
	if err != nil {
		if errors.Is(err, orgDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("Organization")
		}
		return appErrors.NewInternalError("Failed to get organization", err)
	}

	if req.Name != nil {
		org.Name = *req.Name
	}
	if req.BillingEmail != nil {
		org.BillingEmail = *req.BillingEmail
	}
	if req.Plan != nil {
		org.Plan = *req.Plan
	}

	org.UpdatedAt = time.Now()

	err = s.orgRepo.Update(ctx, org)
	if err != nil {
		return appErrors.NewInternalError("Failed to update organization", err)
	}

	return nil
}

func (s *organizationService) DeleteOrganization(ctx context.Context, orgID uuid.UUID) error {
	// Verify organization exists before deletion
	_, err := s.orgRepo.GetByID(ctx, orgID)
	if err != nil {
		if errors.Is(err, orgDomain.ErrNotFound) {
			return appErrors.NewNotFoundError("Organization")
		}
		return appErrors.NewInternalError("Failed to get organization", err)
	}

	err = s.orgRepo.Delete(ctx, orgID)
	if err != nil {
		return appErrors.NewInternalError("Failed to delete organization", err)
	}

	return nil
}

func (s *organizationService) ListOrganizations(ctx context.Context, filters *orgDomain.OrganizationFilters) ([]*orgDomain.Organization, error) {
	return s.orgRepo.List(ctx, filters)
}

func (s *organizationService) GetUserOrganizations(ctx context.Context, userID uuid.UUID) ([]*orgDomain.Organization, error) {
	return s.orgRepo.GetOrganizationsByUserID(ctx, userID)
}

func (s *organizationService) GetUserDefaultOrganization(ctx context.Context, userID uuid.UUID) (*orgDomain.Organization, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, userDomain.ErrNotFound) {
			return nil, appErrors.NewNotFoundError("User")
		}
		return nil, appErrors.NewInternalError("Failed to get user", err)
	}

	if user.DefaultOrganizationID == nil {
		return nil, appErrors.NewNotFoundError("User has no default organization")
	}

	return s.orgRepo.GetByID(ctx, *user.DefaultOrganizationID)
}

func (s *organizationService) SetUserDefaultOrganization(ctx context.Context, userID, orgID uuid.UUID) error {
	// Verify user is member of organization using member service
	isMember, err := s.memberSvc.IsMember(ctx, userID, orgID)
	if err != nil {
		return appErrors.NewInternalError("Failed to check membership", err)
	}
	if !isMember {
		return appErrors.NewForbiddenError("User is not a member of this organization")
	}

	return s.userRepo.SetDefaultOrganization(ctx, userID, orgID)
}

func (s *organizationService) GetUserOrganizationsWithProjects(
	ctx context.Context,
	userID uuid.UUID,
) ([]*orgDomain.OrganizationWithProjectsAndRole, error) {
	return s.orgRepo.GetUserOrganizationsWithProjectsBatch(ctx, userID)
}
