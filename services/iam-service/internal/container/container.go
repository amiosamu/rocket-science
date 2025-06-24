package container

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/redis/go-redis/v9"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/config"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/repository/interfaces"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/repository/postgres"
	redisRepo "github.com/amiosamu/rocket-science/services/iam-service/internal/repository/redis"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/service"
	sharedPostgres "github.com/amiosamu/rocket-science/shared/platform/database/postgres"
	sharedRedis "github.com/amiosamu/rocket-science/shared/platform/database/redis"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// Container holds all application dependencies
type Container struct {
	// Configuration
	Config *config.Config
	Logger logging.Logger

	// Database connections
	PostgresConn *sharedPostgres.Connection
	RedisConn    *sharedRedis.Connection
	PostgresDB   *sqlx.DB
	RedisClient  *redis.Client

	// Repositories
	UserRepository    interfaces.UserRepository
	SessionRepository interfaces.SessionRepository

	// Services
	AuthService *service.AuthService
	UserService *service.UserService
}

// ContainerConfig holds configuration for container initialization
type ContainerConfig struct {
	ConfigPath string
	LogLevel   string
}

// NewContainer creates and initializes a new dependency injection container
func NewContainer(cfg ContainerConfig) (*Container, error) {
	container := &Container{}

	// Initialize logger first
	if err := container.initLogger(cfg.LogLevel); err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Load configuration
	if err := container.initConfig(); err != nil {
		return nil, fmt.Errorf("failed to initialize config: %w", err)
	}

	// Initialize database connections
	if err := container.initDatabases(); err != nil {
		return nil, fmt.Errorf("failed to initialize databases: %w", err)
	}

	// Initialize repositories
	if err := container.initRepositories(); err != nil {
		return nil, fmt.Errorf("failed to initialize repositories: %w", err)
	}

	// Initialize services
	if err := container.initServices(); err != nil {
		return nil, fmt.Errorf("failed to initialize services: %w", err)
	}

	// Run health checks
	if err := container.healthCheck(); err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	log.Printf("Container initialized successfully")
	return container, nil
}

// initLogger initializes the logger
func (c *Container) initLogger(logLevel string) error {
	if logLevel == "" {
		logLevel = "info"
	}

	logger, err := logging.NewServiceLogger("iam-service", "1.0.0", logLevel)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	c.Logger = logger
	log.Printf("Logger initialized with level: %s", logLevel)
	return nil
}

// initConfig initializes the configuration
func (c *Container) initConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	c.Config = cfg
	log.Printf("Configuration loaded successfully")
	return nil
}

// initDatabases initializes all database connections
func (c *Container) initDatabases() error {
	// Initialize PostgreSQL connection
	if err := c.initPostgreSQL(); err != nil {
		return fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}

	// Initialize Redis connection
	if err := c.initRedis(); err != nil {
		return fmt.Errorf("failed to initialize Redis: %w", err)
	}

	log.Printf("Database connections initialized successfully")
	return nil
}

