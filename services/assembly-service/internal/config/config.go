package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amiosamu/rocket-science/shared/platform/messaging/kafka"
)

// Config holds the application configuration
type Config struct {
	Service  ServiceConfig  `json:"service"`
	Kafka    KafkaConfig    `json:"kafka"`
	Logging  LoggingConfig  `json:"logging"`
	Metrics  MetricsConfig  `json:"metrics"`
	Assembly AssemblyConfig `json:"assembly"`
}

// ServiceConfig holds service-specific configuration
type ServiceConfig struct {
	Name            string        `json:"name"`
	Version         string        `json:"version"`
	Environment     string        `json:"environment"`
	Port            int           `json:"port"`
	GracefulTimeout time.Duration `json:"graceful_timeout"`
}

// KafkaConfig holds Kafka configuration
type KafkaConfig struct {
	Consumer kafka.ConsumerConfig `json:"consumer"`
	Producer kafka.ProducerConfig `json:"producer"`
	Topics   TopicsConfig         `json:"topics"`
}

// TopicsConfig defines Kafka topics used by the service
type TopicsConfig struct {
	PaymentProcessed  string `json:"payment_processed"`
	AssemblyStarted   string `json:"assembly_started"`
	AssemblyCompleted string `json:"assembly_completed"`
	AssemblyFailed    string `json:"assembly_failed"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"`
	Output string `json:"output"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled   bool   `json:"enabled"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
	Namespace string `json:"namespace"`
	Subsystem string `json:"subsystem"`
}

