package websocket

import (
	"encoding/json"
	"time"
)

// MessageType represents different types of WebSocket messages
type MessageType string

const (
	// Connection management messages
	MessageTypeConnect    MessageType = "connect"
	MessageTypeDisconnect MessageType = "disconnect"
	MessageTypePing       MessageType = "ping"
	MessageTypePong       MessageType = "pong"

	// Subscription management messages
	MessageTypeSubscribe   MessageType = "subscribe"
	MessageTypeUnsubscribe MessageType = "unsubscribe"

	// Data messages
	MessageTypeMessage MessageType = "message"
	MessageTypeEvent   MessageType = "event"
	MessageTypeData    MessageType = "data"

	// Error and status messages
	MessageTypeError  MessageType = "error"
	MessageTypeStatus MessageType = "status"
	MessageTypeAck    MessageType = "ack"

	// Real-time updates
	MessageTypeMetricUpdate    MessageType = "metric_update"
	MessageTypeAnalyticsUpdate MessageType = "analytics_update"
	MessageTypeUsageUpdate     MessageType = "usage_update"
	MessageTypeProviderUpdate  MessageType = "provider_update"
	MessageTypeRoutingUpdate   MessageType = "routing_update"
	MessageTypeBillingUpdate   MessageType = "billing_update"
	MessageTypeSystemAlert     MessageType = "system_alert"
	MessageTypeNotification    MessageType = "notification"

	// AI Platform specific messages
	MessageTypeRequestStarted   MessageType = "request_started"
	MessageTypeRequestCompleted MessageType = "request_completed"
	MessageTypeRequestFailed    MessageType = "request_failed"
	MessageTypeCacheHit         MessageType = "cache_hit"
	MessageTypeCacheMiss        MessageType = "cache_miss"
	MessageTypeRoutingDecision  MessageType = "routing_decision"
	MessageTypeProviderHealth   MessageType = "provider_health"
	MessageTypeQuotaWarning     MessageType = "quota_warning"
	MessageTypeQuotaExceeded    MessageType = "quota_exceeded"
)

