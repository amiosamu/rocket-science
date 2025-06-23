package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/config"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/repository/interfaces"
)

// AuthService implements authentication business logic
type AuthService struct {
	userRepo    interfaces.UserRepository
	sessionRepo interfaces.SessionRepository
	config      *config.Config
}

// NewAuthService creates a new authentication service
func NewAuthService(
	userRepo interfaces.UserRepository,
	sessionRepo interfaces.SessionRepository,
	config *config.Config,
) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		config:      config,
	}
}

// LoginResult represents the result of a login operation
type LoginResult struct {
	AccessToken  string              `json:"access_token"`
	RefreshToken string              `json:"refresh_token"`
	ExpiresAt    time.Time           `json:"expires_at"`
	SessionID    string              `json:"session_id"`
	User         *UserInfo           `json:"user"`
	SessionInfo  *domain.SessionInfo `json:"session_info"`
}

// UserInfo represents user information in responses
type UserInfo struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TokenValidationResult represents token validation result
type TokenValidationResult struct {
	Valid       bool                `json:"valid"`
	Claims      *domain.JWTClaims   `json:"claims,omitempty"`
	User        *UserInfo           `json:"user,omitempty"`
	SessionInfo *domain.SessionInfo `json:"session_info,omitempty"`
}

// Login authenticates a user and creates a session
func (s *AuthService) Login(ctx context.Context, email, password, ipAddress, userAgent string) (*LoginResult, error) {
	// Input validation
	if email == "" {
		return nil, domain.ErrInvalidEmail
	}
	if password == "" {
		return nil, domain.ErrInvalidPassword
	}

	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))

	// Get user by email
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if err == domain.ErrUserNotFound {
			return nil, domain.ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user account is locked
	if user.IsLocked() {
		return nil, domain.ErrAccountLocked
	}

	// Check if user is active
	if user.Status != domain.StatusActive {
		return nil, domain.ErrAccountInactive
	}

	// Verify password
	if err := user.ValidatePassword(password); err != nil {
		// Record failed login attempt
		s.userRepo.RecordLoginAttempt(ctx, user.ID)
		return nil, domain.ErrInvalidCredentials
	}

	// Reset failed login attempts on successful authentication
	s.userRepo.ResetLoginAttempts(ctx, user.ID)

	// Create new session
	session := domain.NewSession(
		user.ID,
		ipAddress,
		userAgent,
		time.Duration(s.config.JWT.AccessTokenDuration)*time.Hour,
		time.Duration(s.config.JWT.AccessTokenDuration)*time.Hour,
		time.Duration(s.config.JWT.RefreshTokenDuration)*time.Hour,
	)

	// Generate JWT tokens
	if err := session.GenerateTokens(
		user,
		s.config.JWT.SecretKey,
		time.Duration(s.config.JWT.AccessTokenDuration)*time.Hour,
		time.Duration(s.config.JWT.RefreshTokenDuration)*time.Hour,
	); err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// Store session in Redis
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Update user's last login time
	s.userRepo.UpdateLastLogin(ctx, user.ID, time.Now())

	return &LoginResult{
		AccessToken:  session.AccessToken,
		RefreshToken: session.RefreshToken,
		ExpiresAt:    session.ExpiresAt,
		SessionID:    session.ID,
		User:         s.userToInfo(user),
		SessionInfo:  session.ToSessionInfo(),
	}, nil
}

// Logout invalidates a user session
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	// Revoke session
	if err := s.sessionRepo.RevokeSession(ctx, sessionID); err != nil {
		if err == domain.ErrSessionNotFound {
			return nil // Already logged out
		}
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	return nil
}

