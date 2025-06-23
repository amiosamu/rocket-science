package interfaces

import (
	"context"
	"time"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/domain"
)

// UserRepository defines the interface for user data persistence
type UserRepository interface {
	// Basic CRUD operations
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id string) error

	// User listing and search
	List(ctx context.Context, filter UserFilter) ([]*domain.User, int, error)
	Search(ctx context.Context, query string, filter UserFilter) ([]*domain.User, int, error)

	// Authentication and security
	ValidateCredentials(ctx context.Context, email, password string) (*domain.User, error)
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	RecordLoginAttempt(ctx context.Context, userID string) error
	ResetLoginAttempts(ctx context.Context, userID string) error
	LockAccount(ctx context.Context, userID string, lockUntil time.Time) error
	UnlockAccount(ctx context.Context, userID string) error
	UpdateLastLogin(ctx context.Context, userID string, loginTime time.Time) error

	// Profile management
	UpdateProfile(ctx context.Context, userID string, updates ProfileUpdate) error
	UpdateTelegramInfo(ctx context.Context, userID, chatID, username string) error
	GetTelegramInfo(ctx context.Context, userID string) (chatID, username string, err error)

	// Role and status management
	UpdateRole(ctx context.Context, userID string, role domain.UserRole) error
	UpdateStatus(ctx context.Context, userID string, status domain.UserStatus) error
	GetUsersByRole(ctx context.Context, role domain.UserRole) ([]*domain.User, error)
	GetUsersByStatus(ctx context.Context, status domain.UserStatus) ([]*domain.User, error)

	// Metadata management
	UpdateMetadata(ctx context.Context, userID string, metadata map[string]string) error

	// Existence checks
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	ExistsByID(ctx context.Context, id string) (bool, error)

	// Statistics and monitoring
	GetTotalUsers(ctx context.Context) (int, error)
	GetUserStats(ctx context.Context) (*UserStats, error)
	GetLockedUsers(ctx context.Context) ([]*domain.User, error)
	GetRecentUsers(ctx context.Context, limit int) ([]*domain.User, error)

	// Cleanup operations
	DeleteInactiveUsers(ctx context.Context, inactiveSince time.Time) (int, error)
	GetUsersForCleanup(ctx context.Context, criteria CleanupCriteria) ([]*domain.User, error)
}

// UserFilter defines filtering options for user queries
type UserFilter struct {
	Role            *domain.UserRole   `json:"role,omitempty"`
	Status          *domain.UserStatus `json:"status,omitempty"`
	CreatedAfter    *time.Time         `json:"created_after,omitempty"`
	CreatedBefore   *time.Time         `json:"created_before,omitempty"`
	LastLoginAfter  *time.Time         `json:"last_login_after,omitempty"`
	LastLoginBefore *time.Time         `json:"last_login_before,omitempty"`
	IsLocked        *bool              `json:"is_locked,omitempty"`

	// Pagination
	Limit  int `json:"limit"`
	Offset int `json:"offset"`

	// Sorting
	SortBy    string `json:"sort_by"`    // "created_at", "updated_at", "last_login_at", "email", "name"
	SortOrder string `json:"sort_order"` // "asc", "desc"
}

// ProfileUpdate defines the fields that can be updated in a user profile
type ProfileUpdate struct {
	FirstName        *string `json:"first_name,omitempty"`
	LastName         *string `json:"last_name,omitempty"`
	Phone            *string `json:"phone,omitempty"`
	TelegramUsername *string `json:"telegram_username,omitempty"`
	TelegramChatID   *string `json:"telegram_chat_id,omitempty"`
}

// UserStats provides statistics about users
type UserStats struct {
	TotalUsers        int                     `json:"total_users"`
	ActiveUsers       int                     `json:"active_users"`
	InactiveUsers     int                     `json:"inactive_users"`
	SuspendedUsers    int                     `json:"suspended_users"`
	DeletedUsers      int                     `json:"deleted_users"`
	LockedUsers       int                     `json:"locked_users"`
	UsersByRole       map[domain.UserRole]int `json:"users_by_role"`
	RecentSignups     int                     `json:"recent_signups"` // Last 24 hours
	RecentLogins      int                     `json:"recent_logins"`  // Last 24 hours
	UsersWithTelegram int                     `json:"users_with_telegram"`
}

// CleanupCriteria defines criteria for identifying users that need cleanup
type CleanupCriteria struct {
	InactiveSince    time.Time          `json:"inactive_since"`
	DeletedBefore    time.Time          `json:"deleted_before"`
	NeverLoggedIn    bool               `json:"never_logged_in"`
	Status           *domain.UserStatus `json:"status,omitempty"`
	IncludeTestUsers bool               `json:"include_test_users"`
}
