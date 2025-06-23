package domain

import (
	"time"
)

// NotificationType represents the type of notification
type NotificationType string

const (
	NotificationTypeOrderCreated      NotificationType = "order_created"
	NotificationTypeOrderPaid         NotificationType = "order_paid"
	NotificationTypePaymentFailed     NotificationType = "payment_failed"
	NotificationTypeAssemblyStarted   NotificationType = "assembly_started"
	NotificationTypeAssemblyCompleted NotificationType = "assembly_completed"
	NotificationTypeAssemblyFailed    NotificationType = "assembly_failed"
)

// NotificationChannel represents the channel for sending notifications
type NotificationChannel string

const (
	NotificationChannelTelegram NotificationChannel = "telegram"
	NotificationChannelEmail    NotificationChannel = "email"
	NotificationChannelSMS      NotificationChannel = "sms"
	NotificationChannelPush     NotificationChannel = "push"
)

// NotificationStatus represents the status of a notification
type NotificationStatus string

const (
	NotificationStatusPending  NotificationStatus = "pending"
	NotificationStatusSent     NotificationStatus = "sent"
	NotificationStatusFailed   NotificationStatus = "failed"
	NotificationStatusRetrying NotificationStatus = "retrying"
)

// NotificationPriority represents the priority of a notification
type NotificationPriority string

const (
	NotificationPriorityLow    NotificationPriority = "low"
	NotificationPriorityNormal NotificationPriority = "normal"
	NotificationPriorityHigh   NotificationPriority = "high"
	NotificationPriorityUrgent NotificationPriority = "urgent"
)

// Notification represents a notification to be sent to a user
type Notification struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"user_id"`
	Type         NotificationType       `json:"type"`
	Channel      NotificationChannel    `json:"channel"`
	Priority     NotificationPriority   `json:"priority"`
	Status       NotificationStatus     `json:"status"`
	Subject      string                 `json:"subject"`
	Content      string                 `json:"content"`
	Data         map[string]interface{} `json:"data"`
	Metadata     map[string]string      `json:"metadata"`
	RetryCount   int                    `json:"retry_count"`
	MaxRetries   int                    `json:"max_retries"`
	ScheduledAt  *time.Time             `json:"scheduled_at,omitempty"`
	SentAt       *time.Time             `json:"sent_at,omitempty"`
	FailedAt     *time.Time             `json:"failed_at,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
	ExpiresAt    *time.Time             `json:"expires_at,omitempty"`
	ErrorMessage string                 `json:"error_message,omitempty"`
}

// NotificationTemplate represents a template for generating notifications
type NotificationTemplate struct {
	Type       NotificationType     `json:"type"`
	Channel    NotificationChannel  `json:"channel"`
	Priority   NotificationPriority `json:"priority"`
	Subject    string               `json:"subject"`
	Content    string               `json:"content"`
	Variables  []string             `json:"variables"`
	MaxRetries int                  `json:"max_retries"`
	TTL        time.Duration        `json:"ttl"`
}

// TelegramRecipient represents a Telegram recipient
type TelegramRecipient struct {
	UserID string `json:"user_id"`
	ChatID int64  `json:"chat_id"`
}

// Event represents an event that triggers a notification
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Source    string                 `json:"source"`
	Subject   string                 `json:"subject"`
	Data      map[string]interface{} `json:"data"`
	UserID    string                 `json:"user_id"`
	Timestamp time.Time              `json:"timestamp"`
}

// OrderEvent represents order-related events
type OrderEvent struct {
	OrderID     string      `json:"order_id"`
	UserID      string      `json:"user_id"`
	Status      string      `json:"status"`
	TotalAmount float64     `json:"total_amount"`
	Currency    string      `json:"currency"`
	Items       []OrderItem `json:"items"`
	CreatedAt   time.Time   `json:"created_at"`
}

// OrderItem represents an item in an order
type OrderItem struct {
	ItemID    string  `json:"item_id"`
	ItemName  string  `json:"item_name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
	Total     float64 `json:"total"`
}

// PaymentEvent represents payment-related events
type PaymentEvent struct {
	PaymentID     string    `json:"payment_id"`
	OrderID       string    `json:"order_id"`
	UserID        string    `json:"user_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	PaymentMethod string    `json:"payment_method"`
	TransactionID string    `json:"transaction_id"`
	Status        string    `json:"status"`
	ProcessedAt   time.Time `json:"processed_at"`
	ErrorCode     string    `json:"error_code,omitempty"`
	ErrorMessage  string    `json:"error_message,omitempty"`
}

