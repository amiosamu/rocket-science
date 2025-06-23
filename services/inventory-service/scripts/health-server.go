package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Standalone Health Server for Inventory Service
// This server provides HTTP health endpoints that check the main gRPC service

const (
	serviceName    = "inventory-service"
	serviceVersion = "1.0.0"
	defaultPort    = "8080"
	grpcPort       = "50053"
)

type HealthServer struct {
	logger    *slog.Logger
	port      string
	startTime time.Time
}

type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

type ComponentHealth struct {
	Status    HealthStatus `json:"status"`
	Message   string       `json:"message,omitempty"`
	Details   interface{}  `json:"details,omitempty"`
	CheckedAt time.Time    `json:"checked_at"`
	Duration  string       `json:"duration"`
}

type OverallHealthResponse struct {
	Status     HealthStatus               `json:"status"`
	Service    string                     `json:"service"`
	Version    string                     `json:"version"`
	Timestamp  time.Time                  `json:"timestamp"`
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentHealth `json:"components"`
	Summary    HealthSummary              `json:"summary"`
}

type HealthSummary struct {
	TotalComponents     int `json:"total_components"`
	HealthyComponents   int `json:"healthy_components"`
	DegradedComponents  int `json:"degraded_components"`
	UnhealthyComponents int `json:"unhealthy_components"`
}

type SimpleHealthResponse struct {
	Status    HealthStatus `json:"status"`
	Service   string       `json:"service"`
	Timestamp time.Time    `json:"timestamp"`
	Version   string       `json:"version"`
}

func main() {
	// Get health server port from environment
	port := os.Getenv("INVENTORY_HEALTH_PORT")
	if port == "" {
		port = defaultPort
	}

	// Create logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("service", "inventory-health-server")

	// Create health server
	server := &HealthServer{
		logger:    logger,
		port:      port,
		startTime: time.Now(),
	}

	logger.Info("Starting Inventory Service Health Server", "port", port)

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealthCheck)
	mux.HandleFunc("/ready", server.handleReadinessCheck)
	mux.HandleFunc("/live", server.handleLivenessCheck)
	mux.HandleFunc("/metrics", server.handleMetrics)
	mux.HandleFunc("/stats", server.handleStats)

	// Start HTTP server
	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Info("Health endpoints available",
		"health", fmt.Sprintf("http://localhost:%s/health", port),
		"ready", fmt.Sprintf("http://localhost:%s/ready", port),
		"live", fmt.Sprintf("http://localhost:%s/live", port))

	if err := httpServer.ListenAndServe(); err != nil {
		logger.Error("Health server failed", "error", err)
		os.Exit(1)
	}
}

func (h *HealthServer) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	startTime := time.Now()

	// Check all components
	components := make(map[string]ComponentHealth)

	// Check gRPC service
	components["grpc_service"] = h.checkGRPCService(ctx)

	// Check MongoDB
	components["mongodb"] = h.checkMongoDB(ctx)

	// Determine overall status
	overallStatus := h.determineOverallStatus(components)

	// Create summary
	summary := h.createSummary(components)

	response := OverallHealthResponse{
		Status:     overallStatus,
		Service:    serviceName,
		Version:    serviceVersion,
		Timestamp:  time.Now().UTC(),
		Uptime:     time.Since(h.startTime).String(),
		Components: components,
		Summary:    summary,
	}

	// Set appropriate HTTP status
	statusCode := http.StatusOK
	if overallStatus == HealthStatusDegraded {
		statusCode = http.StatusOK // Still OK but with warnings
	} else if overallStatus == HealthStatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}

	h.logger.Debug("Health check completed",
		"status", overallStatus,
		"duration", time.Since(startTime).String(),
		"components", len(components))

	h.writeJSONResponse(w, statusCode, response)
}

