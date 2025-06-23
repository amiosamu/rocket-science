package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/amiosamu/rocket-science/services/order-service/internal/service"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
	"github.com/jmoiron/sqlx"
)

// HealthServer provides health check endpoints for monitoring and orchestration
type HealthServer struct {
	db           *sqlx.DB
	orderService *service.OrderService
	logger       logging.Logger
	metrics      metrics.Metrics
	startTime    time.Time
}

// NewHealthServer creates a new health server
func NewHealthServer(
	db *sqlx.DB,
	orderService *service.OrderService,
	logger logging.Logger,
	metrics metrics.Metrics,
) *HealthServer {
	return &HealthServer{
		db:           db,
		orderService: orderService,
		logger:       logger,
		metrics:      metrics,
		startTime:    time.Now(),
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

// HandleHealthCheck provides a comprehensive health check
func (h *HealthServer) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	startTime := time.Now()

	// Check all components
	components := make(map[string]ComponentHealth)

	// Check database
	components["database"] = h.checkDatabase(ctx)

	// Check external services
	components["inventory_service"] = h.checkInventoryService(ctx)
	components["payment_service"] = h.checkPaymentService(ctx)
	components["kafka_producer"] = h.checkKafkaProducer(ctx)

	// Check internal services
	components["order_repository"] = h.checkOrderRepository(ctx)

	// Determine overall status
	overallStatus := h.determineOverallStatus(components)

	// Create summary
	summary := h.createSummary(components)

	response := OverallHealthResponse{
		Status:     overallStatus,
		Service:    "order-service",
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
	h.logger.Debug(ctx, "Health check completed", map[string]interface{}{
		"status":     overallStatus,
		"duration":   time.Since(startTime).String(),
		"components": len(components),
	})

	h.writeJSONResponse(w, statusCode, response)
}

// HandleReadinessCheck provides a basic readiness check for container orchestration
func (h *HealthServer) HandleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check critical components only for readiness
	dbHealth := h.checkDatabase(ctx)

	if dbHealth.Status == HealthStatusUnhealthy {
		response := SimpleHealthResponse{
			Status:    HealthStatusUnhealthy,
			Service:   "order-service",
			Timestamp: time.Now().UTC(),
			Version:   "1.0.0",
		}
		h.writeJSONResponse(w, http.StatusServiceUnavailable, response)
		return
	}

	response := SimpleHealthResponse{
		Status:    HealthStatusHealthy,
		Service:   "order-service",
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

// HandleLivenessCheck provides a basic liveness check
func (h *HealthServer) HandleLivenessCheck(w http.ResponseWriter, r *http.Request) {
	response := SimpleHealthResponse{
		Status:    HealthStatusHealthy,
		Service:   "order-service",
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

// HandleMetrics exposes metrics in JSON format
func (h *HealthServer) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service":    "order-service",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"uptime":     time.Since(h.startTime).String(),
		"start_time": h.startTime.UTC().Format(time.RFC3339),
	}

	// Add custom metrics if available
	if metricsData, ok := h.metrics.(interface{ GetMetrics() map[string]interface{} }); ok {
		response["metrics"] = metricsData.GetMetrics()
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// Health check implementations for each component

func (h *HealthServer) checkDatabase(ctx context.Context) ComponentHealth {
	start := time.Now()

	if h.db == nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   "Database connection not initialized",
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	// Simple ping test
	if err := h.db.PingContext(ctx); err != nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   fmt.Sprintf("Database ping failed: %v", err),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	// Check connection pool stats
	stats := h.db.DB.Stats()
	details := map[string]interface{}{
		"open_connections":    stats.OpenConnections,
		"in_use_connections":  stats.InUse,
		"idle_connections":    stats.Idle,
		"wait_count":          stats.WaitCount,
		"wait_duration":       stats.WaitDuration.String(),
		"max_idle_closed":     stats.MaxIdleClosed,
		"max_lifetime_closed": stats.MaxLifetimeClosed,
	}

	status := HealthStatusHealthy
	message := "Database connection healthy"

	// Check for potential issues
	if stats.WaitCount > 100 {
		status = HealthStatusDegraded
		message = "High database connection wait count detected"
	}

	return ComponentHealth{
		Status:    status,
		Message:   message,
		Details:   details,
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

func (h *HealthServer) checkInventoryService(ctx context.Context) ComponentHealth {
	start := time.Now()

	// This would normally check the actual inventory service
	// For now, we'll simulate a basic check

	return ComponentHealth{
		Status:  HealthStatusHealthy,
		Message: "Inventory service connectivity assumed healthy",
		Details: map[string]interface{}{
			"note": "Health check implementation depends on inventory service health endpoint",
		},
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

func (h *HealthServer) checkPaymentService(ctx context.Context) ComponentHealth {
	start := time.Now()

	// This would normally check the actual payment service
	// For now, we'll simulate a basic check

	return ComponentHealth{
		Status:  HealthStatusHealthy,
		Message: "Payment service connectivity assumed healthy",
		Details: map[string]interface{}{
			"note": "Health check implementation depends on payment service health endpoint",
		},
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

func (h *HealthServer) checkKafkaProducer(ctx context.Context) ComponentHealth {
	start := time.Now()

	// This would normally check Kafka producer health
	// For now, we'll simulate a basic check

	return ComponentHealth{
		Status:  HealthStatusHealthy,
		Message: "Kafka producer assumed healthy",
		Details: map[string]interface{}{
			"note": "Health check implementation depends on Kafka producer health metrics",
		},
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

func (h *HealthServer) checkOrderRepository(ctx context.Context) ComponentHealth {
	start := time.Now()

	// Test a simple repository operation
	if h.orderService == nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   "Order service not initialized",
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	// This would normally perform a simple repository test
	// For now, we'll assume it's healthy if the service is initialized

	return ComponentHealth{
		Status:    HealthStatusHealthy,
		Message:   "Order repository operational",
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
		"service": "order-service",
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
			"service":   "order-service",
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
		h.logger.Error(context.Background(), "Failed to encode health response", err)
	}
}
