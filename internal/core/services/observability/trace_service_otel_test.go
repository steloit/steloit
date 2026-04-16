package observability

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"brokle/internal/core/domain/observability"
)

type MockTraceRepository struct {
	mock.Mock
}

func (m *MockTraceRepository) InsertSpan(ctx context.Context, span *observability.Span) error {
	args := m.Called(ctx, span)
	return args.Error(0)
}

func (m *MockTraceRepository) InsertSpanBatch(ctx context.Context, spans []*observability.Span) error {
	args := m.Called(ctx, spans)
	return args.Error(0)
}

func (m *MockTraceRepository) GetSpan(ctx context.Context, spanID string) (*observability.Span, error) {
	args := m.Called(ctx, spanID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*observability.Span), args.Error(1)
}

func (m *MockTraceRepository) GetSpansByTraceID(ctx context.Context, traceID string) ([]*observability.Span, error) {
	args := m.Called(ctx, traceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.Span), args.Error(1)
}

func (m *MockTraceRepository) GetSpanChildren(ctx context.Context, parentSpanID string) ([]*observability.Span, error) {
	args := m.Called(ctx, parentSpanID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.Span), args.Error(1)
}

func (m *MockTraceRepository) GetSpanTree(ctx context.Context, traceID string) ([]*observability.Span, error) {
	args := m.Called(ctx, traceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.Span), args.Error(1)
}

func (m *MockTraceRepository) GetSpansByFilter(ctx context.Context, filter *observability.SpanFilter) ([]*observability.Span, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.Span), args.Error(1)
}

func (m *MockTraceRepository) CountSpansByFilter(ctx context.Context, filter *observability.SpanFilter) (int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockTraceRepository) DeleteSpan(ctx context.Context, spanID string) error {
	args := m.Called(ctx, spanID)
	return args.Error(0)
}

func (m *MockTraceRepository) GetRootSpan(ctx context.Context, traceID string) (*observability.Span, error) {
	args := m.Called(ctx, traceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*observability.Span), args.Error(1)
}

func (m *MockTraceRepository) GetRootSpanByProject(ctx context.Context, traceID string, projectID string) (*observability.Span, error) {
	args := m.Called(ctx, traceID, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*observability.Span), args.Error(1)
}

func (m *MockTraceRepository) GetSpanByProject(ctx context.Context, spanID string, projectID string) (*observability.Span, error) {
	args := m.Called(ctx, spanID, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*observability.Span), args.Error(1)
}

func (m *MockTraceRepository) GetTraceSummary(ctx context.Context, traceID string) (*observability.TraceSummary, error) {
	args := m.Called(ctx, traceID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*observability.TraceSummary), args.Error(1)
}

func (m *MockTraceRepository) ListTraces(ctx context.Context, filter *observability.TraceFilter) ([]*observability.TraceSummary, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.TraceSummary), args.Error(1)
}

func (m *MockTraceRepository) CountTraces(ctx context.Context, filter *observability.TraceFilter) (int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockTraceRepository) GetTracesBySessionID(ctx context.Context, sessionID string) ([]*observability.TraceSummary, error) {
	args := m.Called(ctx, sessionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.TraceSummary), args.Error(1)
}

func (m *MockTraceRepository) GetTracesByUserID(ctx context.Context, userID string, filter *observability.TraceFilter) ([]*observability.TraceSummary, error) {
	args := m.Called(ctx, userID, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.TraceSummary), args.Error(1)
}

func (m *MockTraceRepository) CalculateTotalCost(ctx context.Context, traceID string) (float64, error) {
	args := m.Called(ctx, traceID)
	return args.Get(0).(float64), args.Error(1)
}

func (m *MockTraceRepository) CalculateTotalTokens(ctx context.Context, traceID string) (uint64, error) {
	args := m.Called(ctx, traceID)
	return args.Get(0).(uint64), args.Error(1)
}

func (m *MockTraceRepository) CountSpansInTrace(ctx context.Context, traceID string) (int64, error) {
	args := m.Called(ctx, traceID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockTraceRepository) DeleteTrace(ctx context.Context, traceID string) error {
	args := m.Called(ctx, traceID)
	return args.Error(0)
}

func (m *MockTraceRepository) UpdateTraceTags(ctx context.Context, projectID, traceID string, tags []string) error {
	args := m.Called(ctx, projectID, traceID, tags)
	return args.Error(0)
}

func (m *MockTraceRepository) UpdateTraceBookmark(ctx context.Context, projectID, traceID string, bookmarked bool) error {
	args := m.Called(ctx, projectID, traceID, bookmarked)
	return args.Error(0)
}

func (m *MockTraceRepository) GetFilterOptions(ctx context.Context, projectID string) (*observability.TraceFilterOptions, error) {
	args := m.Called(ctx, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*observability.TraceFilterOptions), args.Error(1)
}

func (m *MockTraceRepository) QuerySpansByExpression(ctx context.Context, query string, queryArgs []interface{}) ([]*observability.Span, error) {
	args := m.Called(ctx, query, queryArgs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.Span), args.Error(1)
}

func (m *MockTraceRepository) CountSpansByExpression(ctx context.Context, query string, queryArgs []interface{}) (int64, error) {
	args := m.Called(ctx, query, queryArgs)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockTraceRepository) DiscoverAttributes(ctx context.Context, req *observability.AttributeDiscoveryRequest) (*observability.AttributeDiscoveryResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*observability.AttributeDiscoveryResponse), args.Error(1)
}

