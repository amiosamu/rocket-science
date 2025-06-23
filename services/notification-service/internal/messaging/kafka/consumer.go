package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/amiosamu/rocket-science/services/notification-service/internal/config"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/service"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/transport/grpc/clients"
	"github.com/amiosamu/rocket-science/shared/platform/messaging/kafka"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// EventEnvelope represents the wrapper for all events
type EventEnvelope struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Source      string                 `json:"source"`
	Subject     string                 `json:"subject"`
	Time        time.Time              `json:"time"`
	Data        map[string]interface{} `json:"data"`
	Extensions  map[string]string      `json:"extensions"`
	SpecVersion string                 `json:"spec_version"`
}

// EventConsumer handles consuming and processing events from Kafka
type EventConsumer struct {
	config          config.Config
	logger          logging.Logger
	metrics         metrics.Metrics
	telegramService *service.TelegramService
	iamClient       *clients.IAMClient
	supportedTopics []string
}

// NewEventConsumer creates a new event consumer
func NewEventConsumer(
	cfg config.Config,
	logger logging.Logger,
	metrics metrics.Metrics,
	telegramService *service.TelegramService,
	iamClient *clients.IAMClient,
) *EventConsumer {
	supportedTopics := []string{
		cfg.Kafka.Topics.OrderEvents,
		cfg.Kafka.Topics.PaymentEvents,
		cfg.Kafka.Topics.AssemblyEvents,
	}

	return &EventConsumer{
		config:          cfg,
		logger:          logger,
		metrics:         metrics,
		telegramService: telegramService,
		iamClient:       iamClient,
		supportedTopics: supportedTopics,
	}
}

// HandleMessage implements the MessageHandler interface
func (ec *EventConsumer) HandleMessage(ctx context.Context, message *kafka.Message) error {
	startTime := time.Now()

	// Record processing metrics
	defer func() {
		ec.metrics.RecordDuration("kafka_message_processing_duration", time.Since(startTime), map[string]string{
			"topic": message.Topic,
		})
	}()

	ec.logger.Info(ctx, "Processing Kafka message", map[string]interface{}{
		"topic":      message.Topic,
		"partition":  message.Partition,
		"offset":     message.Offset,
		"event_type": message.EventType,
		"event_id":   message.EventID,
	})

	// Parse the event envelope
	var envelope EventEnvelope
	if err := json.Unmarshal(message.Value, &envelope); err != nil {
		ec.logger.Error(ctx, "Failed to unmarshal event envelope", err, map[string]interface{}{
			"topic":  message.Topic,
			"offset": message.Offset,
		})
		ec.metrics.IncrementCounter("kafka_message_unmarshal_error", map[string]string{
			"topic": message.Topic,
		})
		return fmt.Errorf("failed to unmarshal event envelope: %w", err)
	}

	// Process based on topic
	var err error
	switch message.Topic {
	case ec.config.Kafka.Topics.OrderEvents:
		err = ec.handleOrderEvent(ctx, &envelope)
	case ec.config.Kafka.Topics.PaymentEvents:
		err = ec.handlePaymentEvent(ctx, &envelope)
	case ec.config.Kafka.Topics.AssemblyEvents:
		err = ec.handleAssemblyEvent(ctx, &envelope)
	default:
		ec.logger.Warn(ctx, "Unknown topic, skipping message", map[string]interface{}{
			"topic": message.Topic,
		})
		return nil
	}

	if err != nil {
		ec.logger.Error(ctx, "Failed to process event", err, map[string]interface{}{
			"topic":      message.Topic,
			"event_type": envelope.Type,
			"event_id":   envelope.ID,
		})
		ec.metrics.IncrementCounter("kafka_message_processing_error", map[string]string{
			"topic":      message.Topic,
			"event_type": envelope.Type,
		})
		return err
	}

	ec.logger.Info(ctx, "Successfully processed event", map[string]interface{}{
		"topic":      message.Topic,
		"event_type": envelope.Type,
		"event_id":   envelope.ID,
	})
	ec.metrics.IncrementCounter("kafka_message_processing_success", map[string]string{
		"topic":      message.Topic,
		"event_type": envelope.Type,
	})

	return nil
}

// GetSupportedTopics returns the list of topics this handler supports
func (ec *EventConsumer) GetSupportedTopics() []string {
	return ec.supportedTopics
}

