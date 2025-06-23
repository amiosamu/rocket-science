package interfaces

import (
	"context"
	"time"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/domain"
)

// SessionRepository defines the interface for session data persistence
type SessionRepository interface {
	// Basic session operations
	Create(ctx context.Context, session *domain.Session) error
	GetByID(ctx context.Context, sessionID string) (*domain.Session, error)
	Update(ctx context.Context, session *domain.Session) error
	Delete(ctx context.Context, sessionID string) error

	// Session validation
	ValidateSession(ctx context.Context, sessionID, accessToken string) (*domain.Session, error)
	RefreshSession(ctx context.Context, sessionID, refreshToken string) (*domain.Session, error)

	// User session management
	GetUserSessions(ctx context.Context, userID string) ([]*domain.Session, error)
	GetActiveUserSessions(ctx context.Context, userID string) ([]*domain.Session, error)
	RevokeUserSessions(ctx context.Context, userID string) error
	RevokeUserSessionsExcept(ctx context.Context, userID, keepSessionID string) error

	// Session status management
	RevokeSession(ctx context.Context, sessionID string) error
	ExpireSession(ctx context.Context, sessionID string) error
	InvalidateSession(ctx context.Context, sessionID string) error
	UpdateLastAccessed(ctx context.Context, sessionID string) error

	// Token management
	BlacklistToken(ctx context.Context, tokenID string, expiresAt time.Time) error
	IsTokenBlacklisted(ctx context.Context, tokenID string) (bool, error)
	GetBlacklistedTokens(ctx context.Context) ([]string, error)
	CleanupBlacklistedTokens(ctx context.Context) (int, error)

	// Session queries
	GetSessionsByStatus(ctx context.Context, status domain.SessionStatus) ([]*domain.Session, error)
	GetExpiredSessions(ctx context.Context) ([]*domain.Session, error)
	GetSessionsByUserAgent(ctx context.Context, userAgent string) ([]*domain.Session, error)
	GetSessionsByIPAddress(ctx context.Context, ipAddress string) ([]*domain.Session, error)

	// Session cleanup and maintenance
	CleanupExpiredSessions(ctx context.Context) (*domain.SessionCleanupInfo, error)
	CleanupUserSessions(ctx context.Context, userID string, maxSessions int) error
	GetStaleSessionsForCleanup(ctx context.Context, staleSince time.Time) ([]*domain.Session, error)

	// Session statistics
	GetSessionStats(ctx context.Context) (*SessionStats, error)
	GetActiveSessionCount(ctx context.Context) (int, error)
	GetUserSessionCount(ctx context.Context, userID string) (int, error)
	GetSessionsByTimeRange(ctx context.Context, start, end time.Time) ([]*domain.Session, error)

	// Batch operations
	CreateBatch(ctx context.Context, sessions []*domain.Session) error
	DeleteBatch(ctx context.Context, sessionIDs []string) error
	UpdateBatch(ctx context.Context, sessions []*domain.Session) error

	// Session extension and renewal
	ExtendSession(ctx context.Context, sessionID string, duration time.Duration) error
	RenewSession(ctx context.Context, sessionID string, newExpiryTime time.Time) error

	// Advanced queries
	FindSessionsByFilter(ctx context.Context, filter SessionFilter) ([]*domain.Session, error)
	GetRecentSessions(ctx context.Context, limit int) ([]*domain.Session, error)
	GetLongLivedSessions(ctx context.Context, threshold time.Duration) ([]*domain.Session, error)

	// Security operations
	GetSuspiciousSessions(ctx context.Context, criteria SuspiciousSessionCriteria) ([]*domain.Session, error)
	GetSessionsByMultipleIPs(ctx context.Context, userID string) ([]*domain.Session, error)
	GetConcurrentSessions(ctx context.Context, userID string, timeWindow time.Duration) ([]*domain.Session, error)

	// Health and monitoring
	HealthCheck(ctx context.Context) error
	GetConnectionInfo(ctx context.Context) (*RedisConnectionInfo, error)
}

// SessionFilter defines filtering options for session queries
type SessionFilter struct {
	UserID    *string               `json:"user_id,omitempty"`
	Status    *domain.SessionStatus `json:"status,omitempty"`
	IPAddress *string               `json:"ip_address,omitempty"`
	UserAgent *string               `json:"user_agent,omitempty"`

	// Time filters
	CreatedAfter   *time.Time `json:"created_after,omitempty"`
	CreatedBefore  *time.Time `json:"created_before,omitempty"`
	ExpiresAfter   *time.Time `json:"expires_after,omitempty"`
	ExpiresBefore  *time.Time `json:"expires_before,omitempty"`
	AccessedAfter  *time.Time `json:"accessed_after,omitempty"`
	AccessedBefore *time.Time `json:"accessed_before,omitempty"`

	// Duration filters
	MinDuration *time.Duration `json:"min_duration,omitempty"`
	MaxDuration *time.Duration `json:"max_duration,omitempty"`

	// Pagination
	Limit  int `json:"limit"`
	Offset int `json:"offset"`

	// Sorting
	SortBy    string `json:"sort_by"`    // "created_at", "expires_at", "last_accessed_at"
	SortOrder string `json:"sort_order"` // "asc", "desc"
}

