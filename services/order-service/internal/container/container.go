package container

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/amiosamu/rocket-science/services/order-service/internal/config"
	"github.com/amiosamu/rocket-science/services/order-service/internal/repository/interfaces"
	"github.com/amiosamu/rocket-science/services/order-service/internal/repository/postgres"
)

// Container holds all dependencies for the order service
type Container struct {
	config *config.Config
	logger *slog.Logger
	db     *sqlx.DB

	// Repositories
	orderRepository interfaces.OrderRepository
}

// New creates a new dependency injection container for the order service
func New(cfg *config.Config, logger *slog.Logger) (*Container, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	c := &Container{
		config: cfg,
		logger: logger,
	}

	if err := c.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize container: %w", err)
	}

	return c, nil
}

// initialize sets up all dependencies
func (c *Container) initialize() error {
	// Initialize database connection
	if err := c.initDatabase(); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Initialize repositories
	if err := c.initRepositories(); err != nil {
		return fmt.Errorf("failed to initialize repositories: %w", err)
	}

	return nil
}

// initDatabase initializes the database connection
func (c *Container) initDatabase() error {
	dsn := c.config.Database.DSN()

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(c.config.Database.MaxOpenConns)
	db.SetMaxIdleConns(c.config.Database.MaxIdleConns)
	db.SetConnMaxLifetime(c.config.Database.ConnMaxLifetime)

	// Test the connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	c.db = db
	c.logger.Info("Database connection established",
		slog.String("host", c.config.Database.Host),
		slog.Int("port", c.config.Database.Port),
		slog.String("database", c.config.Database.DBName))

	return nil
}

// initRepositories initializes all repositories
func (c *Container) initRepositories() error {
	// Initialize order repository
	c.orderRepository = postgres.NewOrderRepository(c.db)
	c.logger.Info("Order repository initialized")

	return nil
}

// GetConfig returns the configuration
func (c *Container) GetConfig() *config.Config {
	return c.config
}

// GetLogger returns the logger
func (c *Container) GetLogger() *slog.Logger {
	return c.logger
}

// GetDB returns the database connection
func (c *Container) GetDB() *sqlx.DB {
	return c.db
}

// GetOrderRepository returns the order repository
func (c *Container) GetOrderRepository() interfaces.OrderRepository {
	return c.orderRepository
}

// Close cleans up all resources
func (c *Container) Close() error {
	c.logger.Info("Shutting down container")

	var errors []error

	// Close database connection
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close database: %w", err))
		} else {
			c.logger.Info("Database connection closed")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during container shutdown: %v", errors)
	}

	c.logger.Info("Container shutdown completed successfully")
	return nil
}

// HealthCheck performs a health check on all dependencies
func (c *Container) HealthCheck(ctx context.Context) error {
	// Check database connection
	if c.db != nil {
		if err := c.db.PingContext(ctx); err != nil {
			return fmt.Errorf("database health check failed: %w", err)
		}
	}

	return nil
}
