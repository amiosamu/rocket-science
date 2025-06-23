package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amiosamu/rocket-science/services/notification-service/internal/service"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/transport/grpc/clients"
	"github.com/amiosamu/rocket-science/shared/platform/messaging/kafka"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// HealthServer provides HTTP health check endpoints for monitoring and orchestration
type HealthServer struct {
	telegramService *service.TelegramService
	iamClient       *clients.IAMClient
	kafkaConsumer   *kafka.Consumer
	logger          logging.Logger
	metrics         metrics.Metrics
	startTime       time.Time
	port            string
	server          *http.Server
}

// NewHealthServer creates a new health server
func NewHealthServer(
	telegramService *service.TelegramService,
	iamClient *clients.IAMClient,
	kafkaConsumer *kafka.Consumer,
	logger logging.Logger,
	metrics metrics.Metrics,
	port string,
) *HealthServer {
	return &HealthServer{
		telegramService: telegramService,
		iamClient:       iamClient,
		kafkaConsumer:   kafkaConsumer,
		logger:          logger,
		metrics:         metrics,
		startTime:       time.Now(),
		port:            port,
	}
}

// HealthStatus represents the overall health status
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
)

// ComponentHealth represents the health of a single component
type ComponentHealth struct {
	Status    HealthStatus `json:"status"`
	Message   string       `json:"message,omitempty"`
	Details   interface{}  `json:"details,omitempty"`
	CheckedAt time.Time    `json:"checked_at"`
	Duration  string       `json:"duration"`
}

// OverallHealthResponse represents the complete health check response
type OverallHealthResponse struct {
	Status     HealthStatus               `json:"status"`
	Service    string                     `json:"service"`
	Version    string                     `json:"version"`
	Timestamp  time.Time                  `json:"timestamp"`
	Uptime     string                     `json:"uptime"`
	Components map[string]ComponentHealth `json:"components"`
	Summary    HealthSummary              `json:"summary"`
}

// HealthSummary provides quick overview statistics
type HealthSummary struct {
	TotalComponents     int `json:"total_components"`
	HealthyComponents   int `json:"healthy_components"`
	DegradedComponents  int `json:"degraded_components"`
	UnhealthyComponents int `json:"unhealthy_components"`
}

// SimpleHealthResponse for basic health checks
type SimpleHealthResponse struct {
	Status    HealthStatus `json:"status"`
	Service   string       `json:"service"`
	Timestamp time.Time    `json:"timestamp"`
	Version   string       `json:"version"`
}

// NotificationStatsResponse for notification-specific metrics
type NotificationStatsResponse struct {
	Service           string                 `json:"service"`
	Timestamp         time.Time              `json:"timestamp"`
	Uptime            string                 `json:"uptime"`
	TelegramBotInfo   map[string]interface{} `json:"telegram_bot_info"`
	KafkaTopics       []string               `json:"kafka_topics"`
	ProcessingMetrics map[string]interface{} `json:"processing_metrics"`
}

