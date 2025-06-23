package kafka

import (
	"context"
	"encoding/json"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"

	"github.com/amiosamu/rocket-science/services/order-service/internal/service"
	"github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// Producer handles publishing messages to Kafka topics
type Producer struct {
	producer sarama.SyncProducer
	topic    string
	logger   logging.Logger
}

// NewProducer creates a new Kafka producer for payment events
func NewProducer(brokers []string, topic string, retries int, logger logging.Logger) (*Producer, error) {
	config := sarama.NewConfig()
	
	// Producer configuration for reliability
	config.Producer.RequiredAcks = sarama.WaitForAll // Wait for all replicas
	config.Producer.Retry.Max = retries
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	
	// Performance optimizations
	config.Producer.Compression = sarama.CompressionSnappy
	config.Producer.Flush.Frequency = 500 * time.Millisecond
	config.Producer.Flush.Messages = 100
	
	// Reliability settings
	config.Producer.Idempotent = true
	config.Net.MaxOpenRequests = 1 // Required for idempotent producer
	
	// Message ordering
	config.Producer.Partitioner = sarama.NewManualPartitioner

	producer, err := sarama.NewSyncProducer(brokers, config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Kafka producer")
	}

	logger.Info(nil, "Kafka producer created successfully", map[string]interface{}{
		"brokers": brokers,
		"topic":   topic,
		"retries": retries,
	})

	return &Producer{
		producer: producer,
		topic:    topic,
		logger:   logger,
	}, nil
}

// PublishPaymentEvent publishes a payment event to Kafka
func (p *Producer) PublishPaymentEvent(ctx context.Context, event service.PaymentEvent) error {
	// Add event metadata for traceability
	eventWithMetadata := PaymentEventMessage{
		PaymentEvent: event,
		EventMetadata: EventMetadata{
			EventID:   uuid.New().String(),
			EventType: "payment.processed",
			EventTime: time.Now().UTC(),
			Version:   "1.0",
			Source:    "order-service",
		},
	}

	// Marshal event to JSON
	data, err := json.Marshal(eventWithMetadata)
	if err != nil {
		return errors.Wrap(err, "failed to marshal payment event")
	}

	// Create Kafka message
	message := &sarama.ProducerMessage{
		Topic:     p.topic,
		Key:       sarama.StringEncoder(event.OrderID.String()), // Partition by order ID
		Value:     sarama.ByteEncoder(data),
		Timestamp: eventWithMetadata.EventMetadata.EventTime, // Fixed: access through EventMetadata
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("event-type"),
				Value: []byte(eventWithMetadata.EventMetadata.EventType), // Fixed: access through EventMetadata
			},
			{
				Key:   []byte("event-id"),
				Value: []byte(eventWithMetadata.EventMetadata.EventID), // Fixed: access through EventMetadata
			},
			{
				Key:   []byte("event-version"),
				Value: []byte(eventWithMetadata.EventMetadata.Version), // Fixed: access through EventMetadata
			},
			{
				Key:   []byte("source-service"),
				Value: []byte(eventWithMetadata.EventMetadata.Source), // Fixed: access through EventMetadata
			},
			{
				Key:   []byte("order-id"),
				Value: []byte(event.OrderID.String()),
			},
		},
	}

	// Publish message
	partition, offset, err := p.producer.SendMessage(message)
	if err != nil {
		p.logger.Error(ctx, "Failed to publish payment event", err, map[string]interface{}{
			"order_id":  event.OrderID,
			"event_id":  eventWithMetadata.EventMetadata.EventID, // Fixed: access through EventMetadata
			"topic":     p.topic,
		})
		return errors.Wrap(err, "failed to publish payment event")
	}

	p.logger.Info(ctx, "Payment event published successfully", map[string]interface{}{
		"order_id":       event.OrderID,
		"event_id":       eventWithMetadata.EventMetadata.EventID, // Fixed: access through EventMetadata
		"topic":          p.topic,
		"partition":      partition,
		"offset":         offset,
		"transaction_id": event.TransactionID,
		"amount":         event.Amount,
	})

	return nil
}

// PublishOrderStatusEvent publishes order status change events (for future use)
func (p *Producer) PublishOrderStatusEvent(ctx context.Context, orderID uuid.UUID, oldStatus, newStatus string) error {
	eventWithMetadata := OrderStatusEventMessage{
		OrderStatusEvent: OrderStatusEvent{
			OrderID:   orderID,
			OldStatus: oldStatus,
			NewStatus: newStatus,
		},
		EventMetadata: EventMetadata{
			EventID:   uuid.New().String(),
			EventType: "order.status.changed",
			EventTime: time.Now().UTC(),
			Version:   "1.0",
			Source:    "order-service",
		},
	}

	data, err := json.Marshal(eventWithMetadata)
	if err != nil {
		return errors.Wrap(err, "failed to marshal order status event")
	}

	message := &sarama.ProducerMessage{
		Topic:     p.topic,
		Key:       sarama.StringEncoder(orderID.String()),
		Value:     sarama.ByteEncoder(data),
		Timestamp: eventWithMetadata.EventMetadata.EventTime, // Fixed: access through EventMetadata
		Headers: []sarama.RecordHeader{
			{
				Key:   []byte("event-type"),
				Value: []byte(eventWithMetadata.EventMetadata.EventType), // Fixed: access through EventMetadata
			},
			{
				Key:   []byte("event-id"),
				Value: []byte(eventWithMetadata.EventMetadata.EventID), // Fixed: access through EventMetadata
			},
			{
				Key:   []byte("order-id"),
				Value: []byte(orderID.String()),
			},
		},
	}

	partition, offset, err := p.producer.SendMessage(message)
	if err != nil {
		p.logger.Error(ctx, "Failed to publish order status event", err)
		return errors.Wrap(err, "failed to publish order status event")
	}

	p.logger.Info(ctx, "Order status event published", map[string]interface{}{
		"order_id":   orderID,
		"old_status": oldStatus,
		"new_status": newStatus,
		"partition":  partition,
		"offset":     offset,
	})

	return nil
}

// Close closes the Kafka producer
func (p *Producer) Close() error {
	if p.producer != nil {
		err := p.producer.Close()
		if err != nil {
			p.logger.Error(nil, "Failed to close Kafka producer", err)
			return err
		}
		p.logger.Info(nil, "Kafka producer closed successfully")
	}
	return nil
}

// Event message structures

// EventMetadata contains common metadata for all events
type EventMetadata struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	EventTime time.Time `json:"event_time"`
	Version   string    `json:"version"`
	Source    string    `json:"source"`
}

// PaymentEventMessage represents a payment event with metadata
type PaymentEventMessage struct {
	service.PaymentEvent
	EventMetadata EventMetadata `json:"metadata"`
}

// OrderStatusEvent represents an order status change
type OrderStatusEvent struct {
	OrderID   uuid.UUID `json:"order_id"`
	OldStatus string    `json:"old_status"`
	NewStatus string    `json:"new_status"`
}

// OrderStatusEventMessage represents an order status event with metadata
type OrderStatusEventMessage struct {
	OrderStatusEvent OrderStatusEvent `json:"order_status"`
	EventMetadata    EventMetadata    `json:"metadata"`
}