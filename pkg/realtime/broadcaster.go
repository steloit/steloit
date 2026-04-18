package realtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Channel represents a broadcast channel
type Channel struct {
	CreatedAt   time.Time              `json:"created_at"`
	Filters     *EventFilter           `json:"filters,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	TTL         time.Duration          `json:"ttl,omitempty"`
	MaxSize     int                    `json:"max_size,omitempty"`
	Private     bool                   `json:"private"`
	Persistent  bool                   `json:"persistent"`
}

// Subscription represents a client subscription
type Subscription struct {
	CreatedAt time.Time              `json:"created_at"`
	LastSeen  time.Time              `json:"last_seen"`
	Filters   *EventFilter           `json:"filters,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	ID        string                 `json:"id"`
	Channel   string                 `json:"channel"`
	UserID    string                 `json:"user_id,omitempty"`
	OrgID     string                 `json:"org_id,omitempty"`
	ProjectID string                 `json:"project_id,omitempty"`
}

// Subscriber represents a client that receives events
type Subscriber interface {
	ID() string
	Send(event *Event) error
	Close() error
	Context() context.Context
}

// BroadcasterConfig represents broadcaster configuration
type BroadcasterConfig struct {
	BufferSize        int           `json:"buffer_size"`
	MaxSubscribers    int           `json:"max_subscribers"`
	MaxChannels       int           `json:"max_channels"`
	DefaultChannelTTL time.Duration `json:"default_channel_ttl"`
	CleanupInterval   time.Duration `json:"cleanup_interval"`
	SubscriberTimeout time.Duration `json:"subscriber_timeout"`
	EnableMetrics     bool          `json:"enable_metrics"`
	PersistentStorage bool          `json:"persistent_storage"`
	MaxEventHistory   int           `json:"max_event_history"`
}

// DefaultBroadcasterConfig returns a default broadcaster configuration
func DefaultBroadcasterConfig() *BroadcasterConfig {
	return &BroadcasterConfig{
		BufferSize:        1000,
		MaxSubscribers:    10000,
		MaxChannels:       1000,
		DefaultChannelTTL: 24 * time.Hour,
		CleanupInterval:   5 * time.Minute,
		SubscriberTimeout: 30 * time.Second,
		EnableMetrics:     true,
		PersistentStorage: false,
		MaxEventHistory:   100,
	}
}

// Broadcaster manages real-time event broadcasting
type Broadcaster struct {
	ctx          context.Context
	subChan      chan *Subscription
	subscribers  map[string]Subscriber
	channelSubs  map[string]map[string]*Subscription
	eventHistory map[string][]*Event
	eventChan    chan *Event
	config       *BroadcasterConfig
	unsubChan    chan string
	closeChan    chan struct{}
	channels     map[string]*Channel
	cancel       context.CancelFunc
	metrics      *BroadcasterMetrics
	wg           sync.WaitGroup
	mu           sync.RWMutex
}

// BroadcasterMetrics represents broadcaster metrics
type BroadcasterMetrics struct {
	LastUpdated       time.Time        `json:"last_updated"`
	ChannelStats      map[string]int64 `json:"channel_stats"`
	TotalChannels     int64            `json:"total_channels"`
	TotalSubscribers  int64            `json:"total_subscribers"`
	EventsSent        int64            `json:"events_sent"`
	EventsDropped     int64            `json:"events_dropped"`
	SubscriptionsRate float64          `json:"subscriptions_rate"`
	EventRate         float64          `json:"event_rate"`
	mu                sync.RWMutex     `json:"-"`
}