// ValidateToken validates an access token and returns session info
func (s *AuthService) ValidateToken(ctx context.Context, accessToken string) (*TokenValidationResult, error) {
	if accessToken == "" {
		return nil, domain.ErrInvalidToken
	}

	// Parse and validate JWT token
	claims, err := domain.ValidateJWTToken(accessToken, s.config.JWT.SecretKey)
	if err != nil {
		return nil, err
	}

	// Check if token is blacklisted
	isBlacklisted, err := s.sessionRepo.IsTokenBlacklisted(ctx, claims.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check token blacklist: %w", err)
	}
	if isBlacklisted {
		return nil, fmt.Errorf("token is blacklisted")
	}

	// Validate session
	session, err := s.sessionRepo.ValidateSession(ctx, claims.SessionID, accessToken)
	if err != nil {
		return nil, err
	}

	// Get user information
	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user is still active
	if user.Status != domain.StatusActive {
		// Revoke session for inactive user
		s.sessionRepo.RevokeSession(ctx, session.ID)
		return nil, domain.ErrAccountInactive
	}

	// Update last accessed time (async to avoid impacting performance)
	go func() {
		s.sessionRepo.UpdateLastAccessed(context.Background(), session.ID)
	}()

	return &TokenValidationResult{
		Valid:       true,
		Claims:      claims,
		User:        s.userToInfo(user),
		SessionInfo: session.ToSessionInfo(),
	}, nil
}

// RefreshToken generates a new access token using refresh token
func (s *AuthService) RefreshToken(ctx context.Context, sessionID, refreshToken string) (*LoginResult, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}
	if refreshToken == "" {
		return nil, domain.ErrInvalidRefreshToken
	}

	// Get and validate session with refresh token
	session, err := s.sessionRepo.RefreshSession(ctx, sessionID, refreshToken)
	if err != nil {
		return nil, err
	}

	// Get user information
	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user is still active
	if user.Status != domain.StatusActive {
		// Revoke session for inactive user
		s.sessionRepo.RevokeSession(ctx, session.ID)
		return nil, domain.ErrAccountInactive
	}

	// Generate new access token
	if err := session.RefreshAccessToken(
		user,
		s.config.JWT.SecretKey,
		time.Duration(s.config.JWT.AccessTokenDuration)*time.Hour,
	); err != nil {
		return nil, fmt.Errorf("failed to refresh access token: %w", err)
	}

	// Update session in Redis
	if err := s.sessionRepo.Update(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return &LoginResult{
		AccessToken:  session.AccessToken,
		RefreshToken: session.RefreshToken,
		ExpiresAt:    session.ExpiresAt,
		SessionID:    session.ID,
		User:         s.userToInfo(user),
		SessionInfo:  session.ToSessionInfo(),
	}, nil
}

// GetSessionInfo retrieves session information
func (s *AuthService) GetSessionInfo(ctx context.Context, sessionID string) (*domain.SessionInfo, *UserInfo, error) {
	if sessionID == "" {
		return nil, nil, fmt.Errorf("session ID cannot be empty")
	}

	// Get session
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return nil, nil, err
	}

	// Get user information
	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user: %w", err)
	}

	return session.ToSessionInfo(), s.userToInfo(user), nil
}

// RevokeSession revokes a specific session
func (s *AuthService) RevokeSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("session ID cannot be empty")
	}

	// Revoke session
	if err := s.sessionRepo.RevokeSession(ctx, sessionID); err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	return nil
}

// RevokeAllUserSessions revokes all sessions for a user
func (s *AuthService) RevokeAllUserSessions(ctx context.Context, userID string, keepSessionID string) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	// Revoke all user sessions
	if keepSessionID != "" {
		err := s.sessionRepo.RevokeUserSessionsExcept(ctx, userID, keepSessionID)
		if err != nil {
			return fmt.Errorf("failed to revoke user sessions: %w", err)
		}
	} else {
		err := s.sessionRepo.RevokeUserSessions(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to revoke all user sessions: %w", err)
		}
	}

	return nil
}

// ChangePassword changes a user's password
func (s *AuthService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string, revokeOtherSessions bool, currentSessionID string) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}
	if currentPassword == "" {
		return domain.ErrInvalidPassword
	}
	if newPassword == "" {
		return domain.ErrInvalidPassword
	}
	if currentPassword == newPassword {
		return fmt.Errorf("new password must be different from current password")
	}

	// Get user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Verify current password
	if err := user.ValidatePassword(currentPassword); err != nil {
		return domain.ErrInvalidCredentials
	}

	// Change password using domain method
	if err := user.ChangePassword(currentPassword, newPassword); err != nil {
		return err
	}

	// Update user in database
	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	// Optionally revoke all sessions except current one
	if revokeOtherSessions {
		if currentSessionID != "" {
			s.sessionRepo.RevokeUserSessionsExcept(ctx, userID, currentSessionID)
		} else {
			s.sessionRepo.RevokeUserSessions(ctx, userID)
		}
	}

	return nil
}

