package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the IAM service
type Config struct {
	Server        ServerConfig        `json:"server"`
	Database      DatabaseConfig      `json:"database"`
	Redis         RedisConfig         `json:"redis"`
	JWT           JWTConfig           `json:"jwt"`
	Security      SecurityConfig      `json:"security"`
	Observability ObservabilityConfig `json:"observability"`
}

// ServerConfig holds gRPC server configuration
type ServerConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
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
	// Driver-level timeout configurations
	ConnectTimeout time.Duration `json:"connect_timeout"`
	QueryTimeout   time.Duration `json:"query_timeout"`
	ReadTimeout    time.Duration `json:"read_timeout"`
	WriteTimeout   time.Duration `json:"write_timeout"`
}

// RedisConfig holds Redis configuration for session storage
type RedisConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	Password     string        `json:"password"`
	DB           int           `json:"db"`
	PoolSize     int           `json:"pool_size"`
	MinIdleConns int           `json:"min_idle_conns"`
	DialTimeout  time.Duration `json:"dial_timeout"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
}

// JWTConfig holds JWT token configuration
type JWTConfig struct {
	SecretKey            string        `json:"-"` // Never serialize secret
	AccessTokenDuration  time.Duration `json:"access_token_duration"`
	RefreshTokenDuration time.Duration `json:"refresh_token_duration"`
	SessionDuration      time.Duration `json:"session_duration"`
	Issuer               string        `json:"issuer"`
	Algorithm            string        `json:"algorithm"`
}

// SecurityConfig holds security-related settings
type SecurityConfig struct {
	PasswordMinLength      int           `json:"password_min_length"`
	PasswordRequireUpper   bool          `json:"password_require_upper"`
	PasswordRequireLower   bool          `json:"password_require_lower"`
	PasswordRequireDigits  bool          `json:"password_require_digits"`
	PasswordRequireSymbol  bool          `json:"password_require_symbol"`
	MaxLoginAttempts       int           `json:"max_login_attempts"`
	LoginAttemptWindow     time.Duration `json:"login_attempt_window"`
	AccountLockoutTime     time.Duration `json:"account_lockout_time"`
	SessionCleanupInterval time.Duration `json:"session_cleanup_interval"`
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
			Host:         getEnv("IAM_SERVER_HOST", "0.0.0.0"),
			Port:         getEnvAsInt("IAM_SERVER_PORT", 50051),
			ReadTimeout:  getEnvAsDuration("IAM_SERVER_READ_TIMEOUT", "30s"),
			WriteTimeout: getEnvAsDuration("IAM_SERVER_WRITE_TIMEOUT", "30s"),
		},
		Database: DatabaseConfig{
			Host:            getEnv("IAM_DB_HOST", "localhost"),
			Port:            getEnvAsInt("IAM_DB_PORT", 5432),
			User:            getEnv("IAM_DB_USER", "postgres"),
			Password:        getEnv("IAM_DB_PASSWORD", "password"),
			DBName:          getEnv("IAM_DB_NAME", "iam_db"),
			SSLMode:         getEnv("IAM_DB_SSL_MODE", "disable"),
			MaxOpenConns:    getEnvAsInt("IAM_DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvAsInt("IAM_DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvAsDuration("IAM_DB_CONN_MAX_LIFETIME", "5m"),
			ConnectTimeout:  getEnvAsDuration("IAM_DB_CONNECT_TIMEOUT", "5s"),
			QueryTimeout:    getEnvAsDuration("IAM_DB_QUERY_TIMEOUT", "5s"),
			ReadTimeout:     getEnvAsDuration("IAM_DB_READ_TIMEOUT", "3s"),
			WriteTimeout:    getEnvAsDuration("IAM_DB_WRITE_TIMEOUT", "3s"),
		},
		Redis: RedisConfig{
			Host:         getEnv("IAM_REDIS_HOST", "localhost"),
			Port:         getEnvAsInt("IAM_REDIS_PORT", 6379),
			Password:     getEnv("IAM_REDIS_PASSWORD", ""),
			DB:           getEnvAsInt("IAM_REDIS_DB", 0),
			PoolSize:     getEnvAsInt("IAM_REDIS_POOL_SIZE", 10),
			MinIdleConns: getEnvAsInt("IAM_REDIS_MIN_IDLE_CONNS", 1),
			DialTimeout:  getEnvAsDuration("IAM_REDIS_DIAL_TIMEOUT", "5s"),
			ReadTimeout:  getEnvAsDuration("IAM_REDIS_READ_TIMEOUT", "3s"),
			WriteTimeout: getEnvAsDuration("IAM_REDIS_WRITE_TIMEOUT", "3s"),
			IdleTimeout:  getEnvAsDuration("IAM_REDIS_IDLE_TIMEOUT", "5m"),
		},
		JWT: JWTConfig{
			SecretKey:            getEnv("IAM_JWT_SECRET", "your-secret-key-change-in-production"),
			AccessTokenDuration:  getEnvAsDuration("IAM_JWT_ACCESS_TOKEN_DURATION", "15m"),
			RefreshTokenDuration: getEnvAsDuration("IAM_JWT_REFRESH_TOKEN_DURATION", "24h"),
			SessionDuration:      getEnvAsDuration("IAM_JWT_SESSION_DURATION", "7d"),
			Issuer:               getEnv("IAM_JWT_ISSUER", "rocket-science-iam"),
			Algorithm:            getEnv("IAM_JWT_ALGORITHM", "HS256"),
		},
		Security: SecurityConfig{
			PasswordMinLength:      getEnvAsInt("IAM_PASSWORD_MIN_LENGTH", 8),
			PasswordRequireUpper:   getEnvAsBool("IAM_PASSWORD_REQUIRE_UPPER", true),
			PasswordRequireLower:   getEnvAsBool("IAM_PASSWORD_REQUIRE_LOWER", true),
			PasswordRequireDigits:  getEnvAsBool("IAM_PASSWORD_REQUIRE_DIGITS", true),
			PasswordRequireSymbol:  getEnvAsBool("IAM_PASSWORD_REQUIRE_SYMBOL", false),
			MaxLoginAttempts:       getEnvAsInt("IAM_MAX_LOGIN_ATTEMPTS", 5),
			LoginAttemptWindow:     getEnvAsDuration("IAM_LOGIN_ATTEMPT_WINDOW", "15m"),
			AccountLockoutTime:     getEnvAsDuration("IAM_ACCOUNT_LOCKOUT_TIME", "30m"),
			SessionCleanupInterval: getEnvAsDuration("IAM_SESSION_CLEANUP_INTERVAL", "1h"),
		},
		Observability: ObservabilityConfig{
			ServiceName:    getEnv("SERVICE_NAME", "iam-service"),
			ServiceVersion: getEnv("SERVICE_VERSION", "1.0.0"),
			MetricsEnabled: getEnvAsBool("METRICS_ENABLED", true),
			TracingEnabled: getEnvAsBool("TRACING_ENABLED", true),
			LogLevel:       getEnv("LOG_LEVEL", "info"),
			OTELEndpoint:   getEnv("OTEL_ENDPOINT", "http://localhost:4317"),
		},
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

// validate checks if the configuration is valid
func (c *Config) validate() error {
	// Validate server config
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	// Validate database config
	if c.Database.Host == "" {
		return fmt.Errorf("database host cannot be empty")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database name cannot be empty")
	}
	if c.Database.User == "" {
		return fmt.Errorf("database user cannot be empty")
	}

	// Validate Redis config
	if c.Redis.Host == "" {
		return fmt.Errorf("Redis host cannot be empty")
	}

	// Validate JWT config
	if c.JWT.SecretKey == "" {
		return fmt.Errorf("JWT secret key cannot be empty")
	}
	if c.JWT.SecretKey == "your-secret-key-change-in-production" {
		return fmt.Errorf("JWT secret key must be changed from default value")
	}
	if len(c.JWT.SecretKey) < 32 {
		return fmt.Errorf("JWT secret key must be at least 32 characters long")
	}

	// Validate security config
	if c.Security.PasswordMinLength < 6 {
		return fmt.Errorf("password minimum length cannot be less than 6")
	}
	if c.Security.MaxLoginAttempts < 1 {
		return fmt.Errorf("max login attempts must be at least 1")
	}

	return nil
}

// DSN returns the PostgreSQL database connection string
func (c *DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode)
}

// RedisAddr returns the Redis connection address
func (c *RedisConfig) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
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
