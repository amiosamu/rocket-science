package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/config"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/repository/interfaces"
)

// UserService implements user management business logic
type UserService struct {
	userRepo    interfaces.UserRepository
	sessionRepo interfaces.SessionRepository
	config      *config.Config
}

// NewUserService creates a new user service
func NewUserService(
	userRepo interfaces.UserRepository,
	sessionRepo interfaces.SessionRepository,
	config *config.Config,
) *UserService {
	return &UserService{
		userRepo:    userRepo,
		sessionRepo: sessionRepo,
		config:      config,
	}
}

// CreateUserRequest represents a request to create a new user
type CreateUserRequest struct {
	Email            string          `json:"email"`
	Password         string          `json:"password"`
	FirstName        string          `json:"first_name"`
	LastName         string          `json:"last_name"`
	Role             domain.UserRole `json:"role"`
	Phone            string          `json:"phone,omitempty"`
	TelegramUsername string          `json:"telegram_username,omitempty"`
}

// UpdateUserRequest represents a request to update user information
type UpdateUserRequest struct {
	FirstName        *string            `json:"first_name,omitempty"`
	LastName         *string            `json:"last_name,omitempty"`
	Phone            *string            `json:"phone,omitempty"`
	TelegramUsername *string            `json:"telegram_username,omitempty"`
	Role             *domain.UserRole   `json:"role,omitempty"`
	Status           *domain.UserStatus `json:"status,omitempty"`
}

// UserListOptions represents options for listing users
type UserListOptions struct {
	Role            *domain.UserRole   `json:"role,omitempty"`
	Status          *domain.UserStatus `json:"status,omitempty"`
	Search          string             `json:"search,omitempty"`
	CreatedAfter    *time.Time         `json:"created_after,omitempty"`
	CreatedBefore   *time.Time         `json:"created_before,omitempty"`
	LastLoginAfter  *time.Time         `json:"last_login_after,omitempty"`
	LastLoginBefore *time.Time         `json:"last_login_before,omitempty"`
	IsLocked        *bool              `json:"is_locked,omitempty"`
	Limit           int                `json:"limit"`
	Offset          int                `json:"offset"`
	SortBy          string             `json:"sort_by"`    // "created_at", "updated_at", "last_login_at", "email", "name"
	SortOrder       string             `json:"sort_order"` // "asc", "desc"
}

// UserListResult represents the result of listing users
type UserListResult struct {
	Users []*UserInfo `json:"users"`
	Total int         `json:"total"`
}

// CreateUser creates a new user with validation and business rules
func (s *UserService) CreateUser(ctx context.Context, req *CreateUserRequest) (*UserInfo, error) {
	// Input validation
	if err := s.validateCreateUserRequest(req); err != nil {
		return nil, err
	}

	// Check if email already exists
	exists, err := s.userRepo.ExistsByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to check email existence: %w", err)
	}
	if exists {
		return nil, domain.ErrEmailExists
	}

	// Create user using domain factory
	user, err := domain.NewUser(req.Email, req.Password, req.FirstName, req.LastName, req.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to create user domain object: %w", err)
	}

	// Set optional fields
	if req.Phone != "" {
		user.Phone = strings.TrimSpace(req.Phone)
	}
	if req.TelegramUsername != "" {
		user.TelegramUsername = strings.TrimSpace(req.TelegramUsername)
	}

	// Create user in repository
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return s.userToInfo(user), nil
}

// GetUser retrieves a user by ID
func (s *UserService) GetUser(ctx context.Context, userID string) (*UserInfo, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return s.userToInfo(user), nil
}

// GetUserByEmail retrieves a user by email
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*UserInfo, error) {
	if email == "" {
		return nil, domain.ErrInvalidEmail
	}

	email = strings.ToLower(strings.TrimSpace(email))
	user, err := s.userRepo.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	return s.userToInfo(user), nil
}