func (m *MockTraceRepository) ListSessions(ctx context.Context, filter *observability.SessionFilter) ([]*observability.SessionSummary, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*observability.SessionSummary), args.Error(1)
}

func (m *MockTraceRepository) CountSessions(ctx context.Context, filter *observability.SessionFilter) (int64, error) {
	args := m.Called(ctx, filter)
	return args.Get(0).(int64), args.Error(1)
}

func TestTraceService_GetRootSpan(t *testing.T) {
	tests := []struct {
		name        string
		traceID     string
		mockSetup   func(*MockTraceRepository)
		expectedErr bool
	}{
		{
			name:    "success - valid trace_id",
			traceID: "12345678901234567890123456789012",
			mockSetup: func(repo *MockTraceRepository) {
				rootSpan := &observability.Span{
					SpanID:       "span123456789012",
					TraceID:      "12345678901234567890123456789012",
					ParentSpanID: nil,
					SpanName:     "root-span",
					ProjectID:    "proj123",
				}
				repo.On("GetRootSpan", mock.Anything, "12345678901234567890123456789012").
					Return(rootSpan, nil)
			},
			expectedErr: false,
		},
		{
			name:        "error - invalid trace_id length",
			traceID:     "invalid",
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockTraceRepository)
			tt.mockSetup(mockRepo)

			service := NewTraceService(mockRepo, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
			result, err := service.GetRootSpan(context.Background(), tt.traceID)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestTraceService_GetTrace(t *testing.T) {
	tests := []struct {
		name        string
		traceID     string
		mockSetup   func(*MockTraceRepository)
		expectedErr bool
		checkResult func(*testing.T, *observability.TraceSummary)
	}{
		{
			name:    "success - valid aggregation",
			traceID: "12345678901234567890123456789012",
			mockSetup: func(repo *MockTraceRepository) {
				summary := &observability.TraceSummary{
					TraceID:     "12345678901234567890123456789012",
					RootSpanID:  "root1234567890123",
					ProjectID:   "proj123",
					TotalCost:   decimal.NewFromFloat(0.05),
					TotalTokens: 1000,
					SpanCount:   5,
					HasError:    false,
				}
				repo.On("GetTraceSummary", mock.Anything, "12345678901234567890123456789012").
					Return(summary, nil)
			},
			expectedErr: false,
			checkResult: func(t *testing.T, summary *observability.TraceSummary) {
				assert.Equal(t, int64(5), summary.SpanCount)
				assert.Equal(t, uint64(1000), summary.TotalTokens)
				assert.True(t, summary.TotalCost.GreaterThan(decimal.Zero))
			},
		},
		{
			name:        "error - invalid trace_id",
			traceID:     "short",
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockTraceRepository)
			tt.mockSetup(mockRepo)

			service := NewTraceService(mockRepo, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
			result, err := service.GetTrace(context.Background(), tt.traceID)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestTraceService_ListTraces(t *testing.T) {
	projectID := "proj123"
	now := time.Now()

	tests := []struct {
		name        string
		filter      *observability.TraceFilter
		mockSetup   func(*MockTraceRepository)
		expectedErr bool
		checkResult func(*testing.T, []*observability.TraceSummary)
	}{
		{
			name: "success - list traces with filter",
			filter: &observability.TraceFilter{
				ProjectID: projectID,
			},
			mockSetup: func(repo *MockTraceRepository) {
				endTime1 := now.Add(time.Second)
				endTime2 := now.Add(-time.Hour).Add(2 * time.Second)
				traces := []*observability.TraceSummary{
					{
						TraceID:   "trace1234567890123456789012",
						ProjectID: projectID,
						SpanCount: 3,
						StartTime: now,
						EndTime:   &endTime1,
					},
					{
						TraceID:   "trace2234567890123456789012",
						ProjectID: projectID,
						SpanCount: 5,
						StartTime: now.Add(-time.Hour),
						EndTime:   &endTime2,
					},
				}
				repo.On("ListTraces", mock.Anything, mock.AnythingOfType("*observability.TraceFilter")).
					Return(traces, nil)
			},
			expectedErr: false,
			checkResult: func(t *testing.T, traces []*observability.TraceSummary) {
				assert.Len(t, traces, 2)
				assert.Equal(t, int64(3), traces[0].SpanCount)
				assert.Equal(t, int64(5), traces[1].SpanCount)
			},
		},
		{
			name:        "error - nil filter",
			filter:      nil,
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
		{
			name: "error - empty project_id",
			filter: &observability.TraceFilter{
				ProjectID: "",
			},
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockTraceRepository)
			tt.mockSetup(mockRepo)

			service := NewTraceService(mockRepo, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
			result, err := service.ListTraces(context.Background(), tt.filter)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestTraceService_GetTraceSpans(t *testing.T) {
	tests := []struct {
		name        string
		traceID     string
		mockSetup   func(*MockTraceRepository)
		expectedErr bool
		checkResult func(*testing.T, []*observability.Span)
	}{
		{
			name:    "success - get all spans",
			traceID: "12345678901234567890123456789012",
			mockSetup: func(repo *MockTraceRepository) {
				spans := []*observability.Span{
					{SpanID: "span1234567890123", TraceID: "12345678901234567890123456789012"},
					{SpanID: "span2234567890123", TraceID: "12345678901234567890123456789012"},
					{SpanID: "span3234567890123", TraceID: "12345678901234567890123456789012"},
				}
				repo.On("GetSpansByTraceID", mock.Anything, "12345678901234567890123456789012").
					Return(spans, nil)
			},
			expectedErr: false,
			checkResult: func(t *testing.T, spans []*observability.Span) {
				assert.Len(t, spans, 3)
			},
		},
		{
			name:        "error - invalid trace_id",
			traceID:     "invalid",
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockTraceRepository)
			tt.mockSetup(mockRepo)

			service := NewTraceService(mockRepo, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
			result, err := service.GetTraceSpans(context.Background(), tt.traceID)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestTraceService_CalculateTraceCost(t *testing.T) {
	tests := []struct {
		name         string
		traceID      string
		mockSetup    func(*MockTraceRepository)
		expectedCost float64
		expectedErr  bool
	}{
		{
			name:    "success - calculate cost",
			traceID: "12345678901234567890123456789012",
			mockSetup: func(repo *MockTraceRepository) {
				repo.On("CalculateTotalCost", mock.Anything, "12345678901234567890123456789012").
					Return(0.05, nil)
			},
			expectedCost: 0.05,
			expectedErr:  false,
		},
		{
			name:        "error - invalid trace_id",
			traceID:     "invalid",
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockTraceRepository)
			tt.mockSetup(mockRepo)

			service := NewTraceService(mockRepo, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
			result, err := service.CalculateTraceCost(context.Background(), tt.traceID)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCost, result)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestTraceService_IngestSpan(t *testing.T) {
	tests := []struct {
		name        string
		span        *observability.Span
		mockSetup   func(*MockTraceRepository)
		expectedErr bool
	}{
		{
			name: "success - valid span",
			span: &observability.Span{
				SpanID:    "1234567890123456",
				TraceID:   "12345678901234567890123456789012",
				ProjectID: "proj123",
				SpanName:  "test-span",
			},
			mockSetup: func(repo *MockTraceRepository) {
				repo.On("InsertSpan", mock.Anything, mock.AnythingOfType("*observability.Span")).
					Return(nil)
			},
			expectedErr: false,
		},
		{
			name: "error - missing trace_id",
			span: &observability.Span{
				SpanID:    "1234567890123456",
				ProjectID: "proj123",
				SpanName:  "test-span",
			},
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
		{
			name: "error - missing span_id",
			span: &observability.Span{
				TraceID:   "12345678901234567890123456789012",
				ProjectID: "proj123",
				SpanName:  "test-span",
			},
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
		{
			name: "error - invalid span_id length",
			span: &observability.Span{
				SpanID:    "short",
				TraceID:   "12345678901234567890123456789012",
				ProjectID: "proj123",
				SpanName:  "test-span",
			},
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockTraceRepository)
			tt.mockSetup(mockRepo)

			service := NewTraceService(mockRepo, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
			err := service.IngestSpan(context.Background(), tt.span)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestTraceService_DeleteTrace(t *testing.T) {
	tests := []struct {
		name        string
		traceID     string
		mockSetup   func(*MockTraceRepository)
		expectedErr bool
	}{
		{
			name:    "success - delete trace",
			traceID: "12345678901234567890123456789012",
			mockSetup: func(repo *MockTraceRepository) {
				repo.On("CountSpansInTrace", mock.Anything, "12345678901234567890123456789012").
					Return(int64(5), nil)
				repo.On("DeleteTrace", mock.Anything, "12345678901234567890123456789012").
					Return(nil)
			},
			expectedErr: false,
		},
		{
			name:        "error - invalid trace_id",
			traceID:     "invalid",
			mockSetup:   func(repo *MockTraceRepository) {},
			expectedErr: true,
		},
		{
			name:    "error - trace not found",
			traceID: "12345678901234567890123456789012",
			mockSetup: func(repo *MockTraceRepository) {
				repo.On("CountSpansInTrace", mock.Anything, "12345678901234567890123456789012").
					Return(int64(0), nil)
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockTraceRepository)
			tt.mockSetup(mockRepo)

			service := NewTraceService(mockRepo, slog.New(slog.NewJSONHandler(os.Stderr, nil)))
			err := service.DeleteTrace(context.Background(), tt.traceID)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}
