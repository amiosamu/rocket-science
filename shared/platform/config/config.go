package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amiosamu/rocket-science/shared/platform/errors"
)

// BaseConfig contains common configuration fields for all services
type BaseConfig struct {
	Service       ServiceConfig       `json:"service"`
	Server        ServerConfig        `json:"server"`
	Database      DatabaseConfig      `json:"database"`
	Observability ObservabilityConfig `json:"observability"`
}

// ServiceConfig holds service-specific configuration
type ServiceConfig struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Environment string `json:"environment"`
	Debug       bool   `json:"debug"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
	TLSEnabled   bool          `json:"tls_enabled"`
	CertFile     string        `json:"cert_file"`
	KeyFile      string        `json:"key_file"`
}

// DatabaseConfig holds database configuration for different database types
type DatabaseConfig struct {
	Type         string               `json:"type"` // postgres, mongodb, redis
	PostgreSQL   PostgreSQLConfig     `json:"postgresql"`
	MongoDB      MongoDBConfig        `json:"mongodb"`
	Redis        RedisConfig          `json:"redis"`
}

// PostgreSQLConfig holds PostgreSQL-specific configuration
type PostgreSQLConfig struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	User            string        `json:"user"`
	Password        string        `json:"password"`
	DBName          string        `json:"db_name"`
	SSLMode         string        `json:"ssl_mode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	ConnectTimeout  time.Duration `json:"connect_timeout"`
}

// MongoDBConfig holds MongoDB-specific configuration
type MongoDBConfig struct {
	URI            string        `json:"uri"`
	Database       string        `json:"database"`
	ConnectTimeout time.Duration `json:"connect_timeout"`
	QueryTimeout   time.Duration `json:"query_timeout"`
	MaxPoolSize    uint64        `json:"max_pool_size"`
	MinPoolSize    uint64        `json:"min_pool_size"`
	MaxIdleTime    time.Duration `json:"max_idle_time"`
}

// RedisConfig holds Redis-specific configuration
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
	MaxRetries   int           `json:"max_retries"`
	TLSEnabled   bool          `json:"tls_enabled"`
}

// ObservabilityConfig holds observability configuration
type ObservabilityConfig struct {
	ServiceName    string `json:"service_name"`
	ServiceVersion string `json:"service_version"`
	LogLevel       string `json:"log_level"`
	LogFormat      string `json:"log_format"` // json, text
	MetricsEnabled bool   `json:"metrics_enabled"`
	MetricsPort    int    `json:"metrics_port"`
	TracingEnabled bool   `json:"tracing_enabled"`
	OTELEndpoint   string `json:"otel_endpoint"`
	SamplingRatio  float64 `json:"sampling_ratio"`
}

// KafkaConfig holds Kafka configuration
type KafkaConfig struct {
	Brokers                []string      `json:"brokers"`
	ClientID               string        `json:"client_id"`
	ConsumerGroup          string        `json:"consumer_group"`
	ProducerRetries        int           `json:"producer_retries"`
	ProducerTimeout        time.Duration `json:"producer_timeout"`
	ConsumerSessionTimeout time.Duration `json:"consumer_session_timeout"`
	ConsumerRetries        int           `json:"consumer_retries"`
	EnableAutoCommit       bool          `json:"enable_auto_commit"`
	AutoCommitInterval     time.Duration `json:"auto_commit_interval"`
	InitialOffset          string        `json:"initial_offset"` // oldest, newest
}

// GRPCConfig holds gRPC configuration
type GRPCConfig struct {
	Host          string        `json:"host"`
	Port          int           `json:"port"`
	Timeout       time.Duration `json:"timeout"`
	MaxRetries    int           `json:"max_retries"`
	RetryInterval time.Duration `json:"retry_interval"`
	TLSEnabled    bool          `json:"tls_enabled"`
	CertFile      string        `json:"cert_file"`
	KeyFile       string        `json:"key_file"`
	CAFile        string        `json:"ca_file"`
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	JWTSecret          string        `json:"jwt_secret"`
	JWTExpiration      time.Duration `json:"jwt_expiration"`
	PasswordMinLength  int           `json:"password_min_length"`
	SessionTimeout     time.Duration `json:"session_timeout"`
	EnableRateLimit    bool          `json:"enable_rate_limit"`
	RateLimitRPM       int           `json:"rate_limit_rpm"` // requests per minute
	EnableCORS         bool          `json:"enable_cors"`
	CORSAllowedOrigins []string      `json:"cors_allowed_origins"`
}

