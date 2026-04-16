package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"brokle/internal/core/domain/billing"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/pointers"
	"brokle/pkg/uid"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/datatypes"
)

// Tests use shared mocks from mocks_test.go

// Test: Create contract with audit trail
func TestContractService_CreateContract_Success(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	orgID := uid.New()
	startDate := time.Now()
	endDate := startDate.AddDate(0, 12, 0) // +12 months (handles leap years)

	contract := &billing.Contract{
		ID:                      uid.New(),
		OrganizationID:          orgID,
		ContractName:            "Enterprise Annual 2025",
		ContractNumber:          "ENT-2025-001",
		StartDate:               startDate,
		EndDate:                 &endDate,
		MinimumCommitAmount:     pointers.PtrDecimal(decimal.NewFromFloat(50000.0)),
		Currency:                "USD",
		AccountOwner:            "John Smith",
		SalesRepEmail:           "sales@example.com",
		CustomFreeSpans:         ptrInt64(50000000),
		CustomPricePer100KSpans: pointers.PtrDecimal(decimal.NewFromFloat(0.25)),
		Notes:                   "Annual enterprise contract",
		Status:                  "",
		CreatedBy:               uid.New().String(),
	}

	// Mock organization exists
	billingRepo.On("GetByOrgID", ctx, orgID).Return(&billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         uid.New(),
	}, nil)

	// Mock contract creation
	contractRepo.On("Create", ctx, mock.AnythingOfType("*billing.Contract")).Return(nil)

	// Mock audit log
	historyRepo.On("Log", ctx, mock.AnythingOfType("*billing.ContractHistory")).Return(nil)

	// Execute
	err := service.CreateContract(ctx, contract)

	// Assert
	assert.NoError(t, err)

	billingRepo.AssertExpectations(t)
	contractRepo.AssertExpectations(t)
	historyRepo.AssertExpectations(t)
}

// Test: Activate contract (only one active per org)
func TestContractService_ActivateContract_Success(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()
	orgID := uid.New()
	userID := uid.New()

	// Draft contract to activate
	draftContract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusDraft,
		ContractName:   "New Contract",
	}

	// Existing active contract (should be expired)
	oldContractID := uid.New()
	oldContract := &billing.Contract{
		ID:             oldContractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
		ContractName:   "Old Contract",
	}

	// Mock get draft contract
	contractRepo.On("GetByID", ctx, contractID).Return(draftContract, nil)

	// Mock get existing active contract
	contractRepo.On("GetActiveByOrgID", ctx, orgID).Return(oldContract, nil)

	// Mock update old contract to expired
	contractRepo.On("Update", ctx, mock.MatchedBy(func(c *billing.Contract) bool {
		return c.ID == oldContractID && c.Status == billing.ContractStatusExpired
	})).Return(nil)

	// Mock update new contract to active
	contractRepo.On("Update", ctx, mock.MatchedBy(func(c *billing.Contract) bool {
		return c.ID == contractID && c.Status == billing.ContractStatusActive
	})).Return(nil)

	// Mock audit logs (2 logs: expired old, activated new)
	historyRepo.On("Log", ctx, mock.AnythingOfType("*billing.ContractHistory")).Return(nil).Twice()

	// Execute
	err := service.ActivateContract(ctx, contractID, userID)

	// Assert
	assert.NoError(t, err)

	contractRepo.AssertExpectations(t)
	historyRepo.AssertExpectations(t)
}

// Test: Activate contract fails if not in draft status
func TestContractService_ActivateContract_NotDraft(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()
	userID := uid.New()

	// Already active contract
	activeContract := &billing.Contract{
		ID:     contractID,
		Status: billing.ContractStatusActive,
	}

	contractRepo.On("GetByID", ctx, contractID).Return(activeContract, nil)

	// Execute
	err := service.ActivateContract(ctx, contractID, userID)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Only draft contracts can be activated")

	contractRepo.AssertExpectations(t)
}

