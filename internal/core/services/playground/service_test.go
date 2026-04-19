package playground

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	playgroundDomain "brokle/internal/core/domain/playground"
	appErrors "brokle/pkg/errors"
	"brokle/pkg/uid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ============================================================================
// Mock Repository (Full Interface Implementation)
// ============================================================================

type MockSessionRepository struct {
	mock.Mock
}

func (m *MockSessionRepository) Create(ctx context.Context, session *playgroundDomain.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockSessionRepository) GetByID(ctx context.Context, id uuid.UUID) (*playgroundDomain.Session, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*playgroundDomain.Session), args.Error(1)
}

func (m *MockSessionRepository) List(ctx context.Context, projectID uuid.UUID, limit int) ([]*playgroundDomain.Session, error) {
	args := m.Called(ctx, projectID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*playgroundDomain.Session), args.Error(1)
}

func (m *MockSessionRepository) ListByTags(ctx context.Context, projectID uuid.UUID, tags []string, limit int) ([]*playgroundDomain.Session, error) {
	args := m.Called(ctx, projectID, tags, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*playgroundDomain.Session), args.Error(1)
}

func (m *MockSessionRepository) Update(ctx context.Context, session *playgroundDomain.Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockSessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *MockSessionRepository) UpdateLastRun(ctx context.Context, id uuid.UUID, lastRun playgroundDomain.JSON) error {
	args := m.Called(ctx, id, lastRun)
	return args.Error(0)
}

func (m *MockSessionRepository) UpdateWindows(ctx context.Context, id uuid.UUID, windows playgroundDomain.JSON) error {
	args := m.Called(ctx, id, windows)
	return args.Error(0)
}

func (m *MockSessionRepository) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *MockSessionRepository) ExistsByProjectID(ctx context.Context, sessionID uuid.UUID, projectID uuid.UUID) (bool, error) {
	args := m.Called(ctx, sessionID, projectID)
	return args.Bool(0), args.Error(1)
}

// ============================================================================
// Test Helpers
// ============================================================================

func assertAppErrorType(t *testing.T, err error, expectedType appErrors.ErrorType) {
	t.Helper()
	appErr, ok := appErrors.IsAppError(err)
	assert.True(t, ok, "expected AppError but got: %v", err)
	if ok {
		assert.Equal(t, expectedType, appErr.Type, "expected error type %s but got %s", expectedType, appErr.Type)
	}
}

func stringPtr(s string) *string {
	return &s
}

// ============================================================================
// HIGH-VALUE TESTS: Business Logic - CreateSession
// ============================================================================

