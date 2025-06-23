package grpc

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/config"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/container"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/transport/grpc/handlers"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/transport/grpc/interceptors"
	pb "github.com/amiosamu/rocket-science/services/iam-service/proto/iam"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// Server represents the gRPC server
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	container  *container.Container
	logger     logging.Logger
	config     *config.Config
}

// NewServer creates a new gRPC server
func NewServer(container *container.Container) (*Server, error) {
	cfg := container.GetConfig()
	logger := container.GetLogger()

	// Create listener
	address := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	// Create interceptors
	authInterceptor := interceptors.NewAuthInterceptor(container.GetAuthService(), logger)
	loggingInterceptor := interceptors.NewLoggingInterceptor(logger)
	recoveryInterceptor := interceptors.NewRecoveryInterceptor(logger)

	// Configure server options
	serverOpts := []grpc.ServerOption{
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
		grpc.MaxRecvMsgSize(4 * 1024 * 1024), // 4MB
		grpc.MaxSendMsgSize(4 * 1024 * 1024), // 4MB
		grpc.ChainUnaryInterceptor(
			recoveryInterceptor.UnaryServerInterceptor(),
			loggingInterceptor.UnaryServerInterceptor(),
			authInterceptor.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			recoveryInterceptor.StreamServerInterceptor(),
			loggingInterceptor.StreamServerInterceptor(),
			authInterceptor.StreamServerInterceptor(),
		),
	}

	// Create gRPC server
	grpcServer := grpc.NewServer(serverOpts...)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	// Register IAM service
	iamHandler := handlers.NewIAMHandler(
		container.GetAuthService(),
		container.GetUserService(),
	)
	pb.RegisterIAMServiceServer(grpcServer, iamHandler)

	// Enable reflection for development
	if cfg.Observability.LogLevel == "debug" {
		reflection.Register(grpcServer)
		logger.Info(context.Background(), "gRPC reflection enabled for debugging")
	}

	// Set health status
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("iam-service", grpc_health_v1.HealthCheckResponse_SERVING)

	server := &Server{
		grpcServer: grpcServer,
		listener:   listener,
		container:  container,
		logger:     logger,
		config:     cfg,
	}

	logger.Info(context.Background(), "gRPC server created", map[string]interface{}{
		"address": address,
		"config":  "loaded",
	})

	return server, nil
}

// Start starts the gRPC server
func (s *Server) Start() error {
	s.logger.Info(context.Background(), "Starting gRPC server", map[string]interface{}{
		"address": s.listener.Addr().String(),
	})

	// Check if container is ready
	if !s.container.IsReady() {
		return fmt.Errorf("container is not ready to serve requests")
	}

	// Start server
	if err := s.grpcServer.Serve(s.listener); err != nil {
		return fmt.Errorf("failed to serve gRPC server: %w", err)
	}

	return nil
}

// Stop gracefully stops the gRPC server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info(ctx, "Stopping gRPC server")

	// Create a channel to signal when graceful stop is complete
	stopped := make(chan struct{})

	go func() {
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	// Wait for graceful stop or context timeout
	select {
	case <-stopped:
		s.logger.Info(ctx, "gRPC server stopped gracefully")
		return nil
	case <-ctx.Done():
		s.logger.Warn(ctx, "Graceful stop timeout, forcing stop")
		s.grpcServer.Stop()
		return ctx.Err()
	}
}

// GetAddress returns the server address
func (s *Server) GetAddress() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
}

// HealthCheck performs a health check
func (s *Server) HealthCheck(ctx context.Context) error {
	// Check container health
	if !s.container.IsReady() {
		return fmt.Errorf("container is not ready")
	}

	// Check if server is listening
	if s.listener == nil {
		return fmt.Errorf("server is not listening")
	}

	return nil
}

// GetStats returns server statistics
func (s *Server) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"address":   s.GetAddress(),
		"serving":   s.listener != nil,
		"container": s.container.IsReady(),
	}

	// Add container stats
	if s.container.IsReady() {
		stats["container_stats"] = s.container.GetStats()
		stats["health_status"] = s.container.GetHealthStatus()
		stats["connection_info"] = s.container.GetConnectionInfo()
	}

	return stats
}

// ServerInfo holds server information
type ServerInfo struct {
	Address        string                  `json:"address"`
	Status         string                  `json:"status"`
	ContainerReady bool                    `json:"container_ready"`
	HealthStatus   *container.HealthStatus `json:"health_status,omitempty"`
	Stats          map[string]interface{}  `json:"stats,omitempty"`
}

// GetInfo returns comprehensive server information
func (s *Server) GetInfo() *ServerInfo {
	info := &ServerInfo{
		Address:        s.GetAddress(),
		Status:         "unknown",
		ContainerReady: s.container.IsReady(),
	}

	if s.listener != nil {
		info.Status = "listening"
	}

	if s.container.IsReady() {
		info.HealthStatus = s.container.GetHealthStatus()
		info.Stats = s.GetStats()
	}

	return info
}