// Message represents a WebSocket message
type Message struct {
	ID        string                 `json:"id,omitempty"`
	Type      MessageType            `json:"type"`
	Channel   string                 `json:"channel,omitempty"`
	Event     string                 `json:"event,omitempty"`
	Data      any            `json:"data,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	UserID    string                 `json:"user_id,omitempty"`
	OrgID     string                 `json:"org_id,omitempty"`
	ProjectID string                 `json:"project_id,omitempty"`
}

// NewMessage creates a new WebSocket message
func NewMessage(messageType MessageType, data any) *Message {
	return &Message{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now().UTC(),
		Metadata:  make(map[string]any),
	}
}

// NewEventMessage creates a new event message
func NewEventMessage(event string, data any) *Message {
	return &Message{
		Type:      MessageTypeEvent,
		Event:     event,
		Data:      data,
		Timestamp: time.Now().UTC(),
		Metadata:  make(map[string]any),
	}
}

// NewChannelMessage creates a new channel message
func NewChannelMessage(channel string, data any) *Message {
	return &Message{
		Type:      MessageTypeMessage,
		Channel:   channel,
		Data:      data,
		Timestamp: time.Now().UTC(),
		Metadata:  make(map[string]any),
	}
}

// SetID sets the message ID
func (m *Message) SetID(id string) *Message {
	m.ID = id
	return m
}

// SetChannel sets the message channel
func (m *Message) SetChannel(channel string) *Message {
	m.Channel = channel
	return m
}

// SetUserContext sets user context information
func (m *Message) SetUserContext(userID, orgID, projectID string) *Message {
	m.UserID = userID
	m.OrgID = orgID
	m.ProjectID = projectID
	return m
}

// AddMetadata adds metadata to the message
func (m *Message) AddMetadata(key string, value any) *Message {
	if m.Metadata == nil {
		m.Metadata = make(map[string]any)
	}
	m.Metadata[key] = value
	return m
}

// GetMetadata retrieves metadata from the message
func (m *Message) GetMetadata(key string) (any, bool) {
	if m.Metadata == nil {
		return nil, false
	}
	value, exists := m.Metadata[key]
	return value, exists
}

// ToJSON converts the message to JSON bytes
func (m *Message) ToJSON() ([]byte, error) {
	return json.Marshal(m)
}

// FromJSON creates a message from JSON bytes
func FromJSON(data []byte) (*Message, error) {
	var message Message
	err := json.Unmarshal(data, &message)
	if err != nil {
		return nil, err
	}
	return &message, nil
}

// IsValid validates the message structure
func (m *Message) IsValid() bool {
	return m.Type != ""
}

// IsSubscriptionMessage checks if message is subscription-related
func (m *Message) IsSubscriptionMessage() bool {
	return m.Type == MessageTypeSubscribe || m.Type == MessageTypeUnsubscribe
}

// IsDataMessage checks if message contains data
func (m *Message) IsDataMessage() bool {
	return m.Type == MessageTypeMessage || m.Type == MessageTypeEvent || m.Type == MessageTypeData
}

// IsControlMessage checks if message is a control message
func (m *Message) IsControlMessage() bool {
	switch m.Type {
	case MessageTypeConnect, MessageTypeDisconnect, MessageTypePing, MessageTypePong:
		return true
	default:
		return false
	}
}

// IsErrorMessage checks if message is an error
func (m *Message) IsErrorMessage() bool {
	return m.Type == MessageTypeError
}

// Clone creates a copy of the message
func (m *Message) Clone() *Message {
	clone := &Message{
		ID:        m.ID,
		Type:      m.Type,
		Channel:   m.Channel,
		Event:     m.Event,
		Data:      m.Data,
		Timestamp: m.Timestamp,
		UserID:    m.UserID,
		OrgID:     m.OrgID,
		ProjectID: m.ProjectID,
	}

	// Deep copy metadata
	if m.Metadata != nil {
		clone.Metadata = make(map[string]any)
		for k, v := range m.Metadata {
			clone.Metadata[k] = v
		}
	}

	return clone
}

// SubscribeMessage represents a subscription request
type SubscribeMessage struct {
	Channel   string            `json:"channel"`
	Filters   map[string]string `json:"filters,omitempty"`
	UserID    string            `json:"user_id,omitempty"`
	OrgID     string            `json:"org_id,omitempty"`
	ProjectID string            `json:"project_id,omitempty"`
}

// UnsubscribeMessage represents an unsubscription request
type UnsubscribeMessage struct {
	Channel   string `json:"channel"`
	UserID    string `json:"user_id,omitempty"`
	OrgID     string `json:"org_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// ErrorMessage represents an error message
type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// StatusMessage represents a status message
type StatusMessage struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    any `json:"data,omitempty"`
}

// AckMessage represents an acknowledgment message
type AckMessage struct {
	MessageID string      `json:"message_id"`
	Status    string      `json:"status"`
	Data      any `json:"data,omitempty"`
}

// Real-time data structures for Brokle platform

// MetricUpdate represents a real-time metrics update
type MetricUpdate struct {
	MetricName string                 `json:"metric_name"`
	Value      float64                `json:"value"`
	Unit       string                 `json:"unit,omitempty"`
	Labels     map[string]string      `json:"labels,omitempty"`
	Dimensions map[string]any `json:"dimensions,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	OrgID      string                 `json:"org_id"`
	ProjectID  string                 `json:"project_id"`
}

// AnalyticsUpdate represents a real-time analytics update
type AnalyticsUpdate struct {
	Type      string                 `json:"type"`
	Data      map[string]any `json:"data"`
	Period    string                 `json:"period"`
	Timestamp time.Time              `json:"timestamp"`
	OrgID     string                 `json:"org_id"`
	ProjectID string                 `json:"project_id"`
}

// UsageUpdate represents a real-time usage update
type UsageUpdate struct {
	ResourceType string    `json:"resource_type"`
	Usage        int64     `json:"usage"`
	Limit        int64     `json:"limit"`
	Period       string    `json:"period"`
	Timestamp    time.Time `json:"timestamp"`
	OrgID        string    `json:"org_id"`
	ProjectID    string    `json:"project_id"`
}

// ProviderUpdate represents an AI provider status update
type ProviderUpdate struct {
	ProviderName string                 `json:"provider_name"`
	Status       string                 `json:"status"`
	Health       float64                `json:"health"`
	Latency      float64                `json:"latency"`
	ErrorRate    float64                `json:"error_rate"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Timestamp    time.Time              `json:"timestamp"`
}

