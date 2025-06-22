package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/IBM/sarama"
	"github.com/google/uuid"

	platformError "github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// ProducerConfig holds Kafka producer configuration
type ProducerConfig struct {
	Brokers            []string      `json:"brokers"`
	ClientID           string        `json:"client_id"`
	MaxRetries         int           `json:"max_retries"`
	RetryBackoff       time.Duration `json:"retry_backoff"`
	FlushFrequency     time.Duration `json:"flush_frequency"`
	FlushMessages      int           `json:"flush_messages"`
	CompressionType    string        `json:"compression_type"`
	IdempotentProducer bool          `json:"idempotent_producer"`
	RequiredAcks       int           `json:"required_acks"`
	MaxMessageBytes    int           `json:"max_message_bytes"`
	RequestTimeout     time.Duration `json:"request_timeout"`
}

// DefaultProducerConfig returns default producer configuration
func DefaultProducerConfig() ProducerConfig {
	return ProducerConfig{
		Brokers:            []string{"localhost:9092"},
		ClientID:           "shared-producer",
		MaxRetries:         3,
		RetryBackoff:       100 * time.Millisecond,
		FlushFrequency:     500 * time.Millisecond,
		FlushMessages:      100,
		CompressionType:    "snappy",
		IdempotentProducer: true,
		RequiredAcks:       1,       // WaitForAll
		MaxMessageBytes:    1000000, // 1MB
		RequestTimeout:     30 * time.Second,
	}
}

// Producer provides a high-level Kafka producer interface
type Producer struct {
	producer      sarama.SyncProducer
	asyncProducer sarama.AsyncProducer
	config        ProducerConfig
	logger        logging.Logger
	metrics       metrics.Metrics
	closed        bool
}

// NewProducer creates a new Kafka producer
func NewProducer(config ProducerConfig, logger logging.Logger, metrics metrics.Metrics) (*Producer, error) {
	saramaConfig := sarama.NewConfig()

	// Basic configuration
	saramaConfig.ClientID = config.ClientID
	saramaConfig.Producer.MaxMessageBytes = config.MaxMessageBytes
	saramaConfig.Net.DialTimeout = config.RequestTimeout
	saramaConfig.Net.ReadTimeout = config.RequestTimeout
	saramaConfig.Net.WriteTimeout = config.RequestTimeout

	// Retry configuration
	saramaConfig.Producer.Retry.Max = config.MaxRetries
	saramaConfig.Producer.Retry.Backoff = config.RetryBackoff

	// Reliability configuration
	switch config.RequiredAcks {
	case 0:
		saramaConfig.Producer.RequiredAcks = sarama.NoResponse
	case 1:
		saramaConfig.Producer.RequiredAcks = sarama.WaitForLocal
	case -1:
		saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
	default:
		saramaConfig.Producer.RequiredAcks = sarama.WaitForLocal
	}

	// Idempotent producer
	if config.IdempotentProducer {
		saramaConfig.Producer.Idempotent = true
		saramaConfig.Net.MaxOpenRequests = 1
	}

	// Compression
	switch config.CompressionType {
	case "none":
		saramaConfig.Producer.Compression = sarama.CompressionNone
	case "gzip":
		saramaConfig.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		saramaConfig.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		saramaConfig.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		saramaConfig.Producer.Compression = sarama.CompressionZSTD
	default:
		saramaConfig.Producer.Compression = sarama.CompressionSnappy
	}

	// Batching configuration
	saramaConfig.Producer.Flush.Frequency = config.FlushFrequency
	saramaConfig.Producer.Flush.Messages = config.FlushMessages

	// Return configuration
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.Return.Errors = true

	// Create sync producer
	syncProducer, err := sarama.NewSyncProducer(config.Brokers, saramaConfig)
	if err != nil {
		return nil, platformError.Wrap(err, "failed to create Kafka sync producer")
	}

	// Create async producer for high-throughput scenarios
	asyncProducer, err := sarama.NewAsyncProducer(config.Brokers, saramaConfig)
	if err != nil {
		syncProducer.Close()
		return nil, platformError.Wrap(err, "failed to create Kafka async producer")
	}

	producer := &Producer{
		producer:      syncProducer,
		asyncProducer: asyncProducer,
		config:        config,
		logger:        logger,
		metrics:       metrics,
		closed:        false,
	}

	// Start async producer error handling
	go producer.handleAsyncErrors()

	logger.Info(nil, "Kafka producer created successfully", map[string]interface{}{
		"brokers":       config.Brokers,
		"client_id":     config.ClientID,
		"max_retries":   config.MaxRetries,
		"compression":   config.CompressionType,
		"idempotent":    config.IdempotentProducer,
		"required_acks": config.RequiredAcks,
	})

	return producer, nil
}

