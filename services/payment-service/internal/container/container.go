package container

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/amiosamu/rocket-science/services/payment-service/internal/config"
	"github.com/amiosamu/rocket-science/services/payment-service/internal/service"
	grpcTransport "github.com/amiosamu/rocket-science/services/payment-service/internal/transport/grpc"
	httpTransport "github.com/amiosamu/rocket-science/services/payment-service/internal/transport/http"
)

// Container manages all dependencies for the Payment Service
// It follows the Dependency Injection pattern to provide clean separation of concerns
type Container struct {
	// Configuration
	config *config.Config

	// Infrastructure
	logger *slog.Logger

	// Business Services
	paymentService service.PaymentService

	// Transport Layer
	grpcServer   *grpcTransport.Server
	healthServer *httpTransport.HealthServer

	// Lifecycle management
	initialized bool
	started     bool
}

// ContainerOptions allows customization of container initialization
type ContainerOptions struct {
	// ConfigPath allows loading config from a specific file
	ConfigPath string

	// LogLevel overrides the config log level
	LogLevel string

	// Development mode enables additional logging and debugging features
	Development bool

	// CustomLogger allows injecting a pre-configured logger
	CustomLogger *slog.Logger
}

// NewContainer creates a new dependency injection container
func NewContainer() *Container {
	return &Container{
		initialized: false,
		started:     false,
	}
}

// NewContainerWithOptions creates a container with custom options
func NewContainerWithOptions(opts ContainerOptions) *Container {
	container := NewContainer()

	// Apply custom logger if provided
	if opts.CustomLogger != nil {
		container.logger = opts.CustomLogger
	}

	return container
}

// Initialize sets up all dependencies in the correct order
// This method follows the dependency graph to ensure proper initialization
func (c *Container) Initialize() error {
	if c.initialized {
		return fmt.Errorf("container already initialized")
	}

	// Step 1: Load configuration
	if err := c.initializeConfig(); err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}

	// Step 2: Initialize logging
	if err := c.initializeLogger(); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	c.logger.Info("Starting Payment Service initialization",
		"service", c.config.Observability.ServiceName,
		"version", c.config.Observability.ServiceVersion)

	// Step 3: Initialize business services
	if err := c.initializeServices(); err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}

	// Step 4: Initialize transport layer
	if err := c.initializeTransport(); err != nil {
		return fmt.Errorf("failed to initialize transport: %w", err)
	}

	c.initialized = true
	c.logger.Info("Payment Service initialization completed successfully")

	return nil
}

// Start begins the application lifecycle
func (c *Container) Start(ctx context.Context) error {
	if !c.initialized {
		return fmt.Errorf("container must be initialized before starting")
	}

	if c.started {
		return fmt.Errorf("container already started")
	}

	c.logger.Info("Starting Payment Service")

	// Start the health server first
	if err := c.healthServer.Start(); err != nil {
		return fmt.Errorf("failed to start health server: %w", err)
	}

	// Start the gRPC server
	if err := c.grpcServer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	c.started = true
	return nil
}

// Stop gracefully shuts down all services
func (c *Container) Stop() {
	if !c.started {
		return
	}

	c.logger.Info("Stopping Payment Service")

	// Stop gRPC server
	if c.grpcServer != nil {
		c.grpcServer.Stop()
	}

	// Stop health server
	if c.healthServer != nil {
		c.healthServer.Stop()
	}

	c.logger.Info("Payment Service stopped successfully")
	c.started = false
}

// GetConfig provides access to the configuration
func (c *Container) GetConfig() *config.Config {
	return c.config
}

// GetLogger provides access to the logger
func (c *Container) GetLogger() *slog.Logger {
	return c.logger
}

// GetPaymentService provides access to the payment service
func (c *Container) GetPaymentService() service.PaymentService {
	return c.paymentService
}

// GetGRPCServer provides access to the gRPC server
func (c *Container) GetGRPCServer() *grpcTransport.Server {
	return c.grpcServer
}

// HealthCheck performs a health check on all components
func (c *Container) HealthCheck() error {
	if !c.initialized {
		return fmt.Errorf("container not initialized")
	}

	// Check gRPC server health
	if c.grpcServer != nil {
		if err := c.grpcServer.HealthCheck(); err != nil {
			return fmt.Errorf("gRPC server health check failed: %w", err)
		}
	}

	// In a real application, you might check:
	// - Database connectivity
	// - External service availability
	// - Resource limits

	return nil
}