// RoutingUpdate represents a routing decision update
type RoutingUpdate struct {
	RequestID string                 `json:"request_id"`
	Model     string                 `json:"model"`
	Provider  string                 `json:"provider"`
	Decision  string                 `json:"decision"`
	Reason    string                 `json:"reason"`
	Latency   float64                `json:"latency"`
	Cost      float64                `json:"cost"`
	Quality   float64                `json:"quality,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	OrgID     string                 `json:"org_id"`
	ProjectID string                 `json:"project_id"`
}

// BillingUpdate represents a billing update
type BillingUpdate struct {
	Type        string    `json:"type"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Description string    `json:"description"`
	Period      string    `json:"period"`
	Timestamp   time.Time `json:"timestamp"`
	OrgID       string    `json:"org_id"`
}

// SystemAlert represents a system alert
type SystemAlert struct {
	Level     string                 `json:"level"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Component string                 `json:"component"`
	Action    string                 `json:"action,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	OrgID     string                 `json:"org_id,omitempty"`
	ProjectID string                 `json:"project_id,omitempty"`
}

// Notification represents a user notification
type Notification struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Priority  string                 `json:"priority"`
	Category  string                 `json:"category"`
	ActionURL string                 `json:"action_url,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Read      bool                   `json:"read"`
	Timestamp time.Time              `json:"timestamp"`
	ExpiresAt *time.Time             `json:"expires_at,omitempty"`
	UserID    string                 `json:"user_id"`
	OrgID     string                 `json:"org_id"`
}

// RequestEvent represents an AI request event
type RequestEvent struct {
	RequestID string                 `json:"request_id"`
	Status    string                 `json:"status"`
	Provider  string                 `json:"provider"`
	Model     string                 `json:"model"`
	Tokens    int                    `json:"tokens,omitempty"`
	Cost      float64                `json:"cost,omitempty"`
	Latency   float64                `json:"latency,omitempty"`
	Quality   float64                `json:"quality,omitempty"`
	CacheHit  bool                   `json:"cache_hit,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	OrgID     string                 `json:"org_id"`
	ProjectID string                 `json:"project_id"`
	UserID    string                 `json:"user_id,omitempty"`
}

// CacheEvent represents a cache event
type CacheEvent struct {
	Type       string                 `json:"type"`
	Key        string                 `json:"key"`
	Hit        bool                   `json:"hit"`
	Similarity float64                `json:"similarity,omitempty"`
	SavedCost  float64                `json:"saved_cost,omitempty"`
	SavedTime  float64                `json:"saved_time,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	OrgID      string                 `json:"org_id"`
	ProjectID  string                 `json:"project_id"`
}

// QuotaEvent represents a quota-related event
type QuotaEvent struct {
	Type       string    `json:"type"`
	Resource   string    `json:"resource"`
	Current    int64     `json:"current"`
	Limit      int64     `json:"limit"`
	Percentage float64   `json:"percentage"`
	Period     string    `json:"period"`
	Action     string    `json:"action,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	OrgID      string    `json:"org_id"`
	ProjectID  string    `json:"project_id"`
}

// Helper functions for creating common messages

// NewSubscribeMessage creates a subscribe message
func NewSubscribeMessage(channel string) *Message {
	return NewMessage(MessageTypeSubscribe, SubscribeMessage{
		Channel: channel,
	})
}

// NewUnsubscribeMessage creates an unsubscribe message
func NewUnsubscribeMessage(channel string) *Message {
	return NewMessage(MessageTypeUnsubscribe, UnsubscribeMessage{
		Channel: channel,
	})
}

// NewErrorMessage creates an error message
func NewErrorMessage(code, message, details string) *Message {
	return NewMessage(MessageTypeError, ErrorMessage{
		Code:    code,
		Message: message,
		Details: details,
	})
}

// NewStatusMessage creates a status message
func NewStatusMessage(status, message string, data any) *Message {
	return NewMessage(MessageTypeStatus, StatusMessage{
		Status:  status,
		Message: message,
		Data:    data,
	})
}

// NewAckMessage creates an acknowledgment message
func NewAckMessage(messageID, status string, data any) *Message {
	return NewMessage(MessageTypeAck, AckMessage{
		MessageID: messageID,
		Status:    status,
		Data:      data,
	})
}

// NewPingMessage creates a ping message
func NewPingMessage() *Message {
	return NewMessage(MessageTypePing, nil)
}

// NewPongMessage creates a pong message
func NewPongMessage() *Message {
	return NewMessage(MessageTypePong, nil)
}