// SendMessage sends a message synchronously
func (p *Producer) SendMessage(ctx context.Context, topic, key string, value interface{}, headers map[string]string) error {
	if p.closed {
		return platformError.NewInternal("producer is closed")
	}

	// Serialize value
	data, err := p.serializeValue(value)
	if err != nil {
		return platformError.Wrap(err, "failed to serialize message value")
	}

	// Build headers
	messageHeaders := p.buildHeaders(headers)

	// Create message
	message := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.StringEncoder(key),
		Value:     sarama.ByteEncoder(data),
		Headers:   messageHeaders,
		Timestamp: time.Now(),
	}

	// Send message
	partition, offset, err := p.producer.SendMessage(message)
	if err != nil {
		p.metrics.IncrementCounter("kafka_producer_errors_total", map[string]string{
			"topic": topic,
			"error": "send_failed",
		})
		p.logger.Error(ctx, "Failed to send Kafka message", err, map[string]interface{}{
			"topic": topic,
			"key":   key,
		})
		return platformError.Wrap(err, "failed to send Kafka message")
	}

	// Record metrics
	p.metrics.IncrementCounter("kafka_producer_messages_total", map[string]string{
		"topic": topic,
	})
	p.metrics.RecordValue("kafka_producer_message_size_bytes", float64(len(data)), map[string]string{
		"topic": topic,
	})

	p.logger.Debug(ctx, "Kafka message sent successfully", map[string]interface{}{
		"topic":     topic,
		"key":       key,
		"partition": partition,
		"offset":    offset,
		"size":      len(data),
	})

	return nil
}

