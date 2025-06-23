package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"

	platformErrors "github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// OrderService interface for the consumer (to avoid circular imports)
type OrderService interface {
	HandleAssemblyCompleted(ctx context.Context, orderID uuid.UUID) error
}

// Consumer handles consuming messages from Kafka topics
type Consumer struct {
	consumerGroup sarama.ConsumerGroup
	topics        []string
	handler       *ConsumerHandler
	logger        logging.Logger
	ready         chan bool
}

// NewConsumer creates a new Kafka consumer for assembly events
func NewConsumer(brokers []string, groupID string, topics []string, orderService OrderService, logger logging.Logger) (*Consumer, error) {
	config := sarama.NewConfig()

	// Consumer configuration
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	config.Consumer.Offsets.Initial = sarama.OffsetNewest
	config.Consumer.Group.Session.Timeout = 30 * time.Second
	config.Consumer.Group.Heartbeat.Interval = 3 * time.Second
	config.Consumer.Return.Errors = true

	// Auto-commit settings
	config.Consumer.Offsets.AutoCommit.Enable = true
	config.Consumer.Offsets.AutoCommit.Interval = 1 * time.Second

	consumerGroup, err := sarama.NewConsumerGroup(brokers, groupID, config)
	if err != nil {
		return nil, platformErrors.Wrap(err, "failed to create Kafka consumer group")
	}

	handler := &ConsumerHandler{
		orderService: orderService,
		logger:       logger,
	}

	logger.Info(nil, "Kafka consumer created successfully", map[string]interface{}{
		"brokers":  brokers,
		"group_id": groupID,
		"topics":   topics,
	})

	return &Consumer{
		consumerGroup: consumerGroup,
		topics:        topics,
		handler:       handler,
		logger:        logger,
		ready:         make(chan bool),
	}, nil
}

// Start starts consuming messages in a blocking manner
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info(ctx, "Starting Kafka consumer", map[string]interface{}{
		"topics": c.topics,
	})

	// Start error handling goroutine
	go c.handleErrors(ctx)

	// Start consuming
	for {
		select {
		case <-ctx.Done():
			c.logger.Info(ctx, "Kafka consumer context cancelled")
			return nil
		default:
			// This is a blocking call that will handle rebalancing, heartbeat, etc.
			if err := c.consumerGroup.Consume(ctx, c.topics, c.handler); err != nil {
				c.logger.Error(ctx, "Error consuming from Kafka", err)

				// Check if it's a recoverable error
				if errors.Is(err, sarama.ErrClosedConsumerGroup) {
					c.logger.Info(ctx, "Consumer group closed, stopping consumer")
					return nil
				}

				// For other errors, wait and retry
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(5 * time.Second):
					c.logger.Info(ctx, "Retrying Kafka consumer connection")
					continue
				}
			}
		}
	}
}

// handleErrors processes consumer errors in the background
func (c *Consumer) handleErrors(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case consumerErr := <-c.consumerGroup.Errors():
			if consumerErr != nil {
				// Try to cast to sarama.ConsumerError to get additional details
				if saramaErr, ok := consumerErr.(*sarama.ConsumerError); ok {
					c.logger.Error(ctx, "Kafka consumer error", consumerErr, map[string]interface{}{
						"topic":     saramaErr.Topic,
						"partition": saramaErr.Partition,
						"error":     saramaErr.Err.Error(),
					})
				} else {
					// Fallback for other error types
					c.logger.Error(ctx, "Kafka consumer error", consumerErr)
				}
			}
		}
	}
}

// Close closes the Kafka consumer
func (c *Consumer) Close() error {
	if c.consumerGroup != nil {
		err := c.consumerGroup.Close()
		if err != nil {
			c.logger.Error(nil, "Failed to close Kafka consumer", err)
			return err
		}
		c.logger.Info(nil, "Kafka consumer closed successfully")
	}
	return nil
}