// Loader provides methods to load configuration from various sources
type Loader struct {
	prefix string
}

// NewLoader creates a new configuration loader with optional prefix
func NewLoader(prefix string) *Loader {
	return &Loader{prefix: prefix}
}

// LoadBase loads base configuration common to all services
func (l *Loader) LoadBase() (*BaseConfig, error) {
	config := &BaseConfig{
		Service:       l.loadServiceConfig(),
		Server:        l.loadServerConfig(),
		Database:      l.loadDatabaseConfig(),
		Observability: l.loadObservabilityConfig(),
	}

	if err := l.validateBaseConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// LoadKafka loads Kafka configuration
func (l *Loader) LoadKafka() KafkaConfig {
	return KafkaConfig{
		Brokers:                l.getEnvAsSlice("KAFKA_BROKERS", "localhost:9092"),
		ClientID:               l.getEnv("KAFKA_CLIENT_ID", "rocket-science-client"),
		ConsumerGroup:          l.getEnv("KAFKA_CONSUMER_GROUP", "rocket-science-group"),
		ProducerRetries:        l.getEnvAsInt("KAFKA_PRODUCER_RETRIES", 3),
		ProducerTimeout:        l.getEnvAsDuration("KAFKA_PRODUCER_TIMEOUT", "30s"),
		ConsumerSessionTimeout: l.getEnvAsDuration("KAFKA_CONSUMER_SESSION_TIMEOUT", "30s"),
		ConsumerRetries:        l.getEnvAsInt("KAFKA_CONSUMER_RETRIES", 3),
		EnableAutoCommit:       l.getEnvAsBool("KAFKA_ENABLE_AUTO_COMMIT", true),
		AutoCommitInterval:     l.getEnvAsDuration("KAFKA_AUTO_COMMIT_INTERVAL", "1s"),
		InitialOffset:          l.getEnv("KAFKA_INITIAL_OFFSET", "newest"),
	}
}

// LoadGRPC loads gRPC configuration
func (l *Loader) LoadGRPC() GRPCConfig {
	return GRPCConfig{
		Host:          l.getEnv("GRPC_HOST", "0.0.0.0"),
		Port:          l.getEnvAsInt("GRPC_PORT", 9090),
		Timeout:       l.getEnvAsDuration("GRPC_TIMEOUT", "30s"),
		MaxRetries:    l.getEnvAsInt("GRPC_MAX_RETRIES", 3),
		RetryInterval: l.getEnvAsDuration("GRPC_RETRY_INTERVAL", "1s"),
		TLSEnabled:    l.getEnvAsBool("GRPC_TLS_ENABLED", false),
		CertFile:      l.getEnv("GRPC_CERT_FILE", ""),
		KeyFile:       l.getEnv("GRPC_KEY_FILE", ""),
		CAFile:        l.getEnv("GRPC_CA_FILE", ""),
	}
}

// LoadSecurity loads security configuration
func (l *Loader) LoadSecurity() SecurityConfig {
	return SecurityConfig{
		JWTSecret:          l.getEnv("JWT_SECRET", ""),
		JWTExpiration:      l.getEnvAsDuration("JWT_EXPIRATION", "24h"),
		PasswordMinLength:  l.getEnvAsInt("PASSWORD_MIN_LENGTH", 8),
		SessionTimeout:     l.getEnvAsDuration("SESSION_TIMEOUT", "24h"),
		EnableRateLimit:    l.getEnvAsBool("ENABLE_RATE_LIMIT", true),
		RateLimitRPM:       l.getEnvAsInt("RATE_LIMIT_RPM", 100),
		EnableCORS:         l.getEnvAsBool("ENABLE_CORS", true),
		CORSAllowedOrigins: l.getEnvAsSlice("CORS_ALLOWED_ORIGINS", "*"),
	}
}

// Private methods for loading specific configurations

func (l *Loader) loadServiceConfig() ServiceConfig {
	return ServiceConfig{
		Name:        l.getEnv("SERVICE_NAME", "rocket-science-service"),
		Version:     l.getEnv("SERVICE_VERSION", "1.0.0"),
		Environment: l.getEnv("ENVIRONMENT", "development"),
		Debug:       l.getEnvAsBool("DEBUG", false),
	}
}

func (l *Loader) loadServerConfig() ServerConfig {
	return ServerConfig{
		Host:         l.getEnv("SERVER_HOST", "0.0.0.0"),
		Port:         l.getEnvAsInt("SERVER_PORT", 8080),
		ReadTimeout:  l.getEnvAsDuration("SERVER_READ_TIMEOUT", "30s"),
		WriteTimeout: l.getEnvAsDuration("SERVER_WRITE_TIMEOUT", "30s"),
		IdleTimeout:  l.getEnvAsDuration("SERVER_IDLE_TIMEOUT", "120s"),
		TLSEnabled:   l.getEnvAsBool("SERVER_TLS_ENABLED", false),
		CertFile:     l.getEnv("SERVER_CERT_FILE", ""),
		KeyFile:      l.getEnv("SERVER_KEY_FILE", ""),
	}
}

func (l *Loader) loadDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Type: l.getEnv("DATABASE_TYPE", "postgres"),
		PostgreSQL: PostgreSQLConfig{
			Host:            l.getEnv("DB_HOST", "localhost"),
			Port:            l.getEnvAsInt("DB_PORT", 5432),
			User:            l.getEnv("DB_USER", "postgres"),
			Password:        l.getEnv("DB_PASSWORD", "password"),
			DBName:          l.getEnv("DB_NAME", "rocket_science"),
			SSLMode:         l.getEnv("DB_SSL_MODE", "disable"),
			MaxOpenConns:    l.getEnvAsInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    l.getEnvAsInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: l.getEnvAsDuration("DB_CONN_MAX_LIFETIME", "5m"),
			ConnectTimeout:  l.getEnvAsDuration("DB_CONNECT_TIMEOUT", "30s"),
		},
		MongoDB: MongoDBConfig{
			URI:            l.getEnv("MONGODB_URI", "mongodb://localhost:27017"),
			Database:       l.getEnv("MONGODB_DATABASE", "rocket_science"),
			ConnectTimeout: l.getEnvAsDuration("MONGODB_CONNECT_TIMEOUT", "30s"),
			QueryTimeout:   l.getEnvAsDuration("MONGODB_QUERY_TIMEOUT", "30s"),
			MaxPoolSize:    l.getEnvAsUint64("MONGODB_MAX_POOL_SIZE", 100),
			MinPoolSize:    l.getEnvAsUint64("MONGODB_MIN_POOL_SIZE", 5),
			MaxIdleTime:    l.getEnvAsDuration("MONGODB_MAX_IDLE_TIME", "5m"),
		},
		Redis: RedisConfig{
			Host:         l.getEnv("REDIS_HOST", "localhost"),
			Port:         l.getEnvAsInt("REDIS_PORT", 6379),
			Password:     l.getEnv("REDIS_PASSWORD", ""),
			DB:           l.getEnvAsInt("REDIS_DB", 0),
			PoolSize:     l.getEnvAsInt("REDIS_POOL_SIZE", 10),
			MinIdleConns: l.getEnvAsInt("REDIS_MIN_IDLE_CONNS", 5),
			DialTimeout:  l.getEnvAsDuration("REDIS_DIAL_TIMEOUT", "5s"),
			ReadTimeout:  l.getEnvAsDuration("REDIS_READ_TIMEOUT", "3s"),
			WriteTimeout: l.getEnvAsDuration("REDIS_WRITE_TIMEOUT", "3s"),
			IdleTimeout:  l.getEnvAsDuration("REDIS_IDLE_TIMEOUT", "5m"),
			MaxRetries:   l.getEnvAsInt("REDIS_MAX_RETRIES", 3),
			TLSEnabled:   l.getEnvAsBool("REDIS_TLS_ENABLED", false),
		},
	}
}