// AssemblyConfig holds assembly-specific configuration
type AssemblyConfig struct {
	SimulationDuration      time.Duration `json:"simulation_duration"`
	MaxConcurrentAssemblies int           `json:"max_concurrent_assemblies"`
	FailureRate             float64       `json:"failure_rate"` // 0.0 to 1.0
	QualityThreshold        int           `json:"quality_threshold"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Service: ServiceConfig{
			Name:            "assembly-service",
			Version:         "1.0.0",
			Environment:     getEnv("ENVIRONMENT", "development"),
			Port:            getEnvAsInt("PORT", 8083),
			GracefulTimeout: getEnvAsDuration("GRACEFUL_TIMEOUT", "30s"),
		},
		Kafka: KafkaConfig{
			Consumer: kafka.ConsumerConfig{
				Brokers:            strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
				GroupID:            getEnv("KAFKA_CONSUMER_GROUP_ID", "assembly-service-group"),
				ClientID:           getEnv("KAFKA_CONSUMER_CLIENT_ID", "assembly-service-consumer"),
				Topics:             []string{getEnv("KAFKA_TOPIC_PAYMENT_PROCESSED", "payment.processed")},
				SessionTimeout:     getEnvAsDuration("KAFKA_SESSION_TIMEOUT", "30s"),
				HeartbeatInterval:  getEnvAsDuration("KAFKA_HEARTBEAT_INTERVAL", "3s"),
				RebalanceTimeout:   getEnvAsDuration("KAFKA_REBALANCE_TIMEOUT", "60s"),
				InitialOffset:      getEnv("KAFKA_INITIAL_OFFSET", "newest"),
				EnableAutoCommit:   getEnvAsBool("KAFKA_ENABLE_AUTO_COMMIT", true),
				AutoCommitInterval: getEnvAsDuration("KAFKA_AUTO_COMMIT_INTERVAL", "1s"),
				MaxProcessingTime:  getEnvAsDuration("KAFKA_MAX_PROCESSING_TIME", "30s"),
				ConcurrencyLevel:   getEnvAsInt("KAFKA_CONCURRENCY_LEVEL", 1),
				RetryAttempts:      getEnvAsInt("KAFKA_RETRY_ATTEMPTS", 3),
				RetryBackoff:       getEnvAsDuration("KAFKA_RETRY_BACKOFF", "1s"),
				EnableDeadLetter:   getEnvAsBool("KAFKA_ENABLE_DEAD_LETTER", true),
				DeadLetterTopic:    getEnv("KAFKA_DEAD_LETTER_TOPIC", "assembly.dead-letter"),
			},
			Producer: kafka.ProducerConfig{
				Brokers:            strings.Split(getEnv("KAFKA_BROKERS", "localhost:9092"), ","),
				ClientID:           getEnv("KAFKA_PRODUCER_CLIENT_ID", "assembly-service-producer"),
				MaxRetries:         getEnvAsInt("KAFKA_PRODUCER_MAX_RETRIES", 3),
				RetryBackoff:       getEnvAsDuration("KAFKA_PRODUCER_RETRY_BACKOFF", "100ms"),
				FlushFrequency:     getEnvAsDuration("KAFKA_PRODUCER_FLUSH_FREQUENCY", "500ms"),
				FlushMessages:      getEnvAsInt("KAFKA_PRODUCER_FLUSH_MESSAGES", 100),
				CompressionType:    getEnv("KAFKA_PRODUCER_COMPRESSION", "snappy"),
				IdempotentProducer: getEnvAsBool("KAFKA_PRODUCER_IDEMPOTENT", true),
				RequiredAcks:       getEnvAsInt("KAFKA_PRODUCER_REQUIRED_ACKS", -1),
				MaxMessageBytes:    getEnvAsInt("KAFKA_PRODUCER_MAX_MESSAGE_BYTES", 1000000),
				RequestTimeout:     getEnvAsDuration("KAFKA_PRODUCER_REQUEST_TIMEOUT", "30s"),
			},
			Topics: TopicsConfig{
				PaymentProcessed:  getEnv("KAFKA_TOPIC_PAYMENT_PROCESSED", "payment.processed"),
				AssemblyStarted:   getEnv("KAFKA_TOPIC_ASSEMBLY_STARTED", "assembly.started"),
				AssemblyCompleted: getEnv("KAFKA_TOPIC_ASSEMBLY_COMPLETED", "assembly.completed"),
				AssemblyFailed:    getEnv("KAFKA_TOPIC_ASSEMBLY_FAILED", "assembly.failed"),
			},
		},
		Logging: LoggingConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
			Output: getEnv("LOG_OUTPUT", "stdout"),
		},
		Metrics: MetricsConfig{
			Enabled:   getEnvAsBool("METRICS_ENABLED", true),
			Port:      getEnvAsInt("METRICS_PORT", 9090),
			Path:      getEnv("METRICS_PATH", "/metrics"),
			Namespace: getEnv("METRICS_NAMESPACE", "assembly_service"),
			Subsystem: getEnv("METRICS_SUBSYSTEM", ""),
		},
		Assembly: AssemblyConfig{
			SimulationDuration:      getEnvAsDuration("ASSEMBLY_SIMULATION_DURATION", "10s"),
			MaxConcurrentAssemblies: getEnvAsInt("ASSEMBLY_MAX_CONCURRENT", 10),
			FailureRate:             getEnvAsFloat("ASSEMBLY_FAILURE_RATE", 0.05), // 5% failure rate
			QualityThreshold:        getEnvAsInt("ASSEMBLY_QUALITY_THRESHOLD", 80),
		},
	}
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	config := DefaultConfig()

	// Validate required configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Service.Name == "" {
		return fmt.Errorf("service name is required")
	}

	if len(c.Kafka.Consumer.Brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}

	if c.Kafka.Consumer.GroupID == "" {
		return fmt.Errorf("kafka consumer group ID is required")
	}

	if c.Assembly.SimulationDuration <= 0 {
		return fmt.Errorf("assembly simulation duration must be positive")
	}

	if c.Assembly.MaxConcurrentAssemblies <= 0 {
		return fmt.Errorf("max concurrent assemblies must be positive")
	}

	if c.Assembly.FailureRate < 0 || c.Assembly.FailureRate > 1 {
		return fmt.Errorf("assembly failure rate must be between 0 and 1")
	}

	return nil
}

// Helper functions for environment variable parsing
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvAsFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvAsDuration(key string, defaultValue string) time.Duration {
	value := getEnv(key, defaultValue)
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	// Fallback to default if parsing fails
	if duration, err := time.ParseDuration(defaultValue); err == nil {
		return duration
	}
	return 30 * time.Second // Final fallback
}
