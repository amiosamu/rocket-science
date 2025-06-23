package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Session represents a user session
type Session struct {
	ID               string        `json:"id" redis:"id"`
	UserID           string        `json:"user_id" redis:"user_id"`
	AccessToken      string        `json:"access_token" redis:"access_token"`
	RefreshToken     string        `json:"refresh_token" redis:"refresh_token"`
	CreatedAt        time.Time     `json:"created_at" redis:"created_at"`
	ExpiresAt        time.Time     `json:"expires_at" redis:"expires_at"`
	LastAccessedAt   time.Time     `json:"last_accessed_at" redis:"last_accessed_at"`
	IPAddress        string        `json:"ip_address" redis:"ip_address"`
	UserAgent        string        `json:"user_agent" redis:"user_agent"`
	Status           SessionStatus `json:"status" redis:"status"`
	RefreshExpiresAt time.Time     `json:"refresh_expires_at" redis:"refresh_expires_at"`
}

// SessionStatus represents session status
type SessionStatus string

const (
	SessionStatusActive  SessionStatus = "active"
	SessionStatusExpired SessionStatus = "expired"
	SessionStatusRevoked SessionStatus = "revoked"
	SessionStatusInvalid SessionStatus = "invalid"
)

// JWTClaims represents custom JWT claims
type JWTClaims struct {
	UserID    string    `json:"user_id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"`
	Email     string    `json:"email"`
	IssuedAt  time.Time `json:"iat"`
	jwt.RegisteredClaims
}

// Session-related errors
var (
	ErrSessionNotFound     = errors.New("session not found")
	ErrSessionExpired      = errors.New("session has expired")
	ErrSessionRevoked      = errors.New("session has been revoked")
	ErrSessionInvalid      = errors.New("session is invalid")
	ErrInvalidToken        = errors.New("invalid token")
	ErrTokenExpired        = errors.New("token has expired")
	ErrInvalidRefreshToken = errors.New("invalid refresh token")
	ErrRefreshTokenExpired = errors.New("refresh token has expired")
	ErrInvalidJWTClaims    = errors.New("invalid JWT claims")
)

// NewSession creates a new session for a user
func NewSession(userID, ipAddress, userAgent string, sessionDuration, accessTokenDuration, refreshTokenDuration time.Duration) *Session {
	now := time.Now()
	sessionID := uuid.New().String()

	session := &Session{
		ID:               sessionID,
		UserID:           userID,
		CreatedAt:        now,
		ExpiresAt:        now.Add(sessionDuration),
		LastAccessedAt:   now,
		IPAddress:        ipAddress,
		UserAgent:        userAgent,
		Status:           SessionStatusActive,
		RefreshExpiresAt: now.Add(refreshTokenDuration),
	}

	return session
}

// IsValid checks if the session is valid
func (s *Session) IsValid() error {
	now := time.Now()

	switch s.Status {
	case SessionStatusRevoked:
		return ErrSessionRevoked
	case SessionStatusInvalid:
		return ErrSessionInvalid
	case SessionStatusExpired:
		return ErrSessionExpired
	}

	if now.After(s.ExpiresAt) {
		s.Status = SessionStatusExpired
		return ErrSessionExpired
	}

	return nil
}

// IsRefreshTokenValid checks if the refresh token is still valid
func (s *Session) IsRefreshTokenValid() error {
	if err := s.IsValid(); err != nil {
		return err
	}

	if time.Now().After(s.RefreshExpiresAt) {
		return ErrRefreshTokenExpired
	}

	return nil
}

// UpdateLastAccessed updates the last accessed time
func (s *Session) UpdateLastAccessed() {
	s.LastAccessedAt = time.Now()
}

// Revoke revokes the session
func (s *Session) Revoke() {
	s.Status = SessionStatusRevoked
	s.LastAccessedAt = time.Now()
}

// Expire expires the session
func (s *Session) Expire() {
	s.Status = SessionStatusExpired
	s.LastAccessedAt = time.Now()
}

// Invalidate invalidates the session
func (s *Session) Invalidate() {
	s.Status = SessionStatusInvalid
	s.LastAccessedAt = time.Now()
}

// IsExpired checks if the session is expired
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt) || s.Status == SessionStatusExpired
}

// IsActive checks if the session is active
func (s *Session) IsActive() bool {
	return s.Status == SessionStatusActive && !s.IsExpired()
}

// CanRefresh checks if the session can be refreshed
func (s *Session) CanRefresh() bool {
	return s.IsRefreshTokenValid() == nil
}

// GetRemainingTime returns the remaining time until session expires
func (s *Session) GetRemainingTime() time.Duration {
	if s.IsExpired() {
		return 0
	}
	return time.Until(s.ExpiresAt)
}

// GetRefreshRemainingTime returns the remaining time until refresh token expires
func (s *Session) GetRefreshRemainingTime() time.Duration {
	if time.Now().After(s.RefreshExpiresAt) {
		return 0
	}
	return time.Until(s.RefreshExpiresAt)
}