func (l *Loader) loadObservabilityConfig() ObservabilityConfig {
	return ObservabilityConfig{
		ServiceName:    l.getEnv("SERVICE_NAME", "rocket-science-service"),
		ServiceVersion: l.getEnv("SERVICE_VERSION", "1.0.0"),
		LogLevel:       l.getEnv("LOG_LEVEL", "info"),
		LogFormat:      l.getEnv("LOG_FORMAT", "json"),
		MetricsEnabled: l.getEnvAsBool("METRICS_ENABLED", true),
		MetricsPort:    l.getEnvAsInt("METRICS_PORT", 2112),
		TracingEnabled: l.getEnvAsBool("TRACING_ENABLED", true),
		OTELEndpoint:   l.getEnv("OTEL_ENDPOINT", "http://localhost:4317"),
		SamplingRatio:  l.getEnvAsFloat64("TRACING_SAMPLING_RATIO", 1.0),
	}
}

// Validation methods

func (l *Loader) validateBaseConfig(config *BaseConfig) error {
	if config.Service.Name == "" {
		return errors.NewValidation("service name is required")
	}

	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return errors.NewValidation("server port must be between 1 and 65535")
	}

	if config.Observability.LogLevel != "" {
		validLevels := []string{"debug", "info", "warn", "error", "fatal"}
		valid := false
		for _, level := range validLevels {
			if strings.ToLower(config.Observability.LogLevel) == level {
				valid = true
				break
			}
		}
		if !valid {
			return errors.NewValidation("invalid log level: " + config.Observability.LogLevel)
		}
	}

	if config.Observability.SamplingRatio < 0 || config.Observability.SamplingRatio > 1 {
		return errors.NewValidation("sampling ratio must be between 0 and 1")
	}

	return nil
}

