package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/amiosamu/rocket-science/services/inventory-service/internal/container"
)

const (
	// Service metadata
	serviceName    = "inventory-service"
	serviceVersion = "1.0.0"
	
	// Shutdown timeout
	shutdownTimeout = 30 * time.Second
)

func main() {
	// Create initial logger for bootstrap logging
	bootstrapLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("service", serviceName, "version", serviceVersion)

	bootstrapLogger.Info("🚀 Starting Rocket Science Inventory Service",
		"version", serviceVersion,
		"pid", os.Getpid())

	// Print environment info for debugging
	printEnvironmentInfo(bootstrapLogger)

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Create and initialize the DI container
	c, err := initializeContainer(bootstrapLogger)
	if err != nil {
		bootstrapLogger.Error("Failed to initialize container", "error", err)
		os.Exit(1)
	}

	// Start the application
	if err := startApplication(ctx, c); err != nil {
		c.GetLogger().Error("Failed to start application", "error", err)
		os.Exit(1)
	}

	// Wait for shutdown signal
	waitForShutdown(sigChan, c)
}

// initializeContainer creates and initializes the dependency injection container
func initializeContainer(bootstrapLogger *slog.Logger) (*container.Container, error) {
	bootstrapLogger.Info("Initializing dependency container")

	// Create container
	c := container.NewContainer()

	// Initialize all dependencies
	if err := c.Initialize(); err != nil {
		return nil, fmt.Errorf("container initialization failed: %w", err)
	}

	// Validate configuration
	if err := c.ValidateConfiguration(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Perform health check
	if err := c.HealthCheck(); err != nil {
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	bootstrapLogger.Info("Container initialized successfully")
	return c, nil
}

// startApplication starts the main application services
func startApplication(ctx context.Context, c *container.Container) error {
	logger := c.GetLogger()
	config := c.GetConfig()

	logger.Info("Starting Inventory Service application",
		"port", config.Server.Port,
		"environment", os.Getenv("ENVIRONMENT"),
		"logLevel", config.Observability.LogLevel,
		"databaseName", config.Database.DatabaseName)

	// Print service information
	printServiceInfo(logger, c)

	// Seed test data if in development mode
	if os.Getenv("ENVIRONMENT") == "development" && os.Getenv("SEED_TEST_DATA") == "true" {
		logger.Info("Seeding test data for development")
		if err := c.SeedTestData(ctx); err != nil {
			logger.Warn("Failed to seed test data", "error", err)
		}
	}

	// Start the container (this will start the gRPC server and background jobs)
	if err := c.Start(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	logger.Info("✅ Inventory Service started successfully",
		"status", "ready",
		"grpc_address", fmt.Sprintf(":%s", config.Server.Port),
		"database", config.Database.DatabaseName)

	return nil
}

// waitForShutdown waits for shutdown signals and performs graceful shutdown
func waitForShutdown(sigChan <-chan os.Signal, c *container.Container) {
	logger := c.GetLogger()

	// Wait for signal
	sig := <-sigChan
	logger.Info("🛑 Received shutdown signal", "signal", sig.String())

	// Create shutdown context with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Perform graceful shutdown
	logger.Info("Starting graceful shutdown",
		"timeout", shutdownTimeout.String())

	// Stop the container
	done := make(chan struct{})
	go func() {
		c.Stop()
		close(done)
	}()

	// Wait for shutdown completion or timeout
	select {
	case <-done:
		logger.Info("✅ Graceful shutdown completed successfully")
	case <-shutdownCtx.Done():
		logger.Error("❌ Shutdown timeout exceeded, forcing exit")
	}

	logger.Info("🏁 Inventory Service stopped")
}

// printEnvironmentInfo logs relevant environment information for debugging
func printEnvironmentInfo(logger *slog.Logger) {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		env = "development"
	}

	logger.Info("Environment information",
		"environment", env,
		"go_version", os.Getenv("GO_VERSION"),
		"hostname", getHostname(),
		"working_dir", getWorkingDir())

	// Print key environment variables (without sensitive values)
	envVars := []string{
		"INVENTORY_SERVICE_PORT",
		"MONGODB_CONNECTION_URL",
		"MONGODB_DATABASE_NAME",
		"LOG_LEVEL",
		"METRICS_ENABLED",
		"TRACING_ENABLED",
		"INVENTORY_DEFAULT_STOCK_LEVEL",
		"INVENTORY_LOW_STOCK_THRESHOLD",
		"INVENTORY_MAX_RESERVATION_TIME_MIN",
		"SEED_TEST_DATA",
	}

	for _, envVar := range envVars {
		value := os.Getenv(envVar)
		if value != "" {
			// Mask sensitive values
			if envVar == "MONGODB_CONNECTION_URL" && len(value) > 10 {
				value = value[:10] + "***"
			}
			logger.Debug("Environment variable", "key", envVar, "value", value)
		}
	}
}

// printServiceInfo logs detailed service information
func printServiceInfo(logger *slog.Logger, c *container.Container) {
	serviceInfo := c.GetServiceInfo()
	
	logger.Info("Service information",
		"service_name", serviceInfo["service_name"],
		"service_version", serviceInfo["service_version"],
		"port", serviceInfo["port"],
		"database_name", serviceInfo["database_name"],
		"initialized", serviceInfo["initialized"],
		"started", serviceInfo["started"])

	// Print configuration summary
	config := c.GetConfig()
	logger.Info("Configuration summary",
		"server_port", config.Server.Port,
		"read_timeout", config.Server.ReadTimeout,
		"write_timeout", config.Server.WriteTimeout,
		"mongodb_database", config.Database.DatabaseName,
		"mongodb_max_pool", config.Database.MaxPoolSize,
		"mongodb_min_pool", config.Database.MinPoolSize,
		"default_stock_level", config.Inventory.DefaultStockLevel,
		"low_stock_threshold", config.Inventory.LowStockThreshold,
		"max_reservation_time", config.Inventory.MaxReservationTimeMin,
		"auto_restock_enabled", config.Inventory.AutoRestockEnabled,
		"metrics_enabled", config.Observability.MetricsEnabled,
		"tracing_enabled", config.Observability.TracingEnabled)

	// Print repository statistics if available
	if stats, err := c.GetRepositoryStats(); err == nil {
		logger.Info("Repository statistics", 
			"total_items", stats["total_items"],
			"active_items", stats["active_items"],
			"out_of_stock_items", stats["out_of_stock_items"])
	}
}

// getHostname returns the hostname, defaulting to "unknown" if unavailable
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// getWorkingDir returns the current working directory
func getWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return wd
}

// Development and debugging helpers

// init function for any global setup
func init() {
	// Set timezone to UTC for consistent logging
	os.Setenv("TZ", "UTC")
}

// Health check endpoint for container orchestration
// This could be expanded to include HTTP health endpoints
func healthCheck() error {
	// For now, we rely on the container's health check
	// In production, you might want to expose an HTTP health endpoint
	return nil
}

// Version information
type VersionInfo struct {
	Service   string `json:"service"`
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	BuildTime string `json:"build_time,omitempty"`
	GitCommit string `json:"git_commit,omitempty"`
}

// getVersionInfo returns version information about the service
func getVersionInfo() VersionInfo {
	return VersionInfo{
		Service:   serviceName,
		Version:   serviceVersion,
		GoVersion: os.Getenv("GO_VERSION"),
		BuildTime: os.Getenv("BUILD_TIME"), // Set during Docker build
		GitCommit: os.Getenv("GIT_COMMIT"), // Set during Docker build
	}
}

// Configuration validation for startup
func validateStartupRequirements() error {
	// Check required environment variables
	requiredEnvVars := []string{
		// Add any truly required environment variables here
		// Most have defaults in config, so this is for critical ones only
	}

	for _, envVar := range requiredEnvVars {
		if os.Getenv(envVar) == "" {
			return fmt.Errorf("required environment variable %s is not set", envVar)
		}
	}

	return nil
}

// Recovery function to handle panics gracefully
func handlePanic() {
	if r := recover(); r != nil {
		log.Printf("💥 PANIC: %v", r)
		
		// In production, you might want to:
		// 1. Send panic info to monitoring system
		// 2. Attempt graceful shutdown
		// 3. Restart the service
		
		os.Exit(1)
	}
}

// Example of how to run with panic recovery
func runWithRecovery() {
	defer handlePanic()
	main()
}

// Example environment variable documentation
/*
Environment Variables:

Server Configuration:
- INVENTORY_SERVICE_PORT: gRPC server port (default: 50053)
- INVENTORY_SERVICE_READ_TIMEOUT: Read timeout (default: 30s)
- INVENTORY_SERVICE_WRITE_TIMEOUT: Write timeout (default: 30s)

Database Configuration:
- MONGODB_CONNECTION_URL: MongoDB connection string (default: mongodb://localhost:27017)
- MONGODB_DATABASE_NAME: Database name (default: inventory_db)
- MONGODB_CONNECT_TIMEOUT: Connection timeout (default: 10s)
- MONGODB_QUERY_TIMEOUT: Query timeout (default: 5s)
- MONGODB_MAX_POOL_SIZE: Maximum connection pool size (default: 100)
- MONGODB_MIN_POOL_SIZE: Minimum connection pool size (default: 10)

Inventory Configuration:
- INVENTORY_DEFAULT_STOCK_LEVEL: Default stock level for new items (default: 100)
- INVENTORY_LOW_STOCK_THRESHOLD: Low stock alert threshold (default: 10)
- INVENTORY_MAX_RESERVATION_TIME_MIN: Maximum reservation time in minutes (default: 30)
- INVENTORY_AUTO_RESTOCK_ENABLED: Enable automatic restocking (default: false)

Observability:
- LOG_LEVEL: Logging level - debug, info, warn, error (default: info)
- METRICS_ENABLED: Enable metrics collection (default: true)
- TRACING_ENABLED: Enable distributed tracing (default: true)
- SERVICE_NAME: Service name for observability (default: inventory-service)
- SERVICE_VERSION: Service version (default: 1.0.0)

Development:
- ENVIRONMENT: Environment name - development, staging, production (default: development)
- SEED_TEST_DATA: Seed database with test rocket parts (default: false)
*/