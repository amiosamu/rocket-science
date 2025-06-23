package kafka

import (
	"context"
	"fmt"

	"github.com/amiosamu/rocket-science/services/order-service/internal/config"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// MessagingCoordinator manages both Kafka producer and consumer
type MessagingCoordinator struct {
	Producer *Producer
	Consumer *Consumer
	logger   logging.Logger
}

// NewMessagingCoordinator creates a new messaging coordinator with producer and consumer
func NewMessagingCoordinator(cfg config.KafkaConfig, orderService OrderService, logger logging.Logger) (*MessagingCoordinator, error) {
	// Create producer for payment events
	producer, err := NewProducer(
		cfg.Brokers,
		cfg.PaymentEventsTopic,
		cfg.ProducerRetries,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	// Create consumer for assembly events
	consumer, err := NewConsumer(
		cfg.Brokers,
		cfg.ConsumerGroup,
		[]string{cfg.AssemblyEventsTopic},
		orderService,
		logger,
	)
	if err != nil {
		// Clean up producer if consumer creation fails
		producer.Close()
		return nil, fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	logger.Info(nil, "Messaging coordinator created successfully", map[string]interface{}{
		"payment_topic":  cfg.PaymentEventsTopic,
		"assembly_topic": cfg.AssemblyEventsTopic,
		"consumer_group": cfg.ConsumerGroup,
		"brokers":        cfg.Brokers,
	})

	return &MessagingCoordinator{
		Producer: producer,
		Consumer: consumer,
		logger:   logger,
	}, nil
}

// StartConsumer starts the Kafka consumer in a separate goroutine
func (mc *MessagingCoordinator) StartConsumer(ctx context.Context) <-chan error {
	errChan := make(chan error, 1)
	
	go func() {
		defer close(errChan)
		
		mc.logger.Info(ctx, "Starting Kafka consumer")
		if err := mc.Consumer.Start(ctx); err != nil {
			mc.logger.Error(ctx, "Kafka consumer failed", err)
			errChan <- err
		}
	}()
	
	return errChan
}

// Close closes both producer and consumer
func (mc *MessagingCoordinator) Close() error {
	var errors []error

	if mc.Producer != nil {
		if err := mc.Producer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close producer: %w", err))
		}
	}

	if mc.Consumer != nil {
		if err := mc.Consumer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close consumer: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing messaging components: %v", errors)
	}

	mc.logger.Info(nil, "Messaging coordinator closed successfully")
	return nil
}

// Topic names for reference
const (
	PaymentEventsTopic  = "payment-events"
	AssemblyEventsTopic = "assembly-events"
	OrderEventsTopic    = "order-events"
)

// Event types for reference
const (
	PaymentProcessedEventType   = "payment.processed"
	PaymentFailedEventType      = "payment.failed"
	AssemblyCompletedEventType  = "assembly.completed"
	AssemblyFailedEventType     = "assembly.failed"
	OrderStatusChangedEventType = "order.status.changed"
	OrderCreatedEventType       = "order.created"
)

// Health check for messaging components
func (mc *MessagingCoordinator) HealthCheck() map[string]interface{} {
	health := map[string]interface{}{
		"messaging": "healthy",
	}

	// Add more specific health checks here if needed
	// For example, checking if Kafka brokers are reachable
	
	return health
}

// GetMessageStats returns statistics about message processing
func (mc *MessagingCoordinator) GetMessageStats() map[string]interface{} {
	// In a real implementation, you might track:
	// - Messages published count
	// - Messages consumed count
	// - Failed message count
	// - Last message timestamp
	// - Consumer lag
	
	return map[string]interface{}{
		"producer_active": mc.Producer != nil,
		"consumer_active": mc.Consumer != nil,
		// Add more stats as needed
	}
}