// ResetPassword initiates password reset process
func (s *AuthService) ResetPassword(ctx context.Context, email string) error {
	if email == "" {
		return domain.ErrInvalidEmail
	}

	// Normalize email
	email = strings.ToLower(strings.TrimSpace(email))

	// Get user by email
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		if err == domain.ErrUserNotFound {
			// Don't reveal that user doesn't exist for security
			return nil
		}
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user is active
	if user.Status != domain.StatusActive {
		return nil // Don't reveal inactive status
	}

	// Generate reset token
	resetToken := uuid.New().String()
	expiresAt := time.Now().Add(time.Hour) // 1 hour expiry

	// Store reset token in user metadata
	if user.Metadata == nil {
		user.Metadata = make(map[string]string)
	}
	user.Metadata["password_reset_token"] = resetToken
	user.Metadata["password_reset_expires_at"] = fmt.Sprintf("%d", expiresAt.Unix())

	if err := s.userRepo.UpdateMetadata(ctx, user.ID, user.Metadata); err != nil {
		return fmt.Errorf("failed to store reset token: %w", err)
	}

	// TODO: Send email with reset link
	// For now, we'll just log it
	fmt.Printf("Password reset token for %s: %s\n", email, resetToken)

	return nil
}

// ConfirmPasswordReset completes the password reset process
func (s *AuthService) ConfirmPasswordReset(ctx context.Context, resetToken, newPassword string) error {
	if resetToken == "" {
		return fmt.Errorf("reset token cannot be empty")
	}
	if newPassword == "" {
		return domain.ErrInvalidPassword
	}

	// Find user by reset token - simplified search
	users, _, err := s.userRepo.List(ctx, interfaces.UserFilter{Limit: 1000})
	if err != nil {
		return fmt.Errorf("failed to search users: %w", err)
	}

	var user *domain.User
	for _, u := range users {
		if u.Metadata != nil && u.Metadata["password_reset_token"] == resetToken {
			user = u
			break
		}
	}

	if user == nil {
		return fmt.Errorf("invalid reset token")
	}

	// Check token expiry
	if expiresAtStr, ok := user.Metadata["password_reset_expires_at"]; ok {
		var expiresAt int64
		if _, err := fmt.Sscanf(expiresAtStr, "%d", &expiresAt); err == nil {
			if time.Now().Unix() > expiresAt {
				return fmt.Errorf("reset token has expired")
			}
		}
	}

	// Update password directly in repository
	hashedPassword, err := domain.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	if err := s.userRepo.UpdatePassword(ctx, user.ID, hashedPassword); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	// Clear reset token from metadata
	delete(user.Metadata, "password_reset_token")
	delete(user.Metadata, "password_reset_expires_at")
	s.userRepo.UpdateMetadata(ctx, user.ID, user.Metadata)

	// Revoke all user sessions
	s.sessionRepo.RevokeUserSessions(ctx, user.ID)

	return nil
}

// GetSessionStats returns session statistics
func (s *AuthService) GetSessionStats(ctx context.Context) (*interfaces.SessionStats, error) {
	return s.sessionRepo.GetSessionStats(ctx)
}

// CleanupExpiredSessions removes expired sessions
func (s *AuthService) CleanupExpiredSessions(ctx context.Context) (*domain.SessionCleanupInfo, error) {
	return s.sessionRepo.CleanupExpiredSessions(ctx)
}

// IsHealthy checks if the auth service is healthy
func (s *AuthService) IsHealthy(ctx context.Context) error {
	// Check Redis connection
	if err := s.sessionRepo.HealthCheck(ctx); err != nil {
		return fmt.Errorf("session repository health check failed: %w", err)
	}

	return nil
}

// Helper method to convert domain user to user info
func (s *AuthService) userToInfo(user *domain.User) *UserInfo {
	return &UserInfo{
		ID:        user.ID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Role:      string(user.Role),
		Status:    string(user.Status),
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
