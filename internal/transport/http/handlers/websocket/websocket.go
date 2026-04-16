package websocket

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"brokle/internal/config"
	_ "brokle/pkg/response" // Import for Swagger documentation types
)

// Handler handles WebSocket connections
type Handler struct {
	config   *config.Config
	logger   *slog.Logger
	upgrader websocket.Upgrader
	hub      *Hub
}

// NewHandler creates a new WebSocket handler
func NewHandler(config *config.Config, logger *slog.Logger) *Handler {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			// TODO: Implement proper origin checking based on config
			return true
		},
	}

	hub := NewHub(logger)
	go hub.Run()

	return &Handler{
		config:   config,
		logger:   logger,
		upgrader: upgrader,
		hub:      hub,
	}
}

// Handle handles WebSocket connection requests
// @Summary Establish WebSocket connection
// @Description Upgrade HTTP connection to WebSocket for real-time updates and notifications
// @Tags WebSocket
// @Accept json
// @Produce json
// @Success 101 {string} string "WebSocket connection established"
// @Failure 400 {object} response.ErrorResponse "Bad request - WebSocket upgrade failed"
// @Failure 401 {object} response.ErrorResponse "Unauthorized - authentication required"
// @Failure 403 {object} response.ErrorResponse "Forbidden - insufficient permissions"
// @Failure 500 {object} response.ErrorResponse "Internal server error"
// @Security BearerAuth
// @Router /ws [get]
func (h *Handler) Handle(c *gin.Context) {
	w := c.Writer
	r := c.Request
	// Upgrade HTTP connection to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("Failed to upgrade WebSocket connection", "error", err)
		return
	}

	// Get user context (should be set by authentication middleware)
	userID := h.getUserID(r)
	if userID == "" {
		h.logger.Warn("WebSocket connection attempted without authentication")
		conn.Close()
		return
	}

	// Create new client
	client := &Client{
		ID:     generateClientID(),
		UserID: userID,
		Conn:   conn,
		Send:   make(chan []byte, 256),
		hub:    h.hub,
		logger: h.logger,
	}

	h.logger.Info("New WebSocket connection established", "client_id", client.ID, "user_id", userID)

	// Register client with hub
	h.hub.register <- client

	// Start client goroutines
	go client.writePump()
	go client.readPump()
}

// BroadcastToUser broadcasts a message to all connections of a specific user
func (h *Handler) BroadcastToUser(userID string, messageType string, data interface{}) {
	message := Message{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}

	h.hub.BroadcastToUser(userID, message)
}

// BroadcastToOrganization broadcasts a message to all users in an organization
func (h *Handler) BroadcastToOrganization(orgID string, messageType string, data interface{}) {
	message := Message{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}

	h.hub.BroadcastToOrganization(orgID, message)
}

// BroadcastToProject broadcasts a message to all users in a project
func (h *Handler) BroadcastToProject(projectID string, messageType string, data interface{}) {
	message := Message{
		Type:      messageType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}

	h.hub.BroadcastToProject(projectID, message)
}

// getUserID extracts user ID from request context
func (h *Handler) getUserID(r *http.Request) string {
	// Try to get user from JWT context
	if user := r.Context().Value("user"); user != nil {
		if userMap, ok := user.(map[string]interface{}); ok {
			if userID, ok := userMap["id"].(string); ok {
				return userID
			}
		}
	}

	// Try to get user from API key context
	if apiKey := r.Context().Value("api_key"); apiKey != nil {
		if keyMap, ok := apiKey.(map[string]interface{}); ok {
			if userID, ok := keyMap["user_id"].(string); ok {
				return userID
			}
		}
	}

	return ""
}

// Hub maintains the set of active clients and broadcasts messages to them
type Hub struct {
	clients     map[*Client]bool
	userClients map[string]map[*Client]bool // userID -> clients
	broadcast   chan []byte
	register    chan *Client
	unregister  chan *Client
	logger      *slog.Logger
	mu          sync.RWMutex
}

// NewHub creates a new WebSocket hub
func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		clients:     make(map[*Client]bool),
		userClients: make(map[string]map[*Client]bool),
		broadcast:   make(chan []byte),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		logger:      logger,
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

