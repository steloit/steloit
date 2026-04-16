package billing

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/shopspring/decimal"

	"github.com/google/uuid"

	"brokle/internal/core/domain/billing"

	"github.com/stretchr/testify/mock"
)

// Shared mock repositories for all billing service tests

type MockOrganizationBillingRepository struct {
	mock.Mock
}

func (m *MockOrganizationBillingRepository) Create(ctx context.Context, orgBilling *billing.OrganizationBilling) error {
	args := m.Called(ctx, orgBilling)
	return args.Error(0)
}

func (m *MockOrganizationBillingRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID) (*billing.OrganizationBilling, error) {
	args := m.Called(ctx, orgID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billing.OrganizationBilling), args.Error(1)
}

func (m *MockOrganizationBillingRepository) Update(ctx context.Context, orgBilling *billing.OrganizationBilling) error {
	args := m.Called(ctx, orgBilling)
	return args.Error(0)
}

func (m *MockOrganizationBillingRepository) SetUsage(ctx context.Context, orgID uuid.UUID, spans, bytes, scores int64, cost decimal.Decimal, freeSpansRemaining, freeBytesRemaining, freeScoresRemaining int64) error {
	args := m.Called(ctx, orgID, spans, bytes, scores, cost, freeSpansRemaining, freeBytesRemaining, freeScoresRemaining)
	return args.Error(0)
}

func (m *MockOrganizationBillingRepository) ResetPeriod(ctx context.Context, orgID uuid.UUID, newCycleStart time.Time) error {
	args := m.Called(ctx, orgID, newCycleStart)
	return args.Error(0)
}

type MockPlanRepository struct {
	mock.Mock
}

func (m *MockPlanRepository) GetByID(ctx context.Context, id uuid.UUID) (*billing.Plan, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billing.Plan), args.Error(1)
}

func (m *MockPlanRepository) GetByName(ctx context.Context, name string) (*billing.Plan, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billing.Plan), args.Error(1)
}

func (m *MockPlanRepository) GetDefault(ctx context.Context) (*billing.Plan, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billing.Plan), args.Error(1)
}

func (m *MockPlanRepository) GetActive(ctx context.Context) ([]*billing.Plan, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*billing.Plan), args.Error(1)
}

func (m *MockPlanRepository) Create(ctx context.Context, plan *billing.Plan) error {
	args := m.Called(ctx, plan)
	return args.Error(0)
}

func (m *MockPlanRepository) Update(ctx context.Context, plan *billing.Plan) error {
	args := m.Called(ctx, plan)
	return args.Error(0)
}

type MockContractRepository struct {
	mock.Mock
}

func (m *MockContractRepository) Create(ctx context.Context, contract *billing.Contract) error {
	args := m.Called(ctx, contract)
	return args.Error(0)
}

func (m *MockContractRepository) GetByID(ctx context.Context, id uuid.UUID) (*billing.Contract, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billing.Contract), args.Error(1)
}

func (m *MockContractRepository) GetActiveByOrgID(ctx context.Context, orgID uuid.UUID) (*billing.Contract, error) {
	args := m.Called(ctx, orgID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*billing.Contract), args.Error(1)
}

func (m *MockContractRepository) GetByOrgID(ctx context.Context, orgID uuid.UUID) ([]*billing.Contract, error) {
	args := m.Called(ctx, orgID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*billing.Contract), args.Error(1)
}

func (m *MockContractRepository) Update(ctx context.Context, contract *billing.Contract) error {
	args := m.Called(ctx, contract)
	return args.Error(0)
}

func (m *MockContractRepository) Expire(ctx context.Context, contractID uuid.UUID) error {
	args := m.Called(ctx, contractID)
	return args.Error(0)
}

func (m *MockContractRepository) Cancel(ctx context.Context, contractID uuid.UUID) error {
	args := m.Called(ctx, contractID)
	return args.Error(0)
}

func (m *MockContractRepository) GetExpiring(ctx context.Context, days int) ([]*billing.Contract, error) {
	args := m.Called(ctx, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*billing.Contract), args.Error(1)
}

type MockVolumeDiscountTierRepository struct {
	mock.Mock
}

func (m *MockVolumeDiscountTierRepository) Create(ctx context.Context, tier *billing.VolumeDiscountTier) error {
	args := m.Called(ctx, tier)
	return args.Error(0)
}

func (m *MockVolumeDiscountTierRepository) CreateBatch(ctx context.Context, tiers []*billing.VolumeDiscountTier) error {
	args := m.Called(ctx, tiers)
	return args.Error(0)
}

func (m *MockVolumeDiscountTierRepository) GetByContractID(ctx context.Context, contractID uuid.UUID) ([]*billing.VolumeDiscountTier, error) {
	args := m.Called(ctx, contractID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*billing.VolumeDiscountTier), args.Error(1)
}

func (m *MockVolumeDiscountTierRepository) DeleteByContractID(ctx context.Context, contractID uuid.UUID) error {
	args := m.Called(ctx, contractID)
	return args.Error(0)
}

type MockContractHistoryRepository struct {
	mock.Mock
}

func (m *MockContractHistoryRepository) Log(ctx context.Context, history *billing.ContractHistory) error {
	args := m.Called(ctx, history)
	return args.Error(0)
}

func (m *MockContractHistoryRepository) GetByContractID(ctx context.Context, contractID uuid.UUID) ([]*billing.ContractHistory, error) {
	args := m.Called(ctx, contractID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*billing.ContractHistory), args.Error(1)
}

// Shared test helper functions

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Only show errors in tests
	}))
}

func ptrFloat64(v float64) *float64 {
	return &v
}

func ptrInt64(v int64) *int64 {
	return &v
}

func strPtr(s string) *string {
	return &s
}

// parseTimePtr parses RFC3339 time string to pointer
func parseTimePtr(s string) *time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("Failed to parse time " + s + ": " + err.Error())
	}
	return &t
}

// MockTransactor is a simplified mock implementation of common.Transactor
type MockTransactor struct {
	mock.Mock
}

// NewMockTransactor creates a new mock transactor with default behavior
func NewMockTransactor() *MockTransactor {
	m := &MockTransactor{}
	// Default: execute function directly without error
	m.On("WithinTransaction", mock.Anything, mock.Anything).Return(nil).Maybe()
	return m
}

// WithinTransaction executes the function directly (no actual transaction in tests)
func (m *MockTransactor) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	args := m.Called(ctx, fn)

	// If the function is provided, execute it
	if fn != nil {
		if err := fn(ctx); err != nil {
			return err
		}
	}

	return args.Error(0)
}
