package service

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/amiosamu/rocket-science/services/notification-service/internal/domain"
)

// TelegramServiceInterface defines the contract for Telegram services
type TelegramServiceInterface interface {
	SendNotification(ctx context.Context, notification *domain.Notification, chatID int64) error
	ValidateChatID(ctx context.Context, chatID int64) error
	GetBotInfo() *tgbotapi.User
	Close()
}