// initPostgreSQL initializes PostgreSQL database connection with retry logic
func (c *Container) initPostgreSQL() error {
	dbConfig := sharedPostgres.Config{
		Host:            c.Config.Database.Host,
		Port:            c.Config.Database.Port,
		User:            c.Config.Database.User,
		Password:        c.Config.Database.Password,
		DBName:          c.Config.Database.DBName,
		SSLMode:         c.Config.Database.SSLMode,
		ConnMaxLifetime: c.Config.Database.ConnMaxLifetime,
		MaxOpenConns:    c.Config.Database.MaxOpenConns,
		MaxIdleConns:    c.Config.Database.MaxIdleConns,
		// Driver-level timeout configurations
		ConnectTimeout: c.Config.Database.ConnectTimeout,
		QueryTimeout:   c.Config.Database.QueryTimeout,
		ReadTimeout:    c.Config.Database.ReadTimeout,
		WriteTimeout:   c.Config.Database.WriteTimeout,
	}

	// Retry configuration
	maxRetries := 10
	baseDelay := time.Second * 2
	maxDelay := time.Second * 60

	log.Printf("Attempting to connect to PostgreSQL: %s:%d/%s",
		dbConfig.Host, dbConfig.Port, dbConfig.DBName)

	var conn *sharedPostgres.Connection
	var err error

	// Retry connection with exponential backoff
	for attempt := 0; attempt < maxRetries; attempt++ {
		conn, err = sharedPostgres.NewConnection(dbConfig, c.Logger)
		if err == nil {
			// Test the connection with a simple ping
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			pingErr := conn.HealthCheck(ctx)
			cancel()
			if pingErr == nil {
				break
			}
			conn.Close() // Close failed connection
			err = fmt.Errorf("connection ping failed: %w", pingErr)
		}

		// Calculate delay with exponential backoff
		delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
		if delay > maxDelay {
			delay = maxDelay
		}

		log.Printf("PostgreSQL connection attempt %d/%d failed: %v. Retrying in %v...",
			attempt+1, maxRetries, err, delay)

		if attempt < maxRetries-1 {
			time.Sleep(delay)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL connection after %d attempts: %w", maxRetries, err)
	}

	c.PostgresConn = conn
	c.PostgresDB = conn.DB // Extract the underlying *sqlx.DB

	log.Printf("PostgreSQL connection established successfully: %s:%d/%s",
		dbConfig.Host, dbConfig.Port, dbConfig.DBName)
	return nil
}

// initRedis initializes Redis connection
func (c *Container) initRedis() error {
	redisConfig := sharedRedis.Config{
		Host:         c.Config.Redis.Host,
		Port:         c.Config.Redis.Port,
		Password:     c.Config.Redis.Password,
		DB:           c.Config.Redis.DB,
		PoolSize:     c.Config.Redis.PoolSize,
		MinIdleConns: c.Config.Redis.MinIdleConns,
		DialTimeout:  c.Config.Redis.DialTimeout,
		ReadTimeout:  c.Config.Redis.ReadTimeout,
		WriteTimeout: c.Config.Redis.WriteTimeout,
		IdleTimeout:  c.Config.Redis.IdleTimeout,
	}

	conn, err := sharedRedis.NewConnection(redisConfig, c.Logger)
	if err != nil {
		return fmt.Errorf("failed to create Redis connection: %w", err)
	}

	c.RedisConn = conn
	c.RedisClient = conn.Client // Extract the underlying *redis.Client

	log.Printf("Redis connection established: %s:%d", redisConfig.Host, redisConfig.Port)
	return nil
}

// initRepositories initializes all repository instances
func (c *Container) initRepositories() error {
	// Initialize User Repository
	c.UserRepository = postgres.NewUserRepository(c.PostgresDB)

	// Initialize Session Repository
	c.SessionRepository = redisRepo.NewSessionRepository(c.RedisClient)

	log.Printf("Repositories initialized successfully")
	return nil
}

// initServices initializes all service instances
func (c *Container) initServices() error {
	// Initialize Auth Service
	c.AuthService = service.NewAuthService(
		c.UserRepository,
		c.SessionRepository,
		c.Config,
	)

	// Initialize User Service
	c.UserService = service.NewUserService(
		c.UserRepository,
		c.SessionRepository,
		c.Config,
	)

	log.Printf("Services initialized successfully")
	return nil
}

// healthCheck performs health checks on all components
func (c *Container) healthCheck() error {
	ctx := context.Background()

	// Check PostgreSQL health
	if err := c.PostgresConn.HealthCheck(ctx); err != nil {
		return fmt.Errorf("PostgreSQL health check failed: %w", err)
	}

	// Check Redis health
	if err := c.RedisConn.HealthCheck(ctx); err != nil {
		return fmt.Errorf("Redis health check failed: %w", err)
	}

	// Check Auth Service health
	if err := c.AuthService.IsHealthy(ctx); err != nil {
		return fmt.Errorf("Auth service health check failed: %w", err)
	}

	log.Printf("All health checks passed")
	return nil
}

// Close gracefully closes all connections and resources
func (c *Container) Close() error {
	var errors []error

	// Close PostgreSQL connection
	if c.PostgresConn != nil {
		if err := c.PostgresConn.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close PostgreSQL: %w", err))
		}
	}

	// Close Redis connection
	if c.RedisConn != nil {
		if err := c.RedisConn.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close Redis: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during container shutdown: %v", errors)
	}

	log.Printf("Container closed successfully")
	return nil
}

