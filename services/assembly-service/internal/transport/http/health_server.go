package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/amiosamu/rocket-science/services/assembly-service/internal/config"
	"github.com/amiosamu/rocket-science/services/assembly-service/internal/service"
)

// HealthServer provides HTTP health check endpoints
type HealthServer struct {
	logger          *slog.Logger
	config          *config.Config
	assemblyService *service.AssemblyService
	server          *http.Server
	startTime       time.Time
}

// HealthResponse represents health check response structure
type HealthResponse struct {
	Service     string                 `json:"service"`
	Status      string                 `json:"status"`
	Timestamp   time.Time              `json:"timestamp"`
	Uptime      string                 `json:"uptime"`
	Version     string                 `json:"version"`
	Environment string                 `json:"environment"`
	Components  map[string]interface{} `json:"components,omitempty"`
}

// NewHealthServer creates a new health check server
func NewHealthServer(logger *slog.Logger, cfg *config.Config, assemblyService *service.AssemblyService) *HealthServer {
	return &HealthServer{
		logger:          logger.With("component", "health_server"),
		config:          cfg,
		assemblyService: assemblyService,
		startTime:       time.Now(),
	}
}

// Start starts the HTTP health server
func (h *HealthServer) Start() error {
	port := "8082" // Default health port
	if envPort := os.Getenv("ASSEMBLY_SERVICE_HEALTH_PORT"); envPort != "" {
		port = envPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.healthHandler)
	mux.HandleFunc("/ready", h.readinessHandler)
	mux.HandleFunc("/live", h.livenessHandler)
	mux.HandleFunc("/metrics", h.metricsHandler)
	mux.HandleFunc("/stats", h.statsHandler)

	h.server = &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	h.logger.Info("Starting health server", "port", port)

	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.Error("Health server error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the health server
func (h *HealthServer) Stop() error {
	if h.server == nil {
		return nil
	}
	h.logger.Info("Stopping health server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return h.server.Shutdown(ctx)
}

// healthHandler provides general health information
func (h *HealthServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	h.setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	components := h.checkComponents()
	status := "healthy"
	statusCode := http.StatusOK

	for _, comp := range components {
		if compMap, ok := comp.(map[string]interface{}); ok {
			if compStatus, exists := compMap["status"]; exists && compStatus != "healthy" {
				status = "unhealthy"
				statusCode = http.StatusServiceUnavailable
				break
			}
		}
	}

	response := HealthResponse{
		Service:     "assembly-service",
		Status:      status,
		Timestamp:   time.Now(),
		Uptime:      time.Since(h.startTime).String(),
		Version:     "1.0.0",
		Environment: h.getEnvironment(),
		Components:  components,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// readinessHandler checks if the service is ready
func (h *HealthServer) readinessHandler(w http.ResponseWriter, r *http.Request) {
	h.setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	components := h.checkComponents()
	ready := true
	for _, comp := range components {
		if compMap, ok := comp.(map[string]interface{}); ok {
			if compStatus, exists := compMap["status"]; exists && compStatus != "healthy" {
				ready = false
				break
			}
		}
	}

	status := "ready"
	statusCode := http.StatusOK
	if !ready {
		status = "not ready"
		statusCode = http.StatusServiceUnavailable
	}

	response := HealthResponse{
		Service:   "assembly-service",
		Status:    status,
		Timestamp: time.Now(),
		Uptime:    time.Since(h.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// livenessHandler checks if the service is alive
func (h *HealthServer) livenessHandler(w http.ResponseWriter, r *http.Request) {
	h.setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	response := HealthResponse{
		Service:   "assembly-service",
		Status:    "alive",
		Timestamp: time.Now(),
		Uptime:    time.Since(h.startTime).String(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// metricsHandler provides Prometheus-compatible metrics
func (h *HealthServer) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "OPTIONS" {
		h.setCORSHeaders(w)
		w.WriteHeader(http.StatusOK)
		return
	}

	uptime := time.Since(h.startTime).Seconds()
	stats := h.assemblyService.GetStats(context.Background())
	activeAssemblies := 0
	if val, ok := stats["active_assemblies"].(int); ok {
		activeAssemblies = val
	}

	metrics := fmt.Sprintf(`# HELP assembly_service_uptime_seconds Total uptime of the service in seconds
# TYPE assembly_service_uptime_seconds counter
assembly_service_uptime_seconds %f

# HELP assembly_service_active_assemblies Number of currently active assemblies
# TYPE assembly_service_active_assemblies gauge
assembly_service_active_assemblies %d

# HELP assembly_service_health_status Health status of the service (1=healthy, 0=unhealthy)
# TYPE assembly_service_health_status gauge
assembly_service_health_status 1
`,
		uptime,
		activeAssemblies,
	)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(metrics))
}

// statsHandler provides detailed statistics
func (h *HealthServer) statsHandler(w http.ResponseWriter, r *http.Request) {
	h.setCORSHeaders(w)
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	assemblyStats := h.assemblyService.GetStats(context.Background())
	stats := map[string]interface{}{
		"service":      "assembly-service",
		"version":      "1.0.0",
		"uptime":       time.Since(h.startTime).String(),
		"start_time":   h.startTime,
		"current_time": time.Now(),
		"environment":  h.getEnvironment(),
		"assembly":     assemblyStats,
		"system": map[string]interface{}{
			"hostname": h.getHostname(),
			"pid":      os.Getpid(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(stats)
}

// Helper methods
func (h *HealthServer) checkComponents() map[string]interface{} {
	components := make(map[string]interface{})

	// Check assembly service
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stats := h.assemblyService.GetStats(ctx)
	status := "healthy"
	message := "Assembly service is operational"
	if stats == nil {
		status = "unhealthy"
		message = "Assembly service stats unavailable"
	}

	components["assembly_service"] = map[string]interface{}{
		"status":     status,
		"message":    message,
		"checked_at": time.Now(),
	}

	return components
}

func (h *HealthServer) setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func (h *HealthServer) getEnvironment() string {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		return "development"
	}
	return env
}

func (h *HealthServer) getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