// Test: Update contract with change tracking
func TestContractService_UpdateContract_Success(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()

	existingContract := &billing.Contract{
		ID:                  contractID,
		Status:              billing.ContractStatusDraft,
		ContractName:        "Old Name",
		AccountOwner:        "Old Owner",
		SalesRepEmail:       "old@example.com",
		MinimumCommitAmount: pointers.PtrDecimal(decimal.NewFromFloat(10000.0)),
	}

	updatedContract := &billing.Contract{
		ID:                  contractID,
		Status:              billing.ContractStatusDraft,
		ContractName:        "New Name",
		AccountOwner:        "New Owner",
		SalesRepEmail:       "new@example.com",
		MinimumCommitAmount: pointers.PtrDecimal(decimal.NewFromFloat(20000.0)),
		Notes:               "Updated terms",
	}

	contractRepo.On("GetByID", ctx, contractID).Return(existingContract, nil)
	contractRepo.On("Update", ctx, mock.AnythingOfType("*billing.Contract")).Return(nil)
	historyRepo.On("Log", ctx, mock.AnythingOfType("*billing.ContractHistory")).Return(nil)

	// Execute
	err := service.UpdateContract(ctx, updatedContract)

	// Assert
	assert.NoError(t, err)

	contractRepo.AssertExpectations(t)
	historyRepo.AssertExpectations(t)
}

// Test: Cancel contract with reason
func TestContractService_CancelContract_Success(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()
	userID := uid.New()
	reason := "Customer requested cancellation"

	activeContract := &billing.Contract{
		ID:     contractID,
		Status: billing.ContractStatusActive,
	}

	contractRepo.On("GetByID", ctx, contractID).Return(activeContract, nil)
	contractRepo.On("Cancel", ctx, contractID).Return(nil)
	historyRepo.On("Log", ctx, mock.AnythingOfType("*billing.ContractHistory")).Return(nil)

	// Execute
	err := service.CancelContract(ctx, contractID, reason, userID)

	// Assert
	assert.NoError(t, err)

	contractRepo.AssertExpectations(t)
	historyRepo.AssertExpectations(t)
}

// Test: Expire contract
func TestContractService_ExpireContract_Success(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()
	orgID := uid.New()

	// Setup: Contract must be in active status to be expired
	activeContract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusActive,
	}

	contractRepo.On("GetByID", ctx, contractID).Return(activeContract, nil)
	contractRepo.On("Expire", ctx, contractID).Return(nil)
	historyRepo.On("Log", ctx, mock.AnythingOfType("*billing.ContractHistory")).Return(nil)

	// Execute
	err := service.ExpireContract(ctx, contractID)

	// Assert
	assert.NoError(t, err)

	contractRepo.AssertExpectations(t)
	historyRepo.AssertExpectations(t)
}

// Test: Expire contract fails when not active
func TestContractService_ExpireContract_NotActive(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()
	orgID := uid.New()

	// Setup: Contract is in draft status (not active)
	draftContract := &billing.Contract{
		ID:             contractID,
		OrganizationID: orgID,
		Status:         billing.ContractStatusDraft,
	}

	contractRepo.On("GetByID", ctx, contractID).Return(draftContract, nil)

	// Execute
	err := service.ExpireContract(ctx, contractID)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Only active contracts can be expired")

	contractRepo.AssertExpectations(t)
}

// Test: Get expiring contracts
func TestContractService_GetExpiringContracts_Success(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	// Contracts expiring in 7 days
	futureDate := time.Now().Add(7 * 24 * time.Hour)
	contracts := []*billing.Contract{
		{
			ID:      uid.New(),
			Status:  billing.ContractStatusActive,
			EndDate: &futureDate,
		},
	}

	contractRepo.On("GetExpiring", ctx, 7).Return(contracts, nil)

	// Execute
	result, err := service.GetExpiringContracts(ctx, 7)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, result, 1)

	contractRepo.AssertExpectations(t)
}

// Test: Update volume tiers
func TestContractService_UpdateVolumeTiers_Success(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()

	contract := &billing.Contract{
		ID:     contractID,
		Status: billing.ContractStatusDraft,
	}

	newTiers := []*billing.VolumeDiscountTier{
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100000000),
			PricePerUnit: decimal.NewFromFloat(0.30),
		},
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100000000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.25),
		},
	}

	contractRepo.On("GetByID", ctx, contractID).Return(contract, nil)
	tierRepo.On("DeleteByContractID", ctx, contractID).Return(nil)
	tierRepo.On("CreateBatch", ctx, mock.AnythingOfType("[]*billing.VolumeDiscountTier")).Return(nil)
	historyRepo.On("Log", ctx, mock.AnythingOfType("*billing.ContractHistory")).Return(nil)

	// Execute
	err := service.UpdateVolumeTiers(ctx, contractID, newTiers)

	// Assert
	assert.NoError(t, err)

	contractRepo.AssertExpectations(t)
	tierRepo.AssertExpectations(t)
	historyRepo.AssertExpectations(t)
}

