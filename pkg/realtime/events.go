package realtime

import (
	"encoding/json"
	"time"
)

// EventType represents the type of real-time event
type EventType string

const (
	// System Events
	EventSystemStarted  EventType = "system.started"
	EventSystemShutdown EventType = "system.shutdown"
	EventSystemError    EventType = "system.error"
	EventSystemHealth   EventType = "system.health"

	// User Events
	EventUserConnected    EventType = "user.connected"
	EventUserDisconnected EventType = "user.disconnected"
	EventUserActivity     EventType = "user.activity"

	// Authentication Events
	EventUserLogin  EventType = "auth.login"
	EventUserLogout EventType = "auth.logout"
	EventAPIKeyUsed EventType = "auth.api_key_used"

	// AI Request Events
	EventRequestStarted   EventType = "request.started"
	EventRequestCompleted EventType = "request.completed"
	EventRequestFailed    EventType = "request.failed"
	EventRequestCached    EventType = "request.cached"

	// AI Provider Events
	EventProviderHealthChanged EventType = "provider.health_changed"
	EventProviderError         EventType = "provider.error"
	EventProviderRateLimit     EventType = "provider.rate_limit"

	// Routing Events
	EventRoutingDecision EventType = "routing.decision"
	EventRoutingFailed   EventType = "routing.failed"
	EventModelSwitch     EventType = "routing.model_switch"

	// Metrics Events
	EventMetricThreshold EventType = "metric.threshold"
	EventMetricAnomaly   EventType = "metric.anomaly"
	EventMetricUpdate    EventType = "metric.update"

	// Billing Events
	EventUsageThreshold EventType = "billing.usage_threshold"
	EventQuotaWarning   EventType = "billing.quota_warning"
	EventQuotaExceeded  EventType = "billing.quota_exceeded"
	EventBillingUpdated EventType = "billing.updated"

	// Cache Events
	EventCacheHit    EventType = "cache.hit"
	EventCacheMiss   EventType = "cache.miss"
	EventCacheUpdate EventType = "cache.update"
	EventCacheEvict  EventType = "cache.evict"

	// Analytics Events
	EventAnalyticsComputed  EventType = "analytics.computed"
	EventAnalyticsExported  EventType = "analytics.exported"
	EventDashboardRefreshed EventType = "analytics.dashboard_refreshed"

	// Organization Events
	EventOrgCreated    EventType = "org.created"
	EventOrgUpdated    EventType = "org.updated"
	EventOrgDeleted    EventType = "org.deleted"
	EventMemberAdded   EventType = "org.member_added"
	EventMemberRemoved EventType = "org.member_removed"

	// Project Events
	EventProjectCreated EventType = "project.created"
	EventProjectUpdated EventType = "project.updated"
	EventProjectDeleted EventType = "project.deleted"

	// Configuration Events
	EventConfigChanged   EventType = "config.changed"
	EventSettingsUpdated EventType = "config.settings_updated"

	// Notification Events
	EventNotificationSent      EventType = "notification.sent"
	EventNotificationRead      EventType = "notification.read"
	EventNotificationScheduled EventType = "notification.scheduled"

	// Custom Events (for user-defined events)
	EventCustom EventType = "custom"
)

// Priority levels for events
type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

// Event represents a real-time event
type Event struct {
	ID          string                 `json:"id"`
	Type        EventType              `json:"type"`
	Subject     string                 `json:"subject,omitempty"`
	Action      string                 `json:"action,omitempty"`
	Data        any            `json:"data,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Priority    Priority               `json:"priority"`
	Timestamp   time.Time              `json:"timestamp"`
	Source      string                 `json:"source"`
	UserID      string                 `json:"user_id,omitempty"`
	OrgID       string                 `json:"org_id,omitempty"`
	ProjectID   string                 `json:"project_id,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	TraceID     string                 `json:"trace_id,omitempty"`
	Version     string                 `json:"version"`
}

// NewEvent creates a new real-time event
func NewEvent(eventType EventType, data any) *Event {
	return &Event{
		Type:      eventType,
		Data:      data,
		Priority:  PriorityMedium,
		Timestamp: time.Now().UTC(),
		Metadata:  make(map[string]any),
		Version:   "1.0",
	}
}

// SetID sets the event ID
func (e *Event) SetID(id string) *Event {
	e.ID = id
	return e
}

// SetSubject sets the event subject
func (e *Event) SetSubject(subject string) *Event {
	e.Subject = subject
	return e
}

// SetAction sets the event action
func (e *Event) SetAction(action string) *Event {
	e.Action = action
	return e
}

// SetPriority sets the event priority
func (e *Event) SetPriority(priority Priority) *Event {
	e.Priority = priority
	return e
}

// SetSource sets the event source
func (e *Event) SetSource(source string) *Event {
	e.Source = source
	return e
}

// SetUserContext sets user context information
func (e *Event) SetUserContext(userID, orgID, projectID string) *Event {
	e.UserID = userID
	e.OrgID = orgID
	e.ProjectID = projectID
	return e
}

