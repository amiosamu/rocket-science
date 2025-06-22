package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"

	platformError "github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// ConsumerConfig holds Kafka consumer configuration
type ConsumerConfig struct {
	Brokers              []string      `json:"brokers"`
	GroupID              string        `json:"group_id"`
	ClientID             string        `json:"client_id"`
	Topics               []string      `json:"topics"`
	SessionTimeout       time.Duration `json:"session_timeout"`
	HeartbeatInterval    time.Duration `json:"heartbeat_interval"`
	RebalanceTimeout     time.Duration `json:"rebalance_timeout"`
	InitialOffset        string        `json:"initial_offset"` // "oldest" or "newest"
	EnableAutoCommit     bool          `json:"enable_auto_commit"`
	AutoCommitInterval   time.Duration `json:"auto_commit_interval"`
	MaxProcessingTime    time.Duration `json:"max_processing_time"`
	ConcurrencyLevel     int           `json:"concurrency_level"`
	RetryAttempts        int           `json:"retry_attempts"`
	RetryBackoff         time.Duration `json:"retry_backoff"`
	EnableDeadLetter     bool          `json:"enable_dead_letter"`
	DeadLetterTopic      string        `json:"dead_letter_topic"`
}

// DefaultConsumerConfig returns default consumer configuration
func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		Brokers:              []string{"localhost:9092"},
		GroupID:              "shared-consumer-group",
		ClientID:             "shared-consumer",
		Topics:               []string{},
		SessionTimeout:       30 * time.Second,
		HeartbeatInterval:    3 * time.Second,
		RebalanceTimeout:     60 * time.Second,
		InitialOffset:        "newest",
		EnableAutoCommit:     true,
		AutoCommitInterval:   1 * time.Second,
		MaxProcessingTime:    30 * time.Second,
		ConcurrencyLevel:     1,
		RetryAttempts:        3,
		RetryBackoff:         1 * time.Second,
		EnableDeadLetter:     false,
		DeadLetterTopic:      "",
	}
}

// MessageHandler defines the interface for message processing
type MessageHandler interface {
	HandleMessage(ctx context.Context, message *Message) error
	GetSupportedTopics() []string
}

// Message represents a consumed Kafka message with additional metadata
type Message struct {
	Topic       string            `json:"topic"`
	Partition   int32             `json:"partition"`
	Offset      int64             `json:"offset"`
	Key         string            `json:"key"`
	Value       []byte            `json:"value"`
	Headers     map[string]string `json:"headers"`
	Timestamp   time.Time         `json:"timestamp"`
	EventType   string            `json:"event_type"`
	EventID     string            `json:"event_id"`
	EventSource string            `json:"event_source"`
}

// UnmarshalValue unmarshals the message value into the provided interface
func (m *Message) UnmarshalValue(v interface{}) error {
	return json.Unmarshal(m.Value, v)
}

// Consumer provides a high-level Kafka consumer interface
type Consumer struct {
	consumerGroup sarama.ConsumerGroup
	config        ConsumerConfig
	logger        logging.Logger
	metrics       metrics.Metrics
	handlers      map[string]MessageHandler
	ready         chan bool
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	running       bool
	mu            sync.RWMutex
}

