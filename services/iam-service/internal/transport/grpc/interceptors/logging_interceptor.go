package interceptors

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// LoggingInterceptor handles logging for gRPC calls
type LoggingInterceptor struct {
	logger logging.Logger
}

// NewLoggingInterceptor creates a new logging interceptor
func NewLoggingInterceptor(logger logging.Logger) *LoggingInterceptor {
	return &LoggingInterceptor{
		logger: logger,
	}
}

// UnaryServerInterceptor returns a unary server interceptor for logging
func (l *LoggingInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		// Extract user information from context if available
		userID := l.getUserIDFromContext(ctx)

		// Log request
		l.logger.Info(ctx, "gRPC request started", map[string]interface{}{
			"method":  info.FullMethod,
			"user_id": userID,
		})

		// Call handler
		resp, err := handler(ctx, req)

		// Calculate duration
		duration := time.Since(start)

		// Prepare log fields
		logFields := map[string]interface{}{
			"method":      info.FullMethod,
			"duration_ms": duration.Milliseconds(),
			"user_id":     userID,
		}

		// Log response
		if err != nil {
			// Extract gRPC status
			st, _ := status.FromError(err)
			logFields["status"] = st.Code().String()
			logFields["error"] = st.Message()

			l.logger.Error(ctx, "gRPC request failed", err, logFields)
		} else {
			logFields["status"] = "OK"
			l.logger.Info(ctx, "gRPC request completed", logFields)
		}

		return resp, err
	}
}

// StreamServerInterceptor returns a stream server interceptor for logging
func (l *LoggingInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()

		// Extract user information from context if available
		userID := l.getUserIDFromContext(stream.Context())

		// Log stream start
		l.logger.Info(stream.Context(), "gRPC stream started", map[string]interface{}{
			"method":  info.FullMethod,
			"user_id": userID,
		})

		// Call handler
		err := handler(srv, stream)

		// Calculate duration
		duration := time.Since(start)

		// Prepare log fields
		logFields := map[string]interface{}{
			"method":      info.FullMethod,
			"duration_ms": duration.Milliseconds(),
			"user_id":     userID,
		}

		// Log stream completion
		if err != nil {
			// Extract gRPC status
			st, _ := status.FromError(err)
			logFields["status"] = st.Code().String()
			logFields["error"] = st.Message()

			l.logger.Error(stream.Context(), "gRPC stream failed", err, logFields)
		} else {
			logFields["status"] = "OK"
			l.logger.Info(stream.Context(), "gRPC stream completed", logFields)
		}

		return err
	}
}

// getUserIDFromContext safely extracts user ID from context
func (l *LoggingInterceptor) getUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID
	}
	return "anonymous"
}
