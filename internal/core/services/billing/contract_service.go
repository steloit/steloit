package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"
	"brokle/internal/core/domain/common"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"
)

type contractService struct {
	transactor   common.Transactor
	contractRepo billing.ContractRepository
	tierRepo     billing.VolumeDiscountTierRepository
	historyRepo  billing.ContractHistoryRepository
	billingRepo  billing.OrganizationBillingRepository
	logger       *slog.Logger
}

func NewContractService(
	transactor common.Transactor,
	contractRepo billing.ContractRepository,
	tierRepo billing.VolumeDiscountTierRepository,
	historyRepo billing.ContractHistoryRepository,
	billingRepo billing.OrganizationBillingRepository,
	logger *slog.Logger,
) billing.ContractService {
	return &contractService{
		transactor:   transactor,
		contractRepo: contractRepo,
		tierRepo:     tierRepo,
		historyRepo:  historyRepo,
		billingRepo:  billingRepo,
		logger:       logger,
	}
}

func (s *contractService) CreateContract(ctx context.Context, contract *billing.Contract) error {
	// Validate organization exists
	_, err := s.billingRepo.GetByOrgID(ctx, contract.OrganizationID)
	if err != nil {
		return appErrors.NewNotFoundError(fmt.Sprintf("Organization %s not found", contract.OrganizationID))
	}

	// No normalization - use timestamps as provided
	// Validate: end_date must be at least 1 day after start_date (minimum duration)
	if contract.EndDate != nil {
		minEndDate := contract.StartDate.AddDate(0, 0, 1) // +1 day
		if contract.EndDate.Before(minEndDate) {
			return appErrors.NewValidationError(
				"end_date must be at least 1 day after start_date",
				"Minimum contract duration is 1 day",
			)
		}
	}

	// Set initial status if not set
	if contract.Status == "" {
		contract.Status = billing.ContractStatusDraft
	}

	// Set timestamps
	now := time.Now()
	contract.CreatedAt = now
	contract.UpdatedAt = now

	// Create contract
	if err := s.contractRepo.Create(ctx, contract); err != nil {
		return appErrors.NewInternalError("Failed to create contract", err)
	}

	// Log to audit trail
	s.logContractAction(ctx, contract.ID, billing.ContractActionCreated, contract.CreatedBy, map[string]any{
		"contract_name":   contract.ContractName,
		"organization_id": contract.OrganizationID,
		"status":          contract.Status,
	}, "Contract created")

	s.logger.Info("contract created",
		"contract_id", contract.ID,
		"organization_id", contract.OrganizationID,
		"created_by", contract.CreatedBy,
	)

	return nil
}

func (s *contractService) GetContract(ctx context.Context, contractID uuid.UUID) (*billing.Contract, error) {
	contract, err := s.contractRepo.GetByID(ctx, contractID)
	if err != nil {
		// Check if it's a "not found" error vs database error
		if billing.IsNotFoundError(err) {
			return nil, appErrors.NewNotFoundError(fmt.Sprintf("Contract %s not found", contractID))
		}
		// Wrap real database errors as internal errors
		return nil, appErrors.NewInternalError("Failed to get contract", err)
	}
	return contract, nil
}

func (s *contractService) GetContractsByOrg(ctx context.Context, orgID uuid.UUID) ([]*billing.Contract, error) {
	return s.contractRepo.GetByOrgID(ctx, orgID)
}

func (s *contractService) GetActiveContract(ctx context.Context, orgID uuid.UUID) (*billing.Contract, error) {
	contract, err := s.contractRepo.GetActiveByOrgID(ctx, orgID)
	if err != nil {
		return nil, err // Real database error
	}
	// contract will be nil if no active contract exists (valid state)
	return contract, nil
}

func (s *contractService) UpdateContract(
	ctx context.Context,
	contract *billing.Contract,
) error {
	// Verify contract exists
	existing, err := s.contractRepo.GetByID(ctx, contract.ID)
	if err != nil {
		return appErrors.NewNotFoundError(fmt.Sprintf("Contract %s not found", contract.ID))
	}

	// No normalization - use timestamps as provided
	// Validate: end_date must be at least 1 day after start_date (minimum duration)
	if contract.EndDate != nil {
		minEndDate := contract.StartDate.AddDate(0, 0, 1) // +1 day
		if contract.EndDate.Before(minEndDate) {
			return appErrors.NewValidationError(
				"end_date must be at least 1 day after start_date",
				"Minimum contract duration is 1 day",
			)
		}
	}

	// Track what changed
	changes := s.trackChanges(existing, contract)

	// Update contract
	if err := s.contractRepo.Update(ctx, contract); err != nil {
		return appErrors.NewInternalError("Failed to update contract", err)
	}

	// Log to audit trail if there are changes
	if len(changes) > 0 {
		s.logContractAction(ctx, contract.ID, billing.ContractActionUpdated, "", changes, "Contract updated")
	}

	s.logger.Info("contract updated",
		"contract_id", contract.ID,
		"changes", len(changes),
	)

	return nil
}

