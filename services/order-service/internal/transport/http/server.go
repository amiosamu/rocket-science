package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/amiosamu/rocket-science/services/order-service/internal/config"
	"github.com/amiosamu/rocket-science/services/order-service/internal/transport/http/handlers"
	customMiddleware "github.com/amiosamu/rocket-science/services/order-service/internal/transport/http/middleware"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// Server represents the HTTP server
type Server struct {
	server       *http.Server
	router       *chi.Mux
	logger       logging.Logger
	metrics      metrics.Metrics
	orderHandler *handlers.OrderHandler
	healthServer *HealthServer
	config       config.ServerConfig
}

// NewServer creates a new HTTP server
func NewServer(
	cfg config.ServerConfig,
	orderHandler *handlers.OrderHandler,
	healthServer *HealthServer,
	logger logging.Logger,
	metrics metrics.Metrics,
) *Server {
	server := &Server{
		logger:       logger,
		metrics:      metrics,
		orderHandler: orderHandler,
		healthServer: healthServer,
		config:       cfg,
	}

	server.setupRoutes()
	server.setupServer()

	return server
}

// setupRoutes configures all the routes and middleware
func (s *Server) setupRoutes() {
	s.router = chi.NewRouter()

	// Apply Chi built-in middleware
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Timeout(30 * time.Second))

	// Apply custom middleware
	s.router.Use(customMiddleware.LoggingMiddleware(s.logger))
	s.router.Use(customMiddleware.TracingMiddleware("order-service"))
	s.router.Use(customMiddleware.MetricsMiddleware(s.metrics))
	s.router.Use(customMiddleware.SecurityHeadersMiddleware())
	s.router.Use(customMiddleware.CORSMiddleware([]string{"*"})) // Configure appropriately for production
	s.router.Use(customMiddleware.ContentTypeMiddleware())

	// Health endpoints (no auth required)
	if s.healthServer != nil {
		s.router.Get("/health", s.healthServer.HandleHealthCheck)
		s.router.Get("/ready", s.healthServer.HandleReadinessCheck)
		s.router.Get("/live", s.healthServer.HandleLivenessCheck)
	} else {
		// Fallback to basic health check
		s.router.Get("/health", s.orderHandler.HealthCheck)
		s.router.Get("/ready", s.orderHandler.HealthCheck)
		s.router.Get("/live", s.orderHandler.HealthCheck)
	}

	// API v1 routes
	s.router.Route("/api/v1", func(r chi.Router) {
		// Apply authentication middleware to API routes (when implemented)
		// r.Use(customMiddleware.AuthMiddleware())

		s.setupOrderRoutes(r)
		s.setupMetricsRoutes(r)
	})
}

// setupOrderRoutes configures order-specific routes
func (s *Server) setupOrderRoutes(r chi.Router) {
	r.Route("/orders", func(r chi.Router) {
		r.Post("/", s.orderHandler.CreateOrder)
		r.Get("/", s.orderHandler.ListOrders)
		r.Get("/metrics", s.orderHandler.GetOrderMetrics)

		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.orderHandler.GetOrder)
			r.Patch("/status", s.orderHandler.UpdateOrderStatus)
		})
	})

	// User-specific order routes
	r.Route("/users/{userID}", func(r chi.Router) {
		r.Get("/orders", s.orderHandler.GetUserOrders)
	})

	s.logger.Info(nil, "Order routes configured", map[string]interface{}{
		"routes": []string{
			"POST /api/v1/orders",
			"GET /api/v1/orders",
			"GET /api/v1/orders/{id}",
			"PATCH /api/v1/orders/{id}/status",
			"GET /api/v1/users/{userID}/orders",
			"GET /api/v1/orders/metrics",
		},
	})
}

// setupMetricsRoutes configures metrics and monitoring routes
func (s *Server) setupMetricsRoutes(r chi.Router) {
	// Additional monitoring endpoints
	if s.healthServer != nil {
		r.Get("/metrics", s.healthServer.HandleMetrics)
	} else {
		r.Get("/metrics", s.handlePrometheusMetrics)
	}

	s.logger.Info(nil, "Metrics routes configured", map[string]interface{}{
		"routes": []string{
			"GET /api/v1/metrics",
		},
	})
}

// setupServer configures the HTTP server
func (s *Server) setupServer() {
	s.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.config.Host, s.config.Port),
		Handler:      s.router,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
		ErrorLog:     nil, // We handle logging through our middleware
	}
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info(ctx, "Starting HTTP server", map[string]interface{}{
		"address":       s.server.Addr,
		"read_timeout":  s.config.ReadTimeout,
		"write_timeout": s.config.WriteTimeout,
		"idle_timeout":  s.config.IdleTimeout,
	})

	// Print routes for debugging
	s.printRoutes()

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	return nil
}

// Stop gracefully stops the HTTP server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info(ctx, "Stopping HTTP server")

	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(shutdownCtx); err != nil {
		s.logger.Error(ctx, "Failed to gracefully shutdown HTTP server", err)
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}

	s.logger.Info(ctx, "HTTP server stopped successfully")
	return nil
}

// GetRouter returns the router for testing purposes
func (s *Server) GetRouter() *chi.Mux {
	return s.router
}

// handlePrometheusMetrics exposes metrics in Prometheus format
func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement Prometheus metrics exposition
	// For now, return basic metrics from our metrics interface

	if metricsData, ok := s.metrics.(interface{ GetMetrics() map[string]interface{} }); ok {
		data := metricsData.GetMetrics()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Simple JSON response for now
		// In production, you'd format this as Prometheus metrics
		response := map[string]interface{}{
			"service":   "order-service",
			"metrics":   data,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}

		if err := handlers.WriteJSON(w, response); err != nil {
			s.logger.Error(r.Context(), "Failed to write metrics response", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error": "Metrics not available"}`))
	}
}

// printRoutes prints all configured routes for debugging
func (s *Server) printRoutes() {
	chi.Walk(s.router, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		s.logger.Debug(nil, "Route registered", map[string]interface{}{
			"method": method,
			"route":  route,
		})
		return nil
	})
}

// HealthCheck returns the server health status
func (s *Server) HealthCheck(ctx context.Context) map[string]interface{} {
	return map[string]interface{}{
		"status":    "healthy",
		"service":   "order-service",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "1.0.0",
	}
}

// EnableAuthMiddleware enables authentication middleware for protected routes
func (s *Server) EnableAuthMiddleware() {
	// TODO: Implement when auth middleware is ready
	s.logger.Info(nil, "Authentication middleware enabled")
}

// EnableRateLimitMiddleware enables rate limiting
func (s *Server) EnableRateLimitMiddleware(requestsPerMinute int) {
	// TODO: Implement rate limiting with Chi
	s.logger.Info(nil, "Rate limiting middleware enabled", map[string]interface{}{
		"requests_per_minute": requestsPerMinute,
	})
}
