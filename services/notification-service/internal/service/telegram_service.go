package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/amiosamu/rocket-science/services/notification-service/internal/config"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/domain"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// TelegramService handles sending notifications via Telegram
type TelegramService struct {
	bot     *tgbotapi.BotAPI
	config  config.TelegramConfig
	logger  logging.Logger
	metrics metrics.Metrics
}

// NewTelegramService creates a new TelegramService instance
func NewTelegramService(cfg config.TelegramConfig, logger logging.Logger, metrics metrics.Metrics) (*TelegramService, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram bot: %w", err)
	}

	bot.Debug = false // Set to true for debugging

	logger.Info(nil, "Telegram bot initialized successfully", map[string]interface{}{
		"bot_username": bot.Self.UserName,
		"bot_id":       bot.Self.ID,
	})

	return &TelegramService{
		bot:     bot,
		config:  cfg,
		logger:  logger,
		metrics: metrics,
	}, nil
}

// SendNotification sends a notification via Telegram
func (ts *TelegramService) SendNotification(ctx context.Context, notification *domain.Notification, chatID int64) error {
	startTime := time.Now()

	// Record metrics
	defer func() {
		ts.metrics.RecordDuration("notification_telegram_send_duration", time.Since(startTime), nil)
	}()

	ts.logger.Info(ctx, "Sending Telegram notification", map[string]interface{}{
		"notification_id": notification.ID,
		"user_id":         notification.UserID,
		"chat_id":         chatID,
		"type":            notification.Type,
	})

	// Format the message content
	messageText := ts.formatMessage(notification)

	// Create the message
	msg := tgbotapi.NewMessage(chatID, messageText)
	msg.ParseMode = tgbotapi.ModeMarkdown

	// Add inline keyboard if applicable
	if keyboard := ts.createInlineKeyboard(notification); keyboard != nil {
		msg.ReplyMarkup = keyboard
	}

	// Send the message with retry logic
	err := ts.sendWithRetry(ctx, msg, notification)
	if err != nil {
		ts.logger.Error(ctx, "Failed to send Telegram notification", err, map[string]interface{}{
			"notification_id": notification.ID,
			"user_id":         notification.UserID,
			"chat_id":         chatID,
		})
		ts.metrics.IncrementCounter("notification_telegram_send_error", nil)
		return fmt.Errorf("failed to send Telegram message: %w", err)
	}

	ts.logger.Info(ctx, "Telegram notification sent successfully", map[string]interface{}{
		"notification_id": notification.ID,
		"user_id":         notification.UserID,
		"chat_id":         chatID,
	})
	ts.metrics.IncrementCounter("notification_telegram_send_success", nil)

	return nil
}

// sendWithRetry sends a message with retry logic
func (ts *TelegramService) sendWithRetry(ctx context.Context, msg tgbotapi.MessageConfig, notification *domain.Notification) error {
	var lastErr error

	for attempt := 0; attempt <= ts.config.RetryCount; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(ts.config.RetryDelay * time.Duration(attempt)):
			}

			ts.logger.Info(ctx, "Retrying Telegram message send", map[string]interface{}{
				"notification_id": notification.ID,
				"attempt":         attempt + 1,
				"max_attempts":    ts.config.RetryCount + 1,
			})
		}

		// Send the message with timeout
		_, err := ts.bot.Send(msg)

		if err == nil {
			return nil
		}

		lastErr = err

		// Check if the error is retryable
		if !ts.isRetryableError(err) {
			ts.logger.Warn(ctx, "Non-retryable Telegram error, stopping retries", map[string]interface{}{
				"notification_id": notification.ID,
				"error":           err.Error(),
			})
			break
		}

		ts.logger.Warn(ctx, "Retryable Telegram error", map[string]interface{}{
			"notification_id": notification.ID,
			"attempt":         attempt + 1,
			"error":           err.Error(),
		})
	}

	return lastErr
}

// isRetryableError checks if an error is retryable
func (ts *TelegramService) isRetryableError(err error) bool {
	errStr := err.Error()

	// List of retryable errors
	retryableErrors := []string{
		"timeout",
		"connection refused",
		"network is unreachable",
		"temporary failure",
		"too many requests",
		"internal server error",
	}

	errLower := strings.ToLower(errStr)
	for _, retryableErr := range retryableErrors {
		if strings.Contains(errLower, retryableErr) {
			return true
		}
	}

	return false
}