func TestPlaygroundService_CreateSession(t *testing.T) {
	projectID := uid.New()
	userID := uid.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name            string
		request         *playgroundDomain.CreateSessionRequest
		mockSetup       func(*MockSessionRepository)
		expectErr       bool
		expectedErrType appErrors.ErrorType
		checkResult     func(*testing.T, *playgroundDomain.SessionResponse)
	}{
		{
			name: "success - creates session with name and windows",
			request: &playgroundDomain.CreateSessionRequest{
				ProjectID:   projectID,
				Name:        "My Test Session",
				Description: stringPtr("A test description"),
				Windows:     json.RawMessage(`[{"template":{"messages":[]},"variables":{}}]`),
				Variables:   json.RawMessage(`{"key":"value"}`),
				Tags:        []string{"test", "demo"},
				CreatedBy:   &userID,
			},
			mockSetup: func(repo *MockSessionRepository) {
				repo.On("Create", mock.Anything, mock.MatchedBy(func(s *playgroundDomain.Session) bool {
					return s.Name != nil && *s.Name == "My Test Session" &&
						s.Description != nil && *s.Description == "A test description"
				})).Return(nil)
			},
			expectErr: false,
			checkResult: func(t *testing.T, resp *playgroundDomain.SessionResponse) {
				assert.NotNil(t, resp)
				assert.Equal(t, "My Test Session", *resp.Name)
				assert.Equal(t, []string{"test", "demo"}, resp.Tags)
			},
		},
		{
			name: "error - empty name",
			request: &playgroundDomain.CreateSessionRequest{
				ProjectID: projectID,
				Name:      "",
				Windows:   json.RawMessage(`[{"template":{"messages":[]},"variables":{}}]`),
			},
			mockSetup:       func(repo *MockSessionRepository) {},
			expectErr:       true,
			expectedErrType: appErrors.TypeValidation,
			checkResult:     nil,
		},
		{
			name: "error - name too long",
			request: &playgroundDomain.CreateSessionRequest{
				ProjectID: projectID,
				Name:      string(make([]byte, 250)), // 250 chars > MaxNameLength (200)
				Windows:   json.RawMessage(`[{"template":{"messages":[]},"variables":{}}]`),
			},
			mockSetup:       func(repo *MockSessionRepository) {},
			expectErr:       true,
			expectedErrType: appErrors.TypeValidation,
			checkResult:     nil,
		},
		{
			name: "error - too many tags",
			request: &playgroundDomain.CreateSessionRequest{
				ProjectID: projectID,
				Name:      "Valid Name",
				Windows:   json.RawMessage(`[{"template":{"messages":[]},"variables":{}}]`),
				Tags:      make([]string, 15), // 15 tags > MaxTagsCount (10)
			},
			mockSetup:       func(repo *MockSessionRepository) {},
			expectErr:       true,
			expectedErrType: appErrors.TypeValidation,
			checkResult:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockSessionRepository)
			tt.mockSetup(mockRepo)

			service := NewPlaygroundService(mockRepo, nil, nil, nil, logger)
			result, err := service.CreateSession(context.Background(), tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				assertAppErrorType(t, err, tt.expectedErrType)
			} else {
				assert.NoError(t, err)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// ============================================================================
// HIGH-VALUE TESTS: Business Logic - UpdateSession
// ============================================================================

func TestPlaygroundService_UpdateSession(t *testing.T) {
	projectID := uid.New()
	sessionID := uid.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name            string
		request         *playgroundDomain.UpdateSessionRequest
		mockSetup       func(*MockSessionRepository)
		expectErr       bool
		expectedErrType appErrors.ErrorType
		checkResult     func(*testing.T, *playgroundDomain.SessionResponse)
	}{
		{
			name: "success - updates session metadata",
			request: &playgroundDomain.UpdateSessionRequest{
				SessionID:   sessionID,
				Name:        stringPtr("Updated Name"),
				Description: stringPtr("Updated description"),
				Tags:        []string{"updated", "tags"},
			},
			mockSetup: func(repo *MockSessionRepository) {
				sessionName := "Original Name"
				session := &playgroundDomain.Session{
					ID:         sessionID,
					ProjectID:  projectID,
					Name:       &sessionName,
					Windows:    playgroundDomain.JSON(`[{"template":{"messages":[]}}]`),
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
					LastUsedAt: time.Now(),
				}
				repo.On("GetByID", mock.Anything, sessionID).Return(session, nil)
				repo.On("Update", mock.Anything, mock.MatchedBy(func(s *playgroundDomain.Session) bool {
					return *s.Name == "Updated Name" && *s.Description == "Updated description"
				})).Return(nil)
			},
			expectErr: false,
			checkResult: func(t *testing.T, resp *playgroundDomain.SessionResponse) {
				assert.NotNil(t, resp)
				assert.Equal(t, "Updated Name", *resp.Name)
				assert.Equal(t, "Updated description", *resp.Description)
			},
		},
		{
			name: "error - session not found",
			request: &playgroundDomain.UpdateSessionRequest{
				SessionID: sessionID,
				Name:      stringPtr("Updated Name"),
			},
			mockSetup: func(repo *MockSessionRepository) {
				repo.On("GetByID", mock.Anything, sessionID).Return(nil, playgroundDomain.ErrSessionNotFound)
			},
			expectErr:       true,
			expectedErrType: appErrors.TypeNotFound,
			checkResult:     nil,
		},
		{
			name: "error - name too long",
			request: &playgroundDomain.UpdateSessionRequest{
				SessionID: sessionID,
				Name:      stringPtr(string(make([]byte, 250))),
			},
			mockSetup: func(repo *MockSessionRepository) {
				sessionName := "Original Name"
				session := &playgroundDomain.Session{
					ID:         sessionID,
					ProjectID:  projectID,
					Name:       &sessionName,
					Windows:    playgroundDomain.JSON(`[{"template":{"messages":[]}}]`),
					CreatedAt:  time.Now(),
					UpdatedAt:  time.Now(),
					LastUsedAt: time.Now(),
				}
				repo.On("GetByID", mock.Anything, sessionID).Return(session, nil)
			},
			expectErr:       true,
			expectedErrType: appErrors.TypeValidation,
			checkResult:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockSessionRepository)
			tt.mockSetup(mockRepo)

			service := NewPlaygroundService(mockRepo, nil, nil, nil, logger)
			result, err := service.UpdateSession(context.Background(), tt.request)

			if tt.expectErr {
				assert.Error(t, err)
				assertAppErrorType(t, err, tt.expectedErrType)
			} else {
				assert.NoError(t, err)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// ============================================================================
// HIGH-VALUE TESTS: Business Logic - ListSessions Limit Clamping
// ============================================================================

func TestPlaygroundService_ListSessions_LimitClamping(t *testing.T) {
	projectID := uid.New()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	tests := []struct {
		name          string
		inputLimit    int
		expectedLimit int
	}{
		{
			name:          "zero limit becomes default 20",
			inputLimit:    0,
			expectedLimit: 20,
		},
		{
			name:          "negative limit becomes default 20",
			inputLimit:    -5,
			expectedLimit: 20,
		},
		{
			name:          "over 100 limit clamped to 100",
			inputLimit:    200,
			expectedLimit: 100,
		},
		{
			name:          "normal limit stays unchanged",
			inputLimit:    50,
			expectedLimit: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockSessionRepository)
			mockRepo.On("List", mock.Anything, projectID, tt.expectedLimit).Return([]*playgroundDomain.Session{}, nil)

			service := NewPlaygroundService(mockRepo, nil, nil, nil, logger)
			_, err := service.ListSessions(context.Background(), &playgroundDomain.ListSessionsRequest{
				ProjectID: projectID,
				Limit:     tt.inputLimit,
			})

			assert.NoError(t, err)
			mockRepo.AssertExpectations(t)
		})
	}
}