// SetEnvironment sets the environment
func (e *Event) SetEnvironment(env string) *Event {
	e.Environment = env
	return e
}

// SetTraceID sets the trace ID for distributed tracing
func (e *Event) SetTraceID(traceID string) *Event {
	e.TraceID = traceID
	return e
}

// AddMetadata adds metadata to the event
func (e *Event) AddMetadata(key string, value any) *Event {
	if e.Metadata == nil {
		e.Metadata = make(map[string]any)
	}
	e.Metadata[key] = value
	return e
}

// GetMetadata retrieves metadata from the event
func (e *Event) GetMetadata(key string) (any, bool) {
	if e.Metadata == nil {
		return nil, false
	}
	value, exists := e.Metadata[key]
	return value, exists
}

// ToJSON converts the event to JSON bytes
func (e *Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// FromJSON creates an event from JSON bytes
func FromJSON(data []byte) (*Event, error) {
	var event Event
	err := json.Unmarshal(data, &event)
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// IsValid validates the event structure
func (e *Event) IsValid() bool {
	return e.Type != "" && e.Source != ""
}

// IsCritical checks if the event is critical priority
func (e *Event) IsCritical() bool {
	return e.Priority == PriorityCritical
}

// IsHighPriority checks if the event is high priority or above
func (e *Event) IsHighPriority() bool {
	return e.Priority == PriorityHigh || e.Priority == PriorityCritical
}

// Clone creates a copy of the event
func (e *Event) Clone() *Event {
	clone := &Event{
		ID:          e.ID,
		Type:        e.Type,
		Subject:     e.Subject,
		Action:      e.Action,
		Data:        e.Data,
		Priority:    e.Priority,
		Timestamp:   e.Timestamp,
		Source:      e.Source,
		UserID:      e.UserID,
		OrgID:       e.OrgID,
		ProjectID:   e.ProjectID,
		Environment: e.Environment,
		TraceID:     e.TraceID,
		Version:     e.Version,
	}

	// Deep copy metadata
	if e.Metadata != nil {
		clone.Metadata = make(map[string]any)
		for k, v := range e.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// EventFilter represents a filter for events
type EventFilter struct {
	TimeRange   *EventTimeRange        `json:"time_range,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	UserID      string                 `json:"user_id,omitempty"`
	OrgID       string                 `json:"org_id,omitempty"`
	ProjectID   string                 `json:"project_id,omitempty"`
	Environment string                 `json:"environment,omitempty"`
	Types       []EventType            `json:"types,omitempty"`
	Sources     []string               `json:"sources,omitempty"`
	Priorities  []Priority             `json:"priorities,omitempty"`
}

// EventTimeRange represents a time range for event filtering
type EventTimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Matches checks if an event matches the filter
func (f *EventFilter) Matches(event *Event) bool {
	// Check event types
	if len(f.Types) > 0 {
		matched := false
		for _, eventType := range f.Types {
			if event.Type == eventType {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check sources
	if len(f.Sources) > 0 {
		matched := false
		for _, source := range f.Sources {
			if event.Source == source {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check priorities
	if len(f.Priorities) > 0 {
		matched := false
		for _, priority := range f.Priorities {
			if event.Priority == priority {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check user context
	if f.UserID != "" && event.UserID != f.UserID {
		return false
	}
	if f.OrgID != "" && event.OrgID != f.OrgID {
		return false
	}
	if f.ProjectID != "" && event.ProjectID != f.ProjectID {
		return false
	}
	if f.Environment != "" && event.Environment != f.Environment {
		return false
	}

	// Check time range
	if f.TimeRange != nil {
		if event.Timestamp.Before(f.TimeRange.Start) || event.Timestamp.After(f.TimeRange.End) {
			return false
		}
	}

	// Check metadata filters
	if len(f.Metadata) > 0 {
		for key, expectedValue := range f.Metadata {
			if actualValue, exists := event.GetMetadata(key); !exists || actualValue != expectedValue {
				return false
			}
		}
	}

	return true
}

// Event data structures for specific event types

// RequestEventData represents data for request events
type RequestEventData struct {
	CustomFields map[string]any `json:"custom_fields,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Provider     string                 `json:"provider"`
	Model        string                 `json:"model"`
	Method       string                 `json:"method"`
	Path         string                 `json:"path"`
	RequestID    string                 `json:"request_id"`
	Tokens       int                    `json:"tokens,omitempty"`
	Latency      float64                `json:"latency,omitempty"`
	Quality      float64                `json:"quality,omitempty"`
	Cost         float64                `json:"cost,omitempty"`
	StatusCode   int                    `json:"status_code,omitempty"`
	CacheHit     bool                   `json:"cache_hit,omitempty"`
}

// ProviderEventData represents data for provider events
type ProviderEventData struct {
	Provider       string  `json:"provider"`
	Status         string  `json:"status"`
	Health         float64 `json:"health"`
	Latency        float64 `json:"latency"`
	ErrorRate      float64 `json:"error_rate"`
	SuccessRate    float64 `json:"success_rate"`
	RequestsPerMin int64   `json:"requests_per_min,omitempty"`
}

// MetricEventData represents data for metric events
type MetricEventData struct {
	Labels     map[string]string      `json:"labels,omitempty"`
	Dimensions map[string]any `json:"dimensions,omitempty"`
	MetricName string                 `json:"metric_name"`
	Unit       string                 `json:"unit"`
	Value      float64                `json:"value"`
	PrevValue  float64                `json:"prev_value,omitempty"`
	Threshold  float64                `json:"threshold,omitempty"`
}

// UsageEventData represents data for usage/billing events
type UsageEventData struct {
	ResourceType string  `json:"resource_type"`
	Period       string  `json:"period"`
	Currency     string  `json:"currency,omitempty"`
	Usage        int64   `json:"usage"`
	Limit        int64   `json:"limit"`
	Percentage   float64 `json:"percentage"`
	Cost         float64 `json:"cost,omitempty"`
}

// CacheEventData represents data for cache events
type CacheEventData struct {
	Key        string  `json:"key"`
	Provider   string  `json:"provider,omitempty"`
	Model      string  `json:"model,omitempty"`
	Size       int     `json:"size,omitempty"`
	TTL        int64   `json:"ttl,omitempty"`
	Similarity float64 `json:"similarity,omitempty"`
	SavedCost  float64 `json:"saved_cost,omitempty"`
	SavedTime  float64 `json:"saved_time,omitempty"`
}

// OrganizationEventData represents data for organization events
type OrganizationEventData struct {
	Changes     map[string]any `json:"changes,omitempty"`
	Name        string                 `json:"name"`
	Slug        string                 `json:"slug,omitempty"`
	Plan        string                 `json:"plan,omitempty"`
	MemberCount int                    `json:"member_count,omitempty"`
}

// ProjectEventData represents data for project events
type ProjectEventData struct {
	Changes     map[string]any `json:"changes,omitempty"`
	Name        string                 `json:"name"`
	Environment string                 `json:"environment"`
	Status      string                 `json:"status,omitempty"`
}

// NotificationEventData represents data for notification events
type NotificationEventData struct {
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
	NotificationID string     `json:"notification_id"`
	Type           string     `json:"type"`
	Channel        string     `json:"channel"`
	Title          string     `json:"title"`
	Message        string     `json:"message"`
	Priority       string     `json:"priority"`
	Delivered      bool       `json:"delivered"`
}

// Helper functions for creating common events

// NewRequestEvent creates a request-related event
func NewRequestEvent(eventType EventType, requestData *RequestEventData) *Event {
	return NewEvent(eventType, requestData).
		SetSubject("request").
		SetSource("api-gateway")
}

// NewProviderEvent creates a provider-related event
func NewProviderEvent(eventType EventType, providerData *ProviderEventData) *Event {
	return NewEvent(eventType, providerData).
		SetSubject("provider").
		SetSource("ai-router")
}

// NewMetricEvent creates a metric-related event
func NewMetricEvent(eventType EventType, metricData *MetricEventData) *Event {
	priority := PriorityLow
	if eventType == EventMetricThreshold || eventType == EventMetricAnomaly {
		priority = PriorityHigh
	}

	return NewEvent(eventType, metricData).
		SetSubject("metrics").
		SetSource("analytics").
		SetPriority(priority)
}

// NewUsageEvent creates a usage/billing-related event
func NewUsageEvent(eventType EventType, usageData *UsageEventData) *Event {
	priority := PriorityMedium
	if eventType == EventQuotaExceeded {
		priority = PriorityCritical
	} else if eventType == EventQuotaWarning {
		priority = PriorityHigh
	}

	return NewEvent(eventType, usageData).
		SetSubject("billing").
		SetSource("billing-service").
		SetPriority(priority)
}

// NewCacheEvent creates a cache-related event
func NewCacheEvent(eventType EventType, cacheData *CacheEventData) *Event {
	return NewEvent(eventType, cacheData).
		SetSubject("cache").
		SetSource("cache-service")
}

// NewOrgEvent creates an organization-related event
func NewOrgEvent(eventType EventType, orgData *OrganizationEventData) *Event {
	return NewEvent(eventType, orgData).
		SetSubject("organization").
		SetSource("user-service")
}

// NewProjectEvent creates a project-related event
func NewProjectEvent(eventType EventType, projectData *ProjectEventData) *Event {
	return NewEvent(eventType, projectData).
		SetSubject("project").
		SetSource("user-service")
}

// NewNotificationEvent creates a notification-related event
func NewNotificationEvent(eventType EventType, notificationData *NotificationEventData) *Event {
	return NewEvent(eventType, notificationData).
		SetSubject("notification").
		SetSource("notification-service")
}
