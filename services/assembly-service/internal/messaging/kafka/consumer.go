package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/amiosamu/rocket-science/shared/platform/messaging/kafka"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
	"github.com/amiosamu/rocket-science/shared/proto/events"
)

// PaymentEventHandler handles payment-related events
type PaymentEventHandler interface {
	HandlePaymentProcessed(ctx context.Context, event *events.PaymentProcessedEvent) error
}

// AssemblyConsumer wraps the shared Kafka consumer with assembly-specific logic
type AssemblyConsumer struct {
	consumer       *kafka.Consumer
	paymentHandler PaymentEventHandler
	logger         logging.Logger
	metrics        metrics.Metrics
	topics         []string
}

// NewAssemblyConsumer creates a new assembly consumer
func NewAssemblyConsumer(
	config kafka.ConsumerConfig,
	paymentHandler PaymentEventHandler,
	logger logging.Logger,
	metrics metrics.Metrics,
) (*AssemblyConsumer, error) {
	consumer, err := kafka.NewConsumer(config, logger, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	assemblyConsumer := &AssemblyConsumer{
		consumer:       consumer,
		paymentHandler: paymentHandler,
		logger:         logger,
		metrics:        metrics,
		topics:         config.Topics,
	}

	// Register this consumer as the message handler
	consumer.RegisterHandler(assemblyConsumer)

	return assemblyConsumer, nil
}

// GetSupportedTopics returns the topics this consumer handles
func (c *AssemblyConsumer) GetSupportedTopics() []string {
	return c.topics
}

// HandleMessage processes incoming Kafka messages
func (c *AssemblyConsumer) HandleMessage(ctx context.Context, message *kafka.Message) error {
	c.logger.Debug(ctx, "Received Kafka message", map[string]interface{}{
		"topic":      message.Topic,
		"partition":  message.Partition,
		"offset":     message.Offset,
		"event_type": message.EventType,
		"event_id":   message.EventID,
	})

	// Record metrics
	c.metrics.IncrementCounter("kafka_messages_received_total", map[string]string{
		"topic":      message.Topic,
		"event_type": message.EventType,
	})

	// Timer for processing duration
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		c.metrics.RecordDuration("kafka_message_processing_duration_seconds", duration, map[string]string{
			"topic":      message.Topic,
			"event_type": message.EventType,
		})
	}()

	// Parse the event envelope
	var envelope events.EventEnvelope
	if err := json.Unmarshal(message.Value, &envelope); err != nil {
		c.logger.Error(ctx, "Failed to unmarshal event envelope", err, map[string]interface{}{
			"topic":     message.Topic,
			"offset":    message.Offset,
			"raw_value": string(message.Value),
		})
		c.metrics.IncrementCounter("kafka_message_processing_errors_total", map[string]string{
			"topic": message.Topic,
			"error": "unmarshal_envelope_failed",
		})
		return fmt.Errorf("failed to unmarshal event envelope: %w", err)
	}

	// Route based on event type
	switch envelope.Event.Type {
	case "payment.processed":
		return c.handlePaymentProcessedEvent(ctx, &envelope)
	default:
		c.logger.Warn(ctx, "Received unknown event type", map[string]interface{}{
			"event_type": envelope.Event.Type,
			"event_id":   envelope.Event.Id,
			"topic":      message.Topic,
		})
		c.metrics.IncrementCounter("kafka_message_processing_errors_total", map[string]string{
			"topic": message.Topic,
			"error": "unknown_event_type",
		})
		// Don't return error for unknown events to avoid infinite retries
		return nil
	}
}

// handlePaymentProcessedEvent processes payment processed events
func (c *AssemblyConsumer) handlePaymentProcessedEvent(ctx context.Context, envelope *events.EventEnvelope) error {
	c.logger.Info(ctx, "Processing payment processed event", map[string]interface{}{
		"event_id": envelope.Event.Id,
		"source":   envelope.Event.Source,
	})

	// Extract payment processed event from Any
	var paymentEvent events.PaymentProcessedEvent
	if err := envelope.Event.Data.UnmarshalTo(&paymentEvent); err != nil {
		c.logger.Error(ctx, "Failed to unmarshal payment processed event", err, map[string]interface{}{
			"event_id": envelope.Event.Id,
		})
		c.metrics.IncrementCounter("kafka_message_processing_errors_total", map[string]string{
			"event_type": "payment.processed",
			"error":      "unmarshal_event_data_failed",
		})
		return fmt.Errorf("failed to unmarshal payment processed event: %w", err)
	}

	// Validate required fields
	if paymentEvent.OrderId == "" {
		c.logger.Error(ctx, "Payment event missing order ID", nil, map[string]interface{}{
			"event_id":   envelope.Event.Id,
			"payment_id": paymentEvent.PaymentId,
		})
		c.metrics.IncrementCounter("kafka_message_processing_errors_total", map[string]string{
			"event_type": "payment.processed",
			"error":      "missing_order_id",
		})
		return fmt.Errorf("payment event missing order ID")
	}

	if paymentEvent.UserId == "" {
		c.logger.Error(ctx, "Payment event missing user ID", nil, map[string]interface{}{
			"event_id":   envelope.Event.Id,
			"order_id":   paymentEvent.OrderId,
			"payment_id": paymentEvent.PaymentId,
		})
		c.metrics.IncrementCounter("kafka_message_processing_errors_total", map[string]string{
			"event_type": "payment.processed",
			"error":      "missing_user_id",
		})
		return fmt.Errorf("payment event missing user ID")
	}

	// Check payment status
	if paymentEvent.Status != events.PaymentStatus_PAYMENT_STATUS_COMPLETED {
		c.logger.Warn(ctx, "Received payment event with non-completed status", map[string]interface{}{
			"event_id":   envelope.Event.Id,
			"order_id":   paymentEvent.OrderId,
			"payment_id": paymentEvent.PaymentId,
			"status":     paymentEvent.Status.String(),
		})
		// Don't process incomplete payments
		return nil
	}

	// Hand off to payment handler
	if err := c.paymentHandler.HandlePaymentProcessed(ctx, &paymentEvent); err != nil {
		c.logger.Error(ctx, "Failed to handle payment processed event", err, map[string]interface{}{
			"event_id":   envelope.Event.Id,
			"order_id":   paymentEvent.OrderId,
			"payment_id": paymentEvent.PaymentId,
		})
		c.metrics.IncrementCounter("kafka_message_processing_errors_total", map[string]string{
			"event_type": "payment.processed",
			"error":      "handler_failed",
		})
		return fmt.Errorf("failed to handle payment processed event: %w", err)
	}

	c.logger.Info(ctx, "Successfully processed payment event", map[string]interface{}{
		"event_id":   envelope.Event.Id,
		"order_id":   paymentEvent.OrderId,
		"payment_id": paymentEvent.PaymentId,
	})

	c.metrics.IncrementCounter("kafka_messages_processed_total", map[string]string{
		"event_type": "payment.processed",
		"status":     "success",
	})

	return nil
}

// Start starts the consumer
func (c *AssemblyConsumer) Start(ctx context.Context) error {
	c.logger.Info(ctx, "Starting assembly consumer", map[string]interface{}{
		"topics": c.topics,
	})

	return c.consumer.Start(ctx)
}

// Stop stops the consumer
func (c *AssemblyConsumer) Stop() error {
	c.logger.Info(nil, "Stopping assembly consumer")
	return c.consumer.Stop()
}

// HealthCheck checks the health of the consumer
func (c *AssemblyConsumer) HealthCheck(ctx context.Context) error {
	return c.consumer.HealthCheck(ctx)
}
