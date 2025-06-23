package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the order service
type Config struct {
	Server        ServerConfig        `json:"server"`
	Database      DatabaseConfig      `json:"database"`
	Kafka         KafkaConfig         `json:"kafka"`
	GRPC          GRPCConfig          `json:"grpc"`
	Observability ObservabilityConfig `json:"observability"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
}

// DatabaseConfig holds PostgreSQL database configuration
type DatabaseConfig struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	User            string        `json:"user"`
	Password        string        `json:"password"`
	DBName          string        `json:"db_name"`
	SSLMode         string        `json:"ssl_mode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
}

// KafkaConfig holds Kafka configuration
type KafkaConfig struct {
	Brokers                []string      `json:"brokers"`
	PaymentEventsTopic     string        `json:"payment_events_topic"`
	AssemblyEventsTopic    string        `json:"assembly_events_topic"`
	ConsumerGroup          string        `json:"consumer_group"`
	ProducerRetries        int           `json:"producer_retries"`
	ConsumerSessionTimeout time.Duration `json:"consumer_session_timeout"`
}

// GRPCConfig holds gRPC clients configuration
type GRPCConfig struct {
	InventoryService InventoryServiceConfig `json:"inventory_service"`
	PaymentService   PaymentServiceConfig   `json:"payment_service"`
}

// InventoryServiceConfig holds inventory service gRPC client configuration
type InventoryServiceConfig struct {
	Address       string        `json:"address"`
	Timeout       time.Duration `json:"timeout"`
	MaxRetries    int           `json:"max_retries"`
	RetryInterval time.Duration `json:"retry_interval"`
}

// PaymentServiceConfig holds payment service gRPC client configuration
type PaymentServiceConfig struct {
	Address       string        `json:"address"`
	Timeout       time.Duration `json:"timeout"`
	MaxRetries    int           `json:"max_retries"`
	RetryInterval time.Duration `json:"retry_interval"`
}

// ObservabilityConfig holds observability configuration
type ObservabilityConfig struct {
	ServiceName    string `json:"service_name"`
	ServiceVersion string `json:"service_version"`
	MetricsEnabled bool   `json:"metrics_enabled"`
	TracingEnabled bool   `json:"tracing_enabled"`
	LogLevel       string `json:"log_level"`
	OTELEndpoint   string `json:"otel_endpoint"`
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	config := &Config{
		Server: ServerConfig{
			Host:         getEnv("SERVER_HOST", "0.0.0.0"),
			Port:         getEnvAsInt("SERVER_PORT", 8080),
			ReadTimeout:  getEnvAsDuration("SERVER_READ_TIMEOUT", "30s"),
			WriteTimeout: getEnvAsDuration("SERVER_WRITE_TIMEOUT", "30s"),
			IdleTimeout:  getEnvAsDuration("SERVER_IDLE_TIMEOUT", "120s"),
		},
		Database: DatabaseConfig{
			Host:            getEnv("DB_HOST", "localhost"),
			Port:            getEnvAsInt("DB_PORT", 5432),
			User:            getEnv("DB_USER", "postgres"),
			Password:        getEnv("DB_PASSWORD", "password"),
			DBName:          getEnv("DB_NAME", "orders"),
			SSLMode:         getEnv("DB_SSL_MODE", "disable"),
			MaxOpenConns:    getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvAsDuration("DB_CONN_MAX_LIFETIME", "5m"),
		},
		Kafka: KafkaConfig{
			Brokers:                getEnvAsSlice("KAFKA_BROKERS", "localhost:9092"),
			PaymentEventsTopic:     getEnv("KAFKA_PAYMENT_EVENTS_TOPIC", "payment-events"),
			AssemblyEventsTopic:    getEnv("KAFKA_ASSEMBLY_EVENTS_TOPIC", "assembly-events"),
			ConsumerGroup:          getEnv("KAFKA_CONSUMER_GROUP", "order-service"),
			ProducerRetries:        getEnvAsInt("KAFKA_PRODUCER_RETRIES", 3),
			ConsumerSessionTimeout: getEnvAsDuration("KAFKA_CONSUMER_SESSION_TIMEOUT", "30s"),
		},
		GRPC: GRPCConfig{
			InventoryService: InventoryServiceConfig{
				Address:       getEnv("INVENTORY_SERVICE_ADDRESS", "localhost:50053"),
				Timeout:       getEnvAsDuration("INVENTORY_SERVICE_TIMEOUT", "10s"),
				MaxRetries:    getEnvAsInt("INVENTORY_SERVICE_MAX_RETRIES", 3),
				RetryInterval: getEnvAsDuration("INVENTORY_SERVICE_RETRY_INTERVAL", "1s"),
			},
			PaymentService: PaymentServiceConfig{
				Address:       getEnv("PAYMENT_SERVICE_ADDRESS", "localhost:9002"),
				Timeout:       getEnvAsDuration("PAYMENT_SERVICE_TIMEOUT", "10s"),
				MaxRetries:    getEnvAsInt("PAYMENT_SERVICE_MAX_RETRIES", 3),
				RetryInterval: getEnvAsDuration("PAYMENT_SERVICE_RETRY_INTERVAL", "1s"),
			},
		},
		Observability: ObservabilityConfig{
			ServiceName:    getEnv("SERVICE_NAME", "order-service"),
			ServiceVersion: getEnv("SERVICE_VERSION", "1.0.0"),
			MetricsEnabled: getEnvAsBool("METRICS_ENABLED", true),
			TracingEnabled: getEnvAsBool("TRACING_ENABLED", true),
			LogLevel:       getEnv("LOG_LEVEL", "info"),
			OTELEndpoint:   getEnv("OTEL_ENDPOINT", "http://localhost:4317"),
		},
	}

	return config, nil
}

// DSN returns the PostgreSQL database connection string
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
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

func getEnvAsDuration(key string, defaultValue string) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	duration, _ := time.ParseDuration(defaultValue)
	return duration
}

func getEnvAsSlice(key string, defaultValue string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return strings.Split(defaultValue, ",")
}
