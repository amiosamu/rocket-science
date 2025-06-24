package interceptors

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	iampb "github.com/amiosamu/rocket-science/services/iam-service/proto/iam"
)

type AuthInterceptor struct {
	iamClient iampb.IAMServiceClient
	logger    *slog.Logger
	conn      *grpc.ClientConn
}

func NewAuthInterceptor(iamAddress string, logger *slog.Logger) (*AuthInterceptor, error) {
	conn, err := grpc.Dial(iamAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IAM service: %w", err)
	}

	client := iampb.NewIAMServiceClient(conn)

	return &AuthInterceptor{
		iamClient: client,
		logger:    logger,
		conn:      conn,
	}, nil
}

func (a *AuthInterceptor) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// Skip auth for health checks
		if a.shouldSkipAuth(info.FullMethod) {
			return handler(ctx, req)
		}

		// Validate session
		newCtx, err := a.authenticateRequest(ctx)
		if err != nil {
			a.logger.Error("Authentication failed", "error", err, "method", info.FullMethod)
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		return handler(newCtx, req)
	}
}

func (a *AuthInterceptor) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip auth for health checks
		if a.shouldSkipAuth(info.FullMethod) {
			return handler(srv, ss)
		}

		// Validate session
		newCtx, err := a.authenticateRequest(ss.Context())
		if err != nil {
			a.logger.Error("Authentication failed", "error", err, "method", info.FullMethod)
			return status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		// Create authenticated stream
		authenticatedStream := &authenticatedStream{
			ServerStream: ss,
			ctx:          newCtx,
		}

		return handler(srv, authenticatedStream)
	}
}

func (a *AuthInterceptor) shouldSkipAuth(method string) bool {
	skipMethods := []string{
		"/grpc.health.v1.Health/Check",
		"/grpc.health.v1.Health/Watch",
	}

	for _, skipMethod := range skipMethods {
		if method == skipMethod {
			return true
		}
	}
	return false
}

func (a *AuthInterceptor) authenticateRequest(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, fmt.Errorf("missing metadata")
	}

	// Extract session ID and access token
	sessionIDs := md.Get("x-session-id")
	accessTokens := md.Get("authorization")

	if len(sessionIDs) == 0 {
		return nil, fmt.Errorf("missing session ID")
	}

	if len(accessTokens) == 0 {
		return nil, fmt.Errorf("missing authorization header")
	}

	sessionID := sessionIDs[0]
	authHeader := accessTokens[0]

	// Extract token from Bearer header
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, fmt.Errorf("invalid authorization header format")
	}
	accessToken := strings.TrimPrefix(authHeader, "Bearer ")

	// Validate session with IAM service
	req := &iampb.ValidateSessionRequest{
		SessionId:   sessionID,
		AccessToken: accessToken,
	}

	resp, err := a.iamClient.ValidateSession(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to validate session: %w", err)
	}

	if !resp.Valid {
		return nil, fmt.Errorf("invalid session: %s", resp.Message)
	}

	// Add user info to context
	newCtx := context.WithValue(ctx, "user_id", resp.User.Id)
	newCtx = context.WithValue(newCtx, "user_role", resp.User.Role.String())
	newCtx = context.WithValue(newCtx, "session_id", sessionID)

	return newCtx, nil
}

func (a *AuthInterceptor) Close() error {
	if a.conn != nil {
		return a.conn.Close()
	}
	return nil
}

type authenticatedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authenticatedStream) Context() context.Context {
	return s.ctx
}
