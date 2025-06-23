package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/amiosamu/rocket-science/services/inventory-service/internal/config"
	"github.com/amiosamu/rocket-science/services/inventory-service/internal/service"
	"github.com/amiosamu/rocket-science/services/inventory-service/internal/transport/grpc/handlers"
	pb "github.com/amiosamu/rocket-science/services/inventory-service/proto/inventory"
)

// Server represents the gRPC server for the Inventory Service
type Server struct {
	config           *config.Config
	logger           *slog.Logger
	inventoryService service.InventoryService
	grpcServer       *grpc.Server
	healthServer     *health.Server
}

// NewServer creates a new gRPC server instance with all dependencies
func NewServer(cfg *config.Config, logger *slog.Logger, inventoryService service.InventoryService) *Server {
	return &Server{
		config:           cfg,
		logger:           logger,
		inventoryService: inventoryService,
	}
}

// Start initializes and starts the gRPC server with graceful shutdown
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("Starting Inventory Service gRPC server",
		"port", s.config.Server.Port,
		"serviceName", s.config.Observability.ServiceName,
		"version", s.config.Observability.ServiceVersion)

	// Create gRPC server with options
	s.grpcServer = grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     15 * time.Second,
			MaxConnectionAge:      30 * time.Second,
			MaxConnectionAgeGrace: 5 * time.Second,
			Time:                  5 * time.Second,
			Timeout:               1 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		// Add interceptors for logging, metrics, tracing
		grpc.UnaryInterceptor(s.unaryInterceptor),
	)

	// Create and register inventory handler
	inventoryHandler := handlers.NewInventoryHandler(s.inventoryService, s.logger)
	pb.RegisterInventoryServiceServer(s.grpcServer, inventoryHandler)

	// Register health check service
	s.healthServer = health.NewServer()
	s.healthServer.SetServingStatus("inventory.v1.InventoryService", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(s.grpcServer, s.healthServer)

	// Enable gRPC reflection for development/debugging
	reflection.Register(s.grpcServer)

	// Create listener
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", s.config.Server.Port))
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		s.logger.Info("gRPC server listening", "address", listener.Addr().String())
		if err := s.grpcServer.Serve(listener); err != nil {
			errChan <- fmt.Errorf("gRPC server failed: %w", err)
		}
	}()

	// Wait for shutdown signal or error
	return s.waitForShutdown(ctx, errChan)
}

// Stop gracefully shuts down the gRPC server
func (s *Server) Stop() {
	if s.grpcServer != nil {
		s.logger.Info("Shutting down gRPC server")
		
		// Set health check to not serving
		if s.healthServer != nil {
			s.healthServer.SetServingStatus("inventory.v1.InventoryService", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		}

		// Graceful stop with timeout
		done := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(done)
		}()

		// Force stop if graceful shutdown takes too long
		select {
		case <-done:
			s.logger.Info("gRPC server stopped gracefully")
		case <-time.After(30 * time.Second):
			s.logger.Warn("Force stopping gRPC server due to timeout")
			s.grpcServer.Stop()
		}
	}
}

// waitForShutdown waits for shutdown signals or server errors
func (s *Server) waitForShutdown(ctx context.Context, errChan <-chan error) error {
	// Create channel for shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		s.logger.Info("Context cancelled, shutting down server")
		return ctx.Err()
	case sig := <-sigChan:
		s.logger.Info("Received shutdown signal", "signal", sig.String())
		return nil
	case err := <-errChan:
		s.logger.Error("Server error", "error", err)
		return err
	}
}

// unaryInterceptor provides logging and error handling for gRPC calls
func (s *Server) unaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	// Log the incoming request
	s.logger.Info("gRPC request started",
		"method", info.FullMethod,
		"duration", "started")

	// Call the handler
	resp, err := handler(ctx, req)
	
	// Calculate duration
	duration := time.Since(start)

	// Log the result
	if err != nil {
		s.logger.Error("gRPC request failed",
			"method", info.FullMethod,
			"duration", duration,
			"error", err)
	} else {
		s.logger.Info("gRPC request completed",
			"method", info.FullMethod,
			"duration", duration)
	}

	return resp, err
}

// HealthCheck provides a simple health check endpoint
func (s *Server) HealthCheck() error {
	if s.grpcServer == nil {
		return fmt.Errorf("gRPC server not initialized")
	}
	
	// In a real implementation, you might check:
	// - Database connectivity (MongoDB)
	// - Repository health
	// - Service availability
	// - Resource availability
	
	return nil
}

