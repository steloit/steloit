package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ClientConfig represents WebSocket client configuration
type ClientConfig struct {
	URL            string        `json:"url"`
	Subprotocols   []string      `json:"subprotocols"`
	Headers        http.Header   `json:"headers"`
	PingInterval   time.Duration `json:"ping_interval"`
	PongTimeout    time.Duration `json:"pong_timeout"`
	ReconnectDelay time.Duration `json:"reconnect_delay"`
	MaxReconnects  int           `json:"max_reconnects"`
	BufferSize     int           `json:"buffer_size"`
	Compression    bool          `json:"compression"`
	EnablePing     bool          `json:"enable_ping"`
	AutoReconnect  bool          `json:"auto_reconnect"`
}

// DefaultClientConfig returns a default client configuration
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		PingInterval:   30 * time.Second,
		PongTimeout:    10 * time.Second,
		ReconnectDelay: 5 * time.Second,
		MaxReconnects:  5,
		BufferSize:     256,
		Compression:    false,
		EnablePing:     true,
		AutoReconnect:  true,
	}
}

// ConnectionState represents the connection state
type ConnectionState int

const (
	StateConnecting ConnectionState = iota
	StateConnected
	StateReconnecting
	StateDisconnected
	StateFailed
)

func (s ConnectionState) String() string {
	switch s {
	case StateConnecting:
		return "connecting"
	case StateConnected:
		return "connected"
	case StateReconnecting:
		return "reconnecting"
	case StateDisconnected:
		return "disconnected"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// EventHandler represents a WebSocket event handler
type EventHandler func(event string, data []byte) error

// ErrorHandler represents a WebSocket error handler
type ErrorHandler func(err error)

// StateChangeHandler represents a connection state change handler
type StateChangeHandler func(oldState, newState ConnectionState)

// Client represents a WebSocket client
type Client struct {
	config        *ClientConfig
	conn          *websocket.Conn
	dialer        *websocket.Dialer
	state         ConnectionState
	stateMutex    sync.RWMutex
	eventHandlers map[string]EventHandler
	errorHandler  ErrorHandler
	stateHandler  StateChangeHandler
	sendChan      chan []byte
	receiveChan   chan []byte
	closeChan     chan struct{}
	reconnectChan chan struct{}
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	reconnects    int
}

// NewClient creates a new WebSocket client
func NewClient(config *ClientConfig) *Client {
	if config == nil {
		config = DefaultClientConfig()
	}

	dialer := &websocket.Dialer{
		Subprotocols:    config.Subprotocols,
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	if config.Compression {
		dialer.EnableCompression = true
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		config:        config,
		dialer:        dialer,
		state:         StateDisconnected,
		eventHandlers: make(map[string]EventHandler),
		sendChan:      make(chan []byte, config.BufferSize),
		receiveChan:   make(chan []byte, config.BufferSize),
		closeChan:     make(chan struct{}),
		reconnectChan: make(chan struct{}),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Connect establishes a WebSocket connection
func (c *Client) Connect() error {
	c.setState(StateConnecting)

	conn, resp, err := c.dialer.DialContext(c.ctx, c.config.URL, c.config.Headers)
	if err != nil {
		c.setState(StateFailed)
		if resp != nil {
			return fmt.Errorf("websocket connection failed with status %d: %w", resp.StatusCode, err)
		}
		return fmt.Errorf("websocket connection failed: %w", err)
	}

	c.conn = conn
	c.setState(StateConnected)
	c.reconnects = 0

	// Start goroutines for handling connection
	c.wg.Add(3)
	go c.readLoop()
	go c.writeLoop()
	go c.pingLoop()

	return nil
}

// Disconnect closes the WebSocket connection
func (c *Client) Disconnect() error {
	c.cancel()

	if c.conn != nil {
		// Send close message
		err := c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		if err != nil {
			return err
		}

		// Close connection
		err = c.conn.Close()
		if err != nil {
			return err
		}
	}

	c.setState(StateDisconnected)
	c.wg.Wait()

	return nil
}

// Send sends data through the WebSocket connection
func (c *Client) Send(data []byte) error {
	if c.GetState() != StateConnected {
		return fmt.Errorf("websocket not connected")
	}

	select {
	case c.sendChan <- data:
		return nil
	case <-c.ctx.Done():
		return fmt.Errorf("websocket client closed")
	default:
		return fmt.Errorf("send buffer full")
	}
}

// SendMessage sends a message through the WebSocket connection
func (c *Client) SendMessage(message *Message) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return c.Send(data)
}

// SendText sends a text message
func (c *Client) SendText(text string) error {
	return c.Send([]byte(text))
}

// SendJSON sends a JSON message
func (c *Client) SendJSON(data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	return c.Send(jsonData)
}

// OnEvent registers an event handler
func (c *Client) OnEvent(event string, handler EventHandler) {
	c.eventHandlers[event] = handler
}

// OnError registers an error handler
func (c *Client) OnError(handler ErrorHandler) {
	c.errorHandler = handler
}

// OnStateChange registers a state change handler
func (c *Client) OnStateChange(handler StateChangeHandler) {
	c.stateHandler = handler
}

// GetState returns the current connection state
func (c *Client) GetState() ConnectionState {
	c.stateMutex.RLock()
	defer c.stateMutex.RUnlock()
	return c.state
}

// IsConnected returns true if the client is connected
func (c *Client) IsConnected() bool {
	return c.GetState() == StateConnected
}

// setState sets the connection state and triggers state change handler
func (c *Client) setState(newState ConnectionState) {
	c.stateMutex.Lock()
	oldState := c.state
	c.state = newState
	c.stateMutex.Unlock()

	if c.stateHandler != nil && oldState != newState {
		c.stateHandler(oldState, newState)
	}
}

// readLoop handles incoming messages
func (c *Client) readLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			if c.conn == nil {
				return
			}

			messageType, data, err := c.conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					if c.errorHandler != nil {
						c.errorHandler(fmt.Errorf("websocket read error: %w", err))
					}
				}

				if c.config.AutoReconnect && c.reconnects < c.config.MaxReconnects {
					c.setState(StateReconnecting)
					c.reconnectChan <- struct{}{}
				} else {
					c.setState(StateFailed)
				}
				return
			}

			switch messageType {
			case websocket.TextMessage, websocket.BinaryMessage:
				c.handleMessage(data)
			case websocket.PongMessage:
				// Pong received - connection is alive
			case websocket.CloseMessage:
				c.setState(StateDisconnected)
				return
			}
		}
	}
}