func (s *contractService) ActivateContract(ctx context.Context, contractID uuid.UUID, userID uuid.UUID) error {
	// Use transaction for atomic deactivation + activation + history logging
	return s.transactor.WithinTransaction(ctx, func(ctx context.Context) error {
		// 1. Get contract to activate
		contract, err := s.contractRepo.GetByID(ctx, contractID)
		if err != nil {
			return appErrors.NewNotFoundError(fmt.Sprintf("Contract %s not found", contractID))
		}

		// 2. Validate contract is in draft status
		if contract.Status != billing.ContractStatusDraft {
			return appErrors.NewValidationError(
				"Only draft contracts can be activated",
				fmt.Sprintf("Contract status is %s, expected draft", contract.Status),
			)
		}

		// 3. Deactivate any existing active contract (within transaction)
		// NOTE: No manual check - let database enforce uniqueness
		existingContract, _ := s.contractRepo.GetActiveByOrgID(ctx, contract.OrganizationID)
		if existingContract != nil {
			existingContract.Status = billing.ContractStatusExpired
			existingContract.UpdatedAt = time.Now()

			if err := s.contractRepo.Update(ctx, existingContract); err != nil {
				return appErrors.NewInternalError("Failed to expire existing contract", err)
			}

			// Log expiration of old contract
			if err := s.logContractActionTx(ctx, s.historyRepo, existingContract.ID,
				billing.ContractActionExpired, userID.String(),
				"Automatically expired due to new contract activation", nil); err != nil {
				return err
			}

			s.logger.Info("expired existing active contract",
				"old_contract_id", existingContract.ID,
				"new_contract_id", contractID,
				"organization_id", contract.OrganizationID,
			)
		}

		// 5. Activate new contract
		contract.Status = billing.ContractStatusActive
		contract.UpdatedAt = time.Now()

		if err := s.contractRepo.Update(ctx, contract); err != nil {
			// Check for unique constraint violation (race condition caught by database)
			if appErrors.IsUniqueViolation(err) {
				return appErrors.NewConflictError("Another contract was activated concurrently for this organization")
			}
			return appErrors.NewInternalError("Failed to activate contract", err)
		}

		// 6. Log activation to audit trail
		if err := s.logContractActionTx(ctx, s.historyRepo, contractID,
			billing.ContractActionUpdated, userID.String(),
			"Contract activated", nil); err != nil {
			return err
		}

		s.logger.Info("contract activated",
			"contract_id", contractID,
			"organization_id", contract.OrganizationID,
			"user_id", userID,
		)

		return nil
	})
}

func (s *contractService) CancelContract(ctx context.Context, contractID uuid.UUID, reason string, userID uuid.UUID) error {
	contract, err := s.contractRepo.GetByID(ctx, contractID)
	if err != nil {
		return appErrors.NewNotFoundError(fmt.Sprintf("Contract %s not found", contractID))
	}

	if contract.Status == billing.ContractStatusCancelled {
		return appErrors.NewValidationError("Contract is already cancelled", "status")
	}

	if err := s.contractRepo.Cancel(ctx, contractID); err != nil {
		return appErrors.NewInternalError("Failed to cancel contract", err)
	}

	s.logContractAction(ctx, contractID, billing.ContractActionCancelled, userID.String(), map[string]any{
		"previous_status": contract.Status,
	}, reason)

	s.logger.Info("contract cancelled",
		"contract_id", contractID,
		"reason", reason,
		"cancelled_by", userID,
	)

	return nil
}

func (s *contractService) ExpireContract(ctx context.Context, contractID uuid.UUID) error {
	// Fetch contract to validate state
	contract, err := s.contractRepo.GetByID(ctx, contractID)
	if err != nil {
		if billing.IsNotFoundError(err) {
			return appErrors.NewNotFoundError(fmt.Sprintf("Contract %s not found", contractID))
		}
		return appErrors.NewInternalError("Failed to get contract for expiration", err)
	}

	// Validate state transition: only active contracts can expire
	if contract.Status != billing.ContractStatusActive {
		return appErrors.NewValidationError(
			"Only active contracts can be expired",
			fmt.Sprintf("Contract status is %s, expected active", contract.Status),
		)
	}

	if err := s.contractRepo.Expire(ctx, contractID); err != nil {
		return appErrors.NewInternalError("Failed to expire contract", err)
	}

	s.logContractAction(ctx, contractID, billing.ContractActionExpired, "system", map[string]any{
		"expired_at": time.Now(),
	}, "Contract expired automatically")

	s.logger.Info("contract expired",
		"contract_id", contractID,
	)

	return nil
}

