package container

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/amiosamu/rocket-science/services/inventory-service/internal/config"
	"github.com/amiosamu/rocket-science/services/inventory-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/inventory-service/internal/repository/mongodb"
	"github.com/amiosamu/rocket-science/services/inventory-service/internal/service"
	grpcTransport "github.com/amiosamu/rocket-science/services/inventory-service/internal/transport/grpc"
)

// Container manages all dependencies for the Inventory Service
// It follows the Dependency Injection pattern to provide clean separation of concerns
type Container struct {
	// Configuration
	config *config.Config

	// Infrastructure
	logger *slog.Logger

	// Data layer
	repository domain.InventoryRepository

	// Business Services
	inventoryService service.InventoryService

	// Transport Layer
	grpcServer *grpcTransport.Server

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

	// CustomRepository allows injecting a mock repository for testing
	CustomRepository domain.InventoryRepository
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

	// Apply custom repository if provided (useful for testing)
	if opts.CustomRepository != nil {
		container.repository = opts.CustomRepository
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

	c.logger.Info("Starting Inventory Service initialization",
		"service", c.config.Observability.ServiceName,
		"version", c.config.Observability.ServiceVersion)

	// Step 3: Initialize data layer (MongoDB repository)
	if err := c.initializeRepository(); err != nil {
		return fmt.Errorf("failed to initialize repository: %w", err)
	}

	// Step 4: Initialize business services
	if err := c.initializeServices(); err != nil {
		return fmt.Errorf("failed to initialize services: %w", err)
	}

	// Step 5: Initialize transport layer
	if err := c.initializeTransport(); err != nil {
		return fmt.Errorf("failed to initialize transport: %w", err)
	}

	c.initialized = true
	c.logger.Info("Inventory Service initialization completed successfully")

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

	c.logger.Info("Starting Inventory Service")

	// Start background jobs (reservation cleanup, etc.)
	c.grpcServer.StartBackgroundJobs(ctx)

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

	c.logger.Info("Stopping Inventory Service")

	// Stop gRPC server
	if c.grpcServer != nil {
		c.grpcServer.Stop()
	}

	// Close repository connections
	if c.repository != nil {
		if mongoRepo, ok := c.repository.(*mongodb.MongoInventoryRepository); ok {
			mongoRepo.Close()
		}
	}

	c.logger.Info("Inventory Service stopped successfully")
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

// GetRepository provides access to the repository
func (c *Container) GetRepository() domain.InventoryRepository {
	return c.repository
}

// GetInventoryService provides access to the inventory service
func (c *Container) GetInventoryService() service.InventoryService {
	return c.inventoryService
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

	// Check repository health (MongoDB connectivity)
	if c.repository != nil {
		if mongoRepo, ok := c.repository.(*mongodb.MongoInventoryRepository); ok {
			ctx, cancel := context.WithTimeout(context.Background(), c.config.Database.QueryTimeout)
			defer cancel()

			if err := mongoRepo.HealthCheck(ctx); err != nil {
				return fmt.Errorf("repository health check failed: %w", err)
			}
		}
	}

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
		info["database_name"] = c.config.Database.DatabaseName
		info["database_connection"] = c.config.Database.ConnectionURL
	}

	if c.grpcServer != nil {
		serverInfo := c.grpcServer.GetServerInfo()
		for k, v := range serverInfo {
			info[k] = v
		}
	}

	return info
}

// GetRepositoryStats returns repository statistics
func (c *Container) GetRepositoryStats() (map[string]interface{}, error) {
	if c.repository == nil {
		return nil, fmt.Errorf("repository not initialized")
	}

	if mongoRepo, ok := c.repository.(*mongodb.MongoInventoryRepository); ok {
		ctx, cancel := context.WithTimeout(context.Background(), c.config.Database.QueryTimeout)
		defer cancel()

		return mongoRepo.GetStats(ctx)
	}

	return map[string]interface{}{
		"type": "unknown",
	}, nil
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

// initializeRepository creates the MongoDB repository
func (c *Container) initializeRepository() error {
	// If a custom repository was provided, use it (useful for testing)
	if c.repository != nil {
		c.logger.Debug("Using custom repository")
		return nil
	}

	c.logger.Debug("Initializing MongoDB repository")

	// Create MongoDB repository
	mongoRepo, err := mongodb.NewMongoInventoryRepository(c.config, c.logger)
	if err != nil {
		return fmt.Errorf("failed to create MongoDB repository: %w", err)
	}

	c.repository = mongoRepo

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Database.ConnectTimeout)
	defer cancel()

	if err := mongoRepo.HealthCheck(ctx); err != nil {
		return fmt.Errorf("repository health check failed: %w", err)
	}

	c.logger.Debug("MongoDB repository initialized successfully")
	return nil
}

// initializeServices creates all business services with their dependencies
func (c *Container) initializeServices() error {
	c.logger.Debug("Initializing business services")

	// Create inventory service with dependencies
	c.inventoryService = service.NewInventoryService(c.config, c.logger, c.repository)

	c.logger.Debug("Business services initialized successfully")
	return nil
}

// initializeTransport sets up all transport layers (gRPC)
func (c *Container) initializeTransport() error {
	c.logger.Debug("Initializing transport layer")

	// Create gRPC server with all dependencies
	c.grpcServer = grpcTransport.NewServer(c.config, c.logger, c.inventoryService)

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
		Database: config.DatabaseConfig{
			ConnectionURL:   "mongodb://localhost:27017",
			DatabaseName:    "inventory_test_db",
			ConnectTimeout:  5 * time.Second,
			QueryTimeout:    2 * time.Second,
			MaxPoolSize:     10,
			MinPoolSize:     2,
			MaxConnIdleTime: 1 * time.Minute,
		},
		Inventory: config.InventoryConfig{
			DefaultStockLevel:     50,
			LowStockThreshold:     5,
			MaxReservationTimeMin: 10, // Shorter for tests
			AutoRestockEnabled:    false,
		},
		Observability: config.ObservabilityConfig{
			LogLevel:    "debug",
			ServiceName: "inventory-service-test",
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

	// Initialize services for testing (skip repository - can be mocked)
	container.initializeServices()
	container.initialized = true

	return container
}

// MockContainer creates a container with mock dependencies for unit testing
func MockContainer(mockRepository domain.InventoryRepository) *Container {
	testConfig := &config.Config{
		Observability: config.ObservabilityConfig{
			ServiceName: "inventory-service-mock",
		},
		Inventory: config.InventoryConfig{
			MaxReservationTimeMin: 30,
		},
	}

	testLogger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Minimal logging for mocked tests
	}))

	inventoryService := service.NewInventoryService(testConfig, testLogger, mockRepository)

	return &Container{
		config:           testConfig,
		logger:           testLogger,
		repository:       mockRepository,
		inventoryService: inventoryService,
		initialized:      true,
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

	// Validate database config
	if c.config.Database.ConnectionURL == "" {
		return fmt.Errorf("database connection URL must be specified")
	}
	if c.config.Database.DatabaseName == "" {
		return fmt.Errorf("database name must be specified")
	}

	// Validate inventory config
	if c.config.Inventory.MaxReservationTimeMin <= 0 {
		return fmt.Errorf("max reservation time must be positive")
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

// WithRepository sets the repository
func (w *Wire) WithRepository(repo domain.InventoryRepository) *Wire {
	w.container.repository = repo
	return w
}

// Build completes the wiring and returns the container
func (w *Wire) Build() (*Container, error) {
	if err := w.container.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize container: %w", err)
	}
	return w.container, nil
}

// Utility functions for container management

// SeedTestData seeds the repository with test data (useful for development/testing)
func (c *Container) SeedTestData(ctx context.Context) error {
	if c.repository == nil {
		return fmt.Errorf("repository not initialized")
	}

	c.logger.Info("Seeding test data")

	// Create some sample rocket parts
	testItems := []*domain.InventoryItem{
		// Rocket Engines
		createTestItem("RKT-ENG-001", "Raptor Engine", "High-performance methane-fueled rocket engine", domain.CategoryEngines, 50000.00),
		createTestItem("RKT-ENG-002", "Merlin Engine", "Reliable kerosene-fueled rocket engine", domain.CategoryEngines, 35000.00),

		// Fuel Tanks
		createTestItem("RKT-TANK-500", "Main Fuel Tank", "Large capacity fuel storage tank", domain.CategoryFuelTanks, 15000.00),
		createTestItem("RKT-TANK-100", "Secondary Fuel Tank", "Smaller auxiliary fuel tank", domain.CategoryFuelTanks, 8000.00),

		// Navigation
		createTestItem("RKT-NAV-001", "Flight Computer", "Advanced navigation and flight control system", domain.CategoryNavigation, 25000.00),
		createTestItem("RKT-NAV-002", "GPS Module", "High-precision GPS navigation module", domain.CategoryNavigation, 5000.00),

		// Structural
		createTestItem("RKT-STR-001", "Main Structure", "Primary rocket body structure", domain.CategoryStructural, 20000.00),
		createTestItem("RKT-STR-002", "Nose Cone", "Aerodynamic nose cone assembly", domain.CategoryStructural, 12000.00),
	}

	for _, item := range testItems {
		if err := c.repository.Save(item); err != nil {
			c.logger.Error("Failed to save test item", "sku", item.SKU(), "error", err)
			continue
		}
	}

	c.logger.Info("Test data seeding completed", "itemsCreated", len(testItems))
	return nil
}

// Helper function to create test inventory items
func createTestItem(sku, name, description string, category domain.ItemCategory, price float64) *domain.InventoryItem {
	item, _ := domain.NewInventoryItem(
		sku, name, description, category,
		domain.Money{Amount: price, Currency: "USD"},
	)

	// Add some initial stock
	item.AddStock(100, "Initial stock")

	return item
}