// Test: Get contract history
func TestContractService_GetContractHistory_Success(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()

	changes := map[string]interface{}{
		"contract_name": "Enterprise Contract",
		"status":        "active",
	}
	changesJSON, _ := json.Marshal(changes)

	history := []*billing.ContractHistory{
		{
			ID:         uid.New(),
			ContractID: contractID,
			Action:     billing.ContractActionCreated,
			ChangedBy:  uid.New().String(),
			ChangedAt:  time.Now(),
			Changes:    datatypes.JSON(changesJSON),
			Reason:     "Initial contract creation",
		},
		{
			ID:         uid.New(),
			ContractID: contractID,
			Action:     billing.ContractActionUpdated,
			ChangedBy:  uid.New().String(),
			ChangedAt:  time.Now().Add(1 * time.Hour),
			Changes:    datatypes.JSON(changesJSON),
			Reason:     "Activated contract",
		},
	}

	historyRepo.On("GetByContractID", ctx, contractID).Return(history, nil)

	// Execute
	result, err := service.GetContractHistory(ctx, contractID)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, billing.ContractActionCreated, result[0].Action)
	assert.Equal(t, billing.ContractActionUpdated, result[1].Action)

	historyRepo.AssertExpectations(t)
}

// Test: Get contract by ID
func TestContractService_GetContract_Success(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()
	contract := &billing.Contract{
		ID:           contractID,
		ContractName: "Test Contract",
		Status:       billing.ContractStatusActive,
	}

	contractRepo.On("GetByID", ctx, contractID).Return(contract, nil)

	// Execute
	result, err := service.GetContract(ctx, contractID)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, contractID, result.ID)
	assert.Equal(t, "Test Contract", result.ContractName)

	contractRepo.AssertExpectations(t)
}

// Test: Get contract not found
func TestContractService_GetContract_NotFound(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()

	contractRepo.On("GetByID", ctx, contractID).Return(nil, appErrors.NewNotFoundError("Contract not found"))

	// Execute
	result, err := service.GetContract(ctx, contractID)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)

	contractRepo.AssertExpectations(t)
}

// New tests for timestamp-based implementation

func TestContractService_CreateContract_TimestampPrecision(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	orgID := uid.New()

	// Verify exact timestamps preserved (not normalized to midnight)
	startsAt := parseTime("2026-01-08T10:15:30.123Z") // Milliseconds preserved
	expiresAt := parseTimePtr("2026-02-08T10:15:30.123Z")

	contract := &billing.Contract{
		ID:             uid.New(),
		OrganizationID: orgID,
		ContractName:   "Timestamp Test",
		ContractNumber: "TS-001",
		StartDate:      startsAt,
		EndDate:        expiresAt,
		Currency:       "USD",
		Status:         billing.ContractStatusDraft,
		CreatedBy:      uid.New().String(),
	}

	// Mock organization exists
	billingRepo.On("GetByOrgID", ctx, orgID).Return(&billing.OrganizationBilling{
		OrganizationID: orgID,
		PlanID:         uid.New(),
	}, nil)

	contractRepo.On("Create", ctx, mock.MatchedBy(func(c *billing.Contract) bool {
		return c.StartDate.Equal(startsAt) && c.EndDate.Equal(*expiresAt)
	})).Return(nil)

	historyRepo.On("Log", ctx, mock.AnythingOfType("*billing.ContractHistory")).Return(nil)

	err := service.CreateContract(ctx, contract)

	assert.NoError(t, err)
	assert.True(t, contract.StartDate.Equal(startsAt), "StartDate must preserve exact timestamp")
	assert.True(t, contract.EndDate.Equal(*expiresAt), "EndDate must preserve exact timestamp")

	contractRepo.AssertExpectations(t)
	billingRepo.AssertExpectations(t)
	historyRepo.AssertExpectations(t)
}

