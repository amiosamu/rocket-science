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

	"github.com/amiosamu/rocket-science/services/payment-service/internal/container"
)

const (
	// Service metadata
	serviceName    = "payment-service"
	serviceVersion = "1.0.0"
	
	// Shutdown timeout
	shutdownTimeout = 30 * time.Second
)

func main() {
	// Create initial logger for bootstrap logging
	bootstrapLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("service", serviceName, "version", serviceVersion)

	bootstrapLogger.Info("🚀 Starting Rocket Science Payment Service",
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

	logger.Info("Starting Payment Service application",
		"port", config.Server.Port,
		"environment", os.Getenv("ENVIRONMENT"),
		"logLevel", config.Observability.LogLevel)

	// Print service information
	printServiceInfo(logger, c)

	// Start the container (this will start the gRPC server)
	if err := c.Start(ctx); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	logger.Info("✅ Payment Service started successfully",
		"status", "ready",
		"grpc_address", fmt.Sprintf(":%s", config.Server.Port))

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

	logger.Info("🏁 Payment Service stopped")
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
		"PAYMENT_SERVICE_PORT",
		"LOG_LEVEL",
		"METRICS_ENABLED",
		"TRACING_ENABLED",
		"PAYMENT_SUCCESS_RATE",
		"PAYMENT_MAX_AMOUNT",
	}

	for _, envVar := range envVars {
		value := os.Getenv(envVar)
		if value != "" {
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
		"initialized", serviceInfo["initialized"],
		"started", serviceInfo["started"])

	// Print configuration summary
	config := c.GetConfig()
	logger.Info("Configuration summary",
		"server_port", config.Server.Port,
		"read_timeout", config.Server.ReadTimeout,
		"write_timeout", config.Server.WriteTimeout,
		"processing_time_ms", config.Payment.ProcessingTimeMs,
		"success_rate", config.Payment.SuccessRate,
		"max_amount", config.Payment.MaxAmount,
		"metrics_enabled", config.Observability.MetricsEnabled,
		"tracing_enabled", config.Observability.TracingEnabled)
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
- PAYMENT_SERVICE_PORT: gRPC server port (default: 50052)
- PAYMENT_SERVICE_READ_TIMEOUT: Read timeout (default: 30s)
- PAYMENT_SERVICE_WRITE_TIMEOUT: Write timeout (default: 30s)

Payment Configuration:
- PAYMENT_PROCESSING_TIME_MS: Simulated processing time in milliseconds (default: 500)
- PAYMENT_SUCCESS_RATE: Success rate from 0.0 to 1.0 (default: 0.95)
- PAYMENT_MAX_AMOUNT: Maximum payment amount (default: 1000000.0)

Observability:
- LOG_LEVEL: Logging level - debug, info, warn, error (default: info)
- METRICS_ENABLED: Enable metrics collection (default: true)
- TRACING_ENABLED: Enable distributed tracing (default: true)
- SERVICE_NAME: Service name for observability (default: payment-service)
- SERVICE_VERSION: Service version (default: 1.0.0)

Development:
- ENVIRONMENT: Environment name - development, staging, production (default: development)
*/