func (s *contractService) AddVolumeTiers(ctx context.Context, contractID uuid.UUID, tiers []*billing.VolumeDiscountTier) error {
	// 1. Verify contract exists
	_, err := s.contractRepo.GetByID(ctx, contractID)
	if err != nil {
		return appErrors.NewNotFoundError(fmt.Sprintf("Contract %s not found", contractID))
	}

	// 2. Validate tier configuration BEFORE transaction
	if err := validateVolumeTiers(tiers); err != nil {
		return err
	}

	// 3. Use transaction for atomic create + history logging
	return s.transactor.WithinTransaction(ctx, func(ctx context.Context) error {
		// Set tier IDs and timestamps
		now := time.Now()
		for _, tier := range tiers {
			tier.ID = uid.New()
			tier.ContractID = contractID
			tier.CreatedAt = now
		}

		// Create tiers (within transaction)
		if err := s.tierRepo.CreateBatch(ctx, tiers); err != nil {
			return appErrors.NewInternalError("Failed to create volume tiers", err)
		}

		// Log to audit trail (within transaction)
		tierChanges := map[string]any{
			"tier_count": len(tiers),
			"dimensions": extractDimensions(tiers),
		}

		if err := s.logContractActionTx(ctx, s.historyRepo, contractID,
			billing.ContractActionPricingChanged, "",
			"Volume tiers added", tierChanges); err != nil {
			return err
		}

		s.logger.Info("volume tiers added",
			"contract_id", contractID,
			"tier_count", len(tiers),
		)

		return nil
	})
}

func (s *contractService) UpdateVolumeTiers(ctx context.Context, contractID uuid.UUID, tiers []*billing.VolumeDiscountTier) error {
	// 1. Verify contract exists
	_, err := s.contractRepo.GetByID(ctx, contractID)
	if err != nil {
		return appErrors.NewNotFoundError(fmt.Sprintf("Contract %s not found", contractID))
	}

	// 2. Validate tier configuration BEFORE transaction
	if err := validateVolumeTiers(tiers); err != nil {
		return err
	}

	// 3. Use transaction for atomic delete + create
	return s.transactor.WithinTransaction(ctx, func(ctx context.Context) error {
		// Delete existing tiers (within transaction)
		if err := s.tierRepo.DeleteByContractID(ctx, contractID); err != nil {
			return appErrors.NewInternalError("Failed to delete existing tiers", err)
		}

		// Create new tiers (within transaction)
		if len(tiers) > 0 {
			now := time.Now()
			for _, tier := range tiers {
				tier.ID = uid.New()
				tier.ContractID = contractID
				tier.CreatedAt = now
			}

			if err := s.tierRepo.CreateBatch(ctx, tiers); err != nil {
				return appErrors.NewInternalError("Failed to create volume tiers", err)
			}
		}

		// Log to audit trail (within transaction)
		tierChanges := map[string]any{
			"tier_count": len(tiers),
			"dimensions": extractDimensions(tiers),
		}

		if err := s.logContractActionTx(ctx, s.historyRepo, contractID,
			billing.ContractActionPricingChanged, "",
			"Volume tiers updated", tierChanges); err != nil {
			return err
		}

		s.logger.Info("volume tiers updated",
			"contract_id", contractID,
			"tier_count", len(tiers),
		)

		return nil
	})
}

func (s *contractService) GetContractHistory(ctx context.Context, contractID uuid.UUID) ([]*billing.ContractHistory, error) {
	return s.historyRepo.GetByContractID(ctx, contractID)
}

func (s *contractService) GetExpiringContracts(ctx context.Context, days int) ([]*billing.Contract, error) {
	return s.contractRepo.GetExpiring(ctx, days)
}

// Helper methods

func (s *contractService) logContractAction(ctx context.Context, contractID uuid.UUID, action billing.ContractAction, changedBy string, changes map[string]any, reason string) {
	changesJSON, _ := json.Marshal(changes)

	history := &billing.ContractHistory{
		ID:         uid.New(),
		ContractID: contractID,
		Action:     action,
		ChangedBy:  changedBy,
		ChangedAt:  time.Now(),
		Changes:    changesJSON,
		Reason:     reason,
	}

	if err := s.historyRepo.Log(ctx, history); err != nil {
		s.logger.Error("failed to log contract history",
			"contract_id", contractID,
			"action", action,
			"error", err,
		)
	}
}

