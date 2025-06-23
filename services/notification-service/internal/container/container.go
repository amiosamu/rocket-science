package container

import (
	"context"
	"fmt"

	"github.com/amiosamu/rocket-science/services/notification-service/internal/config"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/messaging/kafka"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/service"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/transport/grpc/clients"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/transport/http"
	kafkaplatform "github.com/amiosamu/rocket-science/shared/platform/messaging/kafka"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// Container holds all service dependencies
type Container struct {
	Config          config.Config
	Logger          logging.Logger
	Metrics         metrics.Metrics
	TelegramService *service.TelegramService
	IAMClient       *clients.IAMClient
	EventConsumer   *kafka.EventConsumer
	KafkaConsumer   *kafkaplatform.Consumer
	HealthServer    *http.HealthServer
}

// NewContainer creates a new container with all dependencies
func NewContainer(cfg config.Config, logger logging.Logger, metrics metrics.Metrics) (*Container, error) {
	// Create Telegram service
	telegramService, err := service.NewTelegramService(cfg.Telegram, logger, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create Telegram service: %w", err)
	}

	// Create IAM client
	iamClient, err := clients.NewIAMClient(cfg.IAMClient, logger, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM client: %w", err)
	}

	// Create event consumer
	eventConsumer := kafka.NewEventConsumer(cfg, logger, metrics, telegramService, iamClient)

	// Create Kafka consumer
	kafkaConsumer, err := kafkaplatform.NewConsumer(cfg.Kafka.Consumer, logger, metrics)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	// Register event consumer as message handler
	kafkaConsumer.RegisterHandler(eventConsumer)

	// Create health server
	healthPort := "8080" // Default health port
	if cfg.Service.HealthPort != 0 {
		healthPort = fmt.Sprintf("%d", cfg.Service.HealthPort)
	}

	healthServer := http.NewHealthServer(
		telegramService,
		iamClient,
		kafkaConsumer,
		logger,
		metrics,
		healthPort,
	)

	logger.Info(nil, "Container created successfully", map[string]interface{}{
		"telegram_bot_id": telegramService.GetBotInfo().ID,
		"kafka_topics":    cfg.Kafka.Consumer.Topics,
		"iam_host":        cfg.IAMClient.Host,
		"health_port":     healthPort,
	})

	return &Container{
		Config:          cfg,
		Logger:          logger,
		Metrics:         metrics,
		TelegramService: telegramService,
		IAMClient:       iamClient,
		EventConsumer:   eventConsumer,
		KafkaConsumer:   kafkaConsumer,
		HealthServer:    healthServer,
	}, nil
}

// Close cleans up all resources
func (c *Container) Close() error {
	ctx := context.Background()

	// Stop health server
	if c.HealthServer != nil {
		if err := c.HealthServer.Stop(ctx); err != nil {
			c.Logger.Error(nil, "Failed to stop health server", err, nil)
		}
	}

	// Stop Kafka consumer
	if err := c.KafkaConsumer.Stop(); err != nil {
		c.Logger.Error(nil, "Failed to stop Kafka consumer", err, nil)
	}

	// Close IAM client
	if err := c.IAMClient.Close(); err != nil {
		c.Logger.Error(nil, "Failed to close IAM client", err, nil)
	}

	// Close Telegram service
	c.TelegramService.Close()

	c.Logger.Info(nil, "Container closed successfully", nil)
	return nil
}