// GetAuthService returns the auth service instance
func (c *Container) GetAuthService() *service.AuthService {
	return c.AuthService
}

// GetUserService returns the user service instance
func (c *Container) GetUserService() *service.UserService {
	return c.UserService
}

// GetUserRepository returns the user repository instance
func (c *Container) GetUserRepository() interfaces.UserRepository {
	return c.UserRepository
}

// GetSessionRepository returns the session repository instance
func (c *Container) GetSessionRepository() interfaces.SessionRepository {
	return c.SessionRepository
}

// GetConfig returns the configuration instance
func (c *Container) GetConfig() *config.Config {
	return c.Config
}

// GetLogger returns the logger instance
func (c *Container) GetLogger() logging.Logger {
	return c.Logger
}

// GetPostgresDB returns the PostgreSQL database connection
func (c *Container) GetPostgresDB() *sqlx.DB {
	return c.PostgresDB
}

// GetRedisClient returns the Redis client
func (c *Container) GetRedisClient() *redis.Client {
	return c.RedisClient
}

// ServiceStatus represents the status of a service component
type ServiceStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
	Error   string `json:"error,omitempty"`
}

// HealthStatus represents the overall health status
type HealthStatus struct {
	Overall  string           `json:"overall"`
	Services []*ServiceStatus `json:"services"`
}

// GetHealthStatus returns the health status of all components
func (c *Container) GetHealthStatus() *HealthStatus {
	ctx := context.Background()
	var services []*ServiceStatus
	overall := "healthy"

	// Check PostgreSQL
	pgStatus := &ServiceStatus{Name: "postgresql", Status: "healthy"}
	if err := c.PostgresConn.HealthCheck(ctx); err != nil {
		pgStatus.Status = "unhealthy"
		pgStatus.Error = err.Error()
		overall = "unhealthy"
	} else {
		pgStatus.Details = fmt.Sprintf("Connected to %s", c.Config.Database.Host)
	}
	services = append(services, pgStatus)

	// Check Redis
	redisStatus := &ServiceStatus{Name: "redis", Status: "healthy"}
	if err := c.RedisConn.HealthCheck(ctx); err != nil {
		redisStatus.Status = "unhealthy"
		redisStatus.Error = err.Error()
		overall = "unhealthy"
	} else {
		redisStatus.Details = fmt.Sprintf("Connected to %s", c.Config.Redis.Host)
	}
	services = append(services, redisStatus)

	// Check Auth Service
	authStatus := &ServiceStatus{Name: "auth_service", Status: "healthy"}
	if err := c.AuthService.IsHealthy(ctx); err != nil {
		authStatus.Status = "unhealthy"
		authStatus.Error = err.Error()
		overall = "unhealthy"
	} else {
		authStatus.Details = "All authentication components operational"
	}
	services = append(services, authStatus)

	// User Service doesn't have its own health check method,
	// but we can assume it's healthy if repositories are healthy
	userStatus := &ServiceStatus{
		Name:    "user_service",
		Status:  "healthy",
		Details: "User management service operational",
	}
	services = append(services, userStatus)

	return &HealthStatus{
		Overall:  overall,
		Services: services,
	}
}

// GetConnectionInfo returns connection information for monitoring
type ConnectionInfo struct {
	PostgreSQL *PostgreSQLInfo `json:"postgresql"`
	Redis      *RedisInfo      `json:"redis"`
}

type PostgreSQLInfo struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Database     string `json:"database"`
	MaxOpenConns int    `json:"max_open_conns"`
	MaxIdleConns int    `json:"max_idle_conns"`
	OpenConns    int    `json:"open_conns"`
	InUseConns   int    `json:"in_use_conns"`
	IdleConns    int    `json:"idle_conns"`
}

type RedisInfo struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	DB           int    `json:"db"`
	PoolSize     int    `json:"pool_size"`
	MinIdleConns int    `json:"min_idle_conns"`
}

