package service

import (
	"context"
	"fmt"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/amiosamu/rocket-science/services/notification-service/internal/config"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/domain"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// MockTelegramService is a mock implementation for development/testing
type MockTelegramService struct {
	config  config.TelegramConfig
	logger  logging.Logger
	metrics metrics.Metrics
	botInfo tgbotapi.User
}

// NewMockTelegramService creates a new mock Telegram service for development
func NewMockTelegramService(cfg config.TelegramConfig, logger logging.Logger, metrics metrics.Metrics) *MockTelegramService {
	logger.Info(nil, "Using Mock Telegram Service (Development Mode)", map[string]interface{}{
		"bot_token": "MOCK_TOKEN",
		"mode":      "development",
	})

	return &MockTelegramService{
		config:  cfg,
		logger:  logger,
		metrics: metrics,
		botInfo: tgbotapi.User{
			ID:        123456789,
			IsBot:     true,
			FirstName: "MockBot",
			UserName:  "mock_rocket_bot",
		},
	}
}

// SendNotification simulates sending a notification via Telegram
func (mts *MockTelegramService) SendNotification(ctx context.Context, notification *domain.Notification, chatID int64) error {
	startTime := time.Now()

	// Record metrics
	defer func() {
		mts.metrics.RecordDuration("notification_telegram_send_duration", time.Since(startTime), nil)
	}()

	mts.logger.Info(ctx, "Mock: Sending Telegram notification", map[string]interface{}{
		"notification_id": notification.ID,
		"user_id":         notification.UserID,
		"chat_id":         chatID,
		"type":            notification.Type,
		"subject":         notification.Subject,
		"content":         notification.Content,
		"mock":            true,
	})

	// Simulate some processing time
	time.Sleep(100 * time.Millisecond)

	// Simulate successful send
	mts.logger.Info(ctx, "Mock: Telegram notification sent successfully", map[string]interface{}{
		"notification_id": notification.ID,
		"user_id":         notification.UserID,
		"chat_id":         chatID,
		"mock":            true,
	})

	mts.metrics.IncrementCounter("notification_telegram_send_success", nil)
	return nil
}

// ValidateChatID simulates validating a chat ID
func (mts *MockTelegramService) ValidateChatID(ctx context.Context, chatID int64) error {
	mts.logger.Info(ctx, "Mock: Validating chat ID", map[string]interface{}{
		"chat_id": chatID,
		"mock":    true,
	})

	// Simulate validation - always succeed for positive IDs
	if chatID > 0 {
		return nil
	}

	return fmt.Errorf("mock validation failed: invalid chat ID %d", chatID)
}

// GetBotInfo returns mock bot information
func (mts *MockTelegramService) GetBotInfo() *tgbotapi.User {
	return &mts.botInfo
}

// Close closes the mock Telegram service
func (mts *MockTelegramService) Close() {
	mts.logger.Info(nil, "Mock Telegram service closed", map[string]interface{}{
		"mock": true,
	})
}