// handleOrderEvent processes order-related events
func (ec *EventConsumer) handleOrderEvent(ctx context.Context, envelope *EventEnvelope) error {
	switch envelope.Type {
	case "order.created":
		return ec.handleOrderCreatedEvent(ctx, envelope)
	case "order.paid":
		return ec.handleOrderPaidEvent(ctx, envelope)
	case "order.cancelled":
		return ec.handleOrderCancelledEvent(ctx, envelope)
	default:
		ec.logger.Debug(ctx, "Unsupported order event type", map[string]interface{}{
			"event_type": envelope.Type,
		})
		return nil
	}
}

// handlePaymentEvent processes payment-related events
func (ec *EventConsumer) handlePaymentEvent(ctx context.Context, envelope *EventEnvelope) error {
	switch envelope.Type {
	case "payment.processed":
		return ec.handlePaymentProcessedEvent(ctx, envelope)
	case "payment.failed":
		return ec.handlePaymentFailedEvent(ctx, envelope)
	default:
		ec.logger.Debug(ctx, "Unsupported payment event type", map[string]interface{}{
			"event_type": envelope.Type,
		})
		return nil
	}
}

// handleAssemblyEvent processes assembly-related events
func (ec *EventConsumer) handleAssemblyEvent(ctx context.Context, envelope *EventEnvelope) error {
	switch envelope.Type {
	case "assembly.started":
		return ec.handleAssemblyStartedEvent(ctx, envelope)
	case "assembly.completed":
		return ec.handleAssemblyCompletedEvent(ctx, envelope)
	case "assembly.failed":
		return ec.handleAssemblyFailedEvent(ctx, envelope)
	default:
		ec.logger.Debug(ctx, "Unsupported assembly event type", map[string]interface{}{
			"event_type": envelope.Type,
		})
		return nil
	}
}

// handleOrderCreatedEvent handles order created events
func (ec *EventConsumer) handleOrderCreatedEvent(ctx context.Context, envelope *EventEnvelope) error {
	// Extract user ID and order data
	userID, ok := envelope.Data["user_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid user_id in order created event")
	}

	orderID, _ := envelope.Data["order_id"].(string)
	totalAmount, _ := envelope.Data["total_amount"].(float64)
	currency, _ := envelope.Data["currency"].(string)

	notification := domain.NewNotification(
		userID,
		domain.NotificationTypeOrderCreated,
		domain.NotificationChannelTelegram,
	)

	notification.Subject = "Order Created Successfully! 📦"
	notification.Content = "Your order has been created successfully!\n\nWe're preparing your rocket parts for assembly."

	// Add order data
	notification.AddData("order_id", orderID)
	notification.AddData("total_amount", totalAmount)
	notification.AddData("currency", currency)
	if items, ok := envelope.Data["items"].([]interface{}); ok {
		notification.AddData("items", items)
	}

	return ec.sendNotification(ctx, notification)
}

// handleOrderPaidEvent handles order paid events
func (ec *EventConsumer) handleOrderPaidEvent(ctx context.Context, envelope *EventEnvelope) error {
	userID, ok := envelope.Data["user_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid user_id in order paid event")
	}

	orderID, _ := envelope.Data["order_id"].(string)
	transactionID, _ := envelope.Data["transaction_id"].(string)
	amount, _ := envelope.Data["amount"].(float64)
	currency, _ := envelope.Data["currency"].(string)
	paymentMethod, _ := envelope.Data["payment_method"].(string)

	notification := domain.NewNotification(
		userID,
		domain.NotificationTypeOrderPaid,
		domain.NotificationChannelTelegram,
	)

	notification.Subject = "Payment Confirmed! 💳"
	notification.Content = "Your payment has been processed successfully!\n\nYour rocket assembly will begin shortly."

	// Add payment data
	notification.AddData("order_id", orderID)
	notification.AddData("transaction_id", transactionID)
	notification.AddData("amount", amount)
	notification.AddData("currency", currency)
	notification.AddData("payment_method", paymentMethod)

	return ec.sendNotification(ctx, notification)
}

// handleOrderCancelledEvent handles order cancelled events
func (ec *EventConsumer) handleOrderCancelledEvent(ctx context.Context, envelope *EventEnvelope) error {
	userID, ok := envelope.Data["user_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid user_id in order cancelled event")
	}

	orderID, _ := envelope.Data["order_id"].(string)
	reason, _ := envelope.Data["reason"].(string)
	refundRequired, _ := envelope.Data["refund_required"].(bool)

	notification := domain.NewNotification(
		userID,
		domain.NotificationTypeOrderCreated, // Reusing order created type for cancelled
		domain.NotificationChannelTelegram,
	)

	notification.Subject = "Order Cancelled ❌"
	notification.Content = fmt.Sprintf(
		"Your order has been cancelled.\n\nReason: %s\n\nIf a refund is required, it will be processed within 3-5 business days.",
		reason,
	)

	// Add order data
	notification.AddData("order_id", orderID)
	notification.AddData("reason", reason)
	notification.AddData("refund_required", refundRequired)

	return ec.sendNotification(ctx, notification)
}

