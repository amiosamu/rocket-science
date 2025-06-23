package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/amiosamu/rocket-science/services/notification-service/internal/config"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/container"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	logger, err := logging.NewServiceLogger(cfg.Service.Name, cfg.Service.Version, cfg.Logging.Level)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// Initialize metrics
	metricsCollector, err := metrics.NewMetrics(cfg.Service.Name)
	if err != nil {
		logger.Error(nil, "Failed to initialize metrics", err, nil)
		os.Exit(1)
	}

	logger.Info(nil, "Starting Notification Service", map[string]interface{}{
		"service":     cfg.Service.Name,
		"version":     cfg.Service.Version,
		"environment": cfg.Service.Environment,
	})

	// Create container with all dependencies
	cont, err := container.NewContainer(*cfg, logger, metricsCollector)
	if err != nil {
		logger.Error(nil, "Failed to create container", err, nil)
		os.Exit(1)
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start health server
	if err := cont.HealthServer.Start(ctx); err != nil {
		logger.Error(ctx, "Failed to start health server", err, nil)
		os.Exit(1)
	}

	// Start Kafka consumer
	if err := cont.KafkaConsumer.Start(ctx); err != nil {
		logger.Error(ctx, "Failed to start Kafka consumer", err, nil)
		os.Exit(1)
	}

	// Record startup metrics
	cont.Metrics.IncrementCounter("notification_service_started", map[string]string{
		"version": cfg.Service.Version,
	})

	logger.Info(ctx, "Notification service started successfully", map[string]interface{}{
		"kafka_topics":    cfg.Kafka.Consumer.Topics,
		"telegram_bot_id": cont.TelegramService.GetBotInfo().ID,
		"iam_host":        cfg.IAMClient.Host,
		"health_port":     "8080",
	})

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigChan
	logger.Info(ctx, "Received shutdown signal", map[string]interface{}{
		"signal": sig.String(),
	})

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Service.GracefulShutdownTimeout)
	defer shutdownCancel()

	// Graceful shutdown
	logger.Info(shutdownCtx, "Shutting down notification service...", nil)

	// Stop the container
	if err := cont.Close(); err != nil {
		logger.Error(shutdownCtx, "Error during shutdown", err, nil)
	}

	// Record shutdown metrics
	cont.Metrics.IncrementCounter("notification_service_stopped", map[string]string{
		"version": cfg.Service.Version,
	})

	logger.Info(shutdownCtx, "Notification service stopped gracefully", nil)
}

// healthCheck performs a simple health check
func healthCheck(ctx context.Context, cont *container.Container) error {
	// Check Kafka consumer health
	if err := cont.KafkaConsumer.HealthCheck(ctx); err != nil {
		return err
	}

	// Check IAM client health
	if err := cont.IAMClient.HealthCheck(ctx); err != nil {
		return err
	}

	// Check Telegram service health
	botInfo := cont.TelegramService.GetBotInfo()
	if botInfo == nil {
		return fmt.Errorf("telegram service unhealthy")
	}

	return nil
}
