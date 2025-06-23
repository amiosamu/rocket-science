package interceptors

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/service"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// AuthInterceptor handles authentication for gRPC calls
type AuthInterceptor struct {
	authService *service.AuthService
	logger      logging.Logger
}

// NewAuthInterceptor creates a new authentication interceptor
func NewAuthInterceptor(authService *service.AuthService, logger logging.Logger) *AuthInterceptor {
	return &AuthInterceptor{
		authService: authService,
		logger:      logger,
	}
}

// UnaryServerInterceptor returns a unary server interceptor for authentication
func (a *AuthInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip authentication for certain methods
		if a.shouldSkipAuth(info.FullMethod) {
			return handler(ctx, req)
		}

		// Extract and validate token
		authCtx, err := a.authenticateRequest(ctx)
		if err != nil {
			a.logger.Warn(ctx, "Authentication failed for gRPC call", map[string]interface{}{
				"method": info.FullMethod,
				"error":  err.Error(),
			})
			return nil, err
		}

		// Call handler with authenticated context
		return handler(authCtx, req)
	}
}

// StreamServerInterceptor returns a stream server interceptor for authentication
func (a *AuthInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip authentication for certain methods
		if a.shouldSkipAuth(info.FullMethod) {
			return handler(srv, stream)
		}

		// Extract and validate token
		authCtx, err := a.authenticateRequest(stream.Context())
		if err != nil {
			a.logger.Warn(stream.Context(), "Authentication failed for gRPC stream", map[string]interface{}{
				"method": info.FullMethod,
				"error":  err.Error(),
			})
			return err
		}

		// Create new stream with authenticated context
		wrappedStream := &authenticatedStream{
			ServerStream: stream,
			ctx:          authCtx,
		}

		return handler(srv, wrappedStream)
	}
}

// shouldSkipAuth determines if authentication should be skipped for a method
func (a *AuthInterceptor) shouldSkipAuth(method string) bool {
	// List of methods that don't require authentication
	publicMethods := []string{
		"/iam.IAMService/Login",
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.Health/Watch",
	}

	for _, publicMethod := range publicMethods {
		if strings.HasSuffix(method, publicMethod) {
			return true
		}
	}

	return false
}

// authenticateRequest extracts and validates authentication token from context
func (a *AuthInterceptor) authenticateRequest(ctx context.Context) (context.Context, error) {
	// Extract metadata from context
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	// Extract authorization header
	authHeaders := md.Get("authorization")
	if len(authHeaders) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	authHeader := authHeaders[0]
	if authHeader == "" {
		return nil, status.Error(codes.Unauthenticated, "empty authorization header")
	}

	// Extract token from "Bearer <token>" format
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return nil, status.Error(codes.Unauthenticated, "invalid authorization header format")
	}

	token := strings.TrimPrefix(authHeader, bearerPrefix)
	if token == "" {
		return nil, status.Error(codes.Unauthenticated, "empty token")
	}

	// Validate token using auth service
	validateResp, err := a.authService.ValidateToken(ctx, token)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}

	// Add user information to context
	authCtx := context.WithValue(ctx, "user_id", validateResp.User.ID)
	authCtx = context.WithValue(authCtx, "user_role", validateResp.User.Role)
	authCtx = context.WithValue(authCtx, "session_id", validateResp.SessionInfo.ID)

	return authCtx, nil
}

// authenticatedStream wraps a ServerStream with an authenticated context
type authenticatedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authenticatedStream) Context() context.Context {
	return s.ctx
}
