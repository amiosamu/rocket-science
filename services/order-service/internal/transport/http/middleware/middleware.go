package middleware

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
	"github.com/amiosamu/rocket-science/shared/platform/observability/tracing"
)

// responseWriter wraps http.ResponseWriter to capture response data
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriter) Write(data []byte) (int, error) {
	n, err := w.ResponseWriter.Write(data)
	w.bytesWritten += n
	return n, err
}

// LoggingMiddleware logs HTTP requests and responses
func LoggingMiddleware(logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Generate request ID if not present
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Add request ID to response header
			w.Header().Set("X-Request-ID", requestID)

			// Add request ID to context
			ctx := context.WithValue(r.Context(), "request_id", requestID)
			r = r.WithContext(ctx)

			// Wrap response writer to capture status code and size
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Log request details
			duration := time.Since(start)
			logger.Info(ctx, "HTTP request processed", map[string]interface{}{
				"method":         r.Method,
				"path":           r.URL.Path,
				"query":          r.URL.RawQuery,
				"status_code":    wrapped.statusCode,
				"duration_ms":    duration.Milliseconds(),
				"request_id":     requestID,
				"user_agent":     r.UserAgent(),
				"remote_addr":    r.RemoteAddr,
				"content_length": r.ContentLength,
				"response_size":  wrapped.bytesWritten,
			})
		})
	}
}

// TracingMiddleware adds OpenTelemetry tracing to HTTP requests
func TracingMiddleware(serviceName string) func(http.Handler) http.Handler {
	tracer := otel.Tracer(serviceName)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract trace context from headers
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

			// Start span
			spanName := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
			ctx, span := tracer.Start(ctx, spanName, trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()

			// Set span attributes
			span.SetAttributes(
				tracing.HTTPMethodKey.String(r.Method),
				tracing.HTTPURLKey.String(r.URL.String()),
				tracing.HTTPUserAgentKey.String(r.UserAgent()),
				tracing.HTTPRemoteAddrKey.String(r.RemoteAddr),
				attribute.String("http.scheme", r.URL.Scheme),
				attribute.String("http.host", r.Host),
				attribute.String("http.target", r.URL.Path),
			)

			// Add request ID if present
			if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
				span.SetAttributes(attribute.String("request.id", requestID))
			}

			// Add trace ID to context for logging
			if span.SpanContext().IsValid() {
				ctx = context.WithValue(ctx, "trace_id", span.SpanContext().TraceID().String())
			}

			// Wrap response writer to capture status code
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Process request with traced context
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Set response attributes
			span.SetAttributes(
				tracing.HTTPStatusCodeKey.Int(wrapped.statusCode),
				attribute.Int("http.response_size", wrapped.bytesWritten),
			)

			// Set span status based on HTTP status code
			if wrapped.statusCode >= 400 {
				span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", wrapped.statusCode))
			}
		})
	}
}

// MetricsMiddleware collects HTTP metrics
func MetricsMiddleware(metrics metrics.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture metrics
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Record metrics
			duration := time.Since(start)
			labels := map[string]string{
				"method": r.Method,
				"path":   r.URL.Path,
				"status": fmt.Sprintf("%d", wrapped.statusCode),
			}

			metrics.IncrementCounter("http_requests_total", labels)
			metrics.RecordDuration("http_request_duration_seconds", duration, labels)
			metrics.RecordValue("http_request_size_bytes", float64(r.ContentLength), labels)
			metrics.RecordValue("http_response_size_bytes", float64(wrapped.bytesWritten), labels)

			// Record status code specific metrics
			if wrapped.statusCode >= 400 {
				metrics.IncrementCounter("http_requests_errors_total", labels)
			}
		})
	}
}

// RecoveryMiddleware recovers from panics and logs them
func RecoveryMiddleware(logger logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					// Log the panic with stack trace
					logger.Error(r.Context(), "HTTP handler panic", fmt.Errorf("panic: %v", err), map[string]interface{}{
						"method":     r.Method,
						"path":       r.URL.Path,
						"request_id": r.Header.Get("X-Request-ID"),
						"stack":      string(debug.Stack()),
					})

					// Record error in span if available
					tracing.RecordError(r.Context(), fmt.Errorf("panic: %v", err))

					// Return 500 error
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error": "Internal server error", "code": 500}`))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware handles Cross-Origin Resource Sharing
func CORSMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range allowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-Trace-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-Trace-ID")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "3600")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeadersMiddleware adds security headers
func SecurityHeadersMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Security headers
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			next.ServeHTTP(w, r)
		})
	}
}

// TimeoutMiddleware adds request timeout
func TimeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.TimeoutHandler(next, timeout, `{"error": "Request timeout", "code": 408}`)
	}
}

// ContentTypeMiddleware ensures JSON content type for API endpoints
func ContentTypeMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// For POST, PUT, PATCH requests, ensure content-type is JSON
			if r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH" {
				contentType := r.Header.Get("Content-Type")
				if contentType != "application/json" && contentType != "application/json; charset=utf-8" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnsupportedMediaType)
					w.Write([]byte(`{"error": "Content-Type must be application/json", "code": 415}`))
					return
				}
			}

			// Set default response content type
			w.Header().Set("Content-Type", "application/json; charset=utf-8")

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMiddleware implements basic rate limiting
func RateLimitMiddleware(requestsPerMinute int) func(http.Handler) http.Handler {
	// This is a simple in-memory rate limiter
	// In production, you'd use Redis or a more sophisticated solution

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: Implement rate limiting logic
			// For now, just pass through
			next.ServeHTTP(w, r)
		})
	}
}

// AuthMiddleware validates authentication (basic implementation)
func AuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simple session validation - check for session header
			sessionID := r.Header.Get("X-Session-ID")
			if sessionID == "" {
				// Check Authorization header for Bearer token
				authHeader := r.Header.Get("Authorization")
				if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
					http.Error(w, `{"error": "Missing authentication", "code": 401}`, http.StatusUnauthorized)
					return
				}
				// Extract token from Bearer header
				// For now, just pass through - full validation would be via IAM service
			}

			// Add user context (simplified)
			ctx := context.WithValue(r.Context(), "session_id", sessionID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