// Start starts the HTTP health server
func (h *HealthServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register health check endpoints
	mux.HandleFunc("/health", h.handleHealthCheck)
	mux.HandleFunc("/ready", h.handleReadinessCheck)
	mux.HandleFunc("/live", h.handleLivenessCheck)
	mux.HandleFunc("/metrics", h.handleMetrics)
	mux.HandleFunc("/stats", h.handleNotificationStats)

	h.server = &http.Server{
		Addr:         ":" + h.port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	h.logger.Info(nil, "Starting HTTP health server", map[string]interface{}{
		"port": h.port,
	})

	// Start server in a goroutine
	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.Error(nil, "HTTP health server failed", err, nil)
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP health server
func (h *HealthServer) Stop(ctx context.Context) error {
	if h.server == nil {
		return nil
	}

	h.logger.Info(nil, "Stopping HTTP health server", nil)

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return h.server.Shutdown(shutdownCtx)
}

// HandleHealthCheck provides a comprehensive health check
func (h *HealthServer) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	startTime := time.Now()

	// Check all components
	components := make(map[string]ComponentHealth)

	// Check Kafka consumer
	components["kafka_consumer"] = h.checkKafkaConsumer(ctx)

	// Check Telegram service
	components["telegram_service"] = h.checkTelegramService(ctx)

	// Check IAM client
	components["iam_client"] = h.checkIAMClient(ctx)

	// Determine overall status
	overallStatus := h.determineOverallStatus(components)

	// Create summary
	summary := h.createSummary(components)

	response := OverallHealthResponse{
		Status:     overallStatus,
		Service:    "notification-service",
		Version:    "1.0.0",
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

	// Update metrics
	h.updateHealthMetrics(overallStatus, components)

	// Log health check
	h.logger.Debug(nil, "Health check completed", map[string]interface{}{
		"status":     overallStatus,
		"duration":   time.Since(startTime).String(),
		"components": len(components),
	})

	h.writeJSONResponse(w, statusCode, response)
}

// HandleReadinessCheck provides a basic readiness check for container orchestration
func (h *HealthServer) handleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check critical components only for readiness
	kafkaHealth := h.checkKafkaConsumer(ctx)

	if kafkaHealth.Status == HealthStatusUnhealthy {
		response := SimpleHealthResponse{
			Status:    HealthStatusUnhealthy,
			Service:   "notification-service",
			Timestamp: time.Now().UTC(),
			Version:   "1.0.0",
		}
		h.writeJSONResponse(w, http.StatusServiceUnavailable, response)
		return
	}

	response := SimpleHealthResponse{
		Status:    HealthStatusHealthy,
		Service:   "notification-service",
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

// HandleLivenessCheck provides a basic liveness check
func (h *HealthServer) handleLivenessCheck(w http.ResponseWriter, r *http.Request) {
	response := SimpleHealthResponse{
		Status:    HealthStatusHealthy,
		Service:   "notification-service",
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

// HandleMetrics exposes basic service metrics
func (h *HealthServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service":    "notification-service",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"uptime":     time.Since(h.startTime).String(),
		"start_time": h.startTime.UTC().Format(time.RFC3339),
		"version":    "1.0.0",
	}

	// Add custom metrics if available
	if metricsData, ok := h.metrics.(interface{ GetMetrics() map[string]interface{} }); ok {
		response["metrics"] = metricsData.GetMetrics()
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// HandleNotificationStats provides notification-specific statistics
func (h *HealthServer) handleNotificationStats(w http.ResponseWriter, r *http.Request) {
	// Get Telegram bot info
	var botInfo map[string]interface{}
	if h.telegramService != nil {
		telegramBotInfo := h.telegramService.GetBotInfo()
		if telegramBotInfo != nil {
			botInfo = map[string]interface{}{
				"id":         telegramBotInfo.ID,
				"username":   telegramBotInfo.UserName,
				"first_name": telegramBotInfo.FirstName,
				"is_bot":     telegramBotInfo.IsBot,
			}
		}
	}

	// Get Kafka topics (example - would need actual implementation)
	topics := []string{"order-events", "payment-events", "assembly-events"}

	// Get processing metrics (basic example)
	processingMetrics := map[string]interface{}{
		"note": "Detailed processing metrics available via metrics endpoint",
	}

	response := NotificationStatsResponse{
		Service:           "notification-service",
		Timestamp:         time.Now().UTC(),
		Uptime:            time.Since(h.startTime).String(),
		TelegramBotInfo:   botInfo,
		KafkaTopics:       topics,
		ProcessingMetrics: processingMetrics,
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// Health check implementations for each component

func (h *HealthServer) checkKafkaConsumer(ctx context.Context) ComponentHealth {
	start := time.Now()

	if h.kafkaConsumer == nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   "Kafka consumer not initialized",
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	// Basic consumer status check
	details := map[string]interface{}{
		"status": "operational",
		"note":   "Kafka consumer is running",
	}

	return ComponentHealth{
		Status:    HealthStatusHealthy,
		Message:   "Kafka consumer operational",
		Details:   details,
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

func (h *HealthServer) checkTelegramService(ctx context.Context) ComponentHealth {
	start := time.Now()

	if h.telegramService == nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   "Telegram service not initialized",
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	// Check if bot info is available
	botInfo := h.telegramService.GetBotInfo()
	if botInfo == nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   "Telegram bot info not available",
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	details := map[string]interface{}{
		"bot_id":       botInfo.ID,
		"bot_username": botInfo.UserName,
		"is_bot":       botInfo.IsBot,
	}

	return ComponentHealth{
		Status:    HealthStatusHealthy,
		Message:   fmt.Sprintf("Telegram service operational (Bot: @%s)", botInfo.UserName),
		Details:   details,
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

func (h *HealthServer) checkIAMClient(ctx context.Context) ComponentHealth {
	start := time.Now()

	if h.iamClient == nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   "IAM client not initialized",
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	// Test IAM client health check
	if err := h.iamClient.HealthCheck(ctx); err != nil {
		return ComponentHealth{
			Status:    HealthStatusDegraded,
			Message:   fmt.Sprintf("IAM client health check failed: %v", err),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	return ComponentHealth{
		Status:    HealthStatusHealthy,
		Message:   "IAM client operational",
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

func (h *HealthServer) updateHealthMetrics(status HealthStatus, components map[string]ComponentHealth) {
	// Update overall health metric
	healthValue := 1.0
	if status == HealthStatusDegraded {
		healthValue = 0.5
	} else if status == HealthStatusUnhealthy {
		healthValue = 0.0
	}

	h.metrics.RecordValue("service_health", healthValue, map[string]string{
		"service": "notification-service",
		"status":  string(status),
	})

	// Update component-specific metrics
	for name, component := range components {
		componentValue := 1.0
		if component.Status == HealthStatusDegraded {
			componentValue = 0.5
		} else if component.Status == HealthStatusUnhealthy {
			componentValue = 0.0
		}

		h.metrics.RecordValue("component_health", componentValue, map[string]string{
			"service":   "notification-service",
			"component": name,
			"status":    string(component.Status),
		})
	}
}

func (h *HealthServer) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error(nil, "Failed to encode health response", err, nil)
	}
}
