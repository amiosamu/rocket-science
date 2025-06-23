package container

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/amiosamu/rocket-science/services/assembly-service/internal/config"
	assemblyKafka "github.com/amiosamu/rocket-science/services/assembly-service/internal/messaging/kafka"
	"github.com/amiosamu/rocket-science/services/assembly-service/internal/service"
	"github.com/amiosamu/rocket-science/services/assembly-service/internal/transport/http"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// Container holds all the application dependencies
type Container struct {
	// Configuration
	Config *config.Config

	// Infrastructure
	Logger  logging.Logger
	Metrics metrics.Metrics

	// Messaging
	AssemblyConsumer *assemblyKafka.AssemblyConsumer
	AssemblyProducer *assemblyKafka.AssemblyProducer

	// Services
	AssemblyService *service.AssemblyService

	// Transport
	HealthServer *http.HealthServer
}

// NewContainer creates and initializes a new dependency injection container
func NewContainer() (*Container, error) {
	container := &Container{}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	container.Config = cfg

	// Initialize logger
	logger, err := logging.NewServiceLogger(
		cfg.Service.Name,
		cfg.Service.Version,
		cfg.Logging.Level,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	container.Logger = logger

	// Initialize metrics
	metrics, err := metrics.NewMetrics(cfg.Service.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics: %w", err)
	}
	container.Metrics = metrics

	// Initialize assembly producer
	assemblyProducer, err := assemblyKafka.NewAssemblyProducer(
		cfg.Kafka.Producer,
		logger,
		metrics,
		cfg.Kafka.Topics.AssemblyStarted,
		cfg.Kafka.Topics.AssemblyCompleted,
		cfg.Kafka.Topics.AssemblyFailed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create assembly producer: %w", err)
	}
	container.AssemblyProducer = assemblyProducer

	// Initialize assembly service
	assemblyService := service.NewAssemblyService(
		cfg.Assembly,
		assemblyProducer,
		logger,
		metrics,
	)
	container.AssemblyService = assemblyService

	// Initialize assembly consumer
	assemblyConsumer, err := assemblyKafka.NewAssemblyConsumer(
		cfg.Kafka.Consumer,
		assemblyService, // AssemblyService implements PaymentEventHandler interface
		logger,
		metrics,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create assembly consumer: %w", err)
	}
	container.AssemblyConsumer = assemblyConsumer

	// Initialize health server
	structuredLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("service", cfg.Service.Name, "version", cfg.Service.Version)

	healthServer := http.NewHealthServer(structuredLogger, cfg, assemblyService)
	container.HealthServer = healthServer

	logger.Info(nil, "Dependency injection container initialized successfully", map[string]interface{}{
		"service_name":    cfg.Service.Name,
		"service_version": cfg.Service.Version,
		"environment":     cfg.Service.Environment,
		"kafka_brokers":   cfg.Kafka.Consumer.Brokers,
		"kafka_topics":    cfg.Kafka.Consumer.Topics,
	})

	return container, nil
}

// Close gracefully shuts down all container dependencies
func (c *Container) Close() error {
	c.Logger.Info(nil, "Shutting down assembly service container")

	// Close assembly consumer
	if c.AssemblyConsumer != nil {
		if err := c.AssemblyConsumer.Stop(); err != nil {
			c.Logger.Error(nil, "Failed to stop assembly consumer", err, nil)
		}
	}

	// Close assembly producer
	if c.AssemblyProducer != nil {
		if err := c.AssemblyProducer.Close(); err != nil {
			c.Logger.Error(nil, "Failed to close assembly producer", err, nil)
		}
	}

	// Stop health server
	if c.HealthServer != nil {
		if err := c.HealthServer.Stop(); err != nil {
			c.Logger.Error(nil, "Failed to stop health server", err, nil)
		}
	}

	c.Logger.Info(nil, "Assembly service container shutdown complete")
	return nil
}

// HealthCheck performs health checks on all container dependencies
func (c *Container) HealthCheck() map[string]interface{} {
	health := map[string]interface{}{
		"service": map[string]interface{}{
			"name":        c.Config.Service.Name,
			"version":     c.Config.Service.Version,
			"environment": c.Config.Service.Environment,
			"status":      "healthy",
		},
	}

	// Check assembly producer health
	if c.AssemblyProducer != nil {
		if err := c.AssemblyProducer.HealthCheck(nil); err != nil {
			health["assembly_producer"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
		} else {
			health["assembly_producer"] = map[string]interface{}{
				"status": "healthy",
			}
		}
	}

	// Check assembly consumer health
	if c.AssemblyConsumer != nil {
		if err := c.AssemblyConsumer.HealthCheck(nil); err != nil {
			health["assembly_consumer"] = map[string]interface{}{
				"status": "unhealthy",
				"error":  err.Error(),
			}
		} else {
			health["assembly_consumer"] = map[string]interface{}{
				"status": "healthy",
			}
		}
	}

	// Get assembly service stats
	if c.AssemblyService != nil {
		health["assembly_service"] = c.AssemblyService.GetStats(nil)
		health["assembly_service"].(map[string]interface{})["status"] = "healthy"
	}

	return health
}
