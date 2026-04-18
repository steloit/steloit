package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"brokle/internal/config"
)

// NotificationWorker handles async notification processing
type NotificationWorker struct {
	config *config.Config
	logger *slog.Logger
	queue  chan NotificationJob
	quit   chan struct{}
	wg     sync.WaitGroup
}

// NotificationJob represents a notification processing job
type NotificationJob struct {
	Timestamp time.Time   `json:"timestamp"`
	Data      any `json:"data"`
	Type      string      `json:"type"`
	Retry     int         `json:"retry"`
}

// EmailJob represents an email notification job
type EmailJob struct {
	TemplateData map[string]any `json:"template_data,omitempty"`
	Subject      string                 `json:"subject"`
	Body         string                 `json:"body"`
	BodyHTML     string                 `json:"body_html,omitempty"`
	Template     string                 `json:"template,omitempty"`
	Priority     string                 `json:"priority,omitempty"`
	UserID       string                 `json:"user_id,omitempty"`
	To           []string               `json:"to"`
	CC           []string               `json:"cc,omitempty"`
	BCC          []string               `json:"bcc,omitempty"`
}

// WebhookJob represents a webhook notification job
type WebhookJob struct {
	Headers    map[string]string      `json:"headers,omitempty"`
	Payload    map[string]any `json:"payload"`
	URL        string                 `json:"url"`
	Method     string                 `json:"method"`
	UserID     string                 `json:"user_id,omitempty"`
	EventType  string                 `json:"event_type,omitempty"`
	Timeout    int                    `json:"timeout,omitempty"`
	RetryCount int                    `json:"retry_count,omitempty"`
}

// SlackJob represents a Slack notification job
type SlackJob struct {
	Channel   string                   `json:"channel"`
	Message   string                   `json:"message"`
	Username  string                   `json:"username,omitempty"`
	IconEmoji string                   `json:"icon_emoji,omitempty"`
	UserID    string                   `json:"user_id,omitempty"`
	EventType string                   `json:"event_type,omitempty"`
	Blocks    []map[string]any `json:"blocks,omitempty"`
}

// SMSJob represents an SMS notification job
type SMSJob struct {
	To      string `json:"to"`
	Message string `json:"message"`
	UserID  string `json:"user_id,omitempty"`
}

// PushJob represents a push notification job
type PushJob struct {
	Data         map[string]any `json:"data,omitempty"`
	Title        string                 `json:"title"`
	Body         string                 `json:"body"`
	Sound        string                 `json:"sound,omitempty"`
	UserID       string                 `json:"user_id,omitempty"`
	DeviceTokens []string               `json:"device_tokens"`
	Badge        int                    `json:"badge,omitempty"`
}

// NewNotificationWorker creates a new notification worker
func NewNotificationWorker(
	config *config.Config,
	logger *slog.Logger,
) *NotificationWorker {
	return &NotificationWorker{
		config: config,
		logger: logger,
		queue:  make(chan NotificationJob, 500), // Buffer for 500 notifications
		quit:   make(chan struct{}),
	}
}

// Start starts the notification worker
func (w *NotificationWorker) Start() {
	w.logger.Info("Starting notification worker")

	// Start multiple worker goroutines
	numWorkers := w.config.Workers.NotificationWorkers
	if numWorkers == 0 {
		numWorkers = 2 // Default
	}

	for i := range numWorkers {
		w.wg.Add(1)
		go w.worker(i)
	}
}

// Stop stops the notification worker and waits for graceful shutdown
func (w *NotificationWorker) Stop() {
	w.logger.Info("Stopping notification worker")
	close(w.quit)
	w.wg.Wait()
}

// QueueJob queues a notification job for processing
func (w *NotificationWorker) QueueJob(jobType string, data any) {
	job := NotificationJob{
		Type:      jobType,
		Data:      data,
		Timestamp: time.Now(),
		Retry:     0,
	}

	select {
	case w.queue <- job:
		w.logger.Debug("Notification job queued", "type", jobType)
	default:
		w.logger.Warn("Notification queue full, dropping job", "type", jobType)
	}
}

// QueueEmail queues an email notification
func (w *NotificationWorker) QueueEmail(email EmailJob) {
	w.QueueJob("email", email)
}

// QueueWebhook queues a webhook notification
func (w *NotificationWorker) QueueWebhook(webhook WebhookJob) {
	w.QueueJob("webhook", webhook)
}

// QueueSlack queues a Slack notification
func (w *NotificationWorker) QueueSlack(slack SlackJob) {
	w.QueueJob("slack", slack)
}

// QueueSMS queues an SMS notification
func (w *NotificationWorker) QueueSMS(sms SMSJob) {
	w.QueueJob("sms", sms)
}