// AssemblyEvent represents assembly-related events
type AssemblyEvent struct {
	AssemblyID        string            `json:"assembly_id"`
	OrderID           string            `json:"order_id"`
	UserID            string            `json:"user_id"`
	Status            string            `json:"status"`
	Components        []RocketComponent `json:"components"`
	EstimatedDuration int               `json:"estimated_duration_seconds"`
	ActualDuration    int               `json:"actual_duration_seconds,omitempty"`
	Quality           string            `json:"quality,omitempty"`
	StartedAt         *time.Time        `json:"started_at,omitempty"`
	CompletedAt       *time.Time        `json:"completed_at,omitempty"`
	FailedAt          *time.Time        `json:"failed_at,omitempty"`
	ErrorCode         string            `json:"error_code,omitempty"`
	ErrorMessage      string            `json:"error_message,omitempty"`
	FailedComponents  []string          `json:"failed_components,omitempty"`
}

// RocketComponent represents a component used in rocket assembly
type RocketComponent struct {
	ComponentID   string `json:"component_id"`
	ComponentName string `json:"component_name"`
	ComponentType string `json:"component_type"`
	Quantity      int    `json:"quantity"`
}

// NewNotification creates a new notification instance
func NewNotification(userID string, notificationType NotificationType, channel NotificationChannel) *Notification {
	now := time.Now()
	return &Notification{
		ID:         generateNotificationID(),
		UserID:     userID,
		Type:       notificationType,
		Channel:    channel,
		Priority:   NotificationPriorityNormal,
		Status:     NotificationStatusPending,
		Data:       make(map[string]interface{}),
		Metadata:   make(map[string]string),
		RetryCount: 0,
		MaxRetries: 3,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// MarkAsSent marks the notification as sent
func (n *Notification) MarkAsSent() {
	now := time.Now()
	n.Status = NotificationStatusSent
	n.SentAt = &now
	n.UpdatedAt = now
}

// MarkAsFailed marks the notification as failed
func (n *Notification) MarkAsFailed(errorMsg string) {
	now := time.Now()
	n.Status = NotificationStatusFailed
	n.FailedAt = &now
	n.ErrorMessage = errorMsg
	n.UpdatedAt = now
}

// IncrementRetryCount increments the retry count
func (n *Notification) IncrementRetryCount() {
	n.RetryCount++
	n.UpdatedAt = time.Now()
	if n.RetryCount < n.MaxRetries {
		n.Status = NotificationStatusRetrying
	} else {
		n.Status = NotificationStatusFailed
	}
}

// CanRetry checks if the notification can be retried
func (n *Notification) CanRetry() bool {
	return n.RetryCount < n.MaxRetries &&
		(n.Status == NotificationStatusFailed || n.Status == NotificationStatusRetrying)
}

// IsExpired checks if the notification has expired
func (n *Notification) IsExpired() bool {
	if n.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*n.ExpiresAt)
}

// SetExpiration sets the expiration time for the notification
func (n *Notification) SetExpiration(duration time.Duration) {
	expiresAt := time.Now().Add(duration)
	n.ExpiresAt = &expiresAt
	n.UpdatedAt = time.Now()
}

// AddMetadata adds metadata to the notification
func (n *Notification) AddMetadata(key, value string) {
	if n.Metadata == nil {
		n.Metadata = make(map[string]string)
	}
	n.Metadata[key] = value
	n.UpdatedAt = time.Now()
}

// AddData adds data to the notification
func (n *Notification) AddData(key string, value interface{}) {
	if n.Data == nil {
		n.Data = make(map[string]interface{})
	}
	n.Data[key] = value
	n.UpdatedAt = time.Now()
}

// generateNotificationID generates a unique notification ID
func generateNotificationID() string {
	// In a real implementation, you would use a proper UUID generator
	// For now, we'll use a timestamp-based approach
	return "notification_" + time.Now().Format("20060102150405") + "_" + generateRandomString(8)
}

// generateRandomString generates a random string of given length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}