// formatMessage formats the notification content for Telegram
func (ts *TelegramService) formatMessage(notification *domain.Notification) string {
	var message strings.Builder

	// Add emoji based on notification type
	emoji := ts.getEmojiForType(notification.Type)
	if emoji != "" {
		message.WriteString(emoji + " ")
	}

	// Add subject if present
	if notification.Subject != "" {
		message.WriteString("*" + notification.Subject + "*\n\n")
	}

	// Add main content
	message.WriteString(notification.Content)

	// Add additional information based on notification data
	if notification.Data != nil {
		ts.addDataToMessage(&message, notification)
	}

	// Ensure message doesn't exceed Telegram's limit
	messageText := message.String()
	if len(messageText) > ts.config.MessageLimit {
		messageText = messageText[:ts.config.MessageLimit-3] + "..."
	}

	return messageText
}

// getEmojiForType returns an emoji for the notification type
func (ts *TelegramService) getEmojiForType(notificationType domain.NotificationType) string {
	switch notificationType {
	case domain.NotificationTypeOrderCreated:
		return "📦"
	case domain.NotificationTypeOrderPaid:
		return "💳"
	case domain.NotificationTypePaymentFailed:
		return "❌"
	case domain.NotificationTypeAssemblyStarted:
		return "🔧"
	case domain.NotificationTypeAssemblyCompleted:
		return "🚀"
	case domain.NotificationTypeAssemblyFailed:
		return "⚠️"
	default:
		return "📢"
	}
}

// addDataToMessage adds additional data to the message based on notification type
func (ts *TelegramService) addDataToMessage(message *strings.Builder, notification *domain.Notification) {
	switch notification.Type {
	case domain.NotificationTypeOrderCreated, domain.NotificationTypeOrderPaid:
		ts.addOrderDataToMessage(message, notification.Data)
	case domain.NotificationTypePaymentFailed:
		ts.addPaymentDataToMessage(message, notification.Data)
	case domain.NotificationTypeAssemblyStarted, domain.NotificationTypeAssemblyCompleted, domain.NotificationTypeAssemblyFailed:
		ts.addAssemblyDataToMessage(message, notification.Data)
	}
}

// addOrderDataToMessage adds order-specific data to the message
func (ts *TelegramService) addOrderDataToMessage(message *strings.Builder, data map[string]interface{}) {
	if orderID, ok := data["order_id"].(string); ok && orderID != "" {
		message.WriteString(fmt.Sprintf("\n\n*Order ID:* `%s`", orderID))
	}

	if totalAmount, ok := data["total_amount"].(float64); ok {
		currency := "USD"
		if c, ok := data["currency"].(string); ok && c != "" {
			currency = c
		}
		message.WriteString(fmt.Sprintf("\n*Total:* %.2f %s", totalAmount, currency))
	}

	if items, ok := data["items"].([]interface{}); ok && len(items) > 0 {
		message.WriteString("\n\n*Items:*")
		for i, item := range items {
			if i >= 3 { // Limit to 3 items to avoid long messages
				message.WriteString(fmt.Sprintf("\n... and %d more items", len(items)-3))
				break
			}
			if itemMap, ok := item.(map[string]interface{}); ok {
				if name, ok := itemMap["item_name"].(string); ok {
					quantity := 1
					if q, ok := itemMap["quantity"].(int); ok {
						quantity = q
					}
					message.WriteString(fmt.Sprintf("\n• %dx %s", quantity, name))
				}
			}
		}
	}
}

// addPaymentDataToMessage adds payment-specific data to the message
func (ts *TelegramService) addPaymentDataToMessage(message *strings.Builder, data map[string]interface{}) {
	if orderID, ok := data["order_id"].(string); ok && orderID != "" {
		message.WriteString(fmt.Sprintf("\n\n*Order ID:* `%s`", orderID))
	}

	if errorCode, ok := data["error_code"].(string); ok && errorCode != "" {
		message.WriteString(fmt.Sprintf("\n*Error Code:* `%s`", errorCode))
	}

	if amount, ok := data["amount"].(float64); ok {
		currency := "USD"
		if c, ok := data["currency"].(string); ok && c != "" {
			currency = c
		}
		message.WriteString(fmt.Sprintf("\n*Amount:* %.2f %s", amount, currency))
	}
}

