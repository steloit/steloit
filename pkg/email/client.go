// Package email provides email sending capabilities using various providers.
package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmailSender defines the interface for sending emails
type EmailSender interface {
	Send(ctx context.Context, params SendEmailParams) error
}

// SendEmailParams contains all parameters for sending an email
type SendEmailParams struct {
	To      []string          // Recipient email addresses
	Subject string            // Email subject
	HTML    string            // HTML content
	Text    string            // Plain text content (optional fallback)
	ReplyTo string            // Reply-to address (optional)
	Tags    map[string]string // Email tags for analytics (optional)
}

// ResendClient implements EmailSender using the Resend API
type ResendClient struct {
	apiKey     string
	fromEmail  string
	fromName   string
	httpClient *http.Client
	baseURL    string
}

// ResendConfig contains configuration for the Resend client
type ResendConfig struct {
	APIKey    string
	FromEmail string
	FromName  string
	Timeout   time.Duration
}

// NewResendClient creates a new Resend API client
func NewResendClient(cfg ResendConfig) *ResendClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &ResendClient{
		apiKey:    cfg.APIKey,
		fromEmail: cfg.FromEmail,
		fromName:  cfg.FromName,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		baseURL: "https://api.resend.com",
	}
}

// resendRequest is the request body for Resend API
type resendRequest struct {
	From    string      `json:"from"`
	To      []string    `json:"to"`
	Subject string      `json:"subject"`
	HTML    string      `json:"html,omitempty"`
	Text    string      `json:"text,omitempty"`
	ReplyTo string      `json:"reply_to,omitempty"`
	Tags    []resendTag `json:"tags,omitempty"`
}

type resendTag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// resendResponse is the response from Resend API
type resendResponse struct {
	ID string `json:"id"`
}

// resendError is the error response from Resend API
type resendError struct {
	StatusCode int    `json:"statusCode"`
	Name       string `json:"name"`
	Message    string `json:"message"`
}

// Send sends an email using the Resend API
func (c *ResendClient) Send(ctx context.Context, params SendEmailParams) error {
	if len(params.To) == 0 {
		return fmt.Errorf("email: recipient list is empty")
	}
	if params.Subject == "" {
		return fmt.Errorf("email: subject is required")
	}
	if params.HTML == "" && params.Text == "" {
		return fmt.Errorf("email: either HTML or Text content is required")
	}

	// Build from address
	from := c.fromEmail
	if c.fromName != "" {
		from = fmt.Sprintf("%s <%s>", c.fromName, c.fromEmail)
	}

	// Build request
	req := resendRequest{
		From:    from,
		To:      params.To,
		Subject: params.Subject,
		HTML:    params.HTML,
		Text:    params.Text,
		ReplyTo: params.ReplyTo,
	}

	// Convert tags
	if len(params.Tags) > 0 {
		req.Tags = make([]resendTag, 0, len(params.Tags))
		for k, v := range params.Tags {
			req.Tags = append(req.Tags, resendTag{Name: k, Value: v})
		}
	}

	// Marshal request body
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("email: failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("email: failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("email: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("email: failed to read response: %w", err)
	}

	// Check for errors
	if resp.StatusCode >= 400 {
		var errResp resendError
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return fmt.Errorf("email: Resend API error (%d): %s - %s", errResp.StatusCode, errResp.Name, errResp.Message)
		}
		return fmt.Errorf("email: Resend API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// IsConfigured returns true if the client has required configuration
func (c *ResendClient) IsConfigured() bool {
	return c.apiKey != "" && c.fromEmail != ""
}

// NoOpEmailSender is a no-op implementation for development/testing
type NoOpEmailSender struct{}

// Send does nothing (for development without email configured)
func (n *NoOpEmailSender) Send(ctx context.Context, params SendEmailParams) error {
	// Log would go here in a real implementation
	return nil
}