func (h *HealthServer) handleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check critical components only for readiness
	grpcHealth := h.checkGRPCService(ctx)

	if grpcHealth.Status == HealthStatusUnhealthy {
		response := SimpleHealthResponse{
			Status:    HealthStatusUnhealthy,
			Service:   serviceName,
			Timestamp: time.Now().UTC(),
			Version:   serviceVersion,
		}
		h.writeJSONResponse(w, http.StatusServiceUnavailable, response)
		return
	}

	response := SimpleHealthResponse{
		Status:    HealthStatusHealthy,
		Service:   serviceName,
		Timestamp: time.Now().UTC(),
		Version:   serviceVersion,
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

func (h *HealthServer) handleLivenessCheck(w http.ResponseWriter, r *http.Request) {
	response := SimpleHealthResponse{
		Status:    HealthStatusHealthy,
		Service:   serviceName,
		Timestamp: time.Now().UTC(),
		Version:   serviceVersion,
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

func (h *HealthServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service":    serviceName,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"uptime":     time.Since(h.startTime).String(),
		"start_time": h.startTime.UTC().Format(time.RFC3339),
		"version":    serviceVersion,
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

func (h *HealthServer) handleStats(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service":   serviceName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"uptime":    time.Since(h.startTime).String(),
		"note":      "Detailed inventory statistics available via gRPC interface",
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// Component health checks

func (h *HealthServer) checkGRPCService(ctx context.Context) ComponentHealth {
	start := time.Now()

	// Connect to gRPC service
	grpcAddr := fmt.Sprintf("localhost:%s", getGRPCPort())
	conn, err := grpc.DialContext(ctx, grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second))

	if err != nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   fmt.Sprintf("Failed to connect to gRPC service: %v", err),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}
	defer conn.Close()

	// Check gRPC health
	healthClient := grpc_health_v1.NewHealthClient(conn)
	healthReq := &grpc_health_v1.HealthCheckRequest{
		Service: "",
	}

	healthCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := healthClient.Check(healthCtx, healthReq)
	if err != nil {
		return ComponentHealth{
			Status:    HealthStatusDegraded,
			Message:   fmt.Sprintf("gRPC health check failed: %v", err),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		return ComponentHealth{
			Status:    HealthStatusDegraded,
			Message:   fmt.Sprintf("gRPC service not serving: %v", resp.Status),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	return ComponentHealth{
		Status:    HealthStatusHealthy,
		Message:   "gRPC service is healthy and serving",
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

func (h *HealthServer) checkMongoDB(ctx context.Context) ComponentHealth {
	start := time.Now()

	// Get MongoDB connection URL from environment
	mongoURL := os.Getenv("MONGODB_CONNECTION_URL")
	if mongoURL == "" {
		mongoURL = "mongodb://localhost:27017"
	}

	// Create MongoDB client
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURL))
	if err != nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   fmt.Sprintf("Failed to connect to MongoDB: %v", err),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}
	defer client.Disconnect(ctx)

	// Ping MongoDB
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   fmt.Sprintf("MongoDB ping failed: %v", err),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	return ComponentHealth{
		Status:    HealthStatusHealthy,
		Message:   "MongoDB connection healthy",
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

// Helper methods

func (h *HealthServer) determineOverallStatus(components map[string]ComponentHealth) HealthStatus {
	hasUnhealthy := false
	hasDegraded := false

	for _, component := range components {
		switch component.Status {
		case HealthStatusUnhealthy:
			hasUnhealthy = true
		case HealthStatusDegraded:
			hasDegraded = true
		}
	}

	if hasUnhealthy {
		return HealthStatusUnhealthy
	} else if hasDegraded {
		return HealthStatusDegraded
	}
	return HealthStatusHealthy
}

func (h *HealthServer) createSummary(components map[string]ComponentHealth) HealthSummary {
	summary := HealthSummary{
		TotalComponents: len(components),
	}

	for _, component := range components {
		switch component.Status {
		case HealthStatusHealthy:
			summary.HealthyComponents++
		case HealthStatusDegraded:
			summary.DegradedComponents++
		case HealthStatusUnhealthy:
			summary.UnhealthyComponents++
		}
	}

	return summary
}

func (h *HealthServer) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode health response", "error", err)
	}
}

func getGRPCPort() string {
	port := os.Getenv("INVENTORY_SERVICE_PORT")
	if port == "" {
		port = grpcPort
	}
	return port
}

// Utility function to convert string to int safely
func safeAtoi(s string, defaultVal int) int {
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	return defaultVal
}