// NewBroadcaster creates a new event broadcaster
func NewBroadcaster(config *BroadcasterConfig) *Broadcaster {
	if config == nil {
		config = DefaultBroadcasterConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Broadcaster{
		config:       config,
		channels:     make(map[string]*Channel),
		subscribers:  make(map[string]Subscriber),
		channelSubs:  make(map[string]map[string]*Subscription),
		eventHistory: make(map[string][]*Event),
		eventChan:    make(chan *Event, config.BufferSize),
		subChan:      make(chan *Subscription, 100),
		unsubChan:    make(chan string, 100),
		closeChan:    make(chan struct{}),
		ctx:          ctx,
		cancel:       cancel,
		metrics: &BroadcasterMetrics{
			ChannelStats: make(map[string]int64),
			LastUpdated:  time.Now(),
		},
	}
}

// Start starts the broadcaster
func (b *Broadcaster) Start() error {
	b.wg.Add(3)
	go b.eventLoop()
	go b.subscriptionLoop()
	go b.cleanupLoop()

	return nil
}

// Stop stops the broadcaster
func (b *Broadcaster) Stop() error {
	b.cancel()
	close(b.closeChan)
	b.wg.Wait()
	return nil
}

// CreateChannel creates a new broadcast channel
func (b *Broadcaster) CreateChannel(name, description string, private, persistent bool) (*Channel, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.channels) >= b.config.MaxChannels {
		return nil, errors.New("maximum number of channels reached")
	}

	if _, exists := b.channels[name]; exists {
		return nil, fmt.Errorf("channel already exists: %s", name)
	}

	channel := &Channel{
		Name:        name,
		Description: description,
		Private:     private,
		Persistent:  persistent,
		TTL:         b.config.DefaultChannelTTL,
		Metadata:    make(map[string]any),
		CreatedAt:   time.Now(),
	}

	b.channels[name] = channel
	b.channelSubs[name] = make(map[string]*Subscription)

	if persistent && b.config.PersistentStorage {
		b.eventHistory[name] = make([]*Event, 0)
	}

	b.updateMetrics()
	return channel, nil
}

// DeleteChannel deletes a broadcast channel
func (b *Broadcaster) DeleteChannel(name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	_, exists := b.channels[name]
	if !exists {
		return fmt.Errorf("channel not found: %s", name)
	}

	// Unsubscribe all subscribers
	if subs, exists := b.channelSubs[name]; exists {
		for subID := range subs {
			b.unsubscribeInternal(subID)
		}
	}

	delete(b.channels, name)
	delete(b.channelSubs, name)
	delete(b.eventHistory, name)

	b.updateMetrics()
	return nil
}

// GetChannel returns a channel by name
func (b *Broadcaster) GetChannel(name string) (*Channel, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	channel, exists := b.channels[name]
	return channel, exists
}

// ListChannels returns all channels
func (b *Broadcaster) ListChannels() []*Channel {
	b.mu.RLock()
	defer b.mu.RUnlock()

	channels := make([]*Channel, 0, len(b.channels))
	for _, channel := range b.channels {
		channels = append(channels, channel)
	}
	return channels
}

// Subscribe subscribes a subscriber to a channel
func (b *Broadcaster) Subscribe(subscriber Subscriber, channelName string, filters *EventFilter) (*Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.subscribers) >= b.config.MaxSubscribers {
		return nil, errors.New("maximum number of subscribers reached")
	}

	channel, exists := b.channels[channelName]
	if !exists {
		return nil, fmt.Errorf("channel not found: %s", channelName)
	}

	// Check if channel is private and subscriber has access
	if channel.Private {
		// Implement access control logic here
		// For now, allow all subscriptions
	}

	subscription := &Subscription{
		ID:        subscriber.ID(),
		Channel:   channelName,
		Filters:   filters,
		Metadata:  make(map[string]any),
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
	}

	b.subscribers[subscriber.ID()] = subscriber
	b.channelSubs[channelName][subscriber.ID()] = subscription

	// Send event history if persistent channel
	if channel.Persistent && b.config.PersistentStorage {
		if history, exists := b.eventHistory[channelName]; exists {
			go b.sendEventHistory(subscriber, history, filters)
		}
	}

	b.updateMetrics()
	return subscription, nil
}

// Unsubscribe unsubscribes a subscriber from all channels
func (b *Broadcaster) Unsubscribe(subscriberID string) error {
	select {
	case b.unsubChan <- subscriberID:
		return nil
	case <-b.ctx.Done():
		return errors.New("broadcaster closed")
	}
}