// Helper methods for environment variable parsing

func (l *Loader) getEnv(key, defaultValue string) string {
	if l.prefix != "" {
		key = l.prefix + "_" + key
	}
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func (l *Loader) getEnvAsInt(key string, defaultValue int) int {
	if l.prefix != "" {
		key = l.prefix + "_" + key
	}
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func (l *Loader) getEnvAsUint64(key string, defaultValue uint64) uint64 {
	if l.prefix != "" {
		key = l.prefix + "_" + key
	}
	if value := os.Getenv(key); value != "" {
		if uintValue, err := strconv.ParseUint(value, 10, 64); err == nil {
			return uintValue
		}
	}
	return defaultValue
}

func (l *Loader) getEnvAsBool(key string, defaultValue bool) bool {
	if l.prefix != "" {
		key = l.prefix + "_" + key
	}
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func (l *Loader) getEnvAsFloat64(key string, defaultValue float64) float64 {
	if l.prefix != "" {
		key = l.prefix + "_" + key
	}
	if value := os.Getenv(key); value != "" {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func (l *Loader) getEnvAsDuration(key string, defaultValue string) time.Duration {
	if l.prefix != "" {
		key = l.prefix + "_" + key
	}
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	duration, _ := time.ParseDuration(defaultValue)
	return duration
}

func (l *Loader) getEnvAsSlice(key string, defaultValue string) []string {
	if l.prefix != "" {
		key = l.prefix + "_" + key
	}
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return strings.Split(defaultValue, ",")
}

// Utility methods for configuration

// GetDSN returns a database connection string based on database type
func (c *DatabaseConfig) GetDSN() string {
	switch c.Type {
	case "postgres":
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
			c.PostgreSQL.Host, c.PostgreSQL.Port, c.PostgreSQL.User, c.PostgreSQL.Password,
			c.PostgreSQL.DBName, c.PostgreSQL.SSLMode, int(c.PostgreSQL.ConnectTimeout.Seconds()))
	case "mongodb":
		return c.MongoDB.URI
	case "redis":
		return fmt.Sprintf("%s:%d", c.Redis.Host, c.Redis.Port)
	default:
		return ""
	}
}

// GetServerAddress returns the server address in host:port format
func (c *ServerConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// GetGRPCAddress returns the gRPC server address in host:port format
func (c *GRPCConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// IsProduction returns true if the environment is production
func (c *ServiceConfig) IsProduction() bool {
	return strings.ToLower(c.Environment) == "production"
}

// IsDevelopment returns true if the environment is development
func (c *ServiceConfig) IsDevelopment() bool {
	return strings.ToLower(c.Environment) == "development"
}

// IsDebugEnabled returns true if debug mode is enabled
func (c *ServiceConfig) IsDebugEnabled() bool {
	return c.Debug || c.IsDevelopment()
}

// Default loader instance
var DefaultLoader = NewLoader("")

// Convenience functions using the default loader

// LoadBase loads base configuration using the default loader
func LoadBase() (*BaseConfig, error) {
	return DefaultLoader.LoadBase()
}

// LoadKafka loads Kafka configuration using the default loader
func LoadKafka() KafkaConfig {
	return DefaultLoader.LoadKafka()
}

// LoadGRPC loads gRPC configuration using the default loader
func LoadGRPC() GRPCConfig {
	return DefaultLoader.LoadGRPC()
}

// LoadSecurity loads security configuration using the default loader
func LoadSecurity() SecurityConfig {
	return DefaultLoader.LoadSecurity()
}

// PrintEnvTemplate prints environment variable templates for documentation
func PrintEnvTemplate(serviceName string) {
	fmt.Printf("# Environment Variables for %s\n\n", serviceName)
	
	fmt.Println("# Service Configuration")
	fmt.Printf("export SERVICE_NAME=%s\n", serviceName)
	fmt.Println("export SERVICE_VERSION=1.0.0")
	fmt.Println("export ENVIRONMENT=development")
	fmt.Println("export DEBUG=false")
	fmt.Println()
	
	fmt.Println("# Server Configuration")
	fmt.Println("export SERVER_HOST=0.0.0.0")
	fmt.Println("export SERVER_PORT=8080")
	fmt.Println("export SERVER_READ_TIMEOUT=30s")
	fmt.Println("export SERVER_WRITE_TIMEOUT=30s")
	fmt.Println("export SERVER_IDLE_TIMEOUT=120s")
	fmt.Println()
	
	fmt.Println("# Database Configuration")
	fmt.Println("export DATABASE_TYPE=postgres")
	fmt.Println("export DB_HOST=localhost")
	fmt.Println("export DB_PORT=5432")
	fmt.Println("export DB_USER=postgres")
	fmt.Println("export DB_PASSWORD=password")
	fmt.Printf("export DB_NAME=%s\n", serviceName)
	fmt.Println("export DB_SSL_MODE=disable")
	fmt.Println()
	
	fmt.Println("# Observability Configuration")
	fmt.Println("export LOG_LEVEL=info")
	fmt.Println("export LOG_FORMAT=json")
	fmt.Println("export METRICS_ENABLED=true")
	fmt.Println("export METRICS_PORT=2112")
	fmt.Println("export TRACING_ENABLED=true")
	fmt.Println("export OTEL_ENDPOINT=http://localhost:4317")
	fmt.Println("export TRACING_SAMPLING_RATIO=1.0")
	fmt.Println()
	
	fmt.Println("# Kafka Configuration")
	fmt.Println("export KAFKA_BROKERS=localhost:9092")
	fmt.Printf("export KAFKA_CLIENT_ID=%s-client\n", serviceName)
	fmt.Printf("export KAFKA_CONSUMER_GROUP=%s-group\n", serviceName)
	fmt.Println()
	
	fmt.Println("# Security Configuration")
	fmt.Println("export JWT_SECRET=your-secret-key")
	fmt.Println("export JWT_EXPIRATION=24h")
	fmt.Println("export ENABLE_RATE_LIMIT=true")
	fmt.Println("export RATE_LIMIT_RPM=100")
	fmt.Println("export ENABLE_CORS=true")
	fmt.Println("export CORS_ALLOWED_ORIGINS=*")
}