// NewConsumer creates a new Kafka consumer
func NewConsumer(config ConsumerConfig, logger logging.Logger, metrics metrics.Metrics) (*Consumer, error) {
	saramaConfig := sarama.NewConfig()
	
	// Basic configuration
	saramaConfig.ClientID = config.ClientID
	saramaConfig.Consumer.Return.Errors = true
	
	// Group configuration
	saramaConfig.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	saramaConfig.Consumer.Group.Session.Timeout = config.SessionTimeout
	saramaConfig.Consumer.Group.Heartbeat.Interval = config.HeartbeatInterval
	saramaConfig.Consumer.Group.Rebalance.Timeout = config.RebalanceTimeout
	
	// Offset configuration
	if config.InitialOffset == "oldest" {
		saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
	} else {
		saramaConfig.Consumer.Offsets.Initial = sarama.OffsetNewest
	}
	
	// Auto-commit configuration
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = config.EnableAutoCommit
	saramaConfig.Consumer.Offsets.AutoCommit.Interval = config.AutoCommitInterval

	// Create consumer group
	consumerGroup, err := sarama.NewConsumerGroup(config.Brokers, config.GroupID, saramaConfig)
	if err != nil {
		return nil, platformError.Wrap(err, "failed to create Kafka consumer group")
	}

	ctx, cancel := context.WithCancel(context.Background())

	consumer := &Consumer{
		consumerGroup: consumerGroup,
		config:        config,
		logger:        logger,
		metrics:       metrics,
		handlers:      make(map[string]MessageHandler),
		ready:         make(chan bool),
		ctx:           ctx,
		cancel:        cancel,
		running:       false,
	}

	logger.Info(nil, "Kafka consumer created successfully", map[string]interface{}{
		"brokers":            config.Brokers,
		"group_id":           config.GroupID,
		"client_id":          config.ClientID,
		"topics":             config.Topics,
		"session_timeout":    config.SessionTimeout,
		"initial_offset":     config.InitialOffset,
		"auto_commit":        config.EnableAutoCommit,
		"concurrency_level":  config.ConcurrencyLevel,
	})

	return consumer, nil
}

// RegisterHandler registers a message handler for specific topics
func (c *Consumer) RegisterHandler(handler MessageHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for _, topic := range handler.GetSupportedTopics() {
		c.handlers[topic] = handler
		c.logger.Info(nil, "Registered message handler", map[string]interface{}{
			"topic":   topic,
			"handler": fmt.Sprintf("%T", handler),
		})
	}
}

// Start starts the consumer
func (c *Consumer) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return platformError.NewInternal("consumer is already running")
	}
	c.running = true
	c.mu.Unlock()

	if len(c.config.Topics) == 0 {
		return platformError.NewValidation("no topics configured for consumer")
	}

	c.logger.Info(ctx, "Starting Kafka consumer", map[string]interface{}{
		"topics":   c.config.Topics,
		"group_id": c.config.GroupID,
	})

	// Start error handling goroutine
	c.wg.Add(1)
	go c.handleErrors()

	// Start consumer group
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.ctx.Done():
				c.logger.Info(ctx, "Consumer context cancelled")
				return
			default:
				handler := &consumerGroupHandler{
					consumer: c,
					ready:    c.ready,
				}
				
				if err := c.consumerGroup.Consume(c.ctx, c.config.Topics, handler); err != nil {
					c.logger.Error(ctx, "Error consuming from Kafka", err)
					
					// Check if it's a recoverable error
					if err == sarama.ErrClosedConsumerGroup {
						c.logger.Info(ctx, "Consumer group closed")
						return
					}
					
					// Wait before retrying
					select {
					case <-c.ctx.Done():
						return
					case <-time.After(5 * time.Second):
						c.logger.Info(ctx, "Retrying Kafka consumer")
						continue
					}
				}
			}
		}
	}()

	// Wait until consumer is ready
	select {
	case <-c.ready:
		c.logger.Info(ctx, "Kafka consumer is ready")
	case <-time.After(30 * time.Second):
		return platformError.NewInternal("timeout waiting for consumer to be ready")
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// Stop stops the consumer
func (c *Consumer) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	c.logger.Info(nil, "Stopping Kafka consumer")
	
	c.cancel()
	c.wg.Wait()
	
	if err := c.consumerGroup.Close(); err != nil {
		c.logger.Error(nil, "Error closing consumer group", err)
		return platformError.Wrap(err, "failed to close consumer group")
	}

	c.logger.Info(nil, "Kafka consumer stopped successfully")
	return nil
}