// Broadcast broadcasts an event to a specific channel
func (b *Broadcaster) Broadcast(channelName string, event *Event) error {
	b.mu.RLock()
	channel, exists := b.channels[channelName]
	b.mu.RUnlock()

	if !exists {
		return fmt.Errorf("channel not found: %s", channelName)
	}

	// Apply channel filters if any
	if channel.Filters != nil && !channel.Filters.Matches(event) {
		return nil // Event filtered out
	}

	// Clone event and set channel info
	eventCopy := event.Clone()
	eventCopy.AddMetadata("channel", channelName)

	select {
	case b.eventChan <- eventCopy:
		return nil
	case <-b.ctx.Done():
		return errors.New("broadcaster closed")
	default:
		b.metrics.mu.Lock()
		b.metrics.EventsDropped++
		b.metrics.mu.Unlock()
		return errors.New("event buffer full")
	}
}

// BroadcastToAll broadcasts an event to all channels
func (b *Broadcaster) BroadcastToAll(event *Event) error {
	b.mu.RLock()
	channels := make([]string, 0, len(b.channels))
	for name := range b.channels {
		channels = append(channels, name)
	}
	b.mu.RUnlock()

	for _, channelName := range channels {
		if err := b.Broadcast(channelName, event); err != nil {
			// Log error but continue broadcasting to other channels
			continue
		}
	}

	return nil
}

// GetSubscription returns a subscription by ID
func (b *Broadcaster) GetSubscription(subscriberID string) (*Subscription, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, channelSubs := range b.channelSubs {
		if subscription, exists := channelSubs[subscriberID]; exists {
			return subscription, true
		}
	}
	return nil, false
}

// GetChannelSubscribers returns all subscribers for a channel
func (b *Broadcaster) GetChannelSubscribers(channelName string) []*Subscription {
	b.mu.RLock()
	defer b.mu.RUnlock()

	subs, exists := b.channelSubs[channelName]
	if !exists {
		return nil
	}

	subscriptions := make([]*Subscription, 0, len(subs))
	for _, subscription := range subs {
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions
}

// GetMetrics returns broadcaster metrics
func (b *Broadcaster) GetMetrics() *BroadcasterMetrics {
	b.metrics.mu.RLock()
	defer b.metrics.mu.RUnlock()

	// Create a copy of metrics
	metrics := &BroadcasterMetrics{
		TotalChannels:     b.metrics.TotalChannels,
		TotalSubscribers:  b.metrics.TotalSubscribers,
		EventsSent:        b.metrics.EventsSent,
		EventsDropped:     b.metrics.EventsDropped,
		SubscriptionsRate: b.metrics.SubscriptionsRate,
		EventRate:         b.metrics.EventRate,
		LastUpdated:       b.metrics.LastUpdated,
		ChannelStats:      make(map[string]int64),
	}

	for k, v := range b.metrics.ChannelStats {
		metrics.ChannelStats[k] = v
	}

	return metrics
}

// Internal methods

// eventLoop processes events
func (b *Broadcaster) eventLoop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			return
		case event := <-b.eventChan:
			b.processEvent(event)
		}
	}
}

// subscriptionLoop processes subscriptions
func (b *Broadcaster) subscriptionLoop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			return
		case subscriberID := <-b.unsubChan:
			b.unsubscribeInternal(subscriberID)
		}
	}
}

// cleanupLoop performs periodic cleanup
func (b *Broadcaster) cleanupLoop() {
	defer b.wg.Done()

	ticker := time.NewTicker(b.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.cleanup()
		}
	}
}

