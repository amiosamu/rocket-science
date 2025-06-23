package kafka

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/amiosamu/rocket-science/services/assembly-service/internal/domain"
	"github.com/amiosamu/rocket-science/shared/platform/messaging/kafka"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
	"github.com/amiosamu/rocket-science/shared/proto/events"
)

// AssemblyProducer wraps the shared Kafka producer with assembly-specific logic
type AssemblyProducer struct {
	producer *kafka.Producer
	logger   logging.Logger
	metrics  metrics.Metrics
	topics   struct {
		assemblyStarted   string
		assemblyCompleted string
		assemblyFailed    string
	}
}

// NewAssemblyProducer creates a new assembly producer
func NewAssemblyProducer(
	config kafka.ProducerConfig,
	logger logging.Logger,
	metrics metrics.Metrics,
	assemblyStartedTopic string,
	assemblyCompletedTopic string,
	assemblyFailedTopic string,
) (*AssemblyProducer, error) {
	producer, err := kafka.NewProducer(config, logger, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	assemblyProducer := &AssemblyProducer{
		producer: producer,
		logger:   logger,
		metrics:  metrics,
	}

	assemblyProducer.topics.assemblyStarted = assemblyStartedTopic
	assemblyProducer.topics.assemblyCompleted = assemblyCompletedTopic
	assemblyProducer.topics.assemblyFailed = assemblyFailedTopic

	return assemblyProducer, nil
}

// PublishAssemblyStarted publishes an assembly started event
func (p *AssemblyProducer) PublishAssemblyStarted(ctx context.Context, assembly *domain.Assembly) error {
	p.logger.Info(ctx, "Publishing assembly started event", map[string]interface{}{
		"assembly_id": assembly.ID,
		"order_id":    assembly.OrderID,
		"user_id":     assembly.UserID,
	})

	// Note: For this demo, we'll use simplified component data in the events
	// In a full implementation, components would be properly mapped

	// Create assembly started event
	assemblyEvent := &events.AssemblyStartedEvent{
		AssemblyId:               assembly.ID,
		OrderId:                  assembly.OrderID,
		UserId:                   assembly.UserID,
		Components:               []*events.RocketComponent{}, // Simplified for demo
		StartedAt:                timestamppb.New(time.Now()),
		EstimatedDurationSeconds: assembly.EstimatedDurationSeconds,
	}

	return p.publishEvent(ctx, p.topics.assemblyStarted, "assembly.started", assembly.OrderID, assemblyEvent)
}

// PublishAssemblyCompleted publishes an assembly completed event
func (p *AssemblyProducer) PublishAssemblyCompleted(ctx context.Context, assembly *domain.Assembly) error {
	p.logger.Info(ctx, "Publishing assembly completed event", map[string]interface{}{
		"assembly_id": assembly.ID,
		"order_id":    assembly.OrderID,
		"user_id":     assembly.UserID,
		"quality":     assembly.Quality.String(),
	})

	// Create assembly completed event
	assemblyEvent := &events.AssemblyCompletedEvent{
		AssemblyId:            assembly.ID,
		OrderId:               assembly.OrderID,
		UserId:                assembly.UserID,
		ActualDurationSeconds: assembly.ActualDurationSeconds,
		Quality:               events.AssemblyQuality(assembly.Quality),
		CompletedAt:           timestamppb.New(*assembly.CompletedAt),
	}

	return p.publishEvent(ctx, p.topics.assemblyCompleted, "assembly.completed", assembly.OrderID, assemblyEvent)
}

// PublishAssemblyFailed publishes an assembly failed event
func (p *AssemblyProducer) PublishAssemblyFailed(ctx context.Context, assembly *domain.Assembly) error {
	p.logger.Error(ctx, "Publishing assembly failed event", nil, map[string]interface{}{
		"assembly_id":    assembly.ID,
		"order_id":       assembly.OrderID,
		"user_id":        assembly.UserID,
		"failure_reason": assembly.FailureReason,
		"error_code":     assembly.ErrorCode,
	})

	// Create assembly failed event
	assemblyEvent := &events.AssemblyFailedEvent{
		AssemblyId:       assembly.ID,
		OrderId:          assembly.OrderID,
		UserId:           assembly.UserID,
		Reason:           assembly.FailureReason,
		ErrorCode:        assembly.ErrorCode,
		FailedAt:         timestamppb.New(*assembly.FailedAt),
		FailedComponents: []string{}, // Could be filled with actual failed components
	}

	return p.publishEvent(ctx, p.topics.assemblyFailed, "assembly.failed", assembly.OrderID, assemblyEvent)
}

// publishEvent is a helper method to publish events with consistent structure
func (p *AssemblyProducer) publishEvent(ctx context.Context, topic, eventType, orderID string, eventData interface{}) error {
	// For demo purposes, we'll use simple JSON serialization
	// In production, this would use proper protobuf serialization

	p.logger.Info(ctx, "Publishing assembly event", map[string]interface{}{
		"event_type": eventType,
		"topic":      topic,
		"order_id":   orderID,
	})

	// Simple event structure for demo
	simpleEvent := map[string]interface{}{
		"id":        uuid.New().String(),
		"type":      eventType,
		"source":    "assembly-service",
		"subject":   orderID,
		"timestamp": time.Now(),
		"data":      eventData,
	}

	// Send the event
	if err := p.producer.SendMessage(ctx, topic, orderID, simpleEvent, nil); err != nil {
		p.logger.Error(ctx, "Failed to publish event", err, map[string]interface{}{
			"event_type": eventType,
			"topic":      topic,
			"order_id":   orderID,
		})
		p.metrics.IncrementCounter("kafka_publish_errors_total", map[string]string{
			"topic": topic,
			"error": "send_failed",
		})
		return fmt.Errorf("failed to publish event: %w", err)
	}

	p.logger.Info(ctx, "Successfully published event", map[string]interface{}{
		"event_type": eventType,
		"topic":      topic,
		"order_id":   orderID,
	})

	p.metrics.IncrementCounter("kafka_events_published_total", map[string]string{
		"topic":      topic,
		"event_type": eventType,
	})

	return nil
}

// Close closes the producer
func (p *AssemblyProducer) Close() error {
	p.logger.Info(nil, "Closing assembly producer")
	return p.producer.Close()
}

// HealthCheck checks the health of the producer
func (p *AssemblyProducer) HealthCheck(ctx context.Context) error {
	return p.producer.HealthCheck(ctx)
}