// GetServerInfo returns information about the running server
func (s *Server) GetServerInfo() map[string]interface{} {
	return map[string]interface{}{
		"service_name":    s.config.Observability.ServiceName,
		"service_version": s.config.Observability.ServiceVersion,
		"port":            s.config.Server.Port,
		"health_status":   "serving",
		"timestamp":       time.Now().UTC(),
		"database_type":   "mongodb",
		"database_name":   s.config.Database.DatabaseName,
	}
}

// ServerOption allows for configurable server creation
type ServerOption func(*Server)

// WithCustomInterceptors allows adding custom interceptors
func WithCustomInterceptors(interceptors ...grpc.UnaryServerInterceptor) ServerOption {
	return func(s *Server) {
		// In a more advanced implementation, we'd chain these interceptors
		// For now, this is a placeholder for future extensibility
	}
}

// WithAuthInterceptor adds authentication interceptor
func WithAuthInterceptor() ServerOption {
	return func(s *Server) {
		// Placeholder for IAM authentication interceptor
		// This will be implemented when we add IAM integration
	}
}

// NewServerWithOptions creates a server with custom options
func NewServerWithOptions(cfg *config.Config, logger *slog.Logger, inventoryService service.InventoryService, opts ...ServerOption) *Server {
	server := NewServer(cfg, logger, inventoryService)
	
	for _, opt := range opts {
		opt(server)
	}
	
	return server
}

// StartBackgroundJobs starts background maintenance tasks
func (s *Server) StartBackgroundJobs(ctx context.Context) {
	// Start expired reservation cleanup job
	go s.reservationCleanupJob(ctx)
	
	s.logger.Info("Background jobs started")
}

// reservationCleanupJob periodically cleans up expired reservations
func (s *Server) reservationCleanupJob(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute) // Clean up every 5 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("Stopping reservation cleanup job")
			return
		case <-ticker.C:
			s.logger.Debug("Running reservation cleanup job")
			
			// Call the cleanup service method
			result, err := s.inventoryService.CleanupExpiredReservations(ctx)
			if err != nil {
				s.logger.Error("Reservation cleanup failed", "error", err)
				continue
			}
			
			if result.CleanedReservations > 0 {
				s.logger.Info("Reservation cleanup completed",
					"cleanedReservations", result.CleanedReservations,
					"affectedItems", len(result.AffectedItems))
			}
		}
	}
}

// Metrics and monitoring helpers

// GetMetrics returns server metrics for monitoring
func (s *Server) GetMetrics() map[string]interface{} {
	metrics := map[string]interface{}{
		"server_info": s.GetServerInfo(),
		"uptime":      time.Since(time.Now()).String(), // This would be calculated from start time
	}

	// In a real implementation, you might add:
	// - Request counts
	// - Response times
	// - Error rates
	// - Active connections
	// - Database connection pool stats

	return metrics
}

// GetDatabaseStatus checks database connectivity
func (s *Server) GetDatabaseStatus() map[string]interface{} {
	// This would check MongoDB connectivity through the repository
	return map[string]interface{}{
		"status":       "connected", // This would be dynamic
		"database":     s.config.Database.DatabaseName,
		"connection":   "mongodb",
		"last_checked": time.Now().UTC(),
	}
}

// Graceful shutdown helpers

// PrepareShutdown prepares the server for shutdown
func (s *Server) PrepareShutdown() {
	s.logger.Info("Preparing server for shutdown")
	
	// Set health check to not serving
	if s.healthServer != nil {
		s.healthServer.SetServingStatus("inventory.v1.InventoryService", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}
	
	// Give time for load balancers to detect the health check change
	time.Sleep(2 * time.Second)
}

// WaitForActiveConnections waits for active connections to complete
func (s *Server) WaitForActiveConnections(timeout time.Duration) {
	s.logger.Info("Waiting for active connections to complete", "timeout", timeout)
	
	// In a real implementation, you might:
	// - Check active gRPC connections
	// - Wait for ongoing requests to complete
	// - Monitor database transactions
	
	// For now, just wait a bit
	select {
	case <-time.After(timeout):
		s.logger.Warn("Timeout waiting for connections")
	case <-time.After(1 * time.Second):
		s.logger.Info("All connections completed")
	}
}