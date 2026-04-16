package email

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// sanitizeHeaderValue removes CR and LF characters to prevent email header injection attacks.
// This is critical for security as CRLF sequences can be used to inject arbitrary headers.
func sanitizeHeaderValue(value string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}

// sanitizeEmailAddress sanitizes an email address for use in headers.
// Wraps sanitizeHeaderValue for semantic clarity.
func sanitizeEmailAddress(addr string) string {
	return sanitizeHeaderValue(addr)
}

// generateBoundary creates a cryptographically random MIME boundary string.
// Using crypto/rand instead of time-based values prevents boundary prediction attacks.
func generateBoundary() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback to time-based if crypto/rand fails (should never happen)
		return fmt.Sprintf("----=_Part_%d", time.Now().UnixNano())
	}
	return "----=_Part_" + hex.EncodeToString(b)
}

// SMTPConfig contains configuration for the SMTP client
type SMTPConfig struct {
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
	FromName  string
	UseTLS    bool // Use STARTTLS
	Timeout   time.Duration
}

// SMTPClient implements EmailSender using SMTP
type SMTPClient struct {
	host      string
	port      int
	username  string
	password  string
	fromEmail string
	fromName  string
	useTLS    bool
	timeout   time.Duration
}

// NewSMTPClient creates a new SMTP client
func NewSMTPClient(cfg SMTPConfig) *SMTPClient {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &SMTPClient{
		host:      cfg.Host,
		port:      cfg.Port,
		username:  cfg.Username,
		password:  cfg.Password,
		fromEmail: cfg.FromEmail,
		fromName:  cfg.FromName,
		useTLS:    cfg.UseTLS,
		timeout:   timeout,
	}
}

// Send sends an email using SMTP
func (c *SMTPClient) Send(ctx context.Context, params SendEmailParams) error {
	if len(params.To) == 0 {
		return fmt.Errorf("email: recipient list is empty")
	}
	if params.Subject == "" {
		return fmt.Errorf("email: subject is required")
	}
	if params.HTML == "" && params.Text == "" {
		return fmt.Errorf("email: either HTML or Text content is required")
	}

	// Build the message
	msg := c.buildMessage(params)

	// Get server address
	addr := net.JoinHostPort(c.host, strconv.Itoa(c.port))

	// Create a channel for the result
	done := make(chan error, 1)

	go func() {
		done <- c.sendMail(addr, params.To, msg)
	}()

	// Wait for completion or context cancellation
	select {
	case <-ctx.Done():
		return fmt.Errorf("email: send cancelled: %w", ctx.Err())
	case err := <-done:
		return err
	}
}

// sendMail handles the actual SMTP connection and sending
func (c *SMTPClient) sendMail(addr string, to []string, msg []byte) error {
	// Connect to SMTP server
	conn, err := net.DialTimeout("tcp", addr, c.timeout)
	if err != nil {
		return fmt.Errorf("email: failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	// Create SMTP client
	client, err := smtp.NewClient(conn, c.host)
	if err != nil {
		return fmt.Errorf("email: failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Send EHLO/HELO
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("email: HELO failed: %w", err)
	}

	// Upgrade to TLS if requested and supported
	if c.useTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				ServerName: c.host,
				MinVersion: tls.VersionTLS12,
			}
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("email: STARTTLS failed: %w", err)
			}
		}
	}

	// Authenticate if credentials provided
	if c.username != "" && c.password != "" {
		auth := smtp.PlainAuth("", c.username, c.password, c.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("email: authentication failed: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(c.fromEmail); err != nil {
		return fmt.Errorf("email: MAIL FROM failed: %w", err)
	}

	// Set recipients
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return fmt.Errorf("email: RCPT TO failed for %s: %w", recipient, err)
		}
	}

	// Send message body
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email: DATA command failed: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("email: failed to write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("email: failed to close message: %w", err)
	}

	// Quit gracefully
	return client.Quit()
}

// buildMessage constructs the email message with headers and body
func (c *SMTPClient) buildMessage(params SendEmailParams) []byte {
	var sb strings.Builder

	// Sanitize all header values to prevent injection attacks (RFC 5322 compliance)
	sanitizedFromName := sanitizeHeaderValue(c.fromName)
	sanitizedFromEmail := sanitizeEmailAddress(c.fromEmail)
	sanitizedSubject := sanitizeHeaderValue(params.Subject)
	sanitizedReplyTo := sanitizeEmailAddress(params.ReplyTo)

	// Sanitize all recipient addresses
	sanitizedTo := make([]string, len(params.To))
	for i, addr := range params.To {
		sanitizedTo[i] = sanitizeEmailAddress(addr)
	}

	// Build From header
	if sanitizedFromName != "" {
		sb.WriteString(fmt.Sprintf("From: %s <%s>\r\n", sanitizedFromName, sanitizedFromEmail))
	} else {
		sb.WriteString(fmt.Sprintf("From: %s\r\n", sanitizedFromEmail))
	}

	// Build To header
	sb.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(sanitizedTo, ", ")))

	// Subject
	sb.WriteString(fmt.Sprintf("Subject: %s\r\n", sanitizedSubject))

	// Reply-To
	if sanitizedReplyTo != "" {
		sb.WriteString(fmt.Sprintf("Reply-To: %s\r\n", sanitizedReplyTo))
	}

	// MIME headers
	sb.WriteString("MIME-Version: 1.0\r\n")

	if params.HTML != "" && params.Text != "" {
		// Multipart message with both HTML and plain text
		boundary := generateBoundary()
		sb.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
		sb.WriteString("\r\n")

		// Plain text part
		sb.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		sb.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(params.Text)
		sb.WriteString("\r\n")

		// HTML part
		sb.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		sb.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(params.HTML)
		sb.WriteString("\r\n")

		// End boundary
		sb.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else if params.HTML != "" {
		// HTML only
		sb.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(params.HTML)
	} else {
		// Plain text only
		sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		sb.WriteString("\r\n")
		sb.WriteString(params.Text)
	}

	return []byte(sb.String())
}

// IsConfigured returns true if the client has required configuration
func (c *SMTPClient) IsConfigured() bool {
	return c.host != "" && c.port > 0 && c.fromEmail != ""
}