// HealthCheck performs a health check on the consumer
func (c *Consumer) HealthCheck(ctx context.Context) error {
	c.mu.RLock()
	running := c.running
	c.mu.RUnlock()

	if !running {
		return platformError.NewInternal("consumer is not running")
	}

	// Check if consumer group is still active
	config := sarama.NewConfig()
	client, err := sarama.NewClient(c.config.Brokers, config)
	if err != nil {
		return platformError.Wrap(err, "failed to create Kafka client for health check")
	}
	defer client.Close()

	// Check broker connectivity by getting topics
	topics, err := client.Topics()
	if err != nil {
		return platformError.Wrap(err, "failed to get Kafka topics")
	}

	// Check if we can get partitions for at least one topic (basic connectivity test)
	if len(topics) > 0 {
		_, err = client.Partitions(topics[0])
		if err != nil {
			return platformError.Wrap(err, "failed to get partitions for topic")
		}
	}

	// Alternatively, you can check if the brokers are available
	brokers := client.Brokers()
	if len(brokers) == 0 {
		return platformError.NewInternal("no brokers available")
	}

	// Check if at least one broker is connected
	connectedBrokers := 0
	for _, broker := range brokers {
		connected, err := broker.Connected()
		if err != nil {
			c.logger.Warn(ctx, "Error checking broker connection", map[string]interface{}{
				"broker": broker.Addr(),
				"error":  err.Error(),
			})
			continue
		}
		if connected {
			connectedBrokers++
		}
	}

	if connectedBrokers == 0 {
		return platformError.NewInternal("no brokers connected")
	}

	return nil
}

// GetStats returns consumer statistics
func (c *Consumer) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return map[string]interface{}{
		"running":              c.running,
		"brokers":              c.config.Brokers,
		"group_id":             c.config.GroupID,
		"client_id":            c.config.ClientID,
		"topics":               c.config.Topics,
		"session_timeout":      c.config.SessionTimeout,
		"initial_offset":       c.config.InitialOffset,
		"auto_commit":          c.config.EnableAutoCommit,
		"concurrency_level":    c.config.ConcurrencyLevel,
		"registered_handlers":  len(c.handlers),
	}
}

// Private methods

func (c *Consumer) handleErrors() {
	defer c.wg.Done()
	
	for {
		select {
		case <-c.ctx.Done():
			return
		case err := <-c.consumerGroup.Errors():
			if err != nil {
				// Type assert to sarama.ConsumerError to access Topic and Partition
				if consumerErr, ok := err.(*sarama.ConsumerError); ok {
					c.metrics.IncrementCounter("kafka_consumer_errors_total", map[string]string{
						"group_id": c.config.GroupID,
						"topic":    consumerErr.Topic,
					})
					c.logger.Error(c.ctx, "Kafka consumer error", err, map[string]interface{}{
						"topic":     consumerErr.Topic,
						"partition": consumerErr.Partition,
					})
				} else {
					// Fallback for other error types
					c.metrics.IncrementCounter("kafka_consumer_errors_total", map[string]string{
						"group_id": c.config.GroupID,
						"topic":    "unknown",
					})
					c.logger.Error(c.ctx, "Kafka consumer error", err, map[string]interface{}{
						"error_type": fmt.Sprintf("%T", err),
					})
				}
			}
		}
	}
}

