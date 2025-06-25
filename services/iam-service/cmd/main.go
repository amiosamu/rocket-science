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

	"github.com/amiosamu/rocket-science/services/iam-service/internal/container"
	grpcTransport "github.com/amiosamu/rocket-science/services/iam-service/internal/transport/grpc"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/transport/http"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

const (
	// Service metadata
	serviceName    = "iam-service"
	serviceVersion = "1.0.0"

	// Shutdown timeouts
	gracefulShutdownTimeout = 30 * time.Second
	forceShutdownTimeout    = 5 * time.Second
)

// Application represents the main application
type Application struct {
	container    *container.Container
	grpcServer   *grpcTransport.Server
	healthServer *http.HealthServer
	logger       logging.Logger

	// Lifecycle management
	ctx        context.Context
	cancel     context.CancelFunc
	shutdownWg sync.WaitGroup
	isShutdown bool
	mu         sync.RWMutex
}

// NewApplication creates a new application instance
func NewApplication() (*Application, error) {
	// Create application context
	ctx, cancel := context.WithCancel(context.Background())

	app := &Application{
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize application components
	if err := app.initializeComponents(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize application: %w", err)
	}

	return app, nil
}

// initializeComponents initializes all application components in proper order
func (app *Application) initializeComponents() error {
	log.Printf("Initializing %s v%s", serviceName, serviceVersion)

	// Step 1: Initialize container (DI, config, databases, repositories, services)
	if err := app.initializeContainer(); err != nil {
		return fmt.Errorf("container initialization failed: %w", err)
	}

	// Step 2: Initialize gRPC server
	if err := app.initializeGRPCServer(); err != nil {
		return fmt.Errorf("gRPC server initialization failed: %w", err)
	}

	// Step 3: Initialize HTTP health server
	if err := app.initializeHTTPHealthServer(); err != nil {
		return fmt.Errorf("HTTP health server initialization failed: %w", err)
	}

	// Step 4: Run post-initialization checks
	if err := app.postInitializationChecks(); err != nil {
		return fmt.Errorf("post-initialization checks failed: %w", err)
	}

	log.Printf("Application initialization completed successfully")
	return nil
}

// initializeContainer creates and initializes the dependency injection container
func (app *Application) initializeContainer() error {
	log.Printf("Initializing dependency injection container...")

	// Determine log level from environment
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	// Create container configuration
	containerConfig := container.ContainerConfig{
		LogLevel: logLevel,
	}

	// Initialize container
	c, err := container.NewContainer(containerConfig)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	app.container = c
	app.logger = c.GetLogger()

	// Log container status
	app.logger.Info(app.ctx, "Container initialized successfully", map[string]interface{}{
		"service":         serviceName,
		"version":         serviceVersion,
		"container_ready": c.IsReady(),
	})

	return nil
}

// initializeGRPCServer creates and configures the gRPC server
func (app *Application) initializeGRPCServer() error {
	app.logger.Info(app.ctx, "Initializing gRPC server...")

	// Create gRPC server
	server, err := grpcTransport.NewServer(app.container)
	if err != nil {
		return fmt.Errorf("failed to create gRPC server: %w", err)
	}

	app.grpcServer = server

	app.logger.Info(app.ctx, "gRPC server initialized successfully", map[string]interface{}{
		"address": server.GetAddress(),
	})

	return nil
}

// initializeHTTPHealthServer creates and configures the HTTP health server
func (app *Application) initializeHTTPHealthServer() error {
	app.logger.Info(app.ctx, "Initializing HTTP health server...")

	// Get health port from environment or use default
	healthPort := os.Getenv("IAM_HEALTH_PORT")
	if healthPort == "" {
		healthPort = "8080" // Default health port
	}

	// Create health server
	healthServer := http.NewHealthServer(app.container, healthPort)
	app.healthServer = healthServer

	app.logger.Info(app.ctx, "HTTP health server initialized successfully", map[string]interface{}{
		"address": healthServer.GetAddress(),
	})

	return nil
}

// postInitializationChecks performs validation checks after initialization
func (app *Application) postInitializationChecks() error {
	app.logger.Info(app.ctx, "Running post-initialization checks...")

	// Container readiness check
	if !app.container.IsReady() {
		return fmt.Errorf("container is not ready")
	}

	// Database migrations check
	if err := app.container.RunMigrations(); err != nil {
		app.logger.Warn(app.ctx, "Migration check completed with warnings", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Health check
	healthStatus := app.container.GetHealthStatus()
	if healthStatus.Overall != "healthy" {
		app.logger.Warn(app.ctx, "Some components are not healthy", map[string]interface{}{
			"health_status": healthStatus,
		})
	}

	// Server health check
	if err := app.grpcServer.HealthCheck(app.ctx); err != nil {
		return fmt.Errorf("gRPC server health check failed: %w", err)
	}

	app.logger.Info(app.ctx, "Post-initialization checks completed", map[string]interface{}{
		"container_ready": app.container.IsReady(),
		"health_status":   healthStatus.Overall,
		"server_address":  app.grpcServer.GetAddress(),
	})

	return nil
}

// Start starts the application
func (app *Application) Start() error {
	app.logger.Info(app.ctx, "Starting IAM service", map[string]interface{}{
		"service": serviceName,
		"version": serviceVersion,
		"address": app.grpcServer.GetAddress(),
	})

	// Setup signal handling for graceful shutdown
	app.setupSignalHandling()

	// Start gRPC server in a goroutine
	app.shutdownWg.Add(1)
	go func() {
		defer app.shutdownWg.Done()

		app.logger.Info(app.ctx, "Starting gRPC server", map[string]interface{}{
			"address": app.grpcServer.GetAddress(),
		})

		if err := app.grpcServer.Start(); err != nil {
			app.logger.Error(app.ctx, "gRPC server failed", err, map[string]interface{}{
				"address": app.grpcServer.GetAddress(),
			})

			// Trigger shutdown on server failure
			app.initiateShutdown("server_failure")
		}
	}()

	// Start HTTP health server in a goroutine
	app.shutdownWg.Add(1)
	go func() {
		defer app.shutdownWg.Done()

		app.logger.Info(app.ctx, "Starting HTTP health server", map[string]interface{}{
			"address": app.healthServer.GetAddress(),
		})

		if err := app.healthServer.Start(app.ctx); err != nil {
			app.logger.Error(app.ctx, "HTTP health server failed", err, map[string]interface{}{
				"address": app.healthServer.GetAddress(),
			})

			// Trigger shutdown on server failure
			app.initiateShutdown("health_server_failure")
		}
	}()

	// Log successful startup
	app.logger.Info(app.ctx, "IAM service started successfully", map[string]interface{}{
		"service":        serviceName,
		"version":        serviceVersion,
		"grpc_address":   app.grpcServer.GetAddress(),
		"http_address":   app.healthServer.GetAddress(),
		"container_info": app.container.GetConnectionInfo(),
		"health_status":  app.container.GetHealthStatus(),
	})

	// Wait for shutdown signal
	<-app.ctx.Done()

	app.logger.Info(app.ctx, "Shutdown signal received, initiating graceful shutdown...")
	return app.shutdown()
}

// setupSignalHandling configures signal handlers for graceful shutdown
func (app *Application) setupSignalHandling() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGINT,  // Ctrl+C
		syscall.SIGTERM, // Termination request
		syscall.SIGHUP,  // Hang up
	)

	go func() {
		sig := <-sigChan
		app.logger.Info(app.ctx, "Received shutdown signal", map[string]interface{}{
			"signal": sig.String(),
		})

		app.initiateShutdown(fmt.Sprintf("signal_%s", sig.String()))
	}()
}

// initiateShutdown triggers the shutdown process
func (app *Application) initiateShutdown(reason string) {
	app.mu.Lock()
	defer app.mu.Unlock()

	if app.isShutdown {
		return // Already shutting down
	}

	app.isShutdown = true
	app.logger.Info(app.ctx, "Initiating shutdown", map[string]interface{}{
		"reason": reason,
	})

	app.cancel()
}

// shutdown performs graceful shutdown of all components
func (app *Application) shutdown() error {
	app.logger.Info(app.ctx, "Starting graceful shutdown process...")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), gracefulShutdownTimeout)
	defer shutdownCancel()

	var shutdownErrors []error

	// Step 1: Stop accepting new requests (stop servers)
	if app.grpcServer != nil {
		app.logger.Info(shutdownCtx, "Stopping gRPC server...")

		if err := app.grpcServer.Stop(shutdownCtx); err != nil {
			app.logger.Error(shutdownCtx, "Failed to stop gRPC server gracefully", err)
			shutdownErrors = append(shutdownErrors, fmt.Errorf("gRPC server shutdown failed: %w", err))
		} else {
			app.logger.Info(shutdownCtx, "gRPC server stopped successfully")
		}
	}

	if app.healthServer != nil {
		app.logger.Info(shutdownCtx, "Stopping HTTP health server...")

		if err := app.healthServer.Stop(shutdownCtx); err != nil {
			app.logger.Error(shutdownCtx, "Failed to stop health server gracefully", err)
			shutdownErrors = append(shutdownErrors, fmt.Errorf("health server shutdown failed: %w", err))
		} else {
			app.logger.Info(shutdownCtx, "HTTP health server stopped successfully")
		}
	}

	// Step 2: Wait for ongoing requests to complete
	app.logger.Info(shutdownCtx, "Waiting for ongoing requests to complete...")

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		app.shutdownWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		app.logger.Info(shutdownCtx, "All ongoing requests completed")
	case <-shutdownCtx.Done():
		app.logger.Warn(shutdownCtx, "Timeout waiting for requests to complete, forcing shutdown")
	}

	// Step 3: Close container and database connections
	if app.container != nil {
		app.logger.Info(shutdownCtx, "Closing container and database connections...")

		if err := app.container.Close(); err != nil {
			app.logger.Error(shutdownCtx, "Failed to close container", err)
			shutdownErrors = append(shutdownErrors, fmt.Errorf("container shutdown failed: %w", err))
		} else {
			app.logger.Info(shutdownCtx, "Container closed successfully")
		}
	}

	// Step 4: Final cleanup
	app.logger.Info(shutdownCtx, "Performing final cleanup...")

	// Log shutdown completion
	if len(shutdownErrors) > 0 {
		app.logger.Error(shutdownCtx, "Shutdown completed with errors", fmt.Errorf("shutdown errors: %v", shutdownErrors))
		return fmt.Errorf("shutdown completed with %d errors", len(shutdownErrors))
	}

	app.logger.Info(shutdownCtx, "Graceful shutdown completed successfully")
	return nil
}

