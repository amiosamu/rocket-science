package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// User represents a user in the system
type User struct {
	ID           string            `json:"id" db:"id"`
	Email        string            `json:"email" db:"email"`
	PasswordHash string            `json:"-" db:"password_hash"` // Never serialize password hash
	FirstName    string            `json:"first_name" db:"first_name"`
	LastName     string            `json:"last_name" db:"last_name"`
	Role         UserRole          `json:"role" db:"role"`
	Status       UserStatus        `json:"status" db:"status"`
	CreatedAt    time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at" db:"updated_at"`
	LastLoginAt  *time.Time        `json:"last_login_at,omitempty" db:"last_login_at"`
	Metadata     map[string]string `json:"metadata,omitempty" db:"metadata"`

	// Security tracking
	LoginAttempts int        `json:"-" db:"login_attempts"`
	LockedUntil   *time.Time `json:"-" db:"locked_until"`

	// Profile information
	Phone            string `json:"phone,omitempty" db:"phone"`
	TelegramUsername string `json:"telegram_username,omitempty" db:"telegram_username"`
	TelegramChatID   string `json:"telegram_chat_id,omitempty" db:"telegram_chat_id"`
}

// UserRole represents user roles in the system
type UserRole string

const (
	RoleCustomer UserRole = "customer"
	RoleAdmin    UserRole = "admin"
	RoleOperator UserRole = "operator"
	RoleSupport  UserRole = "support"
)

// UserStatus represents user status
type UserStatus string

const (
	StatusActive    UserStatus = "active"
	StatusInactive  UserStatus = "inactive"
	StatusSuspended UserStatus = "suspended"
	StatusDeleted   UserStatus = "deleted"
)