// registerClient registers a new client
func (h *Hub) registerClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.clients[client] = true

	// Add to user clients map
	if h.userClients[client.UserID] == nil {
		h.userClients[client.UserID] = make(map[*Client]bool)
	}
	h.userClients[client.UserID][client] = true

	h.logger.Info("Client registered", "client_id", client.ID, "user_id", client.UserID, "total_clients", len(h.clients))

	// Send welcome message
	welcomeMsg := Message{
		Type:      "connection",
		Data:      map[string]string{"status": "connected"},
		Timestamp: time.Now().UTC(),
	}
	client.SendMessage(welcomeMsg)
}

// unregisterClient unregisters a client
func (h *Hub) unregisterClient(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)

		// Remove from user clients map
		if userClients, exists := h.userClients[client.UserID]; exists {
			delete(userClients, client)
			if len(userClients) == 0 {
				delete(h.userClients, client.UserID)
			}
		}

		close(client.Send)

		h.logger.Info("Client unregistered", "client_id", client.ID, "user_id", client.UserID, "total_clients", len(h.clients))
	}
}

// broadcastMessage broadcasts a message to all clients
func (h *Hub) broadcastMessage(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		select {
		case client.Send <- message:
		default:
			close(client.Send)
			delete(h.clients, client)
		}
	}
}

// BroadcastToUser broadcasts a message to all connections of a specific user
func (h *Hub) BroadcastToUser(userID string, message Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	userClients, exists := h.userClients[userID]
	if !exists {
		return
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		h.logger.Error("Failed to marshal WebSocket message", "error", err)
		return
	}

	for client := range userClients {
		select {
		case client.Send <- messageBytes:
		default:
			close(client.Send)
			delete(h.clients, client)
			delete(userClients, client)
		}
	}
}

// BroadcastToOrganization broadcasts to all users in an organization
func (h *Hub) BroadcastToOrganization(orgID string, message Message) {
	// TODO: Implement organization-based broadcasting
	// This would require storing organization membership for each client
	h.logger.Debug("Organization broadcast not yet implemented", "org_id", orgID)
}

// BroadcastToProject broadcasts to all users in a project
func (h *Hub) BroadcastToProject(projectID string, message Message) {
	// TODO: Implement project-based broadcasting
	// This would require storing project membership for each client
	h.logger.Debug("Project broadcast not yet implemented", "project_id", projectID)
}

// GetConnectedUsers returns a list of currently connected user IDs
func (h *Hub) GetConnectedUsers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	users := make([]string, 0, len(h.userClients))
	for userID := range h.userClients {
		users = append(users, userID)
	}

	return users
}

// GetClientCount returns the total number of connected clients
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.clients)
}

// Client represents a WebSocket client
type Client struct {
	ID     string
	UserID string
	Conn   *websocket.Conn
	Send   chan []byte
	hub    *Hub
	logger *slog.Logger
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

// readPump pumps messages from the WebSocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				c.logger.Error("Unexpected WebSocket close error", "error", err)
			}
			break
		}

		// Handle incoming message
		c.handleMessage(message)
	}
}

// writePump pumps messages from the hub to the WebSocket connection
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current write
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage handles incoming messages from client
func (c *Client) handleMessage(message []byte) {
	var msg Message
	if err := json.Unmarshal(message, &msg); err != nil {
		c.logger.Error("Failed to unmarshal WebSocket message", "error", err)
		return
	}

	c.logger.Debug("Received WebSocket message", "client_id", c.ID, "type", msg.Type)

	// Handle different message types
	switch msg.Type {
	case "ping":
		c.SendMessage(Message{
			Type:      "pong",
			Data:      map[string]string{"status": "ok"},
			Timestamp: time.Now().UTC(),
		})

	case "subscribe":
		// TODO: Implement subscription management
		c.logger.Debug("Subscription management not yet implemented")

	case "unsubscribe":
		// TODO: Implement subscription management
		c.logger.Debug("Subscription management not yet implemented")

	default:
		c.logger.Warn("Unknown message type", "type", msg.Type)
	}
}

// SendMessage sends a message to the client
func (c *Client) SendMessage(message Message) {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		c.logger.Error("Failed to marshal message", "error", err)
		return
	}

	select {
	case c.Send <- messageBytes:
	default:
		close(c.Send)
	}
}

// Message represents a WebSocket message
type Message struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// Helper functions

func generateClientID() string {
	return fmt.Sprintf("client_%d", time.Now().UnixNano())
}
