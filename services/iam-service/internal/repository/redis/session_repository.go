package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/repository/interfaces"
)

// SessionRepository implements the SessionRepository interface for Redis
type SessionRepository struct {
	client *redis.Client
}

// NewSessionRepository creates a new Redis session repository
func NewSessionRepository(client *redis.Client) interfaces.SessionRepository {
	return &SessionRepository{
		client: client,
	}
}

// Create creates a new session in Redis
func (r *SessionRepository) Create(ctx context.Context, session *domain.Session) error {
	// Marshal session to JSON
	sessionData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Calculate TTL
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		return fmt.Errorf("session already expired")
	}

	pipe := r.client.Pipeline()

	// Store session data
	sessionKey := session.GetSessionKey()
	pipe.Set(ctx, sessionKey, sessionData, ttl)

	// Add session to user's session set
	userSessionsKey := domain.GetUserSessionsKey(session.UserID)
	pipe.SAdd(ctx, userSessionsKey, session.ID)
	pipe.Expire(ctx, userSessionsKey, ttl)

	// Store session ID in a set for easy cleanup
	pipe.SAdd(ctx, "active_sessions", session.ID)

	// Store session metadata for quick lookups
	metaKey := fmt.Sprintf("session_meta:%s", session.ID)
	metaData := map[string]interface{}{
		"user_id":    session.UserID,
		"created_at": session.CreatedAt.Unix(),
		"expires_at": session.ExpiresAt.Unix(),
		"ip_address": session.IPAddress,
		"user_agent": session.UserAgent,
		"status":     string(session.Status),
	}
	pipe.HMSet(ctx, metaKey, metaData)
	pipe.Expire(ctx, metaKey, ttl)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// GetByID retrieves a session by ID
