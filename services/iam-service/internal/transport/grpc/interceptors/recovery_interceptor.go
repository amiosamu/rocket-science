package interceptors

import (
	"context"
	"fmt"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// RecoveryInterceptor handles panic recovery for gRPC calls
type RecoveryInterceptor struct {
	logger logging.Logger
}

// NewRecoveryInterceptor creates a new recovery interceptor
func NewRecoveryInterceptor(logger logging.Logger) *RecoveryInterceptor {
	return &RecoveryInterceptor{
		logger: logger,
	}
}

// UnaryServerInterceptor returns a unary server interceptor for panic recovery
func (r *RecoveryInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if panicValue := recover(); panicValue != nil {
				// Log the panic with stack trace
				r.logger.Error(ctx, "gRPC handler panic recovered", fmt.Errorf("panic: %v", panicValue), map[string]interface{}{
					"method":  info.FullMethod,
					"panic":   fmt.Sprintf("%v", panicValue),
					"stack":   string(debug.Stack()),
					"user_id": r.getUserIDFromContext(ctx),
				})

				// Return internal server error
				err = status.Error(codes.Internal, "internal server error")
			}
		}()

		return handler(ctx, req)
	}
}

// StreamServerInterceptor returns a stream server interceptor for panic recovery
func (r *RecoveryInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if panicValue := recover(); panicValue != nil {
				// Log the panic with stack trace
				r.logger.Error(stream.Context(), "gRPC stream handler panic recovered", fmt.Errorf("panic: %v", panicValue), map[string]interface{}{
					"method":  info.FullMethod,
					"panic":   fmt.Sprintf("%v", panicValue),
					"stack":   string(debug.Stack()),
					"user_id": r.getUserIDFromContext(stream.Context()),
				})

				// Return internal server error
				err = status.Error(codes.Internal, "internal server error")
			}
		}()

		return handler(srv, stream)
	}
}

// getUserIDFromContext safely extracts user ID from context
func (r *RecoveryInterceptor) getUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID
	}
	return "anonymous"
}
