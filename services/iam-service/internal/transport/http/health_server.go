package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/container"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// HealthServer provides HTTP health check endpoints
type HealthServer struct {
	server    *http.Server
	container *container.Container
	logger    logging.Logger
	port      string
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string                 `json:"status"`
	Service   string                 `json:"service"`
	Version   string                 `json:"version"`
	Timestamp time.Time              `json:"timestamp"`
	Checks    map[string]CheckResult `json:"checks"`
	Uptime    string                 `json:"uptime"`
	RequestID string                 `json:"request_id,omitempty"`
}

// CheckResult represents individual health check result
type CheckResult struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Latency string `json:"latency,omitempty"`
}

// ReadinessResponse represents the readiness check response
type ReadinessResponse struct {
	Ready      bool            `json:"ready"`
	Service    string          `json:"service"`
	Timestamp  time.Time       `json:"timestamp"`
	Components map[string]bool `json:"components"`
	Message    string          `json:"message,omitempty"`
}

var startTime = time.Now()

// NewHealthServer creates a new HTTP health server
func NewHealthServer(container *container.Container, port string) *HealthServer {
	if port == "" {
		port = "8080" // Default port for health checks
	}

	hs := &HealthServer{
		container: container,
		logger:    container.GetLogger(),
		port:      port,
	}

	// Create HTTP server
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/health", hs.healthHandler)
	mux.HandleFunc("/ready", hs.readinessHandler)
	mux.HandleFunc("/metrics", hs.metricsHandler)

	// Debug endpoints (for development)
	mux.HandleFunc("/debug/config", hs.configHandler)
	mux.HandleFunc("/debug/stats", hs.statsHandler)

	hs.server = &http.Server{
		Addr:    ":" + port,
		Handler: mux,

		// Timeouts
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return hs
}

// Start starts the health check HTTP server
func (hs *HealthServer) Start(ctx context.Context) error {
	hs.logger.Info(ctx, "Starting HTTP health server", map[string]interface{}{
		"port": hs.port,
	})

	// Start server in goroutine
	go func() {
		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hs.logger.Error(ctx, "Health server failed", err, map[string]interface{}{
				"port": hs.port,
			})
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	return hs.Stop(context.Background())
}

// Stop stops the health check HTTP server
func (hs *HealthServer) Stop(ctx context.Context) error {
	hs.logger.Info(ctx, "Stopping HTTP health server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return hs.server.Shutdown(shutdownCtx)
}

// GetAddress returns the server address
func (hs *HealthServer) GetAddress() string {
	return ":" + hs.port
}

// healthHandler handles /health endpoint
func (hs *HealthServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx := r.Context()

	// Set headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	// Perform health checks
	checks := make(map[string]CheckResult)
	overallStatus := "healthy"

	// Check container health
	containerStart := time.Now()
	containerHealth := hs.container.GetHealthStatus()
	checks["container"] = CheckResult{
		Status:  containerHealth.Overall,
		Message: fmt.Sprintf("Services: %d", len(containerHealth.Services)),
		Latency: time.Since(containerStart).String(),
	}
	if containerHealth.Overall != "healthy" {
		overallStatus = "unhealthy"
	}

	// Add individual service checks from container health status
	for _, service := range containerHealth.Services {
		checks[service.Name] = CheckResult{
			Status:  service.Status,
			Message: service.Details,
			Latency: time.Since(containerStart).String(),
		}
		if service.Status != "healthy" {
			overallStatus = "unhealthy"
		}
	}

	// Create response
	response := HealthResponse{
		Status:    overallStatus,
		Service:   "iam-service",
		Version:   "1.0.0",
		Timestamp: time.Now(),
		Checks:    checks,
		Uptime:    time.Since(startTime).String(),
		RequestID: r.Header.Get("X-Request-ID"),
	}

	// Set status code
	if overallStatus == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	// Write response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		hs.logger.Error(ctx, "Failed to encode health response", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Log request
	hs.logger.Debug(ctx, "Health check completed", map[string]interface{}{
		"status":   overallStatus,
		"duration": time.Since(start).String(),
		"checks":   len(checks),
	})
}

// readinessHandler handles /ready endpoint
func (hs *HealthServer) readinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Set headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	// Check if service is ready
	ready := true
	message := "Service is ready"
	components := make(map[string]bool)

	// Check container readiness
	components["container"] = hs.container.IsReady()
	if !components["container"] {
		ready = false
		message = "Container not ready"
	}

	// Check critical dependencies using container health status
	containerHealth := hs.container.GetHealthStatus()
	components["overall_health"] = containerHealth.Overall == "healthy"
	if !components["overall_health"] {
		ready = false
		message = "One or more components are unhealthy"
	}

	// Check individual services from health status
	for _, service := range containerHealth.Services {
		componentName := service.Name
		isHealthy := service.Status == "healthy"
		components[componentName] = isHealthy

		if !isHealthy {
			ready = false
			if message == "Service is ready" {
				message = fmt.Sprintf("%s is not healthy", service.Name)
			}
		}
	}

	// Create response
	response := ReadinessResponse{
		Ready:      ready,
		Service:    "iam-service",
		Timestamp:  time.Now(),
		Components: components,
		Message:    message,
	}

	// Set status code
	if ready {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	// Write response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		hs.logger.Error(ctx, "Failed to encode readiness response", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	hs.logger.Debug(ctx, "Readiness check completed", map[string]interface{}{
		"ready":      ready,
		"components": len(components),
	})
}

// metricsHandler handles /metrics endpoint (basic metrics)
func (hs *HealthServer) metricsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Set headers
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	// Write Prometheus-style metrics
	metrics := fmt.Sprintf(`# HELP iam_service_info Information about the IAM service
# TYPE iam_service_info gauge
iam_service_info{version="1.0.0",service="iam-service"} 1

# HELP iam_service_uptime_seconds Total uptime of the service in seconds
# TYPE iam_service_uptime_seconds counter
iam_service_uptime_seconds %f

# HELP iam_service_health_status Current health status (1=healthy, 0=unhealthy)
# TYPE iam_service_health_status gauge
iam_service_health_status %f

# HELP iam_service_components_status Status of service components
# TYPE iam_service_components_status gauge
iam_service_components_status %d
`,
		time.Since(startTime).Seconds(),
		func() float64 {
			if hs.container.GetHealthStatus().Overall == "healthy" {
				return 1.0
			}
			return 0.0
		}(),
		len(hs.container.GetHealthStatus().Services),
	)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(metrics))

	hs.logger.Debug(ctx, "Metrics endpoint accessed")
}