func (r *SessionRepository) GetByID(ctx context.Context, sessionID string) (*domain.Session, error) {
	sessionKey := fmt.Sprintf("session:%s", sessionID)

	sessionData, err := r.client.Get(ctx, sessionKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, domain.ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var session domain.Session
	if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// Update updates an existing session
func (r *SessionRepository) Update(ctx context.Context, session *domain.Session) error {
	// Check if session exists
	sessionKey := session.GetSessionKey()
	exists, err := r.client.Exists(ctx, sessionKey).Result()
	if err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if exists == 0 {
		return domain.ErrSessionNotFound
	}

	// Marshal session to JSON
	sessionData, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	// Calculate TTL
	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		// Session expired, delete it
		return r.Delete(ctx, session.ID)
	}

	pipe := r.client.Pipeline()

	// Update session data
	pipe.Set(ctx, sessionKey, sessionData, ttl)

	// Update session metadata
	metaKey := fmt.Sprintf("session_meta:%s", session.ID)
	metaData := map[string]interface{}{
		"user_id":          session.UserID,
		"created_at":       session.CreatedAt.Unix(),
		"expires_at":       session.ExpiresAt.Unix(),
		"last_accessed_at": session.LastAccessedAt.Unix(),
		"ip_address":       session.IPAddress,
		"user_agent":       session.UserAgent,
		"status":           string(session.Status),
	}
	pipe.HMSet(ctx, metaKey, metaData)
	pipe.Expire(ctx, metaKey, ttl)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// Delete deletes a session
func (r *SessionRepository) Delete(ctx context.Context, sessionID string) error {
	pipe := r.client.Pipeline()

	// Get session to find user ID
	sessionKey := fmt.Sprintf("session:%s", sessionID)
	session, err := r.GetByID(ctx, sessionID)
	if err != nil {
		if err == domain.ErrSessionNotFound {
			return nil // Already deleted
		}
		return err
	}

	// Delete session data
	pipe.Del(ctx, sessionKey)

	// Remove from user sessions set
	userSessionsKey := domain.GetUserSessionsKey(session.UserID)
	pipe.SRem(ctx, userSessionsKey, sessionID)

	// Remove from active sessions set
	pipe.SRem(ctx, "active_sessions", sessionID)

	// Delete session metadata
	metaKey := fmt.Sprintf("session_meta:%s", sessionID)
	pipe.Del(ctx, metaKey)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

// ValidateSession validates a session with session ID and access token
func (r *SessionRepository) ValidateSession(ctx context.Context, sessionID, accessToken string) (*domain.Session, error) {
	session, err := r.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Validate session
	if err := session.IsValid(); err != nil {
		return nil, err
	}

	// Validate access token
	if session.AccessToken != accessToken {
		return nil, domain.ErrInvalidToken
	}

	// Update last accessed time
	session.UpdateLastAccessed()
	if err := r.Update(ctx, session); err != nil {
		// Log error but don't fail validation
		// This is a minor issue and shouldn't prevent access
	}

	return session, nil
}

// RefreshSession validates and refreshes a session using refresh token
func (r *SessionRepository) RefreshSession(ctx context.Context, sessionID, refreshToken string) (*domain.Session, error) {
	session, err := r.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	// Validate refresh token
	if err := session.IsRefreshTokenValid(); err != nil {
		return nil, err
	}

	if session.RefreshToken != refreshToken {
		return nil, domain.ErrInvalidRefreshToken
	}

	return session, nil
}

// GetUserSessions retrieves all sessions for a user
func (r *SessionRepository) GetUserSessions(ctx context.Context, userID string) ([]*domain.Session, error) {
	userSessionsKey := domain.GetUserSessionsKey(userID)
	sessionIDs, err := r.client.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user session IDs: %w", err)
	}

	var sessions []*domain.Session
	for _, sessionID := range sessionIDs {
		session, err := r.GetByID(ctx, sessionID)
		if err != nil {
			if err == domain.ErrSessionNotFound {
				// Remove stale session ID from set
				r.client.SRem(ctx, userSessionsKey, sessionID)
				continue
			}
			return nil, err
		}
		sessions = append(sessions, session)
	}

	return sessions, nil
}

// GetActiveUserSessions retrieves active sessions for a user
func (r *SessionRepository) GetActiveUserSessions(ctx context.Context, userID string) ([]*domain.Session, error) {
	allSessions, err := r.GetUserSessions(ctx, userID)
	if err != nil {
		return nil, err
	}

	var activeSessions []*domain.Session
	for _, session := range allSessions {
		if session.IsActive() {
			activeSessions = append(activeSessions, session)
		}
	}

	return activeSessions, nil
}

// RevokeUserSessions revokes all sessions for a user
func (r *SessionRepository) RevokeUserSessions(ctx context.Context, userID string) error {
	sessions, err := r.GetUserSessions(ctx, userID)
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()
	for _, session := range sessions {
		session.Revoke()

		// Update session
		sessionData, _ := json.Marshal(session)
		sessionKey := session.GetSessionKey()
		pipe.Set(ctx, sessionKey, sessionData, time.Until(session.ExpiresAt))

		// Update metadata
		metaKey := fmt.Sprintf("session_meta:%s", session.ID)
		pipe.HSet(ctx, metaKey, "status", string(session.Status))
	}

	_, err = pipe.Exec(ctx)
	return err
}

// RevokeUserSessionsExcept revokes all user sessions except the specified one
func (r *SessionRepository) RevokeUserSessionsExcept(ctx context.Context, userID, keepSessionID string) error {
	sessions, err := r.GetUserSessions(ctx, userID)
	if err != nil {
		return err
	}

	pipe := r.client.Pipeline()
	for _, session := range sessions {
		if session.ID == keepSessionID {
			continue
		}

		session.Revoke()

		// Update session
		sessionData, _ := json.Marshal(session)
		sessionKey := session.GetSessionKey()
		pipe.Set(ctx, sessionKey, sessionData, time.Until(session.ExpiresAt))

		// Update metadata
		metaKey := fmt.Sprintf("session_meta:%s", session.ID)
		pipe.HSet(ctx, metaKey, "status", string(session.Status))
	}

	_, err = pipe.Exec(ctx)
	return err
}

// RevokeSession revokes a specific session
func (r *SessionRepository) RevokeSession(ctx context.Context, sessionID string) error {
	session, err := r.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Revoke()
	return r.Update(ctx, session)
}

// ExpireSession expires a specific session
func (r *SessionRepository) ExpireSession(ctx context.Context, sessionID string) error {
	session, err := r.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Expire()
	return r.Update(ctx, session)
}

// InvalidateSession invalidates a specific session
func (r *SessionRepository) InvalidateSession(ctx context.Context, sessionID string) error {
	session, err := r.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	session.Invalidate()
	return r.Update(ctx, session)
}

// UpdateLastAccessed updates the last accessed time for a session
func (r *SessionRepository) UpdateLastAccessed(ctx context.Context, sessionID string) error {
	session, err := r.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	session.UpdateLastAccessed()
	return r.Update(ctx, session)
}

// BlacklistToken adds a token to the blacklist
func (r *SessionRepository) BlacklistToken(ctx context.Context, tokenID string, expiresAt time.Time) error {
	blacklistKey := domain.GetTokenBlacklistKey(tokenID)
	ttl := time.Until(expiresAt)
	if ttl <= 0 {
		return nil // Token already expired, no need to blacklist
	}

	err := r.client.Set(ctx, blacklistKey, "blacklisted", ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to blacklist token: %w", err)
	}

	// Add to blacklist set for easier cleanup
	r.client.SAdd(ctx, "blacklisted_tokens", tokenID)
	r.client.Expire(ctx, "blacklisted_tokens", ttl)

	return nil
}

// IsTokenBlacklisted checks if a token is blacklisted
func (r *SessionRepository) IsTokenBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	blacklistKey := domain.GetTokenBlacklistKey(tokenID)
	exists, err := r.client.Exists(ctx, blacklistKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check token blacklist: %w", err)
	}

	return exists > 0, nil
}

// GetBlacklistedTokens retrieves all blacklisted tokens
func (r *SessionRepository) GetBlacklistedTokens(ctx context.Context) ([]string, error) {
	tokens, err := r.client.SMembers(ctx, "blacklisted_tokens").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get blacklisted tokens: %w", err)
	}

	return tokens, nil
}

