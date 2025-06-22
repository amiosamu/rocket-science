package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// CoordinatorConfig holds configuration for the Kafka coordinator
type CoordinatorConfig struct {
	Brokers          []string                `json:"brokers"`
	ProducerConfig   ProducerConfig          `json:"producer"`
	ConsumerConfigs  map[string]ConsumerConfig `json:"consumers"` // key = consumer name
	HealthCheckTopic string                  `json:"health_check_topic"`
	EnableProducer   bool                    `json:"enable_producer"`
	EnableConsumer   bool                    `json:"enable_consumer"`
}

// DefaultCoordinatorConfig returns default coordinator configuration
func DefaultCoordinatorConfig() CoordinatorConfig {
	return CoordinatorConfig{
		Brokers:          []string{"localhost:9092"},
		ProducerConfig:   DefaultProducerConfig(),
		ConsumerConfigs:  make(map[string]ConsumerConfig),
		HealthCheckTopic: "health-check",
		EnableProducer:   true,
		EnableConsumer:   true,
	}
}

// Coordinator manages Kafka producers and consumers for a service
type Coordinator struct {
	config    CoordinatorConfig
	logger    logging.Logger
	metrics   metrics.Metrics
	producer  *Producer
	consumers map[string]*Consumer
	mu        sync.RWMutex
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewCoordinator creates a new Kafka coordinator
func NewCoordinator(config CoordinatorConfig, logger logging.Logger, metrics metrics.Metrics) (*Coordinator, error) {
	ctx, cancel := context.WithCancel(context.Background())
	
	coordinator := &Coordinator{
		config:    config,
		logger:    logger,
		metrics:   metrics,
		consumers: make(map[string]*Consumer),
		ctx:       ctx,
		cancel:    cancel,
		running:   false,
	}

	// Initialize producer if enabled
	if config.EnableProducer {
		producer, err := NewProducer(config.ProducerConfig, logger, metrics)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create Kafka producer")
		}
		coordinator.producer = producer
	}

	// Initialize consumers if enabled
	if config.EnableConsumer {
		for name, consumerConfig := range config.ConsumerConfigs {
			consumer, err := NewConsumer(consumerConfig, logger, metrics)
			if err != nil {
				coordinator.Close() // Clean up already created components
				return nil, errors.Wrap(err, fmt.Sprintf("failed to create Kafka consumer '%s'", name))
			}
			coordinator.consumers[name] = consumer
		}
	}

	logger.Info(nil, "Kafka coordinator created successfully", map[string]interface{}{
		"brokers":          config.Brokers,
		"producer_enabled": config.EnableProducer,
		"consumer_enabled": config.EnableConsumer,
		"consumer_count":   len(config.ConsumerConfigs),
	})

	return coordinator, nil
}

// GetProducer returns the Kafka producer
func (c *Coordinator) GetProducer() *Producer {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.producer
}

// GetConsumer returns a specific consumer by name
func (c *Coordinator) GetConsumer(name string) (*Consumer, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	consumer, exists := c.consumers[name]
	return consumer, exists
}

// RegisterConsumerHandler registers a message handler for a specific consumer
func (c *Coordinator) RegisterConsumerHandler(consumerName string, handler MessageHandler) error {
	c.mu.RLock()
	consumer, exists := c.consumers[consumerName]
	c.mu.RUnlock()
	
	if !exists {
		return errors.NewNotFound(fmt.Sprintf("consumer '%s' not found", consumerName))
	}
	
	consumer.RegisterHandler(handler)
	
	c.logger.Info(nil, "Message handler registered", map[string]interface{}{
		"consumer": consumerName,
		"handler":  fmt.Sprintf("%T", handler),
		"topics":   handler.GetSupportedTopics(),
	})
	
	return nil
}