// configHandler handles /debug/config endpoint (development only)
func (hs *HealthServer) configHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check if in development mode
	if hs.container.GetConfig().Observability.ServiceName != "iam-service" {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Return sanitized config (remove sensitive data)
	config := map[string]interface{}{
		"service": map[string]interface{}{
			"name":    "iam-service",
			"version": "1.0.0",
		},
		"server": map[string]interface{}{
			"host": hs.container.GetConfig().Server.Host,
			"port": hs.container.GetConfig().Server.Port,
		},
		"observability": hs.container.GetConfig().Observability,
	}

	json.NewEncoder(w).Encode(config)
	hs.logger.Debug(ctx, "Config endpoint accessed")
}

// statsHandler handles /debug/stats endpoint
func (hs *HealthServer) statsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	w.Header().Set("Content-Type", "application/json")

	// Create a response with available statistics
	statsResponse := map[string]interface{}{
		"health":      hs.container.GetHealthStatus(),
		"connections": hs.container.GetConnectionInfo(),
		"uptime":      time.Since(startTime).String(),
		"service": map[string]interface{}{
			"name":    "iam-service",
			"version": "1.0.0",
		},
	}

	json.NewEncoder(w).Encode(statsResponse)

	hs.logger.Debug(ctx, "Stats endpoint accessed")
}
