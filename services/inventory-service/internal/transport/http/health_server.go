package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/amiosamu/rocket-science/services/inventory-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/inventory-service/internal/service"
)

// HealthServer provides HTTP health check endpoints for monitoring and orchestration
type HealthServer struct {
	inventoryService service.InventoryService
	repository       domain.InventoryRepository
	logger           *slog.Logger
	startTime        time.Time
	port             string
	server           *http.Server
}

// NewHealthServer creates a new health server
func NewHealthServer(
	inventoryService service.InventoryService,
	repository domain.InventoryRepository,
	logger *slog.Logger,
	port string,
) *HealthServer {
	return &HealthServer{
		inventoryService: inventoryService,
		repository:       repository,
		logger:           logger,
		startTime:        time.Now(),
		port:             port,
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

// InventoryStatsResponse for inventory-specific metrics
type InventoryStatsResponse struct {
	Service       string                 `json:"service"`
	Timestamp     time.Time              `json:"timestamp"`
	Uptime        string                 `json:"uptime"`
	TotalItems    int                    `json:"total_items"`
	ActiveItems   int                    `json:"active_items"`
	LowStockItems int                    `json:"low_stock_items"`
	Categories    map[string]int         `json:"items_by_category"`
	Reservations  map[string]interface{} `json:"reservations"`
}

// Start starts the HTTP health server
func (h *HealthServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register health check endpoints
	mux.HandleFunc("/health", h.handleHealthCheck)
	mux.HandleFunc("/ready", h.handleReadinessCheck)
	mux.HandleFunc("/live", h.handleLivenessCheck)
	mux.HandleFunc("/metrics", h.handleMetrics)
	mux.HandleFunc("/stats", h.handleInventoryStats)

	h.server = &http.Server{
		Addr:         ":" + h.port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	h.logger.Info("Starting HTTP health server", "port", h.port)

	// Start server in a goroutine
	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			h.logger.Error("HTTP health server failed", "error", err)
		}
	}()

	return nil
}

// Stop gracefully stops the HTTP health server
func (h *HealthServer) Stop(ctx context.Context) error {
	if h.server == nil {
		return nil
	}

	h.logger.Info("Stopping HTTP health server")

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

	// Check database/repository
	components["database"] = h.checkDatabase(ctx)

	// Check inventory service
	components["inventory_service"] = h.checkInventoryService(ctx)

	// Check repository operations
	components["repository"] = h.checkRepository(ctx)

	// Determine overall status
	overallStatus := h.determineOverallStatus(components)

	// Create summary
	summary := h.createSummary(components)

	response := OverallHealthResponse{
		Status:     overallStatus,
		Service:    "inventory-service",
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

	// Log health check
	h.logger.Debug("Health check completed",
		"status", overallStatus,
		"duration", time.Since(startTime).String(),
		"components", len(components))

	h.writeJSONResponse(w, statusCode, response)
}

// HandleReadinessCheck provides a basic readiness check for container orchestration
func (h *HealthServer) handleReadinessCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check critical components only for readiness
	dbHealth := h.checkDatabase(ctx)

	if dbHealth.Status == HealthStatusUnhealthy {
		response := SimpleHealthResponse{
			Status:    HealthStatusUnhealthy,
			Service:   "inventory-service",
			Timestamp: time.Now().UTC(),
			Version:   "1.0.0",
		}
		h.writeJSONResponse(w, http.StatusServiceUnavailable, response)
		return
	}

	response := SimpleHealthResponse{
		Status:    HealthStatusHealthy,
		Service:   "inventory-service",
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

// HandleLivenessCheck provides a basic liveness check
func (h *HealthServer) handleLivenessCheck(w http.ResponseWriter, r *http.Request) {
	response := SimpleHealthResponse{
		Status:    HealthStatusHealthy,
		Service:   "inventory-service",
		Timestamp: time.Now().UTC(),
		Version:   "1.0.0",
	}
	h.writeJSONResponse(w, http.StatusOK, response)
}

// HandleMetrics exposes basic service metrics
func (h *HealthServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"service":    "inventory-service",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"uptime":     time.Since(h.startTime).String(),
		"start_time": h.startTime.UTC().Format(time.RFC3339),
		"version":    "1.0.0",
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// HandleInventoryStats provides inventory-specific statistics
func (h *HealthServer) handleInventoryStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get basic stats from repository if it supports it
	stats := map[string]interface{}{
		"note": "Detailed inventory statistics available via gRPC interface",
	}

	// Try to get repository stats if available
	if repoWithStats, ok := h.repository.(interface {
		GetStats(context.Context) (map[string]interface{}, error)
	}); ok {
		if repoStats, err := repoWithStats.GetStats(ctx); err == nil {
			stats = repoStats
		}
	}

	response := InventoryStatsResponse{
		Service:       "inventory-service",
		Timestamp:     time.Now().UTC(),
		Uptime:        time.Since(h.startTime).String(),
		TotalItems:    getIntFromStats(stats, "total_items"),
		ActiveItems:   getIntFromStats(stats, "active_items"),
		LowStockItems: getIntFromStats(stats, "low_stock_items"),
		Categories:    getIntMapFromStats(stats, "categories"),
		Reservations:  getMapFromStats(stats, "reservations"),
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// Health check implementations for each component

func (h *HealthServer) checkDatabase(ctx context.Context) ComponentHealth {
	start := time.Now()

	// Check if repository supports health check
	if repoWithHealth, ok := h.repository.(interface{ HealthCheck(context.Context) error }); ok {
		if err := repoWithHealth.HealthCheck(ctx); err != nil {
			return ComponentHealth{
				Status:    HealthStatusUnhealthy,
				Message:   fmt.Sprintf("Database health check failed: %v", err),
				CheckedAt: time.Now().UTC(),
				Duration:  time.Since(start).String(),
			}
		}
	}

	// Test basic repository operation
	_, err := h.repository.FindAvailableItems()
	if err != nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   fmt.Sprintf("Repository operation failed: %v", err),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	return ComponentHealth{
		Status:    HealthStatusHealthy,
		Message:   "Database connection and operations healthy",
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

func (h *HealthServer) checkInventoryService(ctx context.Context) ComponentHealth {
	start := time.Now()

	if h.inventoryService == nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   "Inventory service not initialized",
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	// Test basic service operation - cleanup expired reservations
	_, err := h.inventoryService.CleanupExpiredReservations(ctx)
	if err != nil {
		return ComponentHealth{
			Status:    HealthStatusDegraded,
			Message:   fmt.Sprintf("Service operation warning: %v", err),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	return ComponentHealth{
		Status:    HealthStatusHealthy,
		Message:   "Inventory service operational",
		CheckedAt: time.Now().UTC(),
		Duration:  time.Since(start).String(),
	}
}

func (h *HealthServer) checkRepository(ctx context.Context) ComponentHealth {
	start := time.Now()

	// Test basic repository operations
	if h.repository == nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   "Repository not initialized",
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	// Test search operation (lightweight test)
	_, err := h.repository.Search("")
	if err != nil {
		return ComponentHealth{
			Status:    HealthStatusUnhealthy,
			Message:   fmt.Sprintf("Repository search failed: %v", err),
			CheckedAt: time.Now().UTC(),
			Duration:  time.Since(start).String(),
		}
	}

	return ComponentHealth{
		Status:    HealthStatusHealthy,
		Message:   "Repository operations functional",
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

// Utility functions for stats extraction

func getIntFromStats(stats map[string]interface{}, key string) int {
	if val, ok := stats[key]; ok {
		if intVal, ok := val.(int); ok {
			return intVal
		}
		if int64Val, ok := val.(int64); ok {
			return int(int64Val)
		}
	}
	return 0
}

func getMapFromStats(stats map[string]interface{}, key string) map[string]interface{} {
	if val, ok := stats[key]; ok {
		if mapVal, ok := val.(map[string]interface{}); ok {
			return mapVal
		}
	}
	return make(map[string]interface{})
}

func getIntMapFromStats(stats map[string]interface{}, key string) map[string]int {
	result := make(map[string]int)
	if val, ok := stats[key]; ok {
		if mapVal, ok := val.(map[string]interface{}); ok {
			for k, v := range mapVal {
				if intVal, ok := v.(int); ok {
					result[k] = intVal
				} else if int64Val, ok := v.(int64); ok {
					result[k] = int(int64Val)
				}
			}
		}
	}
	return result
}
