package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/amiosamu/rocket-science/services/order-service/internal/config"
	"github.com/amiosamu/rocket-science/services/order-service/internal/messaging/kafka"
	"github.com/amiosamu/rocket-science/services/order-service/internal/repository/postgres"
	"github.com/amiosamu/rocket-science/services/order-service/internal/repository/postgres/migrations"
	"github.com/amiosamu/rocket-science/services/order-service/internal/service"
	"github.com/amiosamu/rocket-science/services/order-service/internal/transport/grpc/clients"
	"github.com/amiosamu/rocket-science/services/order-service/internal/transport/http"
	"github.com/amiosamu/rocket-science/services/order-service/internal/transport/http/handlers"
	postgresDB "github.com/amiosamu/rocket-science/shared/platform/database/postgres"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
	"github.com/amiosamu/rocket-science/shared/platform/observability/tracing"
)

const (
	serviceName    = "order-service"
	serviceVersion = "1.0.0"
)

func main() {
	// Create root context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize observability
	logger, err := logging.NewServiceLogger(serviceName, serviceVersion, cfg.Observability.LogLevel)
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	logger.Info(ctx, "Starting Order Service", map[string]interface{}{
		"service": serviceName,
		"version": serviceVersion,
		"config":  cfg,
	})

	// Initialize metrics
	logger.Info(ctx, "Initializing metrics...")
	metrics, err := metrics.NewMetrics(serviceName)
	if err != nil {
		logger.Error(ctx, "Failed to create metrics", err)
		os.Exit(1)
	}
	logger.Info(ctx, "Metrics initialized successfully")

	// Initialize tracing
	logger.Info(ctx, "Initializing tracing...")
	var tracer tracing.Tracer
	if cfg.Observability.TracingEnabled {
		tracer, err = tracing.NewTracer(serviceName, serviceVersion, cfg.Observability.OTELEndpoint)
		if err != nil {
			logger.Error(ctx, "Failed to create tracer", err)
			os.Exit(1)
		}
	} else {
		logger.Info(ctx, "Tracing disabled, using no-op tracer")
		tracer = tracing.NewNoOpTracer()
	}
	defer tracer.Close()
	logger.Info(ctx, "Tracing initialized successfully")

	// Initialize database
	logger.Info(ctx, "Connecting to database...")
	dbConfig := postgresDB.Config{
		Host:            cfg.Database.Host,
		Port:            cfg.Database.Port,
		User:            cfg.Database.User,
		Password:        cfg.Database.Password,
		DBName:          cfg.Database.DBName,
		SSLMode:         cfg.Database.SSLMode,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
		ConnectTimeout:  30 * time.Second,
	}

	dbConn, err := postgresDB.NewConnection(dbConfig, logger)
	if err != nil {
		logger.Error(ctx, "Failed to connect to database", err)
		os.Exit(1)
	}
	defer dbConn.Close()
	logger.Info(ctx, "Database connection established")

	// Run database migrations
	logger.Info(ctx, "Running database migrations...")
	migrator := migrations.NewMigrator(dbConn.DB)
	if err := migrator.Up(ctx); err != nil {
		logger.Error(ctx, "Failed to run database migrations", err)
		os.Exit(1)
	}
	logger.Info(ctx, "Database migrations completed")

	// Initialize repository
	logger.Info(ctx, "Initializing repository...")
	orderRepo := postgres.NewOrderRepository(dbConn.DB)
	logger.Info(ctx, "Repository initialized")

	// Initialize external service clients
	logger.Info(ctx, "Initializing external service clients...")
	inventoryClient, err := clients.NewInventoryGRPCClient(
		cfg.GRPC.InventoryService.Address,
		cfg.GRPC.InventoryService.Timeout,
		cfg.GRPC.InventoryService.MaxRetries,
		cfg.GRPC.InventoryService.RetryInterval,
		logger,
	)
	if err != nil {
		logger.Error(ctx, "Failed to create inventory client", err)
		os.Exit(1)
	}
	defer inventoryClient.Close()
	logger.Info(ctx, "Inventory client initialized")

	paymentClient, err := clients.NewPaymentGRPCClient(
		cfg.GRPC.PaymentService.Address,
		cfg.GRPC.PaymentService.Timeout,
		cfg.GRPC.PaymentService.MaxRetries,
		cfg.GRPC.PaymentService.RetryInterval,
		logger,
	)
	if err != nil {
		logger.Error(ctx, "Failed to create payment client", err)
		os.Exit(1)
	}
	defer paymentClient.Close()
	logger.Info(ctx, "Payment client initialized")

	// Initialize Kafka producer
	logger.Info(ctx, "Initializing Kafka producer...")
	kafkaProducer, err := kafka.NewProducer(
		cfg.Kafka.Brokers,
		cfg.Kafka.PaymentEventsTopic,
		cfg.Kafka.ProducerRetries,
		logger,
	)
	if err != nil {
		logger.Error(ctx, "Failed to create Kafka producer", err)
		os.Exit(1)
	}
	defer kafkaProducer.Close()
	logger.Info(ctx, "Kafka producer initialized")

	// Initialize order service
	logger.Info(ctx, "Initializing order service...")
	externalServices := service.ExternalServices{
		InventoryClient: inventoryClient,
		PaymentClient:   paymentClient,
		MessageProducer: kafkaProducer,
	}

	orderService := service.NewOrderService(orderRepo, externalServices, logger, metrics)
	logger.Info(ctx, "Order service initialized")

	// Initialize Kafka consumer for assembly events
	logger.Info(ctx, "Initializing Kafka consumer...")
	kafkaConsumer, err := kafka.NewConsumer(
		cfg.Kafka.Brokers,
		cfg.Kafka.ConsumerGroup,
		[]string{cfg.Kafka.AssemblyEventsTopic},
		orderService,
		logger,
	)
	if err != nil {
		logger.Error(ctx, "Failed to create Kafka consumer", err)
		os.Exit(1)
	}
	defer kafkaConsumer.Close()
	logger.Info(ctx, "Kafka consumer initialized")

	// Initialize HTTP handlers
	logger.Info(ctx, "Initializing HTTP handlers...")
	orderHandler := handlers.NewOrderHandler(orderService, logger)
	logger.Info(ctx, "HTTP handlers initialized")

	// Initialize health server
	logger.Info(ctx, "Initializing health server...")
	healthServer := http.NewHealthServer(dbConn.DB, orderService, logger, metrics)
	logger.Info(ctx, "Health server initialized")

	// Initialize HTTP server
	logger.Info(ctx, "Initializing HTTP server...")
	httpServer := http.NewServer(cfg.Server, orderHandler, healthServer, logger, metrics)
	logger.Info(ctx, "HTTP server initialized")

	// Setup graceful shutdown
	var wg sync.WaitGroup
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	// Start Kafka consumer
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := kafkaConsumer.Start(ctx); err != nil {
			logger.Error(ctx, "Kafka consumer failed", err)
		}
	}()

	// Start HTTP server
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := httpServer.Start(ctx); err != nil {
			logger.Error(ctx, "HTTP server failed", err)
		}
	}()

	logger.Info(ctx, "Order Service started successfully", map[string]interface{}{
		"http_address": fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		"database":     cfg.Database.Host,
		"kafka":        cfg.Kafka.Brokers,
	})

	// Wait for shutdown signal
	<-shutdownCh
	logger.Info(ctx, "Shutdown signal received, stopping service...")

	// Cancel context to stop all components
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop HTTP server
	if err := httpServer.Stop(shutdownCtx); err != nil {
		logger.Error(shutdownCtx, "Failed to stop HTTP server", err)
	}

	// Stop Kafka consumer (handled by context cancellation)
	// Wait for all goroutines to finish
	wg.Wait()

	logger.Info(ctx, "Order Service stopped successfully")
}

// Example environment variables for running the service:
/*
export SERVER_HOST=0.0.0.0
export SERVER_PORT=8080
export DB_HOST=localhost
export DB_PORT=5432
export DB_USER=postgres
export DB_PASSWORD=password
export DB_NAME=orders
export KAFKA_BROKERS=localhost:9092
export KAFKA_PAYMENT_EVENTS_TOPIC=payment-events
export KAFKA_ASSEMBLY_EVENTS_TOPIC=assembly-events
export KAFKA_CONSUMER_GROUP=order-service
export INVENTORY_SERVICE_ADDRESS=localhost:9001
export PAYMENT_SERVICE_ADDRESS=localhost:9002
export LOG_LEVEL=info
export OTEL_ENDPOINT=http://localhost:4317
*/