// CleanupBlacklistedTokens removes expired blacklisted tokens
func (r *SessionRepository) CleanupBlacklistedTokens(ctx context.Context) (int, error) {
	tokens, err := r.GetBlacklistedTokens(ctx)
	if err != nil {
		return 0, err
	}

	cleaned := 0
	pipe := r.client.Pipeline()

	for _, tokenID := range tokens {
		blacklistKey := domain.GetTokenBlacklistKey(tokenID)
		exists, err := r.client.Exists(ctx, blacklistKey).Result()
		if err != nil {
			continue
		}

		if exists == 0 {
			// Token expired, remove from set
			pipe.SRem(ctx, "blacklisted_tokens", tokenID)
			cleaned++
		}
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup blacklisted tokens: %w", err)
	}

	return cleaned, nil
}

// GetSessionsByStatus retrieves sessions by status
func (r *SessionRepository) GetSessionsByStatus(ctx context.Context, status domain.SessionStatus) ([]*domain.Session, error) {
	// Get all active session IDs
	sessionIDs, err := r.client.SMembers(ctx, "active_sessions").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	var sessions []*domain.Session
	for _, sessionID := range sessionIDs {
		metaKey := fmt.Sprintf("session_meta:%s", sessionID)
		sessionStatus, err := r.client.HGet(ctx, metaKey, "status").Result()
		if err != nil {
			if err == redis.Nil {
				// Clean up stale session ID
				r.client.SRem(ctx, "active_sessions", sessionID)
				continue
			}
			continue
		}

		if sessionStatus == string(status) {
			session, err := r.GetByID(ctx, sessionID)
			if err != nil {
				continue
			}
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// GetExpiredSessions retrieves expired sessions
func (r *SessionRepository) GetExpiredSessions(ctx context.Context) ([]*domain.Session, error) {
	return r.GetSessionsByStatus(ctx, domain.SessionStatusExpired)
}

// GetSessionsByUserAgent retrieves sessions by user agent
func (r *SessionRepository) GetSessionsByUserAgent(ctx context.Context, userAgent string) ([]*domain.Session, error) {
	sessionIDs, err := r.client.SMembers(ctx, "active_sessions").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	var sessions []*domain.Session
	for _, sessionID := range sessionIDs {
		metaKey := fmt.Sprintf("session_meta:%s", sessionID)
		sessionUserAgent, err := r.client.HGet(ctx, metaKey, "user_agent").Result()
		if err != nil {
			continue
		}

		if strings.Contains(strings.ToLower(sessionUserAgent), strings.ToLower(userAgent)) {
			session, err := r.GetByID(ctx, sessionID)
			if err != nil {
				continue
			}
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// GetSessionsByIPAddress retrieves sessions by IP address
func (r *SessionRepository) GetSessionsByIPAddress(ctx context.Context, ipAddress string) ([]*domain.Session, error) {
	sessionIDs, err := r.client.SMembers(ctx, "active_sessions").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	var sessions []*domain.Session
	for _, sessionID := range sessionIDs {
		metaKey := fmt.Sprintf("session_meta:%s", sessionID)
		sessionIP, err := r.client.HGet(ctx, metaKey, "ip_address").Result()
		if err != nil {
			continue
		}

		if sessionIP == ipAddress {
			session, err := r.GetByID(ctx, sessionID)
			if err != nil {
				continue
			}
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// CleanupExpiredSessions removes expired sessions
func (r *SessionRepository) CleanupExpiredSessions(ctx context.Context) (*domain.SessionCleanupInfo, error) {
	info := &domain.SessionCleanupInfo{}

	// Get all session IDs from active_sessions set
	sessionIDs, err := r.client.SMembers(ctx, "active_sessions").Result()
	if err != nil {
		return info, fmt.Errorf("failed to get active sessions: %w", err)
	}

	pipe := r.client.Pipeline()
	cleanupCount := 0

	for _, sessionID := range sessionIDs {
		sessionKey := fmt.Sprintf("session:%s", sessionID)
		exists, err := r.client.Exists(ctx, sessionKey).Result()
		if err != nil {
			continue
		}

		if exists == 0 {
			// Session expired or deleted
			pipe.SRem(ctx, "active_sessions", sessionID)

			// Clean up metadata
			metaKey := fmt.Sprintf("session_meta:%s", sessionID)
			pipe.Del(ctx, metaKey)

			info.ExpiredSessions++
			cleanupCount++
		} else {
			// Check if session is expired but not yet cleaned up by Redis
			session, err := r.GetByID(ctx, sessionID)
			if err != nil {
				continue
			}

			if session.IsExpired() {
				r.Delete(ctx, sessionID)
				info.ExpiredSessions++
				cleanupCount++
			} else {
				// Check status
				switch session.Status {
				case domain.SessionStatusRevoked:
					info.RevokedSessions++
				case domain.SessionStatusInvalid:
					info.InvalidSessions++
				}
			}
		}
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return info, fmt.Errorf("failed to cleanup sessions: %w", err)
	}

	info.TotalCleaned = cleanupCount
	return info, nil
}

// CleanupUserSessions limits the number of sessions per user
func (r *SessionRepository) CleanupUserSessions(ctx context.Context, userID string, maxSessions int) error {
	sessions, err := r.GetUserSessions(ctx, userID)
	if err != nil {
		return err
	}

	if len(sessions) <= maxSessions {
		return nil // No cleanup needed
	}

	// Sort sessions by last accessed time (oldest first)
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[i].LastAccessedAt.After(sessions[j].LastAccessedAt) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	// Remove oldest sessions
	sessionsToRemove := len(sessions) - maxSessions
	for i := 0; i < sessionsToRemove; i++ {
		err := r.Delete(ctx, sessions[i].ID)
		if err != nil {
			return fmt.Errorf("failed to delete session %s: %w", sessions[i].ID, err)
		}
	}

	return nil
}

// GetStaleSessionsForCleanup retrieves sessions that haven't been accessed recently
func (r *SessionRepository) GetStaleSessionsForCleanup(ctx context.Context, staleSince time.Time) ([]*domain.Session, error) {
	sessionIDs, err := r.client.SMembers(ctx, "active_sessions").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	var staleSessions []*domain.Session
	staleTimestamp := staleSince.Unix()

	for _, sessionID := range sessionIDs {
		metaKey := fmt.Sprintf("session_meta:%s", sessionID)
		lastAccessedStr, err := r.client.HGet(ctx, metaKey, "last_accessed_at").Result()
		if err != nil {
			continue
		}

		lastAccessed, err := strconv.ParseInt(lastAccessedStr, 10, 64)
		if err != nil {
			continue
		}

		if lastAccessed < staleTimestamp {
			session, err := r.GetByID(ctx, sessionID)
			if err != nil {
				continue
			}
			staleSessions = append(staleSessions, session)
		}
	}

	return staleSessions, nil
}

// GetSessionStats returns session statistics
func (r *SessionRepository) GetSessionStats(ctx context.Context) (*interfaces.SessionStats, error) {
	stats := &interfaces.SessionStats{
		SessionsByStatus: make(map[domain.SessionStatus]int),
	}

	// Get active session count
	activeCount, err := r.client.SCard(ctx, "active_sessions").Result()
	if err != nil {
		return stats, fmt.Errorf("failed to get active session count: %w", err)
	}
	stats.TotalSessions = int(activeCount)

	// Get blacklisted token count
	blacklistCount, err := r.client.SCard(ctx, "blacklisted_tokens").Result()
	if err == nil {
		stats.BlacklistedTokens = int(blacklistCount)
	}

	// Get detailed stats by examining session metadata
	sessionIDs, err := r.client.SMembers(ctx, "active_sessions").Result()
	if err != nil {
		return stats, err
	}

	now := time.Now()
	uniqueUsers := make(map[string]bool)

	for _, sessionID := range sessionIDs {
		metaKey := fmt.Sprintf("session_meta:%s", sessionID)
		metadata, err := r.client.HGetAll(ctx, metaKey).Result()
		if err != nil {
			continue
		}

		// Count by status
		if status, ok := metadata["status"]; ok {
			stats.SessionsByStatus[domain.SessionStatus(status)]++
			switch domain.SessionStatus(status) {
			case domain.SessionStatusActive:
				stats.ActiveSessions++
			case domain.SessionStatusExpired:
				stats.ExpiredSessions++
			case domain.SessionStatusRevoked:
				stats.RevokedSessions++
			case domain.SessionStatusInvalid:
				stats.InvalidSessions++
			}
		}

		// Count unique users
		if userID, ok := metadata["user_id"]; ok {
			uniqueUsers[userID] = true
		}

		// Count recent sessions (last hour)
		if createdAtStr, ok := metadata["created_at"]; ok {
			createdAt, err := strconv.ParseInt(createdAtStr, 10, 64)
			if err == nil {
				createdTime := time.Unix(createdAt, 0)
				if now.Sub(createdTime) <= time.Hour {
					stats.RecentSessions++
				}
				if now.Sub(createdTime) <= 24*time.Hour {
					stats.TodaySessions++
				}
				if now.Sub(createdTime) <= 7*24*time.Hour {
					stats.WeekSessions++
				}
			}
		}
	}

	stats.UniqueActiveUsers = len(uniqueUsers)
	return stats, nil
}

// GetActiveSessionCount returns the number of active sessions
func (r *SessionRepository) GetActiveSessionCount(ctx context.Context) (int, error) {
	count, err := r.client.SCard(ctx, "active_sessions").Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get active session count: %w", err)
	}

	return int(count), nil
}

// GetUserSessionCount returns the number of sessions for a user
func (r *SessionRepository) GetUserSessionCount(ctx context.Context, userID string) (int, error) {
	userSessionsKey := domain.GetUserSessionsKey(userID)
	count, err := r.client.SCard(ctx, userSessionsKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get user session count: %w", err)
	}

	return int(count), nil
}

// GetSessionsByTimeRange retrieves sessions created within a time range
func (r *SessionRepository) GetSessionsByTimeRange(ctx context.Context, start, end time.Time) ([]*domain.Session, error) {
	sessionIDs, err := r.client.SMembers(ctx, "active_sessions").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	var sessions []*domain.Session
	startTimestamp := start.Unix()
	endTimestamp := end.Unix()

	for _, sessionID := range sessionIDs {
		metaKey := fmt.Sprintf("session_meta:%s", sessionID)
		createdAtStr, err := r.client.HGet(ctx, metaKey, "created_at").Result()
		if err != nil {
			continue
		}

		createdAt, err := strconv.ParseInt(createdAtStr, 10, 64)
		if err != nil {
			continue
		}

		if createdAt >= startTimestamp && createdAt <= endTimestamp {
			session, err := r.GetByID(ctx, sessionID)
			if err != nil {
				continue
			}
			sessions = append(sessions, session)
		}
	}

	return sessions, nil
}

// CreateBatch creates multiple sessions in a batch
func (r *SessionRepository) CreateBatch(ctx context.Context, sessions []*domain.Session) error {
	pipe := r.client.Pipeline()

	for _, session := range sessions {
		sessionData, err := json.Marshal(session)
		if err != nil {
			return fmt.Errorf("failed to marshal session %s: %w", session.ID, err)
		}

		ttl := time.Until(session.ExpiresAt)
		if ttl <= 0 {
			continue // Skip expired sessions
		}

		sessionKey := session.GetSessionKey()
		pipe.Set(ctx, sessionKey, sessionData, ttl)

		userSessionsKey := domain.GetUserSessionsKey(session.UserID)
		pipe.SAdd(ctx, userSessionsKey, session.ID)
		pipe.Expire(ctx, userSessionsKey, ttl)

		pipe.SAdd(ctx, "active_sessions", session.ID)

		metaKey := fmt.Sprintf("session_meta:%s", session.ID)
		metaData := map[string]interface{}{
			"user_id":    session.UserID,
			"created_at": session.CreatedAt.Unix(),
			"expires_at": session.ExpiresAt.Unix(),
			"ip_address": session.IPAddress,
			"user_agent": session.UserAgent,
			"status":     string(session.Status),
		}
		pipe.HMSet(ctx, metaKey, metaData)
		pipe.Expire(ctx, metaKey, ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to create sessions batch: %w", err)
	}

	return nil
}

// DeleteBatch deletes multiple sessions in a batch
func (r *SessionRepository) DeleteBatch(ctx context.Context, sessionIDs []string) error {
	pipe := r.client.Pipeline()

	for _, sessionID := range sessionIDs {
		session, err := r.GetByID(ctx, sessionID)
		if err != nil {
			continue // Skip non-existent sessions
		}

		sessionKey := fmt.Sprintf("session:%s", sessionID)
		pipe.Del(ctx, sessionKey)

		userSessionsKey := domain.GetUserSessionsKey(session.UserID)
		pipe.SRem(ctx, userSessionsKey, sessionID)

		pipe.SRem(ctx, "active_sessions", sessionID)

		metaKey := fmt.Sprintf("session_meta:%s", sessionID)
		pipe.Del(ctx, metaKey)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete sessions batch: %w", err)
	}

	return nil
}

// UpdateBatch updates multiple sessions in a batch
func (r *SessionRepository) UpdateBatch(ctx context.Context, sessions []*domain.Session) error {
	pipe := r.client.Pipeline()

	for _, session := range sessions {
		sessionData, err := json.Marshal(session)
		if err != nil {
			continue
		}

		ttl := time.Until(session.ExpiresAt)
		if ttl <= 0 {
			continue
		}

		sessionKey := session.GetSessionKey()
		pipe.Set(ctx, sessionKey, sessionData, ttl)

		metaKey := fmt.Sprintf("session_meta:%s", session.ID)
		metaData := map[string]interface{}{
			"user_id":          session.UserID,
			"created_at":       session.CreatedAt.Unix(),
			"expires_at":       session.ExpiresAt.Unix(),
			"last_accessed_at": session.LastAccessedAt.Unix(),
			"ip_address":       session.IPAddress,
			"user_agent":       session.UserAgent,
			"status":           string(session.Status),
		}
		pipe.HMSet(ctx, metaKey, metaData)
		pipe.Expire(ctx, metaKey, ttl)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update sessions batch: %w", err)
	}

	return nil
}

// ExtendSession extends the session duration
func (r *SessionRepository) ExtendSession(ctx context.Context, sessionID string, duration time.Duration) error {
	session, err := r.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	session.ExpiresAt = session.ExpiresAt.Add(duration)
	return r.Update(ctx, session)
}

// RenewSession sets a new expiry time for the session
func (r *SessionRepository) RenewSession(ctx context.Context, sessionID string, newExpiryTime time.Time) error {
	session, err := r.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	session.ExpiresAt = newExpiryTime
	return r.Update(ctx, session)
}

// FindSessionsByFilter finds sessions matching the given filter
func (r *SessionRepository) FindSessionsByFilter(ctx context.Context, filter interfaces.SessionFilter) ([]*domain.Session, error) {
	sessionIDs, err := r.client.SMembers(ctx, "active_sessions").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active sessions: %w", err)
	}

	var sessions []*domain.Session
	for _, sessionID := range sessionIDs {
		session, err := r.GetByID(ctx, sessionID)
		if err != nil {
			continue
		}

		if r.matchesFilter(session, filter) {
			sessions = append(sessions, session)
		}
	}

	// Apply pagination
	if filter.Limit > 0 {
		start := filter.Offset
		end := start + filter.Limit
		if start >= len(sessions) {
			return []*domain.Session{}, nil
		}
		if end > len(sessions) {
			end = len(sessions)
		}
		sessions = sessions[start:end]
	}

	return sessions, nil
}

// GetRecentSessions returns the most recently created sessions
func (r *SessionRepository) GetRecentSessions(ctx context.Context, limit int) ([]*domain.Session, error) {
	filter := interfaces.SessionFilter{
		Limit:     limit,
		SortBy:    "created_at",
		SortOrder: "desc",
	}
	return r.FindSessionsByFilter(ctx, filter)
}

// GetLongLivedSessions returns sessions that have been active for longer than the threshold
func (r *SessionRepository) GetLongLivedSessions(ctx context.Context, threshold time.Duration) ([]*domain.Session, error) {
	thresholdTime := time.Now().Add(-threshold)
	filter := interfaces.SessionFilter{
		CreatedBefore: &thresholdTime,
	}
	return r.FindSessionsByFilter(ctx, filter)
}

// GetSuspiciousSessions returns sessions that match suspicious criteria
func (r *SessionRepository) GetSuspiciousSessions(ctx context.Context, criteria interfaces.SuspiciousSessionCriteria) ([]*domain.Session, error) {
	// This is a simplified implementation - in practice, you'd want more sophisticated analysis
	sessionIDs, err := r.client.SMembers(ctx, "active_sessions").Result()
	if err != nil {
		return nil, err
	}

	var suspicious []*domain.Session
	userIPs := make(map[string]map[string]bool) // userID -> set of IPs

	// First pass: collect IP information
	for _, sessionID := range sessionIDs {
		session, err := r.GetByID(ctx, sessionID)
		if err != nil {
			continue
		}

		if userIPs[session.UserID] == nil {
			userIPs[session.UserID] = make(map[string]bool)
		}
		userIPs[session.UserID][session.IPAddress] = true
	}

	// Second pass: identify suspicious sessions
	for _, sessionID := range sessionIDs {
		session, err := r.GetByID(ctx, sessionID)
		if err != nil {
			continue
		}

		isSuspicious := false

		// Check for multiple IPs
		if len(userIPs[session.UserID]) >= criteria.MultipleIPsThreshold {
			isSuspicious = true
		}

		// Check for long duration
		if time.Since(session.CreatedAt) > criteria.LongDurationThreshold {
			isSuspicious = true
		}

		// Check for inactivity
		if time.Since(session.LastAccessedAt) > criteria.InactiveThreshold {
			isSuspicious = true
		}

		if isSuspicious {
			suspicious = append(suspicious, session)
		}
	}

	return suspicious, nil
}

// GetSessionsByMultipleIPs returns sessions for a user from multiple IP addresses
func (r *SessionRepository) GetSessionsByMultipleIPs(ctx context.Context, userID string) ([]*domain.Session, error) {
	sessions, err := r.GetUserSessions(ctx, userID)
	if err != nil {
		return nil, err
	}

	ipMap := make(map[string]bool)
	for _, session := range sessions {
		ipMap[session.IPAddress] = true
	}

	if len(ipMap) <= 1 {
		return []*domain.Session{}, nil
	}

	return sessions, nil
}

// GetConcurrentSessions returns sessions created within a time window for a user
func (r *SessionRepository) GetConcurrentSessions(ctx context.Context, userID string, timeWindow time.Duration) ([]*domain.Session, error) {
	sessions, err := r.GetUserSessions(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	var concurrent []*domain.Session

	for _, session := range sessions {
		if now.Sub(session.CreatedAt) <= timeWindow {
			concurrent = append(concurrent, session)
		}
	}

	return concurrent, nil
}

// HealthCheck checks Redis connection health
func (r *SessionRepository) HealthCheck(ctx context.Context) error {
	_, err := r.client.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis health check failed: %w", err)
	}
	return nil
}

// GetConnectionInfo returns Redis connection information
func (r *SessionRepository) GetConnectionInfo(ctx context.Context) (*interfaces.RedisConnectionInfo, error) {
	info := &interfaces.RedisConnectionInfo{
		Connected: true,
	}

	// Get basic connection info
	poolStats := r.client.PoolStats()
	info.ActiveConns = int(poolStats.Hits)
	info.IdleConns = int(poolStats.Misses)

	// Ping to measure latency
	start := time.Now()
	_, err := r.client.Ping(ctx).Result()
	if err != nil {
		info.Connected = false
		return info, err
	}
	info.Latency = time.Since(start)

	// Get Redis info
	redisInfo, err := r.client.Info(ctx).Result()
	if err == nil {
		// Parse Redis info for version and memory details
		lines := strings.Split(redisInfo, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "redis_version:") {
				info.RedisVersion = strings.TrimPrefix(line, "redis_version:")
			}
		}
	}

	return info, nil
}

// Helper function to check if a session matches the filter criteria
func (r *SessionRepository) matchesFilter(session *domain.Session, filter interfaces.SessionFilter) bool {
	if filter.UserID != nil && session.UserID != *filter.UserID {
		return false
	}

	if filter.Status != nil && session.Status != *filter.Status {
		return false
	}

	if filter.IPAddress != nil && session.IPAddress != *filter.IPAddress {
		return false
	}

	if filter.UserAgent != nil && !strings.Contains(strings.ToLower(session.UserAgent), strings.ToLower(*filter.UserAgent)) {
		return false
	}

	if filter.CreatedAfter != nil && session.CreatedAt.Before(*filter.CreatedAfter) {
		return false
	}

	if filter.CreatedBefore != nil && session.CreatedAt.After(*filter.CreatedBefore) {
		return false
	}

	if filter.ExpiresAfter != nil && session.ExpiresAt.Before(*filter.ExpiresAfter) {
		return false
	}

	if filter.ExpiresBefore != nil && session.ExpiresAt.After(*filter.ExpiresBefore) {
		return false
	}

	if filter.AccessedAfter != nil && session.LastAccessedAt.Before(*filter.AccessedAfter) {
		return false
	}

	if filter.AccessedBefore != nil && session.LastAccessedAt.After(*filter.AccessedBefore) {
		return false
	}

	return true
}