// GetServiceInfo returns information about the running service
func (c *Container) GetServiceInfo() map[string]interface{} {
	info := map[string]interface{}{
		"initialized": c.initialized,
		"started":     c.started,
	}

	if c.config != nil {
		info["service_name"] = c.config.Observability.ServiceName
		info["service_version"] = c.config.Observability.ServiceVersion
		info["port"] = c.config.Server.Port
	}

	if c.grpcServer != nil {
		serverInfo := c.grpcServer.GetServerInfo()
		for k, v := range serverInfo {
			info[k] = v
		}
	}

	return info
}

// Private initialization methods

// initializeConfig loads and validates configuration
func (c *Container) initializeConfig() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	c.config = cfg
	return nil
}

// initializeLogger sets up structured logging
func (c *Container) initializeLogger() error {
	// If a custom logger was provided, use it
	if c.logger != nil {
		return nil
	}

	// Parse log level from config
	var logLevel slog.Level
	switch c.config.Observability.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	// Create logger options
	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug, // Add source file info in debug mode
	}

	// Create structured JSON logger for production, text for development
	var handler slog.Handler
	if os.Getenv("ENVIRONMENT") == "development" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	// Create logger with service context
	c.logger = slog.New(handler).With(
		"service", c.config.Observability.ServiceName,
		"version", c.config.Observability.ServiceVersion,
	)

	return nil
}

// initializeServices creates all business services with their dependencies
func (c *Container) initializeServices() error {
	c.logger.Debug("Initializing business services")

	// Create payment service with dependencies
	// The service factory handles all internal wiring (repository, etc.)
	c.paymentService = service.NewPaymentService(c.config, c.logger)

	c.logger.Debug("Business services initialized successfully")
	return nil
}

// initializeTransport sets up all transport layers (gRPC, HTTP if needed)
func (c *Container) initializeTransport() error {
	c.logger.Debug("Initializing transport layer")

	// Create gRPC server with all dependencies
	c.grpcServer = grpcTransport.NewServer(c.config, c.logger, c.paymentService)

	// Create health server
	c.healthServer = httpTransport.NewHealthServer(c.logger, c.config, c.paymentService)

	c.logger.Debug("Transport layer initialized successfully")
	return nil
}

// Testing utilities

// NewTestContainer creates a container configured for testing
func NewTestContainer() *Container {
	// Create test configuration
	testConfig := &config.Config{
		Server: config.ServerConfig{
			Port: "0", // Use random port for testing
		},
		Payment: config.PaymentConfig{
			ProcessingTimeMs: 10,  // Faster processing for tests
			SuccessRate:      1.0, // Always succeed in tests
			MaxAmount:        10000.0,
		},
		Observability: config.ObservabilityConfig{
			LogLevel:    "debug",
			ServiceName: "payment-service-test",
		},
	}

	// Create test logger
	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	container := &Container{
		config: testConfig,
		logger: testLogger,
	}

	// Initialize services for testing
	container.initializeServices()
	container.initialized = true

	return container
}

// MockContainer creates a container with mock dependencies for unit testing
func MockContainer(paymentService service.PaymentService) *Container {
	testConfig := &config.Config{
		Observability: config.ObservabilityConfig{
			ServiceName: "payment-service-mock",
		},
	}

	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Minimal logging for mocked tests
	}))

	return &Container{
		config:         testConfig,
		logger:         testLogger,
		paymentService: paymentService,
		initialized:    true,
	}
}

// Validation helpers

// ValidateConfiguration checks if the current configuration is valid
func (c *Container) ValidateConfiguration() error {
	if c.config == nil {
		return fmt.Errorf("configuration not loaded")
	}

	// Validate server config
	if c.config.Server.Port == "" {
		return fmt.Errorf("server port must be specified")
	}

	// Validate payment config
	if c.config.Payment.MaxAmount <= 0 {
		return fmt.Errorf("payment max amount must be positive")
	}

	if c.config.Payment.SuccessRate < 0 || c.config.Payment.SuccessRate > 1 {
		return fmt.Errorf("payment success rate must be between 0 and 1")
	}

	// Validate observability config
	if c.config.Observability.ServiceName == "" {
		return fmt.Errorf("service name must be specified")
	}

	return nil
}

// Wire provides a fluent interface for dependency wiring
type Wire struct {
	container *Container
}

// NewWire creates a new wiring builder
func NewWire() *Wire {
	return &Wire{
		container: NewContainer(),
	}
}

// WithConfig sets the configuration
func (w *Wire) WithConfig(cfg *config.Config) *Wire {
	w.container.config = cfg
	return w
}

// WithLogger sets the logger
func (w *Wire) WithLogger(logger *slog.Logger) *Wire {
	w.container.logger = logger
	return w
}

// Build completes the wiring and returns the container
func (w *Wire) Build() (*Container, error) {
	if err := w.container.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize container: %w", err)
	}
	return w.container, nil
}