// Start starts all components
func (c *Coordinator) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return errors.NewInternal("coordinator is already running")
	}
	c.running = true
	c.mu.Unlock()

	c.logger.Info(ctx, "Starting Kafka coordinator")

	// Start consumers
	for name, consumer := range c.consumers {
		c.wg.Add(1)
		go func(consumerName string, cons *Consumer) {
			defer c.wg.Done()
			if err := cons.Start(c.ctx); err != nil {
				c.logger.Error(ctx, "Failed to start consumer", err, map[string]interface{}{
					"consumer": consumerName,
				})
			}
		}(name, consumer)
	}

	// Start health check routine
	c.wg.Add(1)
	go c.healthCheckRoutine()

	c.logger.Info(ctx, "Kafka coordinator started successfully", map[string]interface{}{
		"producer_active": c.producer != nil,
		"consumer_count":  len(c.consumers),
	})

	return nil
}

// Stop stops all components
func (c *Coordinator) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	c.logger.Info(nil, "Stopping Kafka coordinator")

	// Cancel context to stop all goroutines
	c.cancel()
	
	// Wait for all goroutines to finish
	c.wg.Wait()

	// Stop all consumers
	var stopErrors []error
	for name, consumer := range c.consumers {
		if err := consumer.Stop(); err != nil {
			stopErrors = append(stopErrors, errors.Wrap(err, fmt.Sprintf("failed to stop consumer '%s'", name)))
		}
	}

	if len(stopErrors) > 0 {
		c.logger.Error(nil, "Errors while stopping consumers", nil, map[string]interface{}{
			"errors": stopErrors,
		})
	}

	c.logger.Info(nil, "Kafka coordinator stopped successfully")
	return nil
}

// Close closes all components
func (c *Coordinator) Close() error {
	// Stop first
	if err := c.Stop(); err != nil {
		c.logger.Error(nil, "Error stopping coordinator during close", err)
	}

	var closeErrors []error

	// Close producer
	if c.producer != nil {
		if err := c.producer.Close(); err != nil {
			closeErrors = append(closeErrors, errors.Wrap(err, "failed to close producer"))
		}
	}

	// Consumers are already stopped by Stop(), no need to close them separately

	if len(closeErrors) > 0 {
		return closeErrors[0]
	}

	c.logger.Info(nil, "Kafka coordinator closed successfully")
	return nil
}

// HealthCheck performs a comprehensive health check
func (c *Coordinator) HealthCheck(ctx context.Context) map[string]interface{} {
	health := map[string]interface{}{
		"status": "healthy",
		"timestamp": time.Now().UTC(),
	}

	// Check producer health
	if c.producer != nil {
		if err := c.producer.HealthCheck(ctx); err != nil {
			health["producer"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
			health["status"] = "degraded"
		} else {
			health["producer"] = map[string]interface{}{
				"status": "healthy",
			}
		}
	}

	// Check consumer health
	consumerHealth := make(map[string]interface{})
	for name, consumer := range c.consumers {
		if err := consumer.HealthCheck(ctx); err != nil {
			consumerHealth[name] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
			health["status"] = "degraded"
		} else {
			consumerHealth[name] = map[string]interface{}{
				"status": "healthy",
			}
		}
	}
	health["consumers"] = consumerHealth

	// Overall coordinator health
	c.mu.RLock()
	health["running"] = c.running
	c.mu.RUnlock()

	return health
}

// GetStats returns comprehensive statistics
func (c *Coordinator) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"brokers": c.config.Brokers,
		"timestamp": time.Now().UTC(),
	}

	c.mu.RLock()
	stats["running"] = c.running
	c.mu.RUnlock()

	// Producer stats
	if c.producer != nil {
		stats["producer"] = c.producer.GetStats()
	}

	// Consumer stats
	consumerStats := make(map[string]interface{})
	for name, consumer := range c.consumers {
		consumerStats[name] = consumer.GetStats()
	}
	stats["consumers"] = consumerStats

	return stats
}

// SendMessage is a convenience method to send messages via the producer
func (c *Coordinator) SendMessage(ctx context.Context, topic, key string, value interface{}, headers map[string]string) error {
	if c.producer == nil {
		return errors.NewInternal("producer is not available")
	}
	return c.producer.SendMessage(ctx, topic, key, value, headers)
}