func TestContractService_MonthlyBilling_AddDate(t *testing.T) {
	// This test documents Go's AddDate behavior for monthly billing calculations.
	// IMPORTANT: AddDate normalizes overflow (Jan 31 + 1 month = Mar 3, not Feb 28)
	// This is documented Go behavior: time.AddDate normalizes like time.Date does.
	//
	// For business logic requiring "last day of month" handling, use custom logic:
	//   if start.Day() > targetMonth.LastDay() { use targetMonth.LastDay() }
	//
	// Current implementation uses AddDate for 1-day minimum validation only.

	tests := []struct {
		name     string
		startsAt string
		months   int
		expected string
	}{
		{
			name:     "Regular month",
			startsAt: "2026-01-15T10:15:00Z",
			months:   1,
			expected: "2026-02-15T10:15:00Z",
		},
		{
			name:     "Month-end overflow: Jan 31 → Mar 3 (Go normalizes 31-28=3)",
			startsAt: "2026-01-31T10:15:00Z",
			months:   1,
			expected: "2026-03-03T10:15:00Z", // Not Feb 28 - this is Go's documented behavior
		},
		{
			name:     "Leap year overflow: Jan 31 → Mar 2 (Go normalizes 31-29=2)",
			startsAt: "2024-01-31T10:15:00Z",
			months:   1,
			expected: "2024-03-02T10:15:00Z", // Not Feb 29 - this is Go's documented behavior
		},
		{
			name:     "Year boundary",
			startsAt: "2025-12-15T10:15:00Z",
			months:   1,
			expected: "2026-01-15T10:15:00Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := parseTime(tt.startsAt)
			expected := parseTime(tt.expected)
			result := start.AddDate(0, tt.months, 0)
			assert.True(t, result.Equal(expected),
				"AddDate behavior: got %s, expected %s",
				result.Format(time.RFC3339),
				expected.Format(time.RFC3339))
		})
	}
}

func TestContractService_CreateContract_MinimumDurationValidation(t *testing.T) {
	tests := []struct {
		name        string
		startsAt    string
		expiresAt   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Invalid: 23 hours",
			startsAt:    "2026-01-08T10:15:00Z",
			expiresAt:   "2026-01-09T09:15:00Z",
			expectError: true,
			errorMsg:    "end_date must be at least 1 day after start_date",
		},
		{
			name:        "Valid: exactly 24 hours",
			startsAt:    "2026-01-08T10:15:00Z",
			expiresAt:   "2026-01-09T10:15:00Z",
			expectError: false,
		},
		{
			name:        "Valid: 1 month",
			startsAt:    "2026-01-08T10:15:00Z",
			expiresAt:   "2026-02-08T10:15:00Z",
			expectError: false,
		},
		{
			name:        "Invalid: same second",
			startsAt:    "2026-01-08T10:15:00Z",
			expiresAt:   "2026-01-08T10:15:00Z",
			expectError: true,
			errorMsg:    "end_date must be at least 1 day after start_date",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			logger := newTestLogger()

			contractRepo := new(MockContractRepository)
			tierRepo := new(MockVolumeDiscountTierRepository)
			historyRepo := new(MockContractHistoryRepository)
			billingRepo := new(MockOrganizationBillingRepository)

			transactor := NewMockTransactor()
			service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

			orgID := uid.New()
			startsAt := parseTime(tt.startsAt)
			expiresAt := parseTimePtr(tt.expiresAt)

			contract := &billing.Contract{
				ID:             uid.New(),
				OrganizationID: orgID,
				ContractName:   "Duration Test",
				ContractNumber: "DUR-001",
				StartDate:      startsAt,
				EndDate:        expiresAt,
				Currency:       "USD",
				Status:         billing.ContractStatusDraft,
				CreatedBy:      uid.New().String(),
			}

			// Setup GetByOrgID mock for all cases (called before validation)
			billingRepo.On("GetByOrgID", ctx, orgID).Return(&billing.OrganizationBilling{
				OrganizationID: orgID,
				PlanID:         uid.New(),
			}, nil)

			if tt.expectError {
				// Should fail validation after GetByOrgID check
				err := service.CreateContract(ctx, contract)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				// Should pass validation
				contractRepo.On("Create", ctx, mock.AnythingOfType("*billing.Contract")).Return(nil)
				historyRepo.On("Log", ctx, mock.AnythingOfType("*billing.ContractHistory")).Return(nil)

				err := service.CreateContract(ctx, contract)
				assert.NoError(t, err)

				billingRepo.AssertExpectations(t)
				contractRepo.AssertExpectations(t)
				historyRepo.AssertExpectations(t)
			}
		})
	}
}

// parseTime parses RFC3339 time string to time.Time (non-pointer)
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse time %s: %v", s, err))
	}
	return t
}