func (c *Consumer) processMessage(ctx context.Context, message *sarama.ConsumerMessage) error {
	// Convert to our message format
	msg := c.convertMessage(message)
	
	// Record metrics
	c.metrics.IncrementCounter("kafka_consumer_messages_total", map[string]string{
		"topic": msg.Topic,
	})
	c.metrics.RecordValue("kafka_consumer_message_size_bytes", float64(len(msg.Value)), map[string]string{
		"topic": msg.Topic,
	})

	// Find handler for the topic
	c.mu.RLock()
	handler, exists := c.handlers[msg.Topic]
	c.mu.RUnlock()
	
	if !exists {
		c.logger.Warn(ctx, "No handler registered for topic", map[string]interface{}{
			"topic": msg.Topic,
		})
		return nil // Don't fail on missing handlers
	}

	// Process message with timeout
	processCtx, cancel := context.WithTimeout(ctx, c.config.MaxProcessingTime)
	defer cancel()

	// Process with retry logic
	var lastErr error
	for attempt := 0; attempt <= c.config.RetryAttempts; attempt++ {
		err := handler.HandleMessage(processCtx, msg)
		if err == nil {
			c.metrics.IncrementCounter("kafka_consumer_messages_processed_total", map[string]string{
				"topic":  msg.Topic,
				"status": "success",
			})
			return nil
		}

		lastErr = err
		
		// Don't retry validation errors
		if platformError.IsValidation(err) {
			break
		}

		if attempt < c.config.RetryAttempts {
			c.logger.Warn(ctx, "Message processing failed, retrying", map[string]interface{}{
				"topic":     msg.Topic,
				"attempt":   attempt + 1,
				"error":     err.Error(),
				"backoff":   c.config.RetryBackoff,
			})
			
			select {
			case <-processCtx.Done():
				return processCtx.Err()
			case <-time.After(c.config.RetryBackoff):
				continue
			}
		}
	}

	// Record failure metrics
	c.metrics.IncrementCounter("kafka_consumer_messages_processed_total", map[string]string{
		"topic":  msg.Topic,
		"status": "failed",
	})

	c.logger.Error(ctx, "Message processing failed after retries", lastErr, map[string]interface{}{
		"topic":         msg.Topic,
		"key":           msg.Key,
		"partition":     msg.Partition,
		"offset":        msg.Offset,
		"retry_attempts": c.config.RetryAttempts,
	})

	// Send to dead letter topic if enabled
	if c.config.EnableDeadLetter && c.config.DeadLetterTopic != "" {
		c.sendToDeadLetter(ctx, msg, lastErr)
	}

	return lastErr
}

func (c *Consumer) convertMessage(message *sarama.ConsumerMessage) *Message {
	// Convert headers
	headers := make(map[string]string)
	for _, header := range message.Headers {
		headers[string(header.Key)] = string(header.Value)
	}

	msg := &Message{
		Topic:     message.Topic,
		Partition: message.Partition,
		Offset:    message.Offset,
		Key:       string(message.Key),
		Value:     message.Value,
		Headers:   headers,
		Timestamp: message.Timestamp,
	}

	// Extract standard event headers
	if eventType, exists := headers["event-type"]; exists {
		msg.EventType = eventType
	}
	if eventID, exists := headers["event-id"]; exists {
		msg.EventID = eventID
	}
	if eventSource, exists := headers["event-source"]; exists {
		msg.EventSource = eventSource
	}

	return msg
}

func (c *Consumer) sendToDeadLetter(ctx context.Context, msg *Message, processingError error) {
	// This would typically use a producer to send to dead letter topic
	c.logger.Error(ctx, "Sending message to dead letter topic", processingError, map[string]interface{}{
		"original_topic":    msg.Topic,
		"dead_letter_topic": c.config.DeadLetterTopic,
		"message_key":       msg.Key,
		"partition":         msg.Partition,
		"offset":            msg.Offset,
	})
	
	// TODO: Implement dead letter topic producer
	// This would require injecting a producer or creating one specifically for dead letters
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	consumer *Consumer
	ready    chan bool
}

func (h *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	close(h.ready)
	h.consumer.logger.Info(nil, "Consumer group session setup complete")
	return nil
}

func (h *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	h.consumer.logger.Info(nil, "Consumer group session cleanup")
	return nil
}

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	// Create semaphore for concurrency control
	semaphore := make(chan struct{}, h.consumer.config.ConcurrencyLevel)
	
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			// Acquire semaphore
			semaphore <- struct{}{}
			
			// Process message concurrently
			go func(msg *sarama.ConsumerMessage) {
				defer func() { <-semaphore }() // Release semaphore
				
				if err := h.consumer.processMessage(session.Context(), msg); err != nil {
					h.consumer.logger.Error(session.Context(), "Failed to process message", err, map[string]interface{}{
						"topic":     msg.Topic,
						"partition": msg.Partition,
						"offset":    msg.Offset,
					})
				}
				
				// Mark message as processed
				session.MarkMessage(msg, "")
			}(message)

		case <-session.Context().Done():
			return nil
		}
	}
}