// UserProfile represents user profile information
type UserProfile struct {
	UserID           string            `json:"user_id"`
	FirstName        string            `json:"first_name"`
	LastName         string            `json:"last_name"`
	Email            string            `json:"email"`
	Phone            string            `json:"phone,omitempty"`
	TelegramUsername string            `json:"telegram_username,omitempty"`
	TelegramChatID   string            `json:"telegram_chat_id,omitempty"`
	Preferences      map[string]string `json:"preferences,omitempty"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

// Domain errors
var (
	ErrInvalidEmail       = errors.New("invalid email format")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrWeakPassword       = errors.New("password does not meet security requirements")
	ErrEmailExists        = errors.New("email already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountLocked      = errors.New("account is locked due to too many failed login attempts")
	ErrAccountInactive    = errors.New("account is not active")
	ErrUnauthorized       = errors.New("insufficient permissions")
	ErrInvalidRole        = errors.New("invalid user role")
	ErrInvalidStatus      = errors.New("invalid user status")
)

// NewUser creates a new user with the given details
func NewUser(email, password, firstName, lastName string, role UserRole) (*User, error) {
	// Validate inputs
	if err := validateEmail(email); err != nil {
		return nil, err
	}

	if err := ValidatePassword(password); err != nil {
		return nil, err
	}

	if err := validateRole(role); err != nil {
		return nil, err
	}

	if strings.TrimSpace(firstName) == "" {
		return nil, fmt.Errorf("first name cannot be empty")
	}

	if strings.TrimSpace(lastName) == "" {
		return nil, fmt.Errorf("last name cannot be empty")
	}

	// Hash password
	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now()
	user := &User{
		ID:            uuid.New().String(),
		Email:         strings.ToLower(strings.TrimSpace(email)),
		PasswordHash:  passwordHash,
		FirstName:     strings.TrimSpace(firstName),
		LastName:      strings.TrimSpace(lastName),
		Role:          role,
		Status:        StatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
		Metadata:      make(map[string]string),
		LoginAttempts: 0,
	}

	return user, nil
}

// ValidatePassword checks if the provided password matches the user's password
func (u *User) ValidatePassword(password string) error {
	if u.Status != StatusActive {
		return ErrAccountInactive
	}

	if u.IsLocked() {
		return ErrAccountLocked
	}

	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	if err != nil {
		return ErrInvalidCredentials
	}

	return nil
}

// ChangePassword changes the user's password
func (u *User) ChangePassword(currentPassword, newPassword string) error {
	// Verify current password
	if err := u.ValidatePassword(currentPassword); err != nil {
		return err
	}

	// Validate new password
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}

	// Hash new password
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	u.PasswordHash = passwordHash
	u.UpdatedAt = time.Now()

	return nil
}

// UpdateProfile updates the user's profile information
func (u *User) UpdateProfile(firstName, lastName, phone, telegramUsername string) error {
	if strings.TrimSpace(firstName) != "" {
		u.FirstName = strings.TrimSpace(firstName)
	}

	if strings.TrimSpace(lastName) != "" {
		u.LastName = strings.TrimSpace(lastName)
	}

	if strings.TrimSpace(phone) != "" {
		u.Phone = strings.TrimSpace(phone)
	}

	if strings.TrimSpace(telegramUsername) != "" {
		u.TelegramUsername = strings.TrimSpace(telegramUsername)
	}

	u.UpdatedAt = time.Now()
	return nil
}

// UpdateTelegramChatID updates the user's Telegram chat ID
func (u *User) UpdateTelegramChatID(chatID, username string) {
	u.TelegramChatID = strings.TrimSpace(chatID)
	if strings.TrimSpace(username) != "" {
		u.TelegramUsername = strings.TrimSpace(username)
	}
	u.UpdatedAt = time.Now()
}

// RecordLoginAttempt records a failed login attempt
func (u *User) RecordLoginAttempt() {
	u.LoginAttempts++
	u.UpdatedAt = time.Now()
}

// LockAccount locks the user account for a specified duration
func (u *User) LockAccount(duration time.Duration) {
	lockUntil := time.Now().Add(duration)
	u.LockedUntil = &lockUntil
	u.UpdatedAt = time.Now()
}

// UnlockAccount unlocks the user account
func (u *User) UnlockAccount() {
	u.LockedUntil = nil
	u.LoginAttempts = 0
	u.UpdatedAt = time.Now()
}

// IsLocked checks if the account is currently locked
func (u *User) IsLocked() bool {
	if u.LockedUntil == nil {
		return false
	}
	return time.Now().Before(*u.LockedUntil)
}

// RecordSuccessfulLogin records a successful login
func (u *User) RecordSuccessfulLogin() {
	now := time.Now()
	u.LastLoginAt = &now
	u.LoginAttempts = 0
	u.LockedUntil = nil
	u.UpdatedAt = now
}

// HasRole checks if the user has the specified role
func (u *User) HasRole(role UserRole) bool {
	return u.Role == role
}

// IsAdmin checks if the user is an admin
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// CanManageUsers checks if the user can manage other users
func (u *User) CanManageUsers() bool {
	return u.Role == RoleAdmin || u.Role == RoleSupport
}

// CanManageOrders checks if the user can manage orders
func (u *User) CanManageOrders() bool {
	return u.Role == RoleAdmin || u.Role == RoleOperator
}

// CanManageInventory checks if the user can manage inventory
func (u *User) CanManageInventory() bool {
	return u.Role == RoleAdmin || u.Role == RoleOperator
}

// GetFullName returns the user's full name
func (u *User) GetFullName() string {
	return fmt.Sprintf("%s %s", u.FirstName, u.LastName)
}

// ToProfile converts the user to a UserProfile
func (u *User) ToProfile() *UserProfile {
	return &UserProfile{
		UserID:           u.ID,
		FirstName:        u.FirstName,
		LastName:         u.LastName,
		Email:            u.Email,
		Phone:            u.Phone,
		TelegramUsername: u.TelegramUsername,
		TelegramChatID:   u.TelegramChatID,
		Preferences:      u.Metadata,
		UpdatedAt:        u.UpdatedAt,
	}
}

// GetPermissions returns the permissions for the user based on their role
func (u *User) GetPermissions() []string {
	switch u.Role {
	case RoleAdmin:
		return []string{
			"users:read", "users:write", "users:delete",
			"orders:read", "orders:write", "orders:delete",
			"inventory:read", "inventory:write", "inventory:delete",
			"admin:*",
		}
	case RoleOperator:
		return []string{
			"orders:read", "orders:write",
			"inventory:read", "inventory:write",
		}
	case RoleSupport:
		return []string{
			"users:read", "users:write",
			"orders:read",
			"inventory:read",
		}
	case RoleCustomer:
		return []string{
			"orders:read", "orders:create",
			"profile:read", "profile:write",
		}
	default:
		return []string{}
	}
}

// HasPermission checks if the user has a specific permission
func (u *User) HasPermission(resource, action string) bool {
	permissions := u.GetPermissions()

	// Check for admin wildcard
	for _, perm := range permissions {
		if perm == "admin:*" {
			return true
		}

		// Check for exact match
		if perm == fmt.Sprintf("%s:%s", resource, action) {
			return true
		}

		// Check for resource wildcard
		if perm == fmt.Sprintf("%s:*", resource) {
			return true
		}
	}

	return false
}

// Validation functions

func validateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return ErrInvalidEmail
	}

	// Basic email regex
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return ErrInvalidEmail
	}

	return nil
}

// ValidatePassword validates password strength
func ValidatePassword(password string) error {
	if len(password) < 8 {
		return ErrWeakPassword
	}

	var (
		hasUpper  = false
		hasLower  = false
		hasNumber = false
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasNumber = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber {
		return ErrWeakPassword
	}

	return nil
}

func validateRole(role UserRole) error {
	switch role {
	case RoleCustomer, RoleAdmin, RoleOperator, RoleSupport:
		return nil
	default:
		return ErrInvalidRole
	}
}

// HashPassword creates a bcrypt hash of the password
func HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

func hashPassword(password string) (string, error) {
	return HashPassword(password)
}

// IsValidRole checks if a role string is valid
func IsValidRole(role string) bool {
	switch UserRole(role) {
	case RoleCustomer, RoleAdmin, RoleOperator, RoleSupport:
		return true
	default:
		return false
	}
}

// IsValidStatus checks if a status string is valid
func IsValidStatus(status string) bool {
	switch UserStatus(status) {
	case StatusActive, StatusInactive, StatusSuspended, StatusDeleted:
		return true
	default:
		return false
	}
}