// Test: Validation - First tier must start at 0
func TestContractService_AddVolumeTiers_ValidationFirstTierNotZero(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()

	// Mock contract exists
	contractRepo.On("GetByID", ctx, contractID).Return(&billing.Contract{
		ID: contractID,
	}, nil)

	// Invalid: tier starting at 100M (not 0)
	tiers := []*billing.VolumeDiscountTier{
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100_000_000, // ERROR: Should be 0
			TierMax:      ptrInt64(200_000_000),
			PricePerUnit: decimal.NewFromFloat(0.30),
		},
	}

	// Execute
	err := service.AddVolumeTiers(ctx, contractID, tiers)

	// Assert: validation should fail
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must have TierMin=0")

	contractRepo.AssertExpectations(t)
}

// Test: Validation - Gap detection between tiers
func TestContractService_AddVolumeTiers_ValidationGapDetection(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()

	// Mock contract exists
	contractRepo.On("GetByID", ctx, contractID).Return(&billing.Contract{
		ID: contractID,
	}, nil)

	// Invalid: gap between tier 1 and tier 2
	tiers := []*billing.VolumeDiscountTier{
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100_000_000),
			PricePerUnit: decimal.NewFromFloat(0.30),
		},
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      150_000_000, // ERROR: Gap from 100M to 150M
			TierMax:      ptrInt64(200_000_000),
			PricePerUnit: decimal.NewFromFloat(0.25),
		},
	}

	// Execute
	err := service.AddVolumeTiers(ctx, contractID, tiers)

	// Assert: validation should fail
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Gap detected")

	contractRepo.AssertExpectations(t)
}

// Test: Validation - Unlimited tier must be last
func TestContractService_AddVolumeTiers_ValidationUnlimitedNotLast(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()

	// Mock contract exists
	contractRepo.On("GetByID", ctx, contractID).Return(&billing.Contract{
		ID: contractID,
	}, nil)

	// Invalid: unlimited tier followed by another tier
	tiers := []*billing.VolumeDiscountTier{
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      nil, // ERROR: Unlimited but not last
			PricePerUnit: decimal.NewFromFloat(0.30),
		},
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100_000_000,
			TierMax:      ptrInt64(200_000_000),
			PricePerUnit: decimal.NewFromFloat(0.25),
		},
	}

	// Execute
	err := service.AddVolumeTiers(ctx, contractID, tiers)

	// Assert: validation should fail
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unlimited TierMax but tier")

	contractRepo.AssertExpectations(t)
}

// Test: Validation - Multi-dimension validation (one invalid, one valid)
func TestContractService_AddVolumeTiers_MultiDimensionValidation(t *testing.T) {
	ctx := context.Background()
	logger := newTestLogger()

	contractRepo := new(MockContractRepository)
	tierRepo := new(MockVolumeDiscountTierRepository)
	historyRepo := new(MockContractHistoryRepository)
	billingRepo := new(MockOrganizationBillingRepository)

	transactor := NewMockTransactor()
	service := NewContractService(transactor, contractRepo, tierRepo, historyRepo, billingRepo, logger)

	contractID := uid.New()

	// Mock contract exists
	contractRepo.On("GetByID", ctx, contractID).Return(&billing.Contract{
		ID: contractID,
	}, nil)

	// Valid spans tiers, invalid bytes tiers (gap)
	tiers := []*billing.VolumeDiscountTier{
		// Valid spans tiers
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      0,
			TierMax:      ptrInt64(100_000_000),
			PricePerUnit: decimal.NewFromFloat(0.30),
		},
		{
			Dimension:    billing.TierDimensionSpans,
			TierMin:      100_000_000,
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.25),
		},
		// Invalid bytes tiers (gap)
		{
			Dimension:    billing.TierDimensionBytes,
			TierMin:      0,
			TierMax:      ptrInt64(10_737_418_240), // 10 GB
			PricePerUnit: decimal.NewFromFloat(0.50),
		},
		{
			Dimension:    billing.TierDimensionBytes,
			TierMin:      21_474_836_480, // ERROR: Gap from 10GB to 20GB
			TierMax:      nil,
			PricePerUnit: decimal.NewFromFloat(0.40),
		},
	}

	// Execute
	err := service.AddVolumeTiers(ctx, contractID, tiers)

	// Assert: validation should fail for bytes dimension
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bytes")
	assert.Contains(t, err.Error(), "Gap detected")

	contractRepo.AssertExpectations(t)
}
