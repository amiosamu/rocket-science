package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/container"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// SessionValidationServer provides HTTP session validation for Envoy
type SessionValidationServer struct {
	container *container.Container
	logger    logging.Logger
}

// SessionValidationRequest represents the session validation request
type SessionValidationRequest struct {
	SessionToken string `json:"session_token"`
}

// SessionValidationResponse represents the session validation response
type SessionValidationResponse struct {
	Valid   bool   `json:"valid"`
	UserID  string `json:"user_id,omitempty"`
	Email   string `json:"email,omitempty"`
	Role    string `json:"role,omitempty"`
	Message string `json:"message,omitempty"`
}

// NewSessionValidationServer creates a new session validation server
func NewSessionValidationServer(container *container.Container) *SessionValidationServer {
	return &SessionValidationServer{
		container: container,
		logger:    container.GetLogger(),
	}
}

// ValidateSessionHandler handles session validation requests from Envoy
func (s *SessionValidationServer) ValidateSessionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle preflight OPTIONS request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Only allow POST method
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set response content type
	w.Header().Set("Content-Type", "application/json")

	// Extract token from Authorization header or request body
	var sessionToken string

	// First try Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		sessionToken = strings.TrimPrefix(authHeader, "Bearer ")
	} else {
		// Try request body
		var req SessionValidationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.logger.Warn(ctx, "Invalid request body for session validation", map[string]interface{}{
				"error": err.Error(),
			})

			response := SessionValidationResponse{
				Valid:   false,
				Message: "Invalid request format",
			}
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(response)
			return
		}
		sessionToken = req.SessionToken
	}

	// Validate token is provided
	if sessionToken == "" {
		response := SessionValidationResponse{
			Valid:   false,
			Message: "Session token is required",
		}
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Validate token with auth service
	authService := s.container.GetAuthService()
	tokenResult, err := authService.ValidateToken(ctx, sessionToken)
	if err != nil {
		s.logger.Debug(ctx, "Session validation failed", map[string]interface{}{
			"error": err.Error(),
		})

		response := SessionValidationResponse{
			Valid:   false,
			Message: "Invalid or expired session",
		}
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Session is valid, return user info
	response := SessionValidationResponse{
		Valid:   true,
		UserID:  tokenResult.User.ID,
		Email:   tokenResult.User.Email,
		Role:    tokenResult.User.Role,
		Message: "Session is valid",
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	s.logger.Debug(ctx, "Session validation successful", map[string]interface{}{
		"user_id": tokenResult.User.ID,
		"email":   tokenResult.User.Email,
	})
}

// SetupRoutes sets up the HTTP routes for session validation
func (s *SessionValidationServer) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/validate-session", s.ValidateSessionHandler)
	mux.HandleFunc("/auth/validate", s.ValidateSessionHandler) // Alternative endpoint
}

// StartValidationServer starts a simple HTTP server for session validation
func StartValidationServer(container *container.Container, port string) error {
	if port == "" {
		port = "8081" // Default port for validation server
	}

	logger := container.GetLogger()

	// Create validation server
	validationServer := NewSessionValidationServer(container)

	// Setup routes
	mux := http.NewServeMux()
	validationServer.SetupRoutes(mux)

	// Add health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "healthy",
			"service": "iam-validation",
		})
	})

	// Create HTTP server
	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	logger.Info(nil, "Starting session validation server", map[string]interface{}{
		"port": port,
	})

	return server.ListenAndServe()
}