// processEvent processes and broadcasts an event
func (b *Broadcaster) processEvent(event *Event) {
	channelName, _ := event.GetMetadata("channel")
	channel, ok := channelName.(string)
	if !ok {
		return
	}

	b.mu.RLock()
	channelSubs, exists := b.channelSubs[channel]
	if !exists {
		b.mu.RUnlock()
		return
	}

	// Store event in history if persistent
	if b.channels[channel].Persistent && b.config.PersistentStorage {
		b.addToHistory(channel, event)
	}

	// Send to all subscribers
	subscribers := make([]Subscriber, 0, len(channelSubs))
	subscriptions := make([]*Subscription, 0, len(channelSubs))

	for _, subscription := range channelSubs {
		if subscriber, exists := b.subscribers[subscription.ID]; exists {
			// Apply subscription filters
			if subscription.Filters != nil && !subscription.Filters.Matches(event) {
				continue
			}
			subscribers = append(subscribers, subscriber)
			subscriptions = append(subscriptions, subscription)
		}
	}
	b.mu.RUnlock()

	// Send events concurrently
	var wg sync.WaitGroup
	for i, subscriber := range subscribers {
		wg.Add(1)
		go func(sub Subscriber, subscription *Subscription) {
			defer wg.Done()
			if err := sub.Send(event); err != nil {
				// Remove failed subscriber
				b.Unsubscribe(subscription.ID)
			} else {
				// Update last seen
				b.mu.Lock()
				subscription.LastSeen = time.Now()
				b.mu.Unlock()
			}
		}(subscriber, subscriptions[i])
	}
	wg.Wait()

	// Update metrics
	b.metrics.mu.Lock()
	b.metrics.EventsSent++
	b.metrics.ChannelStats[channel]++
	b.metrics.LastUpdated = time.Now()
	b.metrics.mu.Unlock()
}

// unsubscribeInternal handles unsubscription
func (b *Broadcaster) unsubscribeInternal(subscriberID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Remove from subscribers
	if subscriber, exists := b.subscribers[subscriberID]; exists {
		subscriber.Close()
		delete(b.subscribers, subscriberID)
	}

	// Remove from all channel subscriptions
	for _, channelSubs := range b.channelSubs {
		if _, exists := channelSubs[subscriberID]; exists {
			delete(channelSubs, subscriberID)
		}
	}

	b.updateMetrics()
}

// cleanup performs periodic cleanup
func (b *Broadcaster) cleanup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	// Remove expired channels
	for channelName, channel := range b.channels {
		if channel.TTL > 0 && now.Sub(channel.CreatedAt) > channel.TTL {
			delete(b.channels, channelName)
			delete(b.channelSubs, channelName)
			delete(b.eventHistory, channelName)
		}
	}

	// Remove inactive subscribers
	for subscriberID, subscriber := range b.subscribers {
		select {
		case <-subscriber.Context().Done():
			b.unsubscribeInternal(subscriberID)
		default:
			// Check if subscriber has been inactive
			if subscription, exists := b.GetSubscription(subscriberID); exists {
				if now.Sub(subscription.LastSeen) > b.config.SubscriberTimeout {
					b.unsubscribeInternal(subscriberID)
				}
			}
		}
	}

	b.updateMetrics()
}

// addToHistory adds an event to channel history
func (b *Broadcaster) addToHistory(channelName string, event *Event) {
	if history, exists := b.eventHistory[channelName]; exists {
		history = append(history, event)

		// Trim history if it exceeds max size
		if len(history) > b.config.MaxEventHistory {
			history = history[len(history)-b.config.MaxEventHistory:]
		}

		b.eventHistory[channelName] = history
	}
}

// sendEventHistory sends event history to a subscriber
func (b *Broadcaster) sendEventHistory(subscriber Subscriber, history []*Event, filters *EventFilter) {
	for _, event := range history {
		if filters != nil && !filters.Matches(event) {
			continue
		}

		if err := subscriber.Send(event); err != nil {
			break
		}
	}
}

// updateMetrics updates broadcaster metrics
func (b *Broadcaster) updateMetrics() {
	b.metrics.mu.Lock()
	defer b.metrics.mu.Unlock()

	b.metrics.TotalChannels = int64(len(b.channels))
	b.metrics.TotalSubscribers = int64(len(b.subscribers))
	b.metrics.LastUpdated = time.Now()
}
