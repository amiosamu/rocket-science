package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/amiosamu/rocket-science/services/payment-service/internal/config"
	"github.com/amiosamu/rocket-science/services/payment-service/internal/service"
)

// HealthServer provides HTTP health check endpoints for monitoring
type HealthServer struct {
	logger         *slog.Logger
	config         *config.Config
	paymentService service.PaymentService
	server         *http.Server
	startTime      time.Time
}

// HealthResponse represents the structure of health check responses
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
func NewHealthServer(logger *slog.Logger, cfg *config.Config, paymentService service.PaymentService) *HealthServer {
	return &HealthServer{
		logger:         logger.With("component", "health_server"),
		config:         cfg,
		paymentService: paymentService,
		startTime:      time.Now(),
	}
}

// Start starts the HTTP health server
func (h *HealthServer) Start() error {
	port := h.config.Server.HealthPort
	if port == "" {
		port = "8081" // Default health port
	}

	mux := http.NewServeMux()

	// Register health endpoints
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

	// Check if any component is unhealthy
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
		Service:     "payment-service",
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

// readinessHandler checks if the service is ready to serve requests
func (h *HealthServer) readinessHandler(w http.ResponseWriter, r *http.Request) {
	h.setCORSHeaders(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check readiness - all components should be healthy
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
		Service:   "payment-service",
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
		Service:   "payment-service",
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

	metrics := fmt.Sprintf(`# HELP payment_service_uptime_seconds Total uptime of the service in seconds
# TYPE payment_service_uptime_seconds counter
payment_service_uptime_seconds %f

# HELP payment_service_info Information about the payment service
# TYPE payment_service_info gauge
payment_service_info{version="1.0.0",environment="%s"} 1

# HELP payment_service_health_status Health status of the service (1=healthy, 0=unhealthy)
# TYPE payment_service_health_status gauge
payment_service_health_status 1
`,
		uptime,
		h.getEnvironment(),
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

	stats := map[string]interface{}{
		"service":      "payment-service",
		"version":      "1.0.0",
		"uptime":       time.Since(h.startTime).String(),
		"start_time":   h.startTime,
		"current_time": time.Now(),
		"environment":  h.getEnvironment(),
		"configuration": map[string]interface{}{
			"server_port":        h.config.Server.Port,
			"processing_time_ms": h.config.Payment.ProcessingTimeMs,
			"success_rate":       h.config.Payment.SuccessRate,
			"max_amount":         h.config.Payment.MaxAmount,
			"metrics_enabled":    h.config.Observability.MetricsEnabled,
			"tracing_enabled":    h.config.Observability.TracingEnabled,
		},
		"system": map[string]interface{}{
			"hostname": h.getHostname(),
			"pid":      os.Getpid(),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(stats)
}

// checkComponents performs health checks on all service components
func (h *HealthServer) checkComponents() map[string]interface{} {
	components := make(map[string]interface{})

	// Check payment service
	components["payment_service"] = h.checkPaymentService()

	// Check configuration
	components["configuration"] = h.checkConfiguration()

	return components
}

// checkPaymentService checks the health of the payment service
func (h *HealthServer) checkPaymentService() map[string]interface{} {
	status := "healthy"
	message := "Payment service is operational"

	// Test payment service with a simple validation
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try to get payments for a test order (should not fail even if empty)
	_, err := h.paymentService.GetPaymentsByOrderID(ctx, "health-check-test")
	if err != nil {
		status = "unhealthy"
		message = fmt.Sprintf("Payment service error: %v", err)
	}

	return map[string]interface{}{
		"status":     status,
		"message":    message,
		"checked_at": time.Now(),
	}
}

// checkConfiguration validates the service configuration
func (h *HealthServer) checkConfiguration() map[string]interface{} {
	status := "healthy"
	message := "Configuration is valid"

	// Basic configuration validation
	if h.config.Server.Port == "" {
		status = "unhealthy"
		message = "Server port not configured"
	} else if h.config.Payment.SuccessRate < 0 || h.config.Payment.SuccessRate > 1 {
		status = "unhealthy"
		message = "Invalid payment success rate"
	} else if h.config.Payment.MaxAmount <= 0 {
		status = "unhealthy"
		message = "Invalid maximum payment amount"
	}

	return map[string]interface{}{
		"status":     status,
		"message":    message,
		"checked_at": time.Now(),
	}
}

// setCORSHeaders sets CORS headers for browser compatibility
func (h *HealthServer) setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// getEnvironment returns the current environment
func (h *HealthServer) getEnvironment() string {
	env := os.Getenv("ENVIRONMENT")
	if env == "" {
		return "development"
	}
	return env
}

// getHostname returns the system hostname
func (h *HealthServer) getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
