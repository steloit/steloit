package analytics

import (
	"context"
	"errors"
	"time"
)

// EnterpriseAnalytics interface for advanced analytics features
type EnterpriseAnalytics interface {
	GeneratePredictiveInsights(ctx context.Context, timeRange string) (*PredictiveReport, error)
	CreateCustomDashboard(ctx context.Context, dashboard *Dashboard) error
	UpdateCustomDashboard(ctx context.Context, dashboardID string, dashboard *Dashboard) error
	GetCustomDashboard(ctx context.Context, dashboardID string) (*Dashboard, error)
	ListCustomDashboards(ctx context.Context) ([]*Dashboard, error)
	DeleteCustomDashboard(ctx context.Context, dashboardID string) error
	GenerateAdvancedReport(ctx context.Context, req *ReportRequest) (*Report, error)
	ExportData(ctx context.Context, format string, query *ExportQuery) ([]byte, error)
	RunMLModel(ctx context.Context, modelName string, data any) (any, error)
}

// PredictiveReport represents ML-powered insights
type PredictiveReport struct {
	GeneratedAt     time.Time         `json:"generated_at"`
	CostForecast    *CostForecast     `json:"cost_forecast,omitempty"`
	TimeRange       string            `json:"time_range"`
	UsageTrends     []*UsageTrend     `json:"usage_trends,omitempty"`
	Anomalies       []*Anomaly        `json:"anomalies,omitempty"`
	Recommendations []*Recommendation `json:"recommendations,omitempty"`
}

// Dashboard represents a custom dashboard configuration
type Dashboard struct {
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Layout      *Layout   `json:"layout,omitempty"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedBy   string    `json:"created_by"`
	Widgets     []*Widget `json:"widgets"`
}

// Widget represents a dashboard widget
type Widget struct {
	Config   map[string]any `json:"config"`
	Position *Position              `json:"position,omitempty"`
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Title    string                 `json:"title"`
}

// Layout represents dashboard layout configuration
type Layout struct {
	Theme   string `json:"theme,omitempty"`
	Columns int    `json:"columns"`
	Rows    int    `json:"rows"`
}

// Position represents widget position in dashboard
type Position struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// ReportRequest represents advanced report parameters
type ReportRequest struct {
	Filters     map[string]any `json:"filters,omitempty"`
	Type        string                 `json:"type"`
	TimeRange   string                 `json:"time_range"`
	Aggregation string                 `json:"aggregation,omitempty"`
	GroupBy     []string               `json:"group_by,omitempty"`
}

// Report represents generated report
type Report struct {
	GeneratedAt time.Time              `json:"generated_at"`
	ExpiresAt   time.Time              `json:"expires_at,omitempty"`
	Data        map[string]any `json:"data"`
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
}

// ExportQuery represents data export parameters
type ExportQuery struct {
	Table     string                 `json:"table"`
	TimeRange string                 `json:"time_range"`
	Filters   map[string]any `json:"filters,omitempty"`
	Columns   []string               `json:"columns,omitempty"`
}

// Supporting types
type CostForecast struct {
	Trend       string  `json:"trend"`
	NextMonth   float64 `json:"next_month"`
	NextQuarter float64 `json:"next_quarter"`
	Confidence  float64 `json:"confidence"`
}

type UsageTrend struct {
	Metric     string  `json:"metric"`
	Trend      string  `json:"trend"`
	Period     string  `json:"period"`
	Change     float64 `json:"change"`
	Confidence float64 `json:"confidence"`
}

type Anomaly struct {
	Timestamp   time.Time `json:"timestamp"`
	Metric      string    `json:"metric"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	Value       float64   `json:"value"`
	Expected    float64   `json:"expected"`
}

type Recommendation struct {
	Type        string  `json:"type"` // cost, performance, security
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Impact      string  `json:"impact"` // low, medium, high
	Effort      string  `json:"effort"` // low, medium, high
	Savings     float64 `json:"savings,omitempty"`
}

// StubEnterpriseAnalytics provides stub implementation for OSS version
type StubEnterpriseAnalytics struct{}

// New returns the enterprise analytics implementation (stub or real based on build tags)
func New() EnterpriseAnalytics {
	return &StubEnterpriseAnalytics{}
}

func (s *StubEnterpriseAnalytics) GeneratePredictiveInsights(ctx context.Context, timeRange string) (*PredictiveReport, error) {
	return nil, errors.New("predictive insights require Enterprise license")
}

func (s *StubEnterpriseAnalytics) CreateCustomDashboard(ctx context.Context, dashboard *Dashboard) error {
	return errors.New("custom dashboards require Enterprise license")
}

func (s *StubEnterpriseAnalytics) UpdateCustomDashboard(ctx context.Context, dashboardID string, dashboard *Dashboard) error {
	return errors.New("custom dashboards require Enterprise license")
}

func (s *StubEnterpriseAnalytics) GetCustomDashboard(ctx context.Context, dashboardID string) (*Dashboard, error) {
	return nil, errors.New("custom dashboards require Enterprise license")
}

func (s *StubEnterpriseAnalytics) ListCustomDashboards(ctx context.Context) ([]*Dashboard, error) {
	return []*Dashboard{}, errors.New("custom dashboards require Enterprise license")
}

func (s *StubEnterpriseAnalytics) DeleteCustomDashboard(ctx context.Context, dashboardID string) error {
	return errors.New("custom dashboards require Enterprise license")
}

func (s *StubEnterpriseAnalytics) GenerateAdvancedReport(ctx context.Context, req *ReportRequest) (*Report, error) {
	return nil, errors.New("advanced reports require Enterprise license")
}

func (s *StubEnterpriseAnalytics) ExportData(ctx context.Context, format string, query *ExportQuery) ([]byte, error) {
	return nil, errors.New("data export requires Enterprise license")
}

func (s *StubEnterpriseAnalytics) RunMLModel(ctx context.Context, modelName string, data any) (any, error) {
	return nil, errors.New("ML models require Enterprise license")
}