// SendEvent is a convenience method to send events via the producer
func (c *Coordinator) SendEvent(ctx context.Context, topic string, event *Event) error {
	if c.producer == nil {
		return errors.NewInternal("producer is not available")
	}
	return c.producer.SendEvent(ctx, topic, event)
}

// AddConsumer adds a new consumer at runtime
func (c *Coordinator) AddConsumer(name string, config ConsumerConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.consumers[name]; exists {
		return errors.NewConflict(fmt.Sprintf("consumer '%s' already exists", name))
	}

	consumer, err := NewConsumer(config, c.logger, c.metrics)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to create consumer '%s'", name))
	}

	c.consumers[name] = consumer
	c.config.ConsumerConfigs[name] = config

	// Start the consumer if coordinator is running
	if c.running {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			if err := consumer.Start(c.ctx); err != nil {
				c.logger.Error(c.ctx, "Failed to start dynamically added consumer", err, map[string]interface{}{
					"consumer": name,
				})
			}
		}()
	}

	c.logger.Info(nil, "Consumer added successfully", map[string]interface{}{
		"consumer": name,
		"topics":   config.Topics,
	})

	return nil
}

// RemoveConsumer removes a consumer at runtime
func (c *Coordinator) RemoveConsumer(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	consumer, exists := c.consumers[name]
	if !exists {
		return errors.NewNotFound(fmt.Sprintf("consumer '%s' not found", name))
	}

	// Stop the consumer
	if err := consumer.Stop(); err != nil {
		c.logger.Error(nil, "Error stopping consumer during removal", err, map[string]interface{}{
			"consumer": name,
		})
	}

	// Remove from maps
	delete(c.consumers, name)
	delete(c.config.ConsumerConfigs, name)

	c.logger.Info(nil, "Consumer removed successfully", map[string]interface{}{
		"consumer": name,
	})

	return nil
}

// Private methods

func (c *Coordinator) healthCheckRoutine() {
	defer c.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.performHealthCheck()
		}
	}
}

func (c *Coordinator) performHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	health := c.HealthCheck(ctx)
	
	// Record health metrics
	if status, ok := health["status"].(string); ok {
		healthValue := 1.0
		if status != "healthy" {
			healthValue = 0.0
		}
		c.metrics.SetGauge("kafka_coordinator_health", healthValue, nil)
	}

	// Log health status
	c.logger.Debug(ctx, "Health check completed", map[string]interface{}{
		"health": health,
	})
}

// Topic management utilities

// CreateTopics creates topics if they don't exist (requires admin permissions)
func (c *Coordinator) CreateTopics(ctx context.Context, topics []TopicConfig) error {
	// This would require Kafka admin client
	// Implementation depends on whether you want to manage topics programmatically
	c.logger.Info(ctx, "Topic creation requested", map[string]interface{}{
		"topics": topics,
	})
	
	// TODO: Implement topic creation using sarama admin client
	// This is optional functionality that many deployments handle externally
	
	return nil
}

// TopicConfig represents topic configuration
type TopicConfig struct {
	Name              string `json:"name"`
	Partitions        int32  `json:"partitions"`
	ReplicationFactor int16  `json:"replication_factor"`
}

// Utility functions for message handlers

// SimpleHandler is a basic implementation of MessageHandler
type SimpleHandler struct {
	topics  []string
	handler func(ctx context.Context, message *Message) error
}

// NewSimpleHandler creates a simple message handler
func NewSimpleHandler(topics []string, handler func(ctx context.Context, message *Message) error) *SimpleHandler {
	return &SimpleHandler{
		topics:  topics,
		handler: handler,
	}
}

func (h *SimpleHandler) HandleMessage(ctx context.Context, message *Message) error {
	return h.handler(ctx, message)
}

func (h *SimpleHandler) GetSupportedTopics() []string {
	return h.topics
}