// addAssemblyDataToMessage adds assembly-specific data to the message
func (ts *TelegramService) addAssemblyDataToMessage(message *strings.Builder, data map[string]interface{}) {
	if assemblyID, ok := data["assembly_id"].(string); ok && assemblyID != "" {
		message.WriteString(fmt.Sprintf("\n\n*Assembly ID:* `%s`", assemblyID))
	}

	if orderID, ok := data["order_id"].(string); ok && orderID != "" {
		message.WriteString(fmt.Sprintf("\n*Order ID:* `%s`", orderID))
	}

	if duration, ok := data["estimated_duration_seconds"].(int); ok && duration > 0 {
		message.WriteString(fmt.Sprintf("\n*Estimated Duration:* %d seconds", duration))
	}

	if actualDuration, ok := data["actual_duration_seconds"].(int); ok && actualDuration > 0 {
		message.WriteString(fmt.Sprintf("\n*Actual Duration:* %d seconds", actualDuration))
	}

	if quality, ok := data["quality"].(string); ok && quality != "" {
		message.WriteString(fmt.Sprintf("\n*Quality:* %s", quality))
	}

	if components, ok := data["components"].([]interface{}); ok && len(components) > 0 {
		message.WriteString("\n\n*Components:*")
		for i, comp := range components {
			if i >= 3 { // Limit to 3 components
				message.WriteString(fmt.Sprintf("\n... and %d more components", len(components)-3))
				break
			}
			if compMap, ok := comp.(map[string]interface{}); ok {
				if name, ok := compMap["component_name"].(string); ok {
					quantity := 1
					if q, ok := compMap["quantity"].(int); ok {
						quantity = q
					}
					message.WriteString(fmt.Sprintf("\n• %dx %s", quantity, name))
				}
			}
		}
	}
}

// createInlineKeyboard creates an inline keyboard for certain notification types
func (ts *TelegramService) createInlineKeyboard(notification *domain.Notification) *tgbotapi.InlineKeyboardMarkup {
	var keyboard tgbotapi.InlineKeyboardMarkup

	switch notification.Type {
	case domain.NotificationTypeOrderCreated:
		// Add button to view order details
		if orderID, ok := notification.Data["order_id"].(string); ok {
			viewButton := tgbotapi.NewInlineKeyboardButtonData("📋 View Order", "view_order_"+orderID)
			keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []tgbotapi.InlineKeyboardButton{viewButton})
		}
	case domain.NotificationTypeAssemblyCompleted:
		// Add button to track delivery
		if orderID, ok := notification.Data["order_id"].(string); ok {
			trackButton := tgbotapi.NewInlineKeyboardButtonData("📍 Track Delivery", "track_order_"+orderID)
			keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, []tgbotapi.InlineKeyboardButton{trackButton})
		}
	}

	// Return nil if no buttons were added
	if len(keyboard.InlineKeyboard) == 0 {
		return nil
	}

	return &keyboard
}

// ValidateChatID validates if a chat ID is valid by sending a test message
func (ts *TelegramService) ValidateChatID(ctx context.Context, chatID int64) error {
	// Try to get chat information
	chatConfig := tgbotapi.ChatInfoConfig{
		ChatConfig: tgbotapi.ChatConfig{
			ChatID: chatID,
		},
	}

	_, err := ts.bot.GetChat(chatConfig)
	if err != nil {
		return fmt.Errorf("invalid chat ID %d: %w", chatID, err)
	}

	return nil
}

// GetBotInfo returns information about the bot
func (ts *TelegramService) GetBotInfo() *tgbotapi.User {
	return &ts.bot.Self
}

// Close closes the Telegram service
func (ts *TelegramService) Close() {
	ts.bot.StopReceivingUpdates()
	ts.logger.Info(nil, "Telegram service closed")
}
