package interfaces

import (
	"context"
	"time"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/domain"
)

// AuthService defines the interface for authentication operations
type AuthService interface {
	// Authentication
	Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error)
	Logout(ctx context.Context, req *LogoutRequest) error
	RefreshToken(ctx context.Context, req *RefreshTokenRequest) (*RefreshTokenResponse, error)

	// Session validation
	ValidateSession(ctx context.Context, sessionID, accessToken string) (*SessionValidationResult, error)
	ValidateToken(ctx context.Context, token string) (*TokenValidationResult, error)

	// Password operations
	ChangePassword(ctx context.Context, req *ChangePasswordRequest) error
	ResetPassword(ctx context.Context, req *ResetPasswordRequest) error

	// Session management
	GetUserSessions(ctx context.Context, userID string) ([]*domain.SessionInfo, error)
	RevokeSession(ctx context.Context, sessionID string) error
	RevokeUserSessions(ctx context.Context, userID string, keepCurrentSession bool) error

	// Security operations
	CheckSuspiciousActivity(ctx context.Context, userID string) (*SecurityCheck, error)
	BlockUserTemporarily(ctx context.Context, userID string, duration time.Duration, reason string) error
}

// UserService defines the interface for user management operations
type UserService interface {
	// User CRUD operations
	CreateUser(ctx context.Context, req *CreateUserRequest) (*CreateUserResponse, error)
	GetUser(ctx context.Context, req *GetUserRequest) (*GetUserResponse, error)
	UpdateUser(ctx context.Context, req *UpdateUserRequest) (*UpdateUserResponse, error)
	DeleteUser(ctx context.Context, req *DeleteUserRequest) error
	ListUsers(ctx context.Context, req *ListUsersRequest) (*ListUsersResponse, error)

	// Profile management
	GetProfile(ctx context.Context, userID string) (*domain.UserProfile, error)
	UpdateProfile(ctx context.Context, req *UpdateProfileRequest) (*UpdateProfileResponse, error)

	// Telegram integration
	GetUserTelegramInfo(ctx context.Context, userID string) (*TelegramInfo, error)
	UpdateTelegramInfo(ctx context.Context, req *UpdateTelegramInfoRequest) error

	// User statistics and monitoring
	GetUserStats(ctx context.Context) (*UserStats, error)
	GetRecentUsers(ctx context.Context, limit int) ([]*domain.User, error)

	// User search and filtering
	SearchUsers(ctx context.Context, query string, filter UserFilter) ([]*domain.User, int, error)
	GetUsersByRole(ctx context.Context, role domain.UserRole) ([]*domain.User, error)

	// Administrative operations
	SuspendUser(ctx context.Context, userID, reason string) error
	ReactivateUser(ctx context.Context, userID string) error
	PromoteUser(ctx context.Context, userID string, newRole domain.UserRole) error
}

// AuthorizationService defines the interface for authorization operations
type AuthorizationService interface {
	// Permission checking
	CheckPermission(ctx context.Context, userID, resource, action string) (*PermissionResult, error)
	GetUserPermissions(ctx context.Context, userID string) ([]string, error)

	// Role-based access control
	HasRole(ctx context.Context, userID string, role domain.UserRole) (bool, error)
	CanAccessResource(ctx context.Context, userID, resource string) (bool, error)

	// Advanced authorization
	CheckResourceOwnership(ctx context.Context, userID, resourceType, resourceID string) (bool, error)
	ValidateAPIAccess(ctx context.Context, userID, apiEndpoint, method string) (bool, error)
}

// SessionService defines the interface for session management operations
type SessionService interface {
	// Session lifecycle
	CreateSession(ctx context.Context, userID, ipAddress, userAgent string) (*domain.Session, error)
	GetSession(ctx context.Context, sessionID string) (*domain.Session, error)
	UpdateSessionActivity(ctx context.Context, sessionID string) error
	ExtendSession(ctx context.Context, sessionID string, duration time.Duration) error

	// Session cleanup and maintenance
	CleanupExpiredSessions(ctx context.Context) (*domain.SessionCleanupInfo, error)
	GetSessionStats(ctx context.Context) (*SessionStats, error)

	// Security monitoring
	DetectSuspiciousSessions(ctx context.Context) ([]*domain.Session, error)
	GetConcurrentSessions(ctx context.Context, userID string) ([]*domain.Session, error)
}

// Request/Response types for Auth Service

type LoginRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8"`
	UserAgent string `json:"user_agent"`
	IPAddress string `json:"ip_address"`
}

type LoginResponse struct {
	Success      bool                `json:"success"`
	Message      string              `json:"message"`
	AccessToken  string              `json:"access_token"`
	RefreshToken string              `json:"refresh_token"`
	SessionID    string              `json:"session_id"`
	User         *domain.User        `json:"user"`
	ExpiresAt    time.Time           `json:"expires_at"`
	SessionInfo  *domain.SessionInfo `json:"session_info"`
}