// GetStats returns comprehensive application statistics
func (app *Application) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"service":   serviceName,
		"version":   serviceVersion,
		"status":    "running",
		"timestamp": time.Now().UTC(),
	}

	if app.container != nil && app.container.IsReady() {
		stats["container_stats"] = app.container.GetStats()
		stats["health_status"] = app.container.GetHealthStatus()
		stats["connection_info"] = app.container.GetConnectionInfo()
	}

	if app.grpcServer != nil {
		stats["server_stats"] = app.grpcServer.GetStats()
		stats["server_info"] = app.grpcServer.GetInfo()
	}

	return stats
}

// HealthCheck performs a comprehensive health check
func (app *Application) HealthCheck() error {
	if app.container == nil {
		return fmt.Errorf("container not initialized")
	}

	if !app.container.IsReady() {
		return fmt.Errorf("container not ready")
	}

	if app.grpcServer == nil {
		return fmt.Errorf("gRPC server not initialized")
	}

	return app.grpcServer.HealthCheck(app.ctx)
}

// main is the application entry point
func main() {
	log.Printf("Starting %s v%s", serviceName, serviceVersion)

	// Create application
	app, err := NewApplication()
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Handle application lifecycle
	if err := app.Start(); err != nil {
		log.Fatalf("Application failed: %v", err)
	}

	log.Printf("%s shutdown complete", serviceName)
}