// SendMessageAsync sends a message asynchronously
func (p *Producer) SendMessageAsync(ctx context.Context, topic, key string, value interface{}, headers map[string]string) error {
	if p.closed {
		return platformError.NewInternal("producer is closed")
	}

	// Serialize value
	data, err := p.serializeValue(value)
	if err != nil {
		return platformError.Wrap(err, "failed to serialize message value")
	}

	// Build headers
	messageHeaders := p.buildHeaders(headers)

	// Create message
	message := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.StringEncoder(key),
		Value:     sarama.ByteEncoder(data),
		Headers:   messageHeaders,
		Timestamp: time.Now(),
	}

	// Send message asynchronously
	select {
	case p.asyncProducer.Input() <- message:
		p.logger.Debug(ctx, "Kafka message queued for async sending", map[string]interface{}{
			"topic": topic,
			"key":   key,
			"size":  len(data),
		})
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendBatch sends multiple messages in a batch
func (p *Producer) SendBatch(ctx context.Context, messages []BatchMessage) error {
	if p.closed {
		return platformError.NewInternal("producer is closed")
	}

	if len(messages) == 0 {
		return nil
	}

	var failures []error
	successful := 0

	for _, msg := range messages {
		err := p.SendMessage(ctx, msg.Topic, msg.Key, msg.Value, msg.Headers)
		if err != nil {
			failures = append(failures, err)
		} else {
			successful++
		}
	}

	p.logger.Info(ctx, "Kafka batch send completed", map[string]interface{}{
		"total_messages":      len(messages),
		"successful_messages": successful,
		"failed_messages":     len(failures),
	})

	if len(failures) > 0 {
		return platformError.NewInternal(fmt.Sprintf("batch send failed: %d out of %d messages failed", len(failures), len(messages)))
	}

	return nil
}

// Close closes the producer
func (p *Producer) Close() error {
	if p.closed {
		return nil
	}

	p.closed = true

	var closeErrors []error

	if p.producer != nil {
		if err := p.producer.Close(); err != nil {
			closeErrors = append(closeErrors, platformError.Wrap(err, "failed to close sync producer"))
		}
	}

	if p.asyncProducer != nil {
		if err := p.asyncProducer.Close(); err != nil {
			closeErrors = append(closeErrors, platformError.Wrap(err, "failed to close async producer"))
		}
	}

	if len(closeErrors) > 0 {
		p.logger.Error(nil, "Errors while closing Kafka producer", nil, map[string]interface{}{
			"errors": closeErrors,
		})
		return closeErrors[0] // Return first error
	}

	p.logger.Info(nil, "Kafka producer closed successfully")
	return nil
}

// HealthCheck performs a health check on the producer
func (p *Producer) HealthCheck(ctx context.Context) error {
	if p.closed {
		return platformError.NewInternal("producer is closed")
	}

	// Create a client to test connectivity
	config := sarama.NewConfig()
	client, err := sarama.NewClient(p.config.Brokers, config)
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

	// Check if brokers are available
	brokers := client.Brokers()
	if len(brokers) == 0 {
		return platformError.NewInternal("no brokers available")
	}

	// Check if at least one broker is connected
	connectedBrokers := 0
	for _, broker := range brokers {
		connected, err := broker.Connected()
		if err != nil {
			p.logger.Warn(ctx, "Error checking broker connection", map[string]interface{}{
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

// GetStats returns producer statistics
func (p *Producer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"status":            !p.closed,
		"brokers":           p.config.Brokers,
		"client_id":         p.config.ClientID,
		"compression":       p.config.CompressionType,
		"idempotent":        p.config.IdempotentProducer,
		"max_retries":       p.config.MaxRetries,
		"flush_frequency":   p.config.FlushFrequency,
		"flush_messages":    p.config.FlushMessages,
		"max_message_bytes": p.config.MaxMessageBytes,
	}
}

// Private helper methods

func (p *Producer) serializeValue(value interface{}) ([]byte, error) {
	switch v := value.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		// JSON serialize for complex objects
		return json.Marshal(v)
	}
}

func (p *Producer) buildHeaders(headers map[string]string) []sarama.RecordHeader {
	var recordHeaders []sarama.RecordHeader

	// Add custom headers
	for k, v := range headers {
		recordHeaders = append(recordHeaders, sarama.RecordHeader{
			Key:   []byte(k),
			Value: []byte(v),
		})
	}

	// Add standard headers
	recordHeaders = append(recordHeaders, sarama.RecordHeader{
		Key:   []byte("producer-id"),
		Value: []byte(p.config.ClientID),
	})

	recordHeaders = append(recordHeaders, sarama.RecordHeader{
		Key:   []byte("timestamp"),
		Value: []byte(time.Now().UTC().Format(time.RFC3339)),
	})

	recordHeaders = append(recordHeaders, sarama.RecordHeader{
		Key:   []byte("message-id"),
		Value: []byte(uuid.New().String()),
	})

	return recordHeaders
}

func (p *Producer) handleAsyncErrors() {
	for {
		select {
		case err := <-p.asyncProducer.Errors():
			if err != nil {
				// Handle the Key.Encode() properly by checking for errors
				var keyStr string
				if err.Msg.Key != nil {
					keyBytes, encodeErr := err.Msg.Key.Encode()
					if encodeErr != nil {
						keyStr = "encoding_error"
					} else {
						keyStr = string(keyBytes)
					}
				} else {
					keyStr = "no_key"
				}

				p.metrics.IncrementCounter("kafka_producer_async_errors_total", map[string]string{
					"topic": err.Msg.Topic,
				})
				p.logger.Error(nil, "Async Kafka producer error", err.Err, map[string]interface{}{
					"topic":     err.Msg.Topic,
					"partition": err.Msg.Partition,
					"offset":    err.Msg.Offset,
					"key":       keyStr,
				})
			}
		case success := <-p.asyncProducer.Successes():
			if success != nil {
				p.metrics.IncrementCounter("kafka_producer_async_messages_total", map[string]string{
					"topic": success.Topic,
				})
				p.logger.Debug(nil, "Async Kafka message sent successfully", map[string]interface{}{
					"topic":     success.Topic,
					"partition": success.Partition,
					"offset":    success.Offset,
				})
			}
		}
	}
}

// BatchMessage represents a Kafka message for batch operations
type BatchMessage struct {
	Topic   string            `json:"topic"`
	Key     string            `json:"key"`
	Value   interface{}       `json:"value"`
	Headers map[string]string `json:"headers"`
}

// Event represents a standardized event message
type Event struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Source   string                 `json:"source"`
	Subject  string                 `json:"subject"`
	Time     time.Time              `json:"time"`
	Data     interface{}            `json:"data"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewEvent creates a new standardized event
func NewEvent(eventType, source, subject string, data interface{}) *Event {
	return &Event{
		ID:       uuid.New().String(),
		Type:     eventType,
		Source:   source,
		Subject:  subject,
		Time:     time.Now().UTC(),
		Data:     data,
		Metadata: make(map[string]interface{}),
	}
}

// SendEvent sends a standardized event
func (p *Producer) SendEvent(ctx context.Context, topic string, event *Event) error {
	headers := map[string]string{
		"event-type":   event.Type,
		"event-id":     event.ID,
		"event-source": event.Source,
		"content-type": "application/json",
	}

	return p.SendMessage(ctx, topic, event.Subject, event, headers)
}