type LogoutRequest struct {
	SessionID   string `json:"session_id" validate:"required"`
	AccessToken string `json:"access_token" validate:"required"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
	SessionID    string `json:"session_id" validate:"required"`
}

type RefreshTokenResponse struct {
	Success     bool      `json:"success"`
	Message     string    `json:"message"`
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type ChangePasswordRequest struct {
	UserID          string `json:"user_id" validate:"required"`
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

type ResetPasswordRequest struct {
	Email       string `json:"email" validate:"required,email"`
	NewPassword string `json:"new_password" validate:"required,min=8"`
	ResetToken  string `json:"reset_token" validate:"required"`
}

type SessionValidationResult struct {
	Valid   bool            `json:"valid"`
	User    *domain.User    `json:"user,omitempty"`
	Session *domain.Session `json:"session,omitempty"`
	Reason  string          `json:"reason,omitempty"`
}

type TokenValidationResult struct {
	Valid     bool              `json:"valid"`
	Claims    *domain.JWTClaims `json:"claims,omitempty"`
	ExpiresAt time.Time         `json:"expires_at"`
	Reason    string            `json:"reason,omitempty"`
}

type SecurityCheck struct {
	IsSuspicious       bool                `json:"is_suspicious"`
	RiskLevel          string              `json:"risk_level"` // "low", "medium", "high"
	Indicators         []SecurityIndicator `json:"indicators"`
	RecommendedActions []string            `json:"recommended_actions"`
}

type SecurityIndicator struct {
	Type        string    `json:"type"` // "multiple_ips", "rapid_login", "unusual_location"
	Description string    `json:"description"`
	Severity    string    `json:"severity"` // "low", "medium", "high"
	DetectedAt  time.Time `json:"detected_at"`
}

// Request/Response types for User Service

type CreateUserRequest struct {
	Email     string            `json:"email" validate:"required,email"`
	Password  string            `json:"password" validate:"required,min=8"`
	FirstName string            `json:"first_name" validate:"required"`
	LastName  string            `json:"last_name" validate:"required"`
	Role      domain.UserRole   `json:"role" validate:"required"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type CreateUserResponse struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	User    *domain.User `json:"user"`
	UserID  string       `json:"user_id"`
}

type GetUserRequest struct {
	UserID string `json:"user_id,omitempty"`
	Email  string `json:"email,omitempty"`
}

type GetUserResponse struct {
	Found   bool         `json:"found"`
	User    *domain.User `json:"user,omitempty"`
	Message string       `json:"message,omitempty"`
}

type UpdateUserRequest struct {
	UserID    string             `json:"user_id" validate:"required"`
	Email     *string            `json:"email,omitempty"`
	FirstName *string            `json:"first_name,omitempty"`
	LastName  *string            `json:"last_name,omitempty"`
	Role      *domain.UserRole   `json:"role,omitempty"`
	Status    *domain.UserStatus `json:"status,omitempty"`
	Metadata  map[string]string  `json:"metadata,omitempty"`
}

type UpdateUserResponse struct {
	Success bool         `json:"success"`
	Message string       `json:"message"`
	User    *domain.User `json:"user"`
}

type DeleteUserRequest struct {
	UserID string `json:"user_id" validate:"required"`
	Reason string `json:"reason"`
}

type ListUsersRequest struct {
	RoleFilter   *domain.UserRole   `json:"role_filter,omitempty"`
	StatusFilter *domain.UserStatus `json:"status_filter,omitempty"`
	Limit        int                `json:"limit"`
	Offset       int                `json:"offset"`
	SearchQuery  string             `json:"search_query,omitempty"`
}

type ListUsersResponse struct {
	Users      []*domain.User `json:"users"`
	TotalCount int            `json:"total_count"`
	HasMore    bool           `json:"has_more"`
}

type UpdateProfileRequest struct {
	UserID           string            `json:"user_id" validate:"required"`
	FirstName        *string           `json:"first_name,omitempty"`
	LastName         *string           `json:"last_name,omitempty"`
	Phone            *string           `json:"phone,omitempty"`
	TelegramUsername *string           `json:"telegram_username,omitempty"`
	Preferences      map[string]string `json:"preferences,omitempty"`
}

type UpdateProfileResponse struct {
	Success bool                `json:"success"`
	Message string              `json:"message"`
	Profile *domain.UserProfile `json:"profile"`
}

type TelegramInfo struct {
	ChatID   string `json:"chat_id"`
	Username string `json:"username"`
	Found    bool   `json:"found"`
}

type UpdateTelegramInfoRequest struct {
	UserID   string `json:"user_id" validate:"required"`
	ChatID   string `json:"chat_id" validate:"required"`
	Username string `json:"username,omitempty"`
}

// Request/Response types for Authorization Service

type PermissionResult struct {
	Allowed     bool     `json:"allowed"`
	Message     string   `json:"message"`
	Permissions []string `json:"permissions"`
}

// Validation interface for request validation
type RequestValidator interface {
	ValidateLoginRequest(req *LoginRequest) error
	ValidateCreateUserRequest(req *CreateUserRequest) error
	ValidateUpdateUserRequest(req *UpdateUserRequest) error
	ValidateUpdateProfileRequest(req *UpdateProfileRequest) error
	ValidateChangePasswordRequest(req *ChangePasswordRequest) error
}

// Cache interface for caching frequently accessed data
type CacheService interface {
	// User caching
	CacheUser(ctx context.Context, user *domain.User, ttl time.Duration) error
	GetCachedUser(ctx context.Context, userID string) (*domain.User, error)
	InvalidateUserCache(ctx context.Context, userID string) error

	// Permission caching
	CacheUserPermissions(ctx context.Context, userID string, permissions []string, ttl time.Duration) error
	GetCachedUserPermissions(ctx context.Context, userID string) ([]string, error)
	InvalidatePermissionCache(ctx context.Context, userID string) error

	// Session caching
	CacheSessionValidation(ctx context.Context, sessionID string, result *SessionValidationResult, ttl time.Duration) error
	GetCachedSessionValidation(ctx context.Context, sessionID string) (*SessionValidationResult, error)
	InvalidateSessionCache(ctx context.Context, sessionID string) error
}

// Notification interface for sending notifications
type NotificationService interface {
	SendWelcomeEmail(ctx context.Context, user *domain.User) error
	SendPasswordChangeNotification(ctx context.Context, user *domain.User) error
	SendSecurityAlert(ctx context.Context, user *domain.User, alertType string, details map[string]interface{}) error
	SendAccountLockedNotification(ctx context.Context, user *domain.User, duration time.Duration) error
}
