package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the Inventory Service
type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	Inventory     InventoryConfig
	Observability ObservabilityConfig
}

// ServerConfig contains gRPC server configuration
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DatabaseConfig contains MongoDB connection settings
type DatabaseConfig struct {
	ConnectionURL    string
	DatabaseName     string
	ConnectTimeout   time.Duration
	QueryTimeout     time.Duration
	MaxPoolSize      int
	MinPoolSize      int
	MaxConnIdleTime  time.Duration
}

// InventoryConfig contains inventory-specific settings
type InventoryConfig struct {
	DefaultStockLevel     int
	LowStockThreshold     int
	MaxReservationTimeMin int // Maximum time to hold reservations
	AutoRestockEnabled    bool
}

// ObservabilityConfig contains observability settings
type ObservabilityConfig struct {
	LogLevel        string
	MetricsEnabled  bool
	TracingEnabled  bool
	ServiceName     string
	ServiceVersion  string
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         getEnvOrDefault("INVENTORY_SERVICE_PORT", "50053"),
			ReadTimeout:  parseDurationOrDefault("INVENTORY_SERVICE_READ_TIMEOUT", "30s"),
			WriteTimeout: parseDurationOrDefault("INVENTORY_SERVICE_WRITE_TIMEOUT", "30s"),
		},
		Database: DatabaseConfig{
			ConnectionURL:   getEnvOrDefault("MONGODB_CONNECTION_URL", "mongodb://localhost:27017"),
			DatabaseName:    getEnvOrDefault("MONGODB_DATABASE_NAME", "inventory_db"),
			ConnectTimeout:  parseDurationOrDefault("MONGODB_CONNECT_TIMEOUT", "10s"),
			QueryTimeout:    parseDurationOrDefault("MONGODB_QUERY_TIMEOUT", "5s"),
			MaxPoolSize:     parseIntOrDefault("MONGODB_MAX_POOL_SIZE", "100"),
			MinPoolSize:     parseIntOrDefault("MONGODB_MIN_POOL_SIZE", "10"),
			MaxConnIdleTime: parseDurationOrDefault("MONGODB_MAX_CONN_IDLE_TIME", "10m"),
		},
		Inventory: InventoryConfig{
			DefaultStockLevel:     parseIntOrDefault("INVENTORY_DEFAULT_STOCK_LEVEL", "100"),
			LowStockThreshold:     parseIntOrDefault("INVENTORY_LOW_STOCK_THRESHOLD", "10"),
			MaxReservationTimeMin: parseIntOrDefault("INVENTORY_MAX_RESERVATION_TIME_MIN", "30"),
			AutoRestockEnabled:    parseBoolOrDefault("INVENTORY_AUTO_RESTOCK_ENABLED", "false"),
		},
		Observability: ObservabilityConfig{
			LogLevel:        getEnvOrDefault("LOG_LEVEL", "info"),
			MetricsEnabled:  parseBoolOrDefault("METRICS_ENABLED", "true"),
			TracingEnabled:  parseBoolOrDefault("TRACING_ENABLED", "true"),
			ServiceName:     getEnvOrDefault("SERVICE_NAME", "inventory-service"),
			ServiceVersion:  getEnvOrDefault("SERVICE_VERSION", "1.0.0"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// validate checks if the configuration is valid
func (c *Config) validate() error {
	// Validate server config
	if c.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}

	// Validate database config
	if c.Database.ConnectionURL == "" {
		return fmt.Errorf("MongoDB connection URL cannot be empty")
	}
	if c.Database.DatabaseName == "" {
		return fmt.Errorf("MongoDB database name cannot be empty")
	}
	if c.Database.MaxPoolSize <= 0 {
		return fmt.Errorf("MongoDB max pool size must be positive")
	}
	if c.Database.MinPoolSize < 0 {
		return fmt.Errorf("MongoDB min pool size cannot be negative")
	}
	if c.Database.MinPoolSize > c.Database.MaxPoolSize {
		return fmt.Errorf("MongoDB min pool size cannot be greater than max pool size")
	}

	// Validate inventory config
	if c.Inventory.DefaultStockLevel < 0 {
		return fmt.Errorf("default stock level cannot be negative")
	}
	if c.Inventory.LowStockThreshold < 0 {
		return fmt.Errorf("low stock threshold cannot be negative")
	}
	if c.Inventory.MaxReservationTimeMin <= 0 {
		return fmt.Errorf("max reservation time must be positive")
	}

	// Validate observability config
	if c.Observability.ServiceName == "" {
		return fmt.Errorf("service name must be specified")
	}

	return nil
}

// GetMongoDBConfig returns MongoDB client options based on configuration
func (c *Config) GetMongoDBConfig() map[string]interface{} {
	return map[string]interface{}{
		"connection_url":     c.Database.ConnectionURL,
		"database_name":      c.Database.DatabaseName,
		"connect_timeout":    c.Database.ConnectTimeout,
		"query_timeout":      c.Database.QueryTimeout,
		"max_pool_size":      c.Database.MaxPoolSize,
		"min_pool_size":      c.Database.MinPoolSize,
		"max_conn_idle_time": c.Database.MaxConnIdleTime,
	}
}

// Helper functions for environment variable parsing

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseIntOrDefault(key string, defaultValue string) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	if parsed, err := strconv.Atoi(defaultValue); err == nil {
		return parsed
	}
	return 0
}

func parseFloatOrDefault(key string, defaultValue string) float64 {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	if parsed, err := strconv.ParseFloat(defaultValue, 64); err == nil {
		return parsed
	}
	return 0.0
}

func parseBoolOrDefault(key string, defaultValue string) bool {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	if parsed, err := strconv.ParseBool(defaultValue); err == nil {
		return parsed
	}
	return false
}

func parseDurationOrDefault(key string, defaultValue string) time.Duration {
	if value := os.Getenv(key); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	if parsed, err := time.ParseDuration(defaultValue); err == nil {
		return parsed
	}
	return 30 * time.Second
}