// handlePaymentProcessedEvent handles payment processed events
func (ec *EventConsumer) handlePaymentProcessedEvent(ctx context.Context, envelope *EventEnvelope) error {
	userID, ok := envelope.Data["user_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid user_id in payment processed event")
	}

	// Only send notification for successful payments
	status, _ := envelope.Data["status"].(string)
	if status != "success" && status != "SUCCESS" {
		return nil
	}

	paymentID, _ := envelope.Data["payment_id"].(string)
	orderID, _ := envelope.Data["order_id"].(string)
	transactionID, _ := envelope.Data["transaction_id"].(string)
	amount, _ := envelope.Data["amount"].(float64)
	currency, _ := envelope.Data["currency"].(string)
	paymentMethod, _ := envelope.Data["payment_method"].(string)

	notification := domain.NewNotification(
		userID,
		domain.NotificationTypeOrderPaid,
		domain.NotificationChannelTelegram,
	)

	notification.Subject = "Payment Successful! 💰"
	notification.Content = fmt.Sprintf(
		"Your payment has been processed successfully!\n\nTransaction ID: %s",
		transactionID,
	)

	// Add payment data
	notification.AddData("payment_id", paymentID)
	notification.AddData("order_id", orderID)
	notification.AddData("transaction_id", transactionID)
	notification.AddData("amount", amount)
	notification.AddData("currency", currency)
	notification.AddData("payment_method", paymentMethod)

	return ec.sendNotification(ctx, notification)
}

// handlePaymentFailedEvent handles payment failed events
func (ec *EventConsumer) handlePaymentFailedEvent(ctx context.Context, envelope *EventEnvelope) error {
	userID, ok := envelope.Data["user_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid user_id in payment failed event")
	}

	paymentID, _ := envelope.Data["payment_id"].(string)
	orderID, _ := envelope.Data["order_id"].(string)
	amount, _ := envelope.Data["amount"].(float64)
	currency, _ := envelope.Data["currency"].(string)
	reason, _ := envelope.Data["reason"].(string)
	errorCode, _ := envelope.Data["error_code"].(string)

	notification := domain.NewNotification(
		userID,
		domain.NotificationTypePaymentFailed,
		domain.NotificationChannelTelegram,
	)

	notification.Subject = "Payment Failed ❌"
	notification.Content = fmt.Sprintf(
		"Unfortunately, your payment could not be processed.\n\nReason: %s\n\nPlease try again or contact support.",
		reason,
	)

	// Add payment data
	notification.AddData("payment_id", paymentID)
	notification.AddData("order_id", orderID)
	notification.AddData("amount", amount)
	notification.AddData("currency", currency)
	notification.AddData("reason", reason)
	notification.AddData("error_code", errorCode)

	return ec.sendNotification(ctx, notification)
}

// handleAssemblyStartedEvent handles assembly started events
func (ec *EventConsumer) handleAssemblyStartedEvent(ctx context.Context, envelope *EventEnvelope) error {
	userID, ok := envelope.Data["user_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid user_id in assembly started event")
	}

	assemblyID, _ := envelope.Data["assembly_id"].(string)
	orderID, _ := envelope.Data["order_id"].(string)
	estimatedDuration, _ := envelope.Data["estimated_duration_seconds"].(float64)

	notification := domain.NewNotification(
		userID,
		domain.NotificationTypeAssemblyStarted,
		domain.NotificationChannelTelegram,
	)

	notification.Subject = "Rocket Assembly Started! 🔧"
	notification.Content = fmt.Sprintf(
		"Great news! We've started assembling your rocket.\n\nEstimated completion time: %.0f seconds",
		estimatedDuration,
	)

	// Add assembly data
	notification.AddData("assembly_id", assemblyID)
	notification.AddData("order_id", orderID)
	notification.AddData("estimated_duration_seconds", int(estimatedDuration))
	if components, ok := envelope.Data["components"].([]interface{}); ok {
		notification.AddData("components", components)
	}

	return ec.sendNotification(ctx, notification)
}