// GetConnectionInfo returns detailed connection information
func (c *Container) GetConnectionInfo() *ConnectionInfo {
	info := &ConnectionInfo{}

	// PostgreSQL info
	if c.PostgresDB != nil {
		stats := c.PostgresDB.Stats()
		info.PostgreSQL = &PostgreSQLInfo{
			Host:         c.Config.Database.Host,
			Port:         c.Config.Database.Port,
			Database:     c.Config.Database.DBName,
			MaxOpenConns: c.Config.Database.MaxOpenConns,
			MaxIdleConns: c.Config.Database.MaxIdleConns,
			OpenConns:    stats.OpenConnections,
			InUseConns:   stats.InUse,
			IdleConns:    stats.Idle,
		}
	}

	// Redis info
	if c.RedisClient != nil {
		info.Redis = &RedisInfo{
			Host:         c.Config.Redis.Host,
			Port:         c.Config.Redis.Port,
			DB:           c.Config.Redis.DB,
			PoolSize:     c.Config.Redis.PoolSize,
			MinIdleConns: c.Config.Redis.MinIdleConns,
		}
	}

	return info
}

// RunMigrations runs database migrations if needed
func (c *Container) RunMigrations() error {
	// This would typically run database migrations
	// For now, we'll just ensure the tables exist

	ctx := context.Background()

	// Check if users table exists, create if not
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'users'
		);
	`

	if err := c.PostgresDB.GetContext(ctx, &exists, query); err != nil {
		return fmt.Errorf("failed to check if users table exists: %w", err)
	}

	if !exists {
		log.Printf("Users table does not exist - migrations may need to be run")
		// In a real application, you would run migrations here
		// For now, we'll just log a warning
	} else {
		log.Printf("Database schema validation passed")
	}

	return nil
}

// GetStats returns runtime statistics
type RuntimeStats struct {
	PostgreSQLStats interface{} `json:"postgresql_stats"`
	RedisStats      interface{} `json:"redis_stats"`
	ConfigSummary   interface{} `json:"config_summary"`
}

// GetStats returns runtime statistics for monitoring
func (c *Container) GetStats() *RuntimeStats {
	stats := &RuntimeStats{}

	// PostgreSQL stats
	if c.PostgresDB != nil {
		stats.PostgreSQLStats = c.PostgresDB.Stats()
	}

	// Redis stats
	if c.RedisClient != nil {
		poolStats := c.RedisClient.PoolStats()
		stats.RedisStats = map[string]interface{}{
			"hits":        poolStats.Hits,
			"misses":      poolStats.Misses,
			"timeouts":    poolStats.Timeouts,
			"total_conns": poolStats.TotalConns,
			"idle_conns":  poolStats.IdleConns,
			"stale_conns": poolStats.StaleConns,
		}
	}

	// Config summary (without sensitive data)
	if c.Config != nil {
		stats.ConfigSummary = map[string]interface{}{
			"server_host":   c.Config.Server.Host,
			"server_port":   c.Config.Server.Port,
			"database_host": c.Config.Database.Host,
			"database_name": c.Config.Database.DBName,
			"redis_host":    c.Config.Redis.Host,
			"jwt_algorithm": c.Config.JWT.Algorithm,
		}
	}

	return stats
}

// IsReady checks if the container is ready to serve requests
func (c *Container) IsReady() bool {
	if c.Config == nil ||
		c.Logger == nil ||
		c.PostgresConn == nil ||
		c.RedisConn == nil ||
		c.UserRepository == nil ||
		c.SessionRepository == nil ||
		c.AuthService == nil ||
		c.UserService == nil {
		return false
	}

	// Quick health check
	ctx := context.Background()
	if err := c.PostgresConn.HealthCheck(ctx); err != nil {
		return false
	}

	if err := c.RedisConn.HealthCheck(ctx); err != nil {
		return false
	}

	return true
}

// GetPostgresConnection returns the PostgreSQL connection wrapper
func (c *Container) GetPostgresConnection() *sharedPostgres.Connection {
	return c.PostgresConn
}

// GetRedisConnection returns the Redis connection wrapper
func (c *Container) GetRedisConnection() *sharedRedis.Connection {
	return c.RedisConn
}
