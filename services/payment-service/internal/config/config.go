package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the Payment Service
type Config struct {
	Server        ServerConfig
	Payment       PaymentConfig
	Observability ObservabilityConfig
}

// ServerConfig contains gRPC server configuration
type ServerConfig struct {
	Port         string
	HealthPort   string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// PaymentConfig contains payment processing configuration
type PaymentConfig struct {
	ProcessingTimeMs int
	SuccessRate      float64 // Probability of successful payment (0.0 - 1.0)
	MaxAmount        float64
}

// ObservabilityConfig contains observability settings
type ObservabilityConfig struct {
	LogLevel       string
	MetricsEnabled bool
	TracingEnabled bool
	ServiceName    string
	ServiceVersion string
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:         getEnvOrDefault("PAYMENT_SERVICE_PORT", "50052"),
			HealthPort:   getEnvOrDefault("PAYMENT_SERVICE_HEALTH_PORT", "8081"),
			ReadTimeout:  parseDurationOrDefault("PAYMENT_SERVICE_READ_TIMEOUT", "30s"),
			WriteTimeout: parseDurationOrDefault("PAYMENT_SERVICE_WRITE_TIMEOUT", "30s"),
		},
		Payment: PaymentConfig{
			ProcessingTimeMs: parseIntOrDefault("PAYMENT_PROCESSING_TIME_MS", "500"),
			SuccessRate:      parseFloatOrDefault("PAYMENT_SUCCESS_RATE", "0.95"),
			MaxAmount:        parseFloatOrDefault("PAYMENT_MAX_AMOUNT", "1000000.0"),
		},
		Observability: ObservabilityConfig{
			LogLevel:       getEnvOrDefault("LOG_LEVEL", "info"),
			MetricsEnabled: parseBoolOrDefault("METRICS_ENABLED", "true"),
			TracingEnabled: parseBoolOrDefault("TRACING_ENABLED", "true"),
			ServiceName:    getEnvOrDefault("SERVICE_NAME", "payment-service"),
			ServiceVersion: getEnvOrDefault("SERVICE_VERSION", "1.0.0"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// validate checks if the configuration is valid
func (c *Config) validate() error {
	if c.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}

	if c.Payment.SuccessRate < 0.0 || c.Payment.SuccessRate > 1.0 {
		return fmt.Errorf("payment success rate must be between 0.0 and 1.0")
	}

	if c.Payment.MaxAmount <= 0 {
		return fmt.Errorf("payment max amount must be positive")
	}

	if c.Payment.ProcessingTimeMs < 0 {
		return fmt.Errorf("payment processing time cannot be negative")
	}

	return nil
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