// UpdateUser updates user information
func (s *UserService) UpdateUser(ctx context.Context, userID string, req *UpdateUserRequest) (*UserInfo, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	// Get existing user
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Apply updates
	updated := false

	// Update profile fields
	if req.FirstName != nil && *req.FirstName != user.FirstName {
		user.FirstName = strings.TrimSpace(*req.FirstName)
		updated = true
	}

	if req.LastName != nil && *req.LastName != user.LastName {
		user.LastName = strings.TrimSpace(*req.LastName)
		updated = true
	}

	if req.Phone != nil && *req.Phone != user.Phone {
		user.Phone = strings.TrimSpace(*req.Phone)
		updated = true
	}

	if req.TelegramUsername != nil && *req.TelegramUsername != user.TelegramUsername {
		user.TelegramUsername = strings.TrimSpace(*req.TelegramUsername)
		updated = true
	}

	// Update role (requires admin privileges - should be checked by caller)
	if req.Role != nil && *req.Role != user.Role {
		if err := s.validateRoleChange(user.Role, *req.Role); err != nil {
			return nil, err
		}
		user.Role = *req.Role
		updated = true
	}

	// Update status (requires admin privileges - should be checked by caller)
	if req.Status != nil && *req.Status != user.Status {
		if err := s.validateStatusChange(user.Status, *req.Status); err != nil {
			return nil, err
		}
		user.Status = *req.Status
		updated = true

		// If user is being deactivated, revoke all sessions
		if *req.Status != domain.StatusActive {
			s.sessionRepo.RevokeUserSessions(ctx, userID)
		}
	}

	if updated {
		user.UpdatedAt = time.Now()
		if err := s.userRepo.Update(ctx, user); err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}
	}

	return s.userToInfo(user), nil
}

// UpdateUserProfile updates user profile information (for self-service)
func (s *UserService) UpdateUserProfile(ctx context.Context, userID string, updates interfaces.ProfileUpdate) (*UserInfo, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID cannot be empty")
	}

	// Update profile using repository method
	if err := s.userRepo.UpdateProfile(ctx, userID, updates); err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// Return updated user
	return s.GetUser(ctx, userID)
}

// DeleteUser soft deletes a user by setting status to deleted
func (s *UserService) DeleteUser(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	// Get user to validate existence
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Prevent deletion of the last admin
	if user.Role == domain.RoleAdmin {
		if err := s.validateAdminDeletion(ctx); err != nil {
			return err
		}
	}

	// Update status to deleted
	user.Status = domain.StatusDeleted
	user.UpdatedAt = time.Now()

	if err := s.userRepo.Update(ctx, user); err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	// Revoke all user sessions
	s.sessionRepo.RevokeUserSessions(ctx, userID)

	return nil
}

// HardDeleteUser permanently deletes a user (admin only)
func (s *UserService) HardDeleteUser(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	// Get user to validate existence and check admin status
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Prevent deletion of the last admin
	if user.Role == domain.RoleAdmin {
		if err := s.validateAdminDeletion(ctx); err != nil {
			return err
		}
	}

	// Revoke all user sessions first
	s.sessionRepo.RevokeUserSessions(ctx, userID)

	// Hard delete from repository
	if err := s.userRepo.Delete(ctx, userID); err != nil {
		return fmt.Errorf("failed to hard delete user: %w", err)
	}

	return nil
}

// ListUsers retrieves users with filtering and pagination
func (s *UserService) ListUsers(ctx context.Context, options UserListOptions) (*UserListResult, error) {
	// Convert options to repository filter
	filter := interfaces.UserFilter{
		Role:            options.Role,
		Status:          options.Status,
		CreatedAfter:    options.CreatedAfter,
		CreatedBefore:   options.CreatedBefore,
		LastLoginAfter:  options.LastLoginAfter,
		LastLoginBefore: options.LastLoginBefore,
		IsLocked:        options.IsLocked,
		Limit:           options.Limit,
		Offset:          options.Offset,
		SortBy:          options.SortBy,
		SortOrder:       options.SortOrder,
	}

	// Set defaults
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.SortBy == "" {
		filter.SortBy = "created_at"
	}
	if filter.SortOrder == "" {
		filter.SortOrder = "desc"
	}

	var users []*domain.User
	var total int
	var err error

	// Use search if provided, otherwise use list
	if options.Search != "" {
		users, total, err = s.userRepo.Search(ctx, options.Search, filter)
	} else {
		users, total, err = s.userRepo.List(ctx, filter)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	// Convert to response format
	userInfos := make([]*UserInfo, len(users))
	for i, user := range users {
		userInfos[i] = s.userToInfo(user)
	}

	return &UserListResult{
		Users: userInfos,
		Total: total,
	}, nil
}

// GetUsersByRole retrieves users by role
func (s *UserService) GetUsersByRole(ctx context.Context, role domain.UserRole) ([]*UserInfo, error) {
	users, err := s.userRepo.GetUsersByRole(ctx, role)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by role: %w", err)
	}

	userInfos := make([]*UserInfo, len(users))
	for i, user := range users {
		userInfos[i] = s.userToInfo(user)
	}

	return userInfos, nil
}