func (s *contractService) trackChanges(old, new *billing.Contract) map[string]any {
	changes := make(map[string]any)

	if old.ContractName != new.ContractName {
		changes["contract_name"] = map[string]string{"old": old.ContractName, "new": new.ContractName}
	}
	if old.Status != new.Status {
		changes["status"] = map[string]string{"old": string(old.Status), "new": string(new.Status)}
	}
	if !old.StartDate.Equal(new.StartDate) {
		changes["start_date"] = map[string]time.Time{"old": old.StartDate, "new": new.StartDate}
	}
	if !equalTimePtr(old.EndDate, new.EndDate) {
		changes["end_date"] = map[string]any{
			"old": old.EndDate,
			"new": new.EndDate,
		}
	}

	// Track pricing changes
	if !equalDecimalPtr(old.CustomPricePer100KSpans, new.CustomPricePer100KSpans) {
		changes["price_per_100k_spans"] = map[string]any{
			"old": old.CustomPricePer100KSpans,
			"new": new.CustomPricePer100KSpans,
		}
	}
	if !equalDecimalPtr(old.CustomPricePerGB, new.CustomPricePerGB) {
		changes["price_per_gb"] = map[string]any{
			"old": old.CustomPricePerGB,
			"new": new.CustomPricePerGB,
		}
	}

	return changes
}

func equalDecimalPtr(a, b *decimal.Decimal) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func equalTimePtr(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// validateVolumeTiers ensures tier configuration is valid for billing correctness
func validateVolumeTiers(tiers []*billing.VolumeDiscountTier) error {
	if len(tiers) == 0 {
		return nil
	}

	// Group by dimension
	tiersByDimension := make(map[billing.TierDimension][]*billing.VolumeDiscountTier)
	for _, tier := range tiers {
		tiersByDimension[tier.Dimension] = append(tiersByDimension[tier.Dimension], tier)
	}

	// Validate each dimension independently
	for dimension, dimensionTiers := range tiersByDimension {
		if err := validateDimensionTiers(dimension, dimensionTiers); err != nil {
			return err
		}
	}
	return nil
}

// validateDimensionTiers validates tier configuration for a single dimension
func validateDimensionTiers(dimension billing.TierDimension, tiers []*billing.VolumeDiscountTier) error {
	if len(tiers) == 0 {
		return nil
	}

	// Sort by TierMin
	sort.Slice(tiers, func(i, j int) bool {
		return tiers[i].TierMin < tiers[j].TierMin
	})

	// Rule 1: First tier must start at 0
	if tiers[0].TierMin != 0 {
		return appErrors.NewValidationError(
			fmt.Sprintf("First tier for dimension %s must have TierMin=0, got %d", dimension, tiers[0].TierMin),
			"tier_min_must_be_zero",
		)
	}

	// Rule 2: Tiers must be contiguous
	for i := 1; i < len(tiers); i++ {
		prevTier := tiers[i-1]
		currentTier := tiers[i]

		if prevTier.TierMax == nil {
			return appErrors.NewValidationError(
				fmt.Sprintf("Tier %d for dimension %s has unlimited TierMax but tier %d follows it", i-1, dimension, i),
				"unlimited_tier_not_last",
			)
		}

		if currentTier.TierMin != *prevTier.TierMax {
			return appErrors.NewValidationError(
				fmt.Sprintf("Gap detected in dimension %s: tier %d ends at %d, tier %d starts at %d",
					dimension, i-1, *prevTier.TierMax, i, currentTier.TierMin),
				"tier_gap_detected",
			)
		}
	}

	// Rule 3: Validate tier ranges
	for i, tier := range tiers {
		if tier.TierMax != nil && *tier.TierMax <= tier.TierMin {
			return appErrors.NewValidationError(
				fmt.Sprintf("Tier %d for dimension %s has invalid range: TierMax (%d) <= TierMin (%d)",
					i, dimension, *tier.TierMax, tier.TierMin),
				"invalid_tier_range",
			)
		}
	}

	return nil
}

// logContractActionTx logs contract action within a transaction
func (s *contractService) logContractActionTx(
	ctx context.Context,
	historyRepo billing.ContractHistoryRepository,
	contractID uuid.UUID,
	action billing.ContractAction,
	changedBy string,
	reason string,
	changes map[string]any,
) error {
	changesJSON, _ := json.Marshal(changes)

	history := &billing.ContractHistory{
		ID:         uid.New(),
		ContractID: contractID,
		Action:     action,
		ChangedBy:  changedBy,
		ChangedAt:  time.Now(),
		Changes:    json.RawMessage(changesJSON),
		Reason:     reason,
	}

	if err := historyRepo.Log(ctx, history); err != nil {
		return appErrors.NewInternalError("Failed to log contract history", err)
	}

	return nil
}

// extractDimensions returns list of dimensions present in tier set
func extractDimensions(tiers []*billing.VolumeDiscountTier) []string {
	dimensionMap := make(map[billing.TierDimension]bool)
	for _, tier := range tiers {
		dimensionMap[tier.Dimension] = true
	}

	dimensions := make([]string, 0, len(dimensionMap))
	for dim := range dimensionMap {
		dimensions = append(dimensions, string(dim))
	}
	sort.Strings(dimensions)
	return dimensions
}