// handleAssemblyCompletedEvent handles assembly completed events
func (ec *EventConsumer) handleAssemblyCompletedEvent(ctx context.Context, envelope *EventEnvelope) error {
	userID, ok := envelope.Data["user_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid user_id in assembly completed event")
	}

	assemblyID, _ := envelope.Data["assembly_id"].(string)
	orderID, _ := envelope.Data["order_id"].(string)
	actualDuration, _ := envelope.Data["actual_duration_seconds"].(float64)
	quality, _ := envelope.Data["quality"].(string)

	notification := domain.NewNotification(
		userID,
		domain.NotificationTypeAssemblyCompleted,
		domain.NotificationChannelTelegram,
	)

	notification.Subject = "Rocket Assembly Complete! 🚀"
	notification.Content = fmt.Sprintf(
		"Congratulations! Your rocket has been successfully assembled.\n\nAssembly took %.0f seconds with %s quality.",
		actualDuration,
		quality,
	)

	// Add assembly data
	notification.AddData("assembly_id", assemblyID)
	notification.AddData("order_id", orderID)
	notification.AddData("actual_duration_seconds", int(actualDuration))
	notification.AddData("quality", quality)

	return ec.sendNotification(ctx, notification)
}

// handleAssemblyFailedEvent handles assembly failed events
func (ec *EventConsumer) handleAssemblyFailedEvent(ctx context.Context, envelope *EventEnvelope) error {
	userID, ok := envelope.Data["user_id"].(string)
	if !ok {
		return fmt.Errorf("missing or invalid user_id in assembly failed event")
	}

	assemblyID, _ := envelope.Data["assembly_id"].(string)
	orderID, _ := envelope.Data["order_id"].(string)
	reason, _ := envelope.Data["reason"].(string)
	errorCode, _ := envelope.Data["error_code"].(string)

	notification := domain.NewNotification(
		userID,
		domain.NotificationTypeAssemblyFailed,
		domain.NotificationChannelTelegram,
	)

	notification.Subject = "Assembly Failed ⚠️"
	notification.Content = fmt.Sprintf(
		"Unfortunately, there was an issue with your rocket assembly.\n\nReason: %s\n\nOur team is working to resolve this issue.",
		reason,
	)

	// Add assembly data
	notification.AddData("assembly_id", assemblyID)
	notification.AddData("order_id", orderID)
	notification.AddData("reason", reason)
	notification.AddData("error_code", errorCode)
	if failedComponents, ok := envelope.Data["failed_components"].([]interface{}); ok {
		notification.AddData("failed_components", failedComponents)
	}

	return ec.sendNotification(ctx, notification)
}

// sendNotification orchestrates the process of sending a notification
func (ec *EventConsumer) sendNotification(ctx context.Context, notification *domain.Notification) error {
	// Get user's Telegram chat ID from IAM service
	chatID, err := ec.iamClient.GetUserTelegramChatID(ctx, notification.UserID)
	if err != nil {
		ec.logger.Warn(ctx, "Failed to get Telegram chat ID for user", map[string]interface{}{
			"user_id": notification.UserID,
			"error":   err.Error(),
		})
		ec.metrics.IncrementCounter("notification_chat_id_lookup_failed", map[string]string{
			"notification_type": string(notification.Type),
		})
		return fmt.Errorf("failed to get Telegram chat ID for user %s: %w", notification.UserID, err)
	}

	// Send notification via Telegram (chatID is already int64)
	err = ec.telegramService.SendNotification(ctx, notification, chatID)
	if err != nil {
		notification.MarkAsFailed(err.Error())
		ec.logger.Error(ctx, "Failed to send Telegram notification", err, map[string]interface{}{
			"notification_id": notification.ID,
			"user_id":         notification.UserID,
			"chat_id":         chatID,
		})
		ec.metrics.IncrementCounter("notification_send_failed", map[string]string{
			"notification_type": string(notification.Type),
			"channel":           string(notification.Channel),
		})
		return fmt.Errorf("failed to send notification: %w", err)
	}

	// Mark notification as sent
	notification.MarkAsSent()

	ec.logger.Info(ctx, "Notification sent successfully", map[string]interface{}{
		"notification_id": notification.ID,
		"user_id":         notification.UserID,
		"type":            notification.Type,
		"chat_id":         chatID,
	})
	ec.metrics.IncrementCounter("notification_send_success", map[string]string{
		"notification_type": string(notification.Type),
		"channel":           string(notification.Channel),
	})

	return nil
}