// GetUsersByStatus retrieves users by status
func (s *UserService) GetUsersByStatus(ctx context.Context, status domain.UserStatus) ([]*UserInfo, error) {
	users, err := s.userRepo.GetUsersByStatus(ctx, status)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by status: %w", err)
	}

	userInfos := make([]*UserInfo, len(users))
	for i, user := range users {
		userInfos[i] = s.userToInfo(user)
	}

	return userInfos, nil
}

// LockUser locks a user account
func (s *UserService) LockUser(ctx context.Context, userID string, duration time.Duration) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	// Lock user account
	lockUntil := time.Now().Add(duration)
	if err := s.userRepo.LockAccount(ctx, userID, lockUntil); err != nil {
		return fmt.Errorf("failed to lock user account: %w", err)
	}

	// Revoke all user sessions
	s.sessionRepo.RevokeUserSessions(ctx, userID)

	return nil
}

// UnlockUser unlocks a user account
func (s *UserService) UnlockUser(ctx context.Context, userID string) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	if err := s.userRepo.UnlockAccount(ctx, userID); err != nil {
		return fmt.Errorf("failed to unlock user account: %w", err)
	}

	return nil
}

// GetLockedUsers retrieves all locked users
func (s *UserService) GetLockedUsers(ctx context.Context) ([]*UserInfo, error) {
	users, err := s.userRepo.GetLockedUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get locked users: %w", err)
	}

	userInfos := make([]*UserInfo, len(users))
	for i, user := range users {
		userInfos[i] = s.userToInfo(user)
	}

	return userInfos, nil
}

// UpdateUserRole updates a user's role (admin operation)
func (s *UserService) UpdateUserRole(ctx context.Context, userID string, newRole domain.UserRole) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	// Get current user to validate role change
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Validate role change
	if err := s.validateRoleChange(user.Role, newRole); err != nil {
		return err
	}

	// Update role
	if err := s.userRepo.UpdateRole(ctx, userID, newRole); err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}

	return nil
}

// UpdateUserStatus updates a user's status (admin operation)
func (s *UserService) UpdateUserStatus(ctx context.Context, userID string, newStatus domain.UserStatus) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	// Get current user to validate status change
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	// Validate status change
	if err := s.validateStatusChange(user.Status, newStatus); err != nil {
		return err
	}

	// Update status
	if err := s.userRepo.UpdateStatus(ctx, userID, newStatus); err != nil {
		return fmt.Errorf("failed to update user status: %w", err)
	}

	// If user is being deactivated, revoke all sessions
	if newStatus != domain.StatusActive {
		s.sessionRepo.RevokeUserSessions(ctx, userID)
	}

	return nil
}

// UpdateTelegramInfo updates user's Telegram information
func (s *UserService) UpdateTelegramInfo(ctx context.Context, userID, chatID, username string) error {
	if userID == "" {
		return fmt.Errorf("user ID cannot be empty")
	}

	if err := s.userRepo.UpdateTelegramInfo(ctx, userID, chatID, username); err != nil {
		return fmt.Errorf("failed to update Telegram info: %w", err)
	}

	return nil
}

// GetTelegramInfo retrieves user's Telegram information
func (s *UserService) GetTelegramInfo(ctx context.Context, userID string) (string, string, error) {
	if userID == "" {
		return "", "", fmt.Errorf("user ID cannot be empty")
	}

	chatID, username, err := s.userRepo.GetTelegramInfo(ctx, userID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get Telegram info: %w", err)
	}

	return chatID, username, nil
}