// QueuePush queues a push notification
func (w *NotificationWorker) QueuePush(push PushJob) {
	w.QueueJob("push", push)
}

// worker processes jobs from the queue
func (w *NotificationWorker) worker(id int) {
	defer w.wg.Done()
	w.logger.Info("Notification worker started", "worker_id", id)

	for {
		select {
		case job := <-w.queue:
			w.processJob(id, job)

		case <-w.quit:
			w.logger.Info("Notification worker stopping", "worker_id", id)
			return
		}
	}
}

// processJob processes a single notification job
func (w *NotificationWorker) processJob(workerID int, job NotificationJob) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := w.logger

	logger.Debug("Processing notification job")

	var err error
	switch job.Type {
	case "email":
		err = w.processEmail(ctx, job.Data)
	case "webhook":
		err = w.processWebhook(ctx, job.Data)
	case "slack":
		err = w.processSlack(ctx, job.Data)
	case "sms":
		err = w.processSMS(ctx, job.Data)
	case "push":
		err = w.processPush(ctx, job.Data)
	default:
		logger.Warn("Unknown notification job type")
		return
	}

	if err != nil {
		logger.Error("Failed to process notification job", "error", err)

		// Retry logic with exponential backoff
		if job.Retry < 3 {
			job.Retry++
			delay := time.Duration(job.Retry*job.Retry) * time.Minute
			time.Sleep(delay)
			w.queue <- job
			logger.Info("Retrying notification job", "retry", job.Retry)
		} else {
			logger.Error("Max retries exceeded, dropping notification job")
		}
	} else {
		logger.Debug("Notification job processed successfully")
	}
}

// processEmail processes an email notification
func (w *NotificationWorker) processEmail(ctx context.Context, data any) error {
	jobData, ok := data.(EmailJob)
	if !ok {
		// Try to unmarshal if it's a map
		if mapData, ok := data.(map[string]any); ok {
			jsonData, err := json.Marshal(mapData)
			if err != nil {
				return fmt.Errorf("failed to marshal email data: %w", err)
			}
			if err := json.Unmarshal(jsonData, &jobData); err != nil {
				return fmt.Errorf("failed to unmarshal email data: %w", err)
			}
		} else {
			return errors.New("invalid email data type")
		}
	}

	w.logger.Info("Sending email", "to", jobData.To, "subject", jobData.Subject, "template", jobData.Template)

	// TODO: Implement actual email sending logic
	// This would integrate with services like SendGrid, AWS SES, etc.

	// Simulate email sending
	time.Sleep(100 * time.Millisecond)

	w.logger.Info("Email sent successfully", "to", jobData.To)
	return nil
}

// processWebhook processes a webhook notification
func (w *NotificationWorker) processWebhook(ctx context.Context, data any) error {
	jobData, ok := data.(WebhookJob)
	if !ok {
		// Try to unmarshal if it's a map
		if mapData, ok := data.(map[string]any); ok {
			jsonData, err := json.Marshal(mapData)
			if err != nil {
				return fmt.Errorf("failed to marshal webhook data: %w", err)
			}
			if err := json.Unmarshal(jsonData, &jobData); err != nil {
				return fmt.Errorf("failed to unmarshal webhook data: %w", err)
			}
		} else {
			return errors.New("invalid webhook data type")
		}
	}

	w.logger.Info("Sending webhook", "url", jobData.URL, "method", jobData.Method, "event_type", jobData.EventType)

	// TODO: Implement actual webhook sending logic
	// This would make HTTP requests to the specified URLs

	// Simulate webhook sending
	time.Sleep(200 * time.Millisecond)

	w.logger.Info("Webhook sent successfully", "url", jobData.URL)
	return nil
}

// processSlack processes a Slack notification
func (w *NotificationWorker) processSlack(ctx context.Context, data any) error {
	jobData, ok := data.(SlackJob)
	if !ok {
		// Try to unmarshal if it's a map
		if mapData, ok := data.(map[string]any); ok {
			jsonData, err := json.Marshal(mapData)
			if err != nil {
				return fmt.Errorf("failed to marshal slack data: %w", err)
			}
			if err := json.Unmarshal(jsonData, &jobData); err != nil {
				return fmt.Errorf("failed to unmarshal slack data: %w", err)
			}
		} else {
			return errors.New("invalid slack data type")
		}
	}

	w.logger.Info("Sending Slack message", "channel", jobData.Channel, "event_type", jobData.EventType)

	// TODO: Implement actual Slack API integration
	// This would use Slack's Web API to send messages

	// Simulate Slack sending
	time.Sleep(150 * time.Millisecond)

	w.logger.Info("Slack message sent successfully", "channel", jobData.Channel)
	return nil
}