// writeLoop handles outgoing messages
func (c *Client) writeLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case data := <-c.sendChan:
			if c.conn == nil {
				continue
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				if c.errorHandler != nil {
					c.errorHandler(fmt.Errorf("websocket write error: %w", err))
				}
				return
			}
		}
	}
}

// pingLoop handles ping/pong for keeping connection alive
func (c *Client) pingLoop() {
	defer c.wg.Done()

	if !c.config.EnablePing {
		return
	}

	ticker := time.NewTicker(c.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if c.conn == nil || c.GetState() != StateConnected {
				continue
			}

			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				if c.errorHandler != nil {
					c.errorHandler(fmt.Errorf("websocket ping error: %w", err))
				}
				return
			}
		}
	}
}

// handleMessage processes incoming messages
func (c *Client) handleMessage(data []byte) {
	// Try to parse as Message struct first
	var message Message
	if err := json.Unmarshal(data, &message); err == nil && message.Type != "" {
		if handler, exists := c.eventHandlers[string(message.Type)]; exists {
			if err := handler(string(message.Type), data); err != nil && c.errorHandler != nil {
				c.errorHandler(fmt.Errorf("event handler error: %w", err))
			}
		}
		return
	}

	// If not a structured message, trigger generic message handler
	if handler, exists := c.eventHandlers["message"]; exists {
		if err := handler("message", data); err != nil && c.errorHandler != nil {
			c.errorHandler(fmt.Errorf("message handler error: %w", err))
		}
	}
}

// reconnect attempts to reconnect to the WebSocket
func (c *Client) reconnect() {
	c.reconnects++

	time.Sleep(c.config.ReconnectDelay)

	if err := c.Connect(); err != nil {
		if c.errorHandler != nil {
			c.errorHandler(fmt.Errorf("reconnect failed: %w", err))
		}

		if c.reconnects < c.config.MaxReconnects {
			go c.reconnect()
		} else {
			c.setState(StateFailed)
		}
	}
}

// GetConnectionInfo returns connection information
func (c *Client) GetConnectionInfo() map[string]any {
	return map[string]any{
		"url":           c.config.URL,
		"state":         c.GetState().String(),
		"reconnects":    c.reconnects,
		"connected":     c.IsConnected(),
		"ping_interval": c.config.PingInterval.String(),
	}
}

// SetReadDeadline sets read deadline for the connection
func (c *Client) SetReadDeadline(t time.Time) error {
	if c.conn == nil {
		return fmt.Errorf("websocket not connected")
	}
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets write deadline for the connection
func (c *Client) SetWriteDeadline(t time.Time) error {
	if c.conn == nil {
		return fmt.Errorf("websocket not connected")
	}
	return c.conn.SetWriteDeadline(t)
}

// GetLocalAddr returns the local network address
func (c *Client) GetLocalAddr() string {
	if c.conn == nil {
		return ""
	}
	return c.conn.LocalAddr().String()
}

// GetRemoteAddr returns the remote network address
func (c *Client) GetRemoteAddr() string {
	if c.conn == nil {
		return ""
	}
	return c.conn.RemoteAddr().String()
}
