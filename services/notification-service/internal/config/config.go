package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amiosamu/rocket-science/shared/platform/messaging/kafka"
)

// Config holds all configuration for the notification service
type Config struct {
	Service   ServiceConfig   `json:"service"`
	Kafka     KafkaConfig     `json:"kafka"`
	Telegram  TelegramConfig  `json:"telegram"`
	IAMClient IAMClientConfig `json:"iam_client"`
	Logging   LoggingConfig   `json:"logging"`
	Metrics   MetricsConfig   `json:"metrics"`
	Tracing   TracingConfig   `json:"tracing"`
}

// ServiceConfig holds general service configuration
type ServiceConfig struct {
	Name                    string        `json:"name"`
	Version                 string        `json:"version"`
	Environment             string        `json:"environment"`
	Host                    string        `json:"host"`
	Port                    int           `json:"port"`
	HealthPort              int           `json:"health_port"`
	GracefulShutdownTimeout time.Duration `json:"graceful_shutdown_timeout"`
}

// KafkaConfig holds Kafka consumer configuration
type KafkaConfig struct {
	Consumer kafka.ConsumerConfig `json:"consumer"`
	Topics   TopicConfig          `json:"topics"`
}

// TopicConfig holds topic names for different event types
type TopicConfig struct {
	OrderEvents    string `json:"order_events"`
	PaymentEvents  string `json:"payment_events"`
	AssemblyEvents string `json:"assembly_events"`
}

// TelegramConfig holds Telegram bot configuration
type TelegramConfig struct {
	BotToken        string        `json:"bot_token"`
	DevelopmentMode bool          `json:"development_mode"`
	Timeout         time.Duration `json:"timeout"`
	RetryCount      int           `json:"retry_count"`
	RetryDelay      time.Duration `json:"retry_delay"`
	MessageLimit    int           `json:"message_limit"`
	EnableWebhook   bool          `json:"enable_webhook"`
	WebhookURL      string        `json:"webhook_url"`
}