// GetUserStats retrieves user statistics
func (s *UserService) GetUserStats(ctx context.Context) (*interfaces.UserStats, error) {
	stats, err := s.userRepo.GetUserStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}

	return stats, nil
}

// GetRecentUsers retrieves recently created users
func (s *UserService) GetRecentUsers(ctx context.Context, limit int) ([]*UserInfo, error) {
	if limit <= 0 {
		limit = 10
	}

	users, err := s.userRepo.GetRecentUsers(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent users: %w", err)
	}

	userInfos := make([]*UserInfo, len(users))
	for i, user := range users {
		userInfos[i] = s.userToInfo(user)
	}

	return userInfos, nil
}

// CleanupInactiveUsers removes users that have been inactive for a specified duration
func (s *UserService) CleanupInactiveUsers(ctx context.Context, inactiveSince time.Time) (int, error) {
	count, err := s.userRepo.DeleteInactiveUsers(ctx, inactiveSince)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup inactive users: %w", err)
	}

	return count, nil
}

// UserExists checks if a user exists by ID
func (s *UserService) UserExists(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, fmt.Errorf("user ID cannot be empty")
	}

	exists, err := s.userRepo.ExistsByID(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return exists, nil
}

// EmailExists checks if an email is already registered
func (s *UserService) EmailExists(ctx context.Context, email string) (bool, error) {
	if email == "" {
		return false, domain.ErrInvalidEmail
	}

	email = strings.ToLower(strings.TrimSpace(email))
	exists, err := s.userRepo.ExistsByEmail(ctx, email)
	if err != nil {
		return false, fmt.Errorf("failed to check email existence: %w", err)
	}

	return exists, nil
}

// GetTotalUsers returns the total number of users
func (s *UserService) GetTotalUsers(ctx context.Context) (int, error) {
	total, err := s.userRepo.GetTotalUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get total users: %w", err)
	}

	return total, nil
}

// Validation helper methods

func (s *UserService) validateCreateUserRequest(req *CreateUserRequest) error {
	if req.Email == "" {
		return domain.ErrInvalidEmail
	}
	if req.Password == "" {
		return domain.ErrInvalidPassword
	}
	if strings.TrimSpace(req.FirstName) == "" {
		return fmt.Errorf("first name cannot be empty")
	}
	if strings.TrimSpace(req.LastName) == "" {
		return fmt.Errorf("last name cannot be empty")
	}
	if req.Role == "" {
		return domain.ErrInvalidRole
	}

	// Validate role
	if !domain.IsValidRole(string(req.Role)) {
		return domain.ErrInvalidRole
	}

	return nil
}

func (s *UserService) validateRoleChange(currentRole, newRole domain.UserRole) error {
	if currentRole == newRole {
		return nil // No change
	}

	// Add business rules for role changes if needed
	// For example, prevent downgrading the last admin
	if currentRole == domain.RoleAdmin && newRole != domain.RoleAdmin {
		// This would require checking if this is the last admin
		// Implementation depends on business requirements
	}

	return nil
}

func (s *UserService) validateStatusChange(currentStatus, newStatus domain.UserStatus) error {
	if currentStatus == newStatus {
		return nil // No change
	}

	// Add business rules for status changes if needed
	// For example, prevent certain transitions
	if currentStatus == domain.StatusDeleted && newStatus != domain.StatusDeleted {
		return fmt.Errorf("cannot reactivate deleted user")
	}

	return nil
}

func (s *UserService) validateAdminDeletion(ctx context.Context) error {
	// Check if this is the last admin
	admins, err := s.userRepo.GetUsersByRole(ctx, domain.RoleAdmin)
	if err != nil {
		return fmt.Errorf("failed to check admin count: %w", err)
	}

	activeAdmins := 0
	for _, admin := range admins {
		if admin.Status == domain.StatusActive {
			activeAdmins++
		}
	}

	if activeAdmins <= 1 {
		return fmt.Errorf("cannot delete the last active admin user")
	}

	return nil
}

// Helper method to convert domain user to user info
func (s *UserService) userToInfo(user *domain.User) *UserInfo {
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