// SessionStats provides statistics about sessions
type SessionStats struct {
	TotalSessions    int                          `json:"total_sessions"`
	ActiveSessions   int                          `json:"active_sessions"`
	ExpiredSessions  int                          `json:"expired_sessions"`
	RevokedSessions  int                          `json:"revoked_sessions"`
	InvalidSessions  int                          `json:"invalid_sessions"`
	SessionsByStatus map[domain.SessionStatus]int `json:"sessions_by_status"`

	// Time-based stats
	RecentSessions int `json:"recent_sessions"` // Last hour
	TodaySessions  int `json:"today_sessions"`  // Today
	WeekSessions   int `json:"week_sessions"`   // This week

	// User activity
	UniqueActiveUsers  int           `json:"unique_active_users"`
	AvgSessionDuration time.Duration `json:"avg_session_duration"`

	// Security metrics
	MultipleIPSessions int `json:"multiple_ip_sessions"`
	SuspiciousSessions int `json:"suspicious_sessions"`
	BlacklistedTokens  int `json:"blacklisted_tokens"`

	// Performance metrics
	CleanupCandidates int `json:"cleanup_candidates"`
	LongLivedSessions int `json:"long_lived_sessions"`
}

// SuspiciousSessionCriteria defines criteria for identifying suspicious sessions
type SuspiciousSessionCriteria struct {
	MultipleIPsThreshold     int           `json:"multiple_ips_threshold"` // Sessions from multiple IPs
	RapidLoginThreshold      time.Duration `json:"rapid_login_threshold"`  // Multiple logins in short time
	UnusualUserAgentPatterns []string      `json:"unusual_user_agent_patterns"`
	GeographicAnomalies      bool          `json:"geographic_anomalies"`
	LongDurationThreshold    time.Duration `json:"long_duration_threshold"` // Unusually long sessions
	InactiveThreshold        time.Duration `json:"inactive_threshold"`      // Long inactive sessions
}

// RedisConnectionInfo provides information about Redis connection status
type RedisConnectionInfo struct {
	Connected    bool             `json:"connected"`
	Address      string           `json:"address"`
	Database     int              `json:"database"`
	PoolSize     int              `json:"pool_size"`
	ActiveConns  int              `json:"active_connections"`
	IdleConns    int              `json:"idle_connections"`
	Latency      time.Duration    `json:"latency"`
	TotalCmds    int64            `json:"total_commands"`
	FailedCmds   int64            `json:"failed_commands"`
	RedisVersion string           `json:"redis_version"`
	Memory       *RedisMemoryInfo `json:"memory,omitempty"`
}

// RedisMemoryInfo provides information about Redis memory usage
type RedisMemoryInfo struct {
	Used      int64   `json:"used_memory"`
	Peak      int64   `json:"used_memory_peak"`
	Total     int64   `json:"total_system_memory"`
	UsedRatio float64 `json:"used_memory_ratio"`
}

// SessionBatch represents a batch of session operations
type SessionBatch struct {
	Create []CreateSessionRequest `json:"create,omitempty"`
	Update []UpdateSessionRequest `json:"update,omitempty"`
	Delete []string               `json:"delete,omitempty"`
}

// CreateSessionRequest represents a request to create a session
type CreateSessionRequest struct {
	Session *domain.Session `json:"session"`
	TTL     *time.Duration  `json:"ttl,omitempty"`
}

// UpdateSessionRequest represents a request to update a session
type UpdateSessionRequest struct {
	SessionID string          `json:"session_id"`
	Session   *domain.Session `json:"session"`
	TTL       *time.Duration  `json:"ttl,omitempty"`
}

// SessionCleanupResult represents the result of a session cleanup operation
type SessionCleanupResult struct {
	CleanupInfo    *domain.SessionCleanupInfo `json:"cleanup_info"`
	ProcessedCount int                        `json:"processed_count"`
	ErrorCount     int                        `json:"error_count"`
	Duration       time.Duration              `json:"duration"`
	Errors         []string                   `json:"errors,omitempty"`
}