// GenerateTokens generates JWT access and refresh tokens for the session
func (s *Session) GenerateTokens(user *User, secretKey string, accessDuration, refreshDuration time.Duration) error {
	now := time.Now()

	// Generate access token
	accessClaims := &JWTClaims{
		UserID:    user.ID,
		SessionID: s.ID,
		Role:      string(user.Role),
		Email:     user.Email,
		IssuedAt:  now,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(accessDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "rocket-science-iam",
			Subject:   user.ID,
			ID:        s.ID,
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(secretKey))
	if err != nil {
		return fmt.Errorf("failed to generate access token: %w", err)
	}

	// Generate refresh token
	refreshClaims := &JWTClaims{
		UserID:    user.ID,
		SessionID: s.ID,
		Role:      string(user.Role),
		Email:     user.Email,
		IssuedAt:  now,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(refreshDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "rocket-science-iam",
			Subject:   user.ID,
			ID:        s.ID + "_refresh",
		},
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString([]byte(secretKey))
	if err != nil {
		return fmt.Errorf("failed to generate refresh token: %w", err)
	}

	s.AccessToken = accessTokenString
	s.RefreshToken = refreshTokenString
	s.ExpiresAt = now.Add(accessDuration)
	s.RefreshExpiresAt = now.Add(refreshDuration)

	return nil
}

// RefreshAccessToken generates a new access token using the refresh token
func (s *Session) RefreshAccessToken(user *User, secretKey string, accessDuration time.Duration) error {
	if err := s.IsRefreshTokenValid(); err != nil {
		return err
	}

	now := time.Now()

	// Generate new access token
	accessClaims := &JWTClaims{
		UserID:    user.ID,
		SessionID: s.ID,
		Role:      string(user.Role),
		Email:     user.Email,
		IssuedAt:  now,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(accessDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "rocket-science-iam",
			Subject:   user.ID,
			ID:        s.ID,
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString([]byte(secretKey))
	if err != nil {
		return fmt.Errorf("failed to generate new access token: %w", err)
	}

	s.AccessToken = accessTokenString
	s.ExpiresAt = now.Add(accessDuration)
	s.UpdateLastAccessed()

	return nil
}

// ValidateJWTToken validates a JWT token and returns the claims
func ValidateJWTToken(tokenString, secretKey string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secretKey), nil
	})

	if err != nil {
		// Check for expired token error in v5 API
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, ErrInvalidJWTClaims
	}

	// Additional validation
	if claims.UserID == "" || claims.SessionID == "" {
		return nil, ErrInvalidJWTClaims
	}

	return claims, nil
}

// ExtractTokenFromAuthHeader extracts bearer token from authorization header
func ExtractTokenFromAuthHeader(authHeader string) (string, error) {
	const bearerPrefix = "Bearer "
	if len(authHeader) < len(bearerPrefix) || authHeader[:len(bearerPrefix)] != bearerPrefix {
		return "", ErrInvalidToken
	}
	return authHeader[len(bearerPrefix):], nil
}

// GetSessionKey returns the Redis key for storing session
func (s *Session) GetSessionKey() string {
	return fmt.Sprintf("session:%s", s.ID)
}

// GetUserSessionsKey returns the Redis key for storing user's session list
func GetUserSessionsKey(userID string) string {
	return fmt.Sprintf("user_sessions:%s", userID)
}

// GetTokenBlacklistKey returns the Redis key for blacklisted tokens
func GetTokenBlacklistKey(tokenID string) string {
	return fmt.Sprintf("blacklist_token:%s", tokenID)
}

// SessionInfo provides session information for responses
type SessionInfo struct {
	ID             string        `json:"id"`
	UserID         string        `json:"user_id"`
	CreatedAt      time.Time     `json:"created_at"`
	ExpiresAt      time.Time     `json:"expires_at"`
	LastAccessedAt time.Time     `json:"last_accessed_at"`
	IPAddress      string        `json:"ip_address"`
	UserAgent      string        `json:"user_agent"`
	Status         SessionStatus `json:"status"`
	IsActive       bool          `json:"is_active"`
	RemainingTime  string        `json:"remaining_time"`
}

// ToSessionInfo converts session to session info
func (s *Session) ToSessionInfo() *SessionInfo {
	return &SessionInfo{
		ID:             s.ID,
		UserID:         s.UserID,
		CreatedAt:      s.CreatedAt,
		ExpiresAt:      s.ExpiresAt,
		LastAccessedAt: s.LastAccessedAt,
		IPAddress:      s.IPAddress,
		UserAgent:      s.UserAgent,
		Status:         s.Status,
		IsActive:       s.IsActive(),
		RemainingTime:  s.GetRemainingTime().String(),
	}
}

// IsValidStatus checks if a session status string is valid
func IsValidSessionStatus(status string) bool {
	switch SessionStatus(status) {
	case SessionStatusActive, SessionStatusExpired, SessionStatusRevoked, SessionStatusInvalid:
		return true
	default:
		return false
	}
}

// GetSessionsByUserKey returns the Redis key pattern for user sessions
func GetSessionsByUserKey(userID string) string {
	return fmt.Sprintf("user:%s:sessions", userID)
}

// SessionCleanupInfo provides information about session cleanup
type SessionCleanupInfo struct {
	ExpiredSessions int `json:"expired_sessions"`
	RevokedSessions int `json:"revoked_sessions"`
	InvalidSessions int `json:"invalid_sessions"`
	TotalCleaned    int `json:"total_cleaned"`
}