// processSMS processes an SMS notification
func (w *NotificationWorker) processSMS(ctx context.Context, data any) error {
	jobData, ok := data.(SMSJob)
	if !ok {
		// Try to unmarshal if it's a map
		if mapData, ok := data.(map[string]any); ok {
			jsonData, err := json.Marshal(mapData)
			if err != nil {
				return fmt.Errorf("failed to marshal sms data: %w", err)
			}
			if err := json.Unmarshal(jsonData, &jobData); err != nil {
				return fmt.Errorf("failed to unmarshal sms data: %w", err)
			}
		} else {
			return errors.New("invalid sms data type")
		}
	}

	w.logger.Info("Sending SMS", "to", jobData.To)

	// TODO: Implement actual SMS sending logic
	// This would integrate with services like Twilio, AWS SNS, etc.

	// Simulate SMS sending
	time.Sleep(100 * time.Millisecond)

	w.logger.Info("SMS sent successfully", "to", jobData.To)
	return nil
}

// processPush processes a push notification
func (w *NotificationWorker) processPush(ctx context.Context, data any) error {
	jobData, ok := data.(PushJob)
	if !ok {
		// Try to unmarshal if it's a map
		if mapData, ok := data.(map[string]any); ok {
			jsonData, err := json.Marshal(mapData)
			if err != nil {
				return fmt.Errorf("failed to marshal push data: %w", err)
			}
			if err := json.Unmarshal(jsonData, &jobData); err != nil {
				return fmt.Errorf("failed to unmarshal push data: %w", err)
			}
		} else {
			return errors.New("invalid push data type")
		}
	}

	w.logger.Info("Sending push notifications", "devices", len(jobData.DeviceTokens), "title", jobData.Title)

	// TODO: Implement actual push notification logic
	// This would integrate with Firebase Cloud Messaging, Apple Push Notifications, etc.

	// Simulate push sending
	time.Sleep(300 * time.Millisecond)

	w.logger.Info("Push notifications sent successfully", "devices", len(jobData.DeviceTokens))
	return nil
}

// GetQueueLength returns the current queue length
func (w *NotificationWorker) GetQueueLength() int {
	return len(w.queue)
}

// GetStats returns worker statistics
func (w *NotificationWorker) GetStats() map[string]any {
	return map[string]any{
		"queue_length":   w.GetQueueLength(),
		"queue_capacity": cap(w.queue),
		"workers":        w.config.Workers.NotificationWorkers,
	}
}

// Helper methods for common notifications

// SendWelcomeEmail sends a welcome email to new users
func (w *NotificationWorker) SendWelcomeEmail(userEmail, userName string) {
	w.QueueEmail(EmailJob{
		To:       []string{userEmail},
		Subject:  "Welcome to Brokle!",
		Template: "welcome",
		TemplateData: map[string]any{
			"name": userName,
		},
		Priority: "normal",
	})
}

// SendPasswordResetEmail sends a password reset email
func (w *NotificationWorker) SendPasswordResetEmail(userEmail, resetToken string) {
	w.QueueEmail(EmailJob{
		To:       []string{userEmail},
		Subject:  "Reset your Brokle password",
		Template: "password_reset",
		TemplateData: map[string]any{
			"reset_token": resetToken,
		},
		Priority: "high",
	})
}

// SendBillingAlert sends a billing alert notification
func (w *NotificationWorker) SendBillingAlert(userEmail string, amount float64, threshold float64) {
	w.QueueEmail(EmailJob{
		To:       []string{userEmail},
		Subject:  "Billing Alert - Usage Threshold Exceeded",
		Template: "billing_alert",
		TemplateData: map[string]any{
			"amount":    amount,
			"threshold": threshold,
		},
		Priority: "high",
	})
}

// SendSystemAlert sends a system alert to administrators
func (w *NotificationWorker) SendSystemAlert(message, severity string) {
	// Send Slack notification
	w.QueueSlack(SlackJob{
		Channel:   "#alerts",
		Message:   fmt.Sprintf(":warning: *System Alert*\n*Severity:* %s\n*Message:* %s", severity, message),
		Username:  "Brokle Monitor",
		IconEmoji: ":warning:",
		EventType: "system_alert",
	})

	// Also send webhook if configured
	if w.config.Notifications.AlertWebhookURL != "" {
		w.QueueWebhook(WebhookJob{
			URL:    w.config.Notifications.AlertWebhookURL,
			Method: "POST",
			Payload: map[string]any{
				"type":      "system_alert",
				"severity":  severity,
				"message":   message,
				"timestamp": time.Now().Unix(),
			},
			EventType: "system_alert",
		})
	}
}
