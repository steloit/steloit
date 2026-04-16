package billing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"brokle/internal/core/domain/billing"
	"brokle/pkg/uid"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	// Auto-migrate the Contract and VolumeDiscountTier models
	err = db.AutoMigrate(&billing.Contract{}, &billing.VolumeDiscountTier{})
	require.NoError(t, err)

	return db
}

func TestContractRepository_GetExpiring_Today(t *testing.T) {
	db := setupTestDB(t)
	repo := NewContractRepository(db)
	ctx := context.Background()

	// Setup: Create contract that expired 1 hour ago (timestamp-based)
	now := time.Now().UTC()
	oneHourAgo := now.Add(-1 * time.Hour)

	contract := &billing.Contract{
		ID:             uid.New(),
		OrganizationID: uid.New(),
		ContractName:   "Test Contract Already Expired",
		ContractNumber: "TEST-001",
		Status:         billing.ContractStatusActive,
		StartDate:      oneHourAgo.AddDate(0, -1, 0), // Started 1 month ago
		EndDate:        &oneHourAgo,                  // Expired 1 hour ago
		Currency:       "USD",
	}

	// Save contract
	err := repo.Create(ctx, contract)
	require.NoError(t, err)

	// Test: GetExpiring with days=0 should find contracts expired up to now
	contracts, err := repo.GetExpiring(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, contracts, 1, "Should find contract that expired 1 hour ago")
	assert.Equal(t, contract.ID, contracts[0].ID)
}

func TestContractRepository_GetExpiring_Tomorrow(t *testing.T) {
	db := setupTestDB(t)
	repo := NewContractRepository(db)
	ctx := context.Background()

	// Setup: Create contract expiring in 2 hours (future timestamp)
	now := time.Now().UTC()
	twoHoursFromNow := now.Add(2 * time.Hour)

	contract := &billing.Contract{
		ID:             uid.New(),
		OrganizationID: uid.New(),
		ContractName:   "Test Contract Expiring Soon",
		ContractNumber: "TEST-003",
		Status:         billing.ContractStatusActive,
		StartDate:      twoHoursFromNow.AddDate(0, -1, 0),
		EndDate:        &twoHoursFromNow, // Expires in 2 hours
		Currency:       "USD",
	}

	err := repo.Create(ctx, contract)
	require.NoError(t, err)

	// Test: GetExpiring with days=0 should NOT find future contracts
	contracts, err := repo.GetExpiring(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, contracts, 0, "Should NOT find contract expiring in 2 hours (not expired yet)")

	// Test: GetExpiring with days=1 should include contracts expiring within 24 hours
	contracts, err = repo.GetExpiring(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, contracts, 1, "Should find contract expiring within 24 hours")
	assert.Equal(t, contract.ID, contracts[0].ID)
}

func TestContractRepository_GetExpiring_TimestampPrecision(t *testing.T) {
	db := setupTestDB(t)
	repo := NewContractRepository(db)
	ctx := context.Background()

	// Create 3 contracts with different expiration times:
	// 1. Expired yesterday at 14:00
	// 2. Expired 1 hour ago
	// 3. Expires in 2 hours

	now := time.Now().UTC()
	yesterday := now.Add(-24 * time.Hour).Add(-10 * time.Hour) // 34 hours ago
	oneHourAgo := now.Add(-1 * time.Hour)
	twoHoursFromNow := now.Add(2 * time.Hour)

	contract1 := &billing.Contract{
		ID:             uid.New(),
		OrganizationID: uid.New(),
		ContractName:   "Contract Expired Yesterday",
		ContractNumber: "PREC-001",
		Status:         billing.ContractStatusActive,
		StartDate:      yesterday.AddDate(0, -1, 0),
		EndDate:        &yesterday,
		Currency:       "USD",
	}

	contract2 := &billing.Contract{
		ID:             uid.New(),
		OrganizationID: uid.New(),
		ContractName:   "Contract Expired 1 Hour Ago",
		ContractNumber: "PREC-002",
		Status:         billing.ContractStatusActive,
		StartDate:      oneHourAgo.AddDate(0, -1, 0),
		EndDate:        &oneHourAgo,
		Currency:       "USD",
	}

	contract3 := &billing.Contract{
		ID:             uid.New(),
		OrganizationID: uid.New(),
		ContractName:   "Contract Expires in 2 Hours",
		ContractNumber: "PREC-003",
		Status:         billing.ContractStatusActive,
		StartDate:      twoHoursFromNow.AddDate(0, -1, 0),
		EndDate:        &twoHoursFromNow,
		Currency:       "USD",
	}

	// Save all contracts
	require.NoError(t, repo.Create(ctx, contract1))
	require.NoError(t, repo.Create(ctx, contract2))
	require.NoError(t, repo.Create(ctx, contract3))

	// Test: GetExpiring(0) should find first two contracts (already expired)
	contracts, err := repo.GetExpiring(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, contracts, 2, "Should find 2 contracts that have already expired")

	// Verify they're ordered by end_date ASC
	assert.Equal(t, contract1.ID, contracts[0].ID, "First should be contract expired yesterday")
	assert.Equal(t, contract2.ID, contracts[1].ID, "Second should be contract expired 1 hour ago")

	// Test: GetExpiring(1) should find all three (within 24 hours from now)
	contracts, err = repo.GetExpiring(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, contracts, 3, "Should find all 3 contracts (within 24 hours)")
}