// ConsumerHandler implements sarama.ConsumerGroupHandler
type ConsumerHandler struct {
	orderService OrderService
	logger       logging.Logger
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (h *ConsumerHandler) Setup(sarama.ConsumerGroupSession) error {
	h.logger.Info(nil, "Kafka consumer session setup")
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (h *ConsumerHandler) Cleanup(sarama.ConsumerGroupSession) error {
	h.logger.Info(nil, "Kafka consumer session cleanup")
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages()
func (h *ConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	// Start consuming messages from the claim
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			if err := h.handleMessage(session.Context(), message); err != nil {
				h.logger.Error(session.Context(), "Failed to handle message", err, map[string]interface{}{
					"topic":     message.Topic,
					"partition": message.Partition,
					"offset":    message.Offset,
					"key":       string(message.Key),
				})
				// Continue processing other messages even if one fails
			}

			// Mark message as processed
			session.MarkMessage(message, "")

		case <-session.Context().Done():
			return nil
		}
	}
}

// handleMessage processes individual Kafka messages
func (h *ConsumerHandler) handleMessage(ctx context.Context, message *sarama.ConsumerMessage) error {
	h.logger.Debug(ctx, "Received Kafka message", map[string]interface{}{
		"topic":     message.Topic,
		"partition": message.Partition,
		"offset":    message.Offset,
		"timestamp": message.Timestamp,
	})

	// Get event type from headers
	eventType := h.getHeaderValue(message.Headers, "event-type")
	eventID := h.getHeaderValue(message.Headers, "event-id")

	h.logger.Debug(ctx, "Processing event", map[string]interface{}{
		"event_type": eventType,
		"event_id":   eventID,
	})

	switch eventType {
	case "assembly.completed":
		return h.handleAssemblyCompletedEvent(ctx, message.Value, eventID)
	case "assembly.failed":
		return h.handleAssemblyFailedEvent(ctx, message.Value, eventID)
	default:
		h.logger.Warn(ctx, "Unknown event type received", map[string]interface{}{
			"event_type": eventType,
			"event_id":   eventID,
		})
		return nil // Don't fail on unknown events
	}
}

// handleAssemblyCompletedEvent handles assembly completed events
func (h *ConsumerHandler) handleAssemblyCompletedEvent(ctx context.Context, data []byte, eventID string) error {
	var event AssemblyCompletedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return platformErrors.Wrap(err, "failed to unmarshal assembly completed event")
	}

	orderID, err := uuid.Parse(event.OrderID)
	if err != nil {
		return platformErrors.Wrap(err, "invalid order ID in assembly completed event")
	}

	h.logger.Info(ctx, "Processing assembly completed event", map[string]interface{}{
		"order_id":     orderID,
		"event_id":     eventID,
		"completed_at": event.CompletedAt,
	})

	// Delegate to order service
	if err := h.orderService.HandleAssemblyCompleted(ctx, orderID); err != nil {
		h.logger.Error(ctx, "Failed to handle assembly completed event", err, map[string]interface{}{
			"order_id": orderID,
			"event_id": eventID,
		})
		return platformErrors.Wrap(err, "failed to handle assembly completed")
	}

	h.logger.Info(ctx, "Assembly completed event processed successfully", map[string]interface{}{
		"order_id": orderID,
		"event_id": eventID,
	})

	return nil
}

// handleAssemblyFailedEvent handles assembly failed events
func (h *ConsumerHandler) handleAssemblyFailedEvent(ctx context.Context, data []byte, eventID string) error {
	var event AssemblyFailedEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return platformErrors.Wrap(err, "failed to unmarshal assembly failed event")
	}

	orderID, err := uuid.Parse(event.OrderID)
	if err != nil {
		return platformErrors.Wrap(err, "invalid order ID in assembly failed event")
	}

	h.logger.Warn(ctx, "Assembly failed event received", map[string]interface{}{
		"order_id": orderID,
		"event_id": eventID,
		"reason":   event.Reason,
	})

	// TODO: Implement failure handling logic
	// For now, just log the event
	// In a real system, you might want to:
	// - Update order status to failed
	// - Initiate refund process
	// - Send notification to customer
	// - Release inventory reservation

	return nil
}

// getHeaderValue extracts a header value from Kafka message headers
func (h *ConsumerHandler) getHeaderValue(headers []*sarama.RecordHeader, key string) string {
	for _, header := range headers {
		if string(header.Key) == key {
			return string(header.Value)
		}
	}
	return ""
}

// Event structures for incoming messages

// AssemblyCompletedEvent represents an assembly completed event from Assembly Service
type AssemblyCompletedEvent struct {
	EventID     string    `json:"event_id"`
	EventType   string    `json:"event_type"`
	EventTime   time.Time `json:"event_time"`
	Version     string    `json:"version"`
	Source      string    `json:"source"`
	OrderID     string    `json:"order_id"`
	UserID      string    `json:"user_id"`
	CompletedAt time.Time `json:"completed_at"`
	Duration    int       `json:"duration_seconds"` // Assembly duration in seconds
}

// AssemblyFailedEvent represents an assembly failed event from Assembly Service
type AssemblyFailedEvent struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	EventTime time.Time `json:"event_time"`
	Version   string    `json:"version"`
	Source    string    `json:"source"`
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	Reason    string    `json:"reason"`
	FailedAt  time.Time `json:"failed_at"`
}