// IAMClientConfig holds IAM service client configuration
type IAMClientConfig struct {
	Host        string        `json:"host"`
	Port        int           `json:"port"`
	Timeout     time.Duration `json:"timeout"`
	RetryCount  int           `json:"retry_count"`
	RetryDelay  time.Duration `json:"retry_delay"`
	EnableTLS   bool          `json:"enable_tls"`
	TLSInsecure bool          `json:"tls_insecure"`
	CertFile    string        `json:"cert_file"`
	KeyFile     string        `json:"key_file"`
	CAFile      string        `json:"ca_file"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level        string `json:"level"`
	Format       string `json:"format"` // json, text
	Output       string `json:"output"` // stdout, stderr, file
	FilePath     string `json:"file_path"`
	MaxSize      int    `json:"max_size"` // MB
	MaxBackups   int    `json:"max_backups"`
	MaxAge       int    `json:"max_age"` // days
	Compress     bool   `json:"compress"`
	EnableCaller bool   `json:"enable_caller"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled   bool   `json:"enabled"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
	Namespace string `json:"namespace"`
	Subsystem string `json:"subsystem"`
}

// TracingConfig holds tracing configuration
type TracingConfig struct {
	Enabled        bool          `json:"enabled"`
	ServiceName    string        `json:"service_name"`
	ServiceVersion string        `json:"service_version"`
	Environment    string        `json:"environment"`
	JaegerEndpoint string        `json:"jaeger_endpoint"`
	SamplingRate   float64       `json:"sampling_rate"`
	BatchTimeout   time.Duration `json:"batch_timeout"`
	MaxBatchSize   int           `json:"max_batch_size"`
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	config := &Config{
		Service: ServiceConfig{
			Name:                    getEnvWithDefault("SERVICE_NAME", "notification-service"),
			Version:                 getEnvWithDefault("SERVICE_VERSION", "1.0.0"),
			Environment:             getEnvWithDefault("ENVIRONMENT", "development"),
			Host:                    getEnvWithDefault("SERVICE_HOST", "0.0.0.0"),
			Port:                    getEnvAsIntWithDefault("SERVICE_PORT", 8080),
			HealthPort:              getEnvAsIntWithDefault("HEALTH_PORT", 8081),
			GracefulShutdownTimeout: getEnvAsDurationWithDefault("GRACEFUL_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Kafka: KafkaConfig{
			Consumer: kafka.ConsumerConfig{
				Brokers:            strings.Split(getEnvWithDefault("KAFKA_BROKERS", "localhost:9092"), ","),
				GroupID:            getEnvWithDefault("KAFKA_GROUP_ID", "notification-service-group"),
				ClientID:           getEnvWithDefault("KAFKA_CLIENT_ID", "notification-service"),
				Topics:             []string{}, // Will be populated from topic config
				SessionTimeout:     getEnvAsDurationWithDefault("KAFKA_SESSION_TIMEOUT", 30*time.Second),
				HeartbeatInterval:  getEnvAsDurationWithDefault("KAFKA_HEARTBEAT_INTERVAL", 3*time.Second),
				RebalanceTimeout:   getEnvAsDurationWithDefault("KAFKA_REBALANCE_TIMEOUT", 60*time.Second),
				InitialOffset:      getEnvWithDefault("KAFKA_INITIAL_OFFSET", "newest"),
				EnableAutoCommit:   getEnvAsBoolWithDefault("KAFKA_ENABLE_AUTO_COMMIT", true),
				AutoCommitInterval: getEnvAsDurationWithDefault("KAFKA_AUTO_COMMIT_INTERVAL", 1*time.Second),
				MaxProcessingTime:  getEnvAsDurationWithDefault("KAFKA_MAX_PROCESSING_TIME", 30*time.Second),
				ConcurrencyLevel:   getEnvAsIntWithDefault("KAFKA_CONCURRENCY_LEVEL", 1),
				RetryAttempts:      getEnvAsIntWithDefault("KAFKA_RETRY_ATTEMPTS", 3),
				RetryBackoff:       getEnvAsDurationWithDefault("KAFKA_RETRY_BACKOFF", 1*time.Second),
				EnableDeadLetter:   getEnvAsBoolWithDefault("KAFKA_ENABLE_DEAD_LETTER", true),
				DeadLetterTopic:    getEnvWithDefault("KAFKA_DEAD_LETTER_TOPIC", "notification-dead-letter"),
			},
			Topics: TopicConfig{
				OrderEvents:    getEnvWithDefault("KAFKA_ORDER_EVENTS_TOPIC", "order-events"),
				PaymentEvents:  getEnvWithDefault("KAFKA_PAYMENT_EVENTS_TOPIC", "payment-events"),
				AssemblyEvents: getEnvWithDefault("KAFKA_ASSEMBLY_EVENTS_TOPIC", "assembly-events"),
			},
		},
		Telegram: TelegramConfig{
			BotToken:        getEnvWithDefault("TELEGRAM_BOT_TOKEN", ""),
			DevelopmentMode: getEnvAsBoolWithDefault("TELEGRAM_DEVELOPMENT_MODE", true),
			Timeout:         getEnvAsDurationWithDefault("TELEGRAM_TIMEOUT", 30*time.Second),
			RetryCount:      getEnvAsIntWithDefault("TELEGRAM_RETRY_COUNT", 3),
			RetryDelay:      getEnvAsDurationWithDefault("TELEGRAM_RETRY_DELAY", 1*time.Second),
			MessageLimit:    getEnvAsIntWithDefault("TELEGRAM_MESSAGE_LIMIT", 4096),
			EnableWebhook:   getEnvAsBoolWithDefault("TELEGRAM_ENABLE_WEBHOOK", false),
			WebhookURL:      getEnvWithDefault("TELEGRAM_WEBHOOK_URL", ""),
		},
		IAMClient: IAMClientConfig{
			Host:        getEnvWithDefault("IAM_SERVICE_HOST", "localhost"),
			Port:        getEnvAsIntWithDefault("IAM_SERVICE_PORT", 50051),
			Timeout:     getEnvAsDurationWithDefault("IAM_CLIENT_TIMEOUT", 10*time.Second),
			RetryCount:  getEnvAsIntWithDefault("IAM_CLIENT_RETRY_COUNT", 3),
			RetryDelay:  getEnvAsDurationWithDefault("IAM_CLIENT_RETRY_DELAY", 1*time.Second),
			EnableTLS:   getEnvAsBoolWithDefault("IAM_CLIENT_ENABLE_TLS", false),
			TLSInsecure: getEnvAsBoolWithDefault("IAM_CLIENT_TLS_INSECURE", true),
			CertFile:    getEnvWithDefault("IAM_CLIENT_CERT_FILE", ""),
			KeyFile:     getEnvWithDefault("IAM_CLIENT_KEY_FILE", ""),
			CAFile:      getEnvWithDefault("IAM_CLIENT_CA_FILE", ""),
		},
		Logging: LoggingConfig{
			Level:        getEnvWithDefault("LOG_LEVEL", "info"),
			Format:       getEnvWithDefault("LOG_FORMAT", "json"),
			Output:       getEnvWithDefault("LOG_OUTPUT", "stdout"),
			FilePath:     getEnvWithDefault("LOG_FILE_PATH", "/var/log/notification-service.log"),
			MaxSize:      getEnvAsIntWithDefault("LOG_MAX_SIZE", 100),
			MaxBackups:   getEnvAsIntWithDefault("LOG_MAX_BACKUPS", 3),
			MaxAge:       getEnvAsIntWithDefault("LOG_MAX_AGE", 7),
			Compress:     getEnvAsBoolWithDefault("LOG_COMPRESS", true),
			EnableCaller: getEnvAsBoolWithDefault("LOG_ENABLE_CALLER", true),
		},
		Metrics: MetricsConfig{
			Enabled:   getEnvAsBoolWithDefault("METRICS_ENABLED", true),
			Port:      getEnvAsIntWithDefault("METRICS_PORT", 9090),
			Path:      getEnvWithDefault("METRICS_PATH", "/metrics"),
			Namespace: getEnvWithDefault("METRICS_NAMESPACE", "rocket_science"),
			Subsystem: getEnvWithDefault("METRICS_SUBSYSTEM", "notification_service"),
		},
		Tracing: TracingConfig{
			Enabled:        getEnvAsBoolWithDefault("TRACING_ENABLED", true),
			ServiceName:    getEnvWithDefault("TRACING_SERVICE_NAME", "notification-service"),
			ServiceVersion: getEnvWithDefault("TRACING_SERVICE_VERSION", "1.0.0"),
			Environment:    getEnvWithDefault("TRACING_ENVIRONMENT", "development"),
			JaegerEndpoint: getEnvWithDefault("JAEGER_ENDPOINT", "http://localhost:14268/api/traces"),
			SamplingRate:   getEnvAsFloatWithDefault("TRACING_SAMPLING_RATE", 1.0),
			BatchTimeout:   getEnvAsDurationWithDefault("TRACING_BATCH_TIMEOUT", 1*time.Second),
			MaxBatchSize:   getEnvAsIntWithDefault("TRACING_MAX_BATCH_SIZE", 100),
		},
	}

	// Populate Kafka topics
	config.Kafka.Consumer.Topics = []string{
		config.Kafka.Topics.OrderEvents,
		config.Kafka.Topics.PaymentEvents,
		config.Kafka.Topics.AssemblyEvents,
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate Telegram bot token (skip in development mode)
	if !c.Telegram.DevelopmentMode && c.Telegram.BotToken == "" {
		return fmt.Errorf("telegram bot token is required")
	}

	// Validate Kafka brokers
	if len(c.Kafka.Consumer.Brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}

	// Validate topics
	if c.Kafka.Topics.OrderEvents == "" || c.Kafka.Topics.PaymentEvents == "" || c.Kafka.Topics.AssemblyEvents == "" {
		return fmt.Errorf("all kafka topics must be configured")
	}

	// Validate IAM client
	if c.IAMClient.Host == "" {
		return fmt.Errorf("IAM service host is required")
	}

	return nil
}

// Helper functions for environment variable parsing

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsIntWithDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBoolWithDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvAsFloatWithDefault(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvAsDurationWithDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
