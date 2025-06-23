package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	_ "github.com/lib/pq"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/repository/interfaces"
)

// UserRepository implements the UserRepository interface for PostgreSQL
type UserRepository struct {
	db *sqlx.DB
}

// NewUserRepository creates a new PostgreSQL user repository
func NewUserRepository(db *sqlx.DB) interfaces.UserRepository {
	return &UserRepository{
		db: db,
	}
}

// Create creates a new user in the database
func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	query := `
		INSERT INTO users (
			id, email, password_hash, first_name, last_name, role, status,
			created_at, updated_at, last_login_at, login_attempts, locked_until,
			phone, telegram_username, telegram_chat_id, metadata
		) VALUES (
			:id, :email, :password_hash, :first_name, :last_name, :role, :status,
			:created_at, :updated_at, :last_login_at, :login_attempts, :locked_until,
			:phone, :telegram_username, :telegram_chat_id, :metadata
		)`

	metadataJSON, err := json.Marshal(user.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	params := map[string]interface{}{
		"id":                user.ID,
		"email":             user.Email,
		"password_hash":     user.PasswordHash,
		"first_name":        user.FirstName,
		"last_name":         user.LastName,
		"role":              string(user.Role),
		"status":            string(user.Status),
		"created_at":        user.CreatedAt,
		"updated_at":        user.UpdatedAt,
		"last_login_at":     user.LastLoginAt,
		"login_attempts":    user.LoginAttempts,
		"locked_until":      user.LockedUntil,
		"phone":             user.Phone,
		"telegram_username": user.TelegramUsername,
		"telegram_chat_id":  user.TelegramChatID,
		"metadata":          metadataJSON,
	}

	_, err = r.db.NamedExecContext(ctx, query, params)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok {
			switch pqErr.Code {
			case "23505": // unique_violation
				if strings.Contains(pqErr.Message, "email") {
					return domain.ErrEmailExists
				}
			}
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, role, status,
			   created_at, updated_at, last_login_at, login_attempts, locked_until,
			   phone, telegram_username, telegram_chat_id, metadata
		FROM users 
		WHERE id = $1 AND status != 'deleted'`

	user, err := r.scanUser(ctx, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	return user, nil
}

// GetByEmail retrieves a user by email
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, role, status,
			   created_at, updated_at, last_login_at, login_attempts, locked_until,
			   phone, telegram_username, telegram_chat_id, metadata
		FROM users 
		WHERE email = $1 AND status != 'deleted'`

	user, err := r.scanUser(ctx, query, strings.ToLower(email))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by email: %w", err)
	}

	return user, nil
}

// Update updates an existing user
func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	query := `
		UPDATE users SET
			email = :email,
			password_hash = :password_hash,
			first_name = :first_name,
			last_name = :last_name,
			role = :role,
			status = :status,
			updated_at = :updated_at,
			last_login_at = :last_login_at,
			login_attempts = :login_attempts,
			locked_until = :locked_until,
			phone = :phone,
			telegram_username = :telegram_username,
			telegram_chat_id = :telegram_chat_id,
			metadata = :metadata
		WHERE id = :id`

	metadataJSON, err := json.Marshal(user.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	params := map[string]interface{}{
		"id":                user.ID,
		"email":             user.Email,
		"password_hash":     user.PasswordHash,
		"first_name":        user.FirstName,
		"last_name":         user.LastName,
		"role":              string(user.Role),
		"status":            string(user.Status),
		"updated_at":        time.Now(),
		"last_login_at":     user.LastLoginAt,
		"login_attempts":    user.LoginAttempts,
		"locked_until":      user.LockedUntil,
		"phone":             user.Phone,
		"telegram_username": user.TelegramUsername,
		"telegram_chat_id":  user.TelegramChatID,
		"metadata":          metadataJSON,
	}

	result, err := r.db.NamedExecContext(ctx, query, params)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok {
			switch pqErr.Code {
			case "23505": // unique_violation
				if strings.Contains(pqErr.Message, "email") {
					return domain.ErrEmailExists
				}
			}
		}
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// Delete soft deletes a user
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	query := `
		UPDATE users 
		SET status = 'deleted', updated_at = NOW()
		WHERE id = $1 AND status != 'deleted'`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// List retrieves users with filtering and pagination
func (r *UserRepository) List(ctx context.Context, filter interfaces.UserFilter) ([]*domain.User, int, error) {
	// Build the WHERE clause
	where, args := r.buildWhereClause(filter)

	// Build the ORDER BY clause
	orderBy := r.buildOrderByClause(filter.SortBy, filter.SortOrder)

	// Count total records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM users %s", where)
	var total int
	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// Get users with pagination
	query := fmt.Sprintf(`
		SELECT id, email, password_hash, first_name, last_name, role, status,
			   created_at, updated_at, last_login_at, login_attempts, locked_until,
			   phone, telegram_username, telegram_chat_id, metadata
		FROM users %s %s
		LIMIT $%d OFFSET $%d`,
		where, orderBy, len(args)+1, len(args)+2)

	args = append(args, filter.Limit, filter.Offset)

	users, err := r.scanUsers(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	return users, total, nil
}

// Search searches users by query string
func (r *UserRepository) Search(ctx context.Context, query string, filter interfaces.UserFilter) ([]*domain.User, int, error) {
	// Add search condition to filter
	searchWhere := ""
	searchArgs := []interface{}{}

	if query != "" {
		searchWhere = "AND (LOWER(first_name) LIKE LOWER($1) OR LOWER(last_name) LIKE LOWER($1) OR LOWER(email) LIKE LOWER($1))"
		searchArgs = append(searchArgs, "%"+query+"%")
	}

	// Build the WHERE clause
	where, args := r.buildWhereClause(filter)
	if searchWhere != "" {
		where += " " + searchWhere
		// Adjust parameter numbers in searchArgs
		for i := range searchArgs {
			args = append(args, searchArgs[i])
		}
	}

	// Build the ORDER BY clause
	orderBy := r.buildOrderByClause(filter.SortBy, filter.SortOrder)

	// Count total records
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM users %s", where)
	var total int
	err := r.db.GetContext(ctx, &total, countQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count search results: %w", err)
	}

	// Get users with pagination
	searchQuery := fmt.Sprintf(`
		SELECT id, email, password_hash, first_name, last_name, role, status,
			   created_at, updated_at, last_login_at, login_attempts, locked_until,
			   phone, telegram_username, telegram_chat_id, metadata
		FROM users %s %s
		LIMIT $%d OFFSET $%d`,
		where, orderBy, len(args)+1, len(args)+2)

	args = append(args, filter.Limit, filter.Offset)

	users, err := r.scanUsers(ctx, searchQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to search users: %w", err)
	}

	return users, total, nil
}

// ValidateCredentials validates user credentials and returns the user if valid
func (r *UserRepository) ValidateCredentials(ctx context.Context, email, password string) (*domain.User, error) {
	user, err := r.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	if err := user.ValidatePassword(password); err != nil {
		// Record failed login attempt
		_ = r.RecordLoginAttempt(ctx, user.ID)
		return nil, err
	}

	return user, nil
}

// UpdatePassword updates a user's password
func (r *UserRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	query := `
		UPDATE users 
		SET password_hash = $1, updated_at = NOW()
		WHERE id = $2`

	result, err := r.db.ExecContext(ctx, query, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// RecordLoginAttempt records a failed login attempt
func (r *UserRepository) RecordLoginAttempt(ctx context.Context, userID string) error {
	query := `
		UPDATE users 
		SET login_attempts = login_attempts + 1, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to record login attempt: %w", err)
	}

	return nil
}

// ResetLoginAttempts resets the login attempts counter
func (r *UserRepository) ResetLoginAttempts(ctx context.Context, userID string) error {
	query := `
		UPDATE users 
		SET login_attempts = 0, locked_until = NULL, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to reset login attempts: %w", err)
	}

	return nil
}

// LockAccount locks a user account until a specified time
func (r *UserRepository) LockAccount(ctx context.Context, userID string, lockUntil time.Time) error {
	query := `
		UPDATE users 
		SET locked_until = $1, updated_at = NOW()
		WHERE id = $2`

	_, err := r.db.ExecContext(ctx, query, lockUntil, userID)
	if err != nil {
		return fmt.Errorf("failed to lock account: %w", err)
	}

	return nil
}

// UnlockAccount unlocks a user account
func (r *UserRepository) UnlockAccount(ctx context.Context, userID string) error {
	query := `
		UPDATE users 
		SET locked_until = NULL, login_attempts = 0, updated_at = NOW()
		WHERE id = $1`

	_, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to unlock account: %w", err)
	}

	return nil
}

// UpdateLastLogin updates the last login timestamp
func (r *UserRepository) UpdateLastLogin(ctx context.Context, userID string, loginTime time.Time) error {
	query := `
		UPDATE users 
		SET last_login_at = $1, login_attempts = 0, locked_until = NULL, updated_at = NOW()
		WHERE id = $2`

	_, err := r.db.ExecContext(ctx, query, loginTime, userID)
	if err != nil {
		return fmt.Errorf("failed to update last login: %w", err)
	}

	return nil
}

// UpdateProfile updates user profile information
func (r *UserRepository) UpdateProfile(ctx context.Context, userID string, updates interfaces.ProfileUpdate) error {
	setParts := []string{}
	args := []interface{}{}
	argIndex := 1

	if updates.FirstName != nil {
		setParts = append(setParts, fmt.Sprintf("first_name = $%d", argIndex))
		args = append(args, *updates.FirstName)
		argIndex++
	}

	if updates.LastName != nil {
		setParts = append(setParts, fmt.Sprintf("last_name = $%d", argIndex))
		args = append(args, *updates.LastName)
		argIndex++
	}

	if updates.Phone != nil {
		setParts = append(setParts, fmt.Sprintf("phone = $%d", argIndex))
		args = append(args, *updates.Phone)
		argIndex++
	}

	if updates.TelegramUsername != nil {
		setParts = append(setParts, fmt.Sprintf("telegram_username = $%d", argIndex))
		args = append(args, *updates.TelegramUsername)
		argIndex++
	}

	if updates.TelegramChatID != nil {
		setParts = append(setParts, fmt.Sprintf("telegram_chat_id = $%d", argIndex))
		args = append(args, *updates.TelegramChatID)
		argIndex++
	}

	if len(setParts) == 0 {
		return nil // No updates to perform
	}

	setParts = append(setParts, fmt.Sprintf("updated_at = $%d", argIndex))
	args = append(args, time.Now())
	argIndex++

	query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d",
		strings.Join(setParts, ", "), argIndex)
	args = append(args, userID)

	result, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update profile: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// UpdateTelegramInfo updates user's Telegram information
func (r *UserRepository) UpdateTelegramInfo(ctx context.Context, userID, chatID, username string) error {
	query := `
		UPDATE users 
		SET telegram_chat_id = $1, telegram_username = $2, updated_at = NOW()
		WHERE id = $3`

	result, err := r.db.ExecContext(ctx, query, chatID, username, userID)
	if err != nil {
		return fmt.Errorf("failed to update telegram info: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// GetTelegramInfo retrieves user's Telegram information
func (r *UserRepository) GetTelegramInfo(ctx context.Context, userID string) (chatID, username string, err error) {
	query := `
		SELECT COALESCE(telegram_chat_id, ''), COALESCE(telegram_username, '')
		FROM users 
		WHERE id = $1 AND status != 'deleted'`

	err = r.db.QueryRowContext(ctx, query, userID).Scan(&chatID, &username)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", domain.ErrUserNotFound
		}
		return "", "", fmt.Errorf("failed to get telegram info: %w", err)
	}

	return chatID, username, nil
}

// UpdateRole updates a user's role
func (r *UserRepository) UpdateRole(ctx context.Context, userID string, role domain.UserRole) error {
	query := `
		UPDATE users 
		SET role = $1, updated_at = NOW()
		WHERE id = $2`

	result, err := r.db.ExecContext(ctx, query, string(role), userID)
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// UpdateStatus updates a user's status
func (r *UserRepository) UpdateStatus(ctx context.Context, userID string, status domain.UserStatus) error {
	query := `
		UPDATE users 
		SET status = $1, updated_at = NOW()
		WHERE id = $2`

	result, err := r.db.ExecContext(ctx, query, string(status), userID)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// GetUsersByRole retrieves users by role
func (r *UserRepository) GetUsersByRole(ctx context.Context, role domain.UserRole) ([]*domain.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, role, status,
			   created_at, updated_at, last_login_at, login_attempts, locked_until,
			   phone, telegram_username, telegram_chat_id, metadata
		FROM users 
		WHERE role = $1 AND status != 'deleted'
		ORDER BY created_at DESC`

	users, err := r.scanUsers(ctx, query, string(role))
	if err != nil {
		return nil, fmt.Errorf("failed to get users by role: %w", err)
	}

	return users, nil
}

// GetUsersByStatus retrieves users by status
func (r *UserRepository) GetUsersByStatus(ctx context.Context, status domain.UserStatus) ([]*domain.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, role, status,
			   created_at, updated_at, last_login_at, login_attempts, locked_until,
			   phone, telegram_username, telegram_chat_id, metadata
		FROM users 
		WHERE status = $1
		ORDER BY created_at DESC`

	users, err := r.scanUsers(ctx, query, string(status))
	if err != nil {
		return nil, fmt.Errorf("failed to get users by status: %w", err)
	}

	return users, nil
}

// UpdateMetadata updates user metadata
func (r *UserRepository) UpdateMetadata(ctx context.Context, userID string, metadata map[string]string) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		UPDATE users 
		SET metadata = $1, updated_at = NOW()
		WHERE id = $2`

	result, err := r.db.ExecContext(ctx, query, metadataJSON, userID)
	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// ExistsByEmail checks if a user exists by email
func (r *UserRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM users 
			WHERE email = $1 AND status != 'deleted'
		)`

	var exists bool
	err := r.db.GetContext(ctx, &exists, query, strings.ToLower(email))
	if err != nil {
		return false, fmt.Errorf("failed to check if user exists by email: %w", err)
	}

	return exists, nil
}

// ExistsByID checks if a user exists by ID
func (r *UserRepository) ExistsByID(ctx context.Context, id string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM users 
			WHERE id = $1 AND status != 'deleted'
		)`

	var exists bool
	err := r.db.GetContext(ctx, &exists, query, id)
	if err != nil {
		return false, fmt.Errorf("failed to check if user exists by ID: %w", err)
	}

	return exists, nil
}

// GetTotalUsers returns the total number of users
func (r *UserRepository) GetTotalUsers(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM users WHERE status != 'deleted'`

	var total int
	err := r.db.GetContext(ctx, &total, query)
	if err != nil {
		return 0, fmt.Errorf("failed to get total users: %w", err)
	}

	return total, nil
}

// GetUserStats returns user statistics
func (r *UserRepository) GetUserStats(ctx context.Context) (*interfaces.UserStats, error) {
	stats := &interfaces.UserStats{
		UsersByRole: make(map[domain.UserRole]int),
	}

	// Get basic counts
	query := `
		SELECT 
			COUNT(*) as total_users,
			COUNT(CASE WHEN status = 'active' THEN 1 END) as active_users,
			COUNT(CASE WHEN status = 'inactive' THEN 1 END) as inactive_users,
			COUNT(CASE WHEN status = 'suspended' THEN 1 END) as suspended_users,
			COUNT(CASE WHEN status = 'deleted' THEN 1 END) as deleted_users,
			COUNT(CASE WHEN locked_until IS NOT NULL AND locked_until > NOW() THEN 1 END) as locked_users,
			COUNT(CASE WHEN telegram_chat_id IS NOT NULL AND telegram_chat_id != '' THEN 1 END) as users_with_telegram,
			COUNT(CASE WHEN created_at > NOW() - INTERVAL '24 hours' THEN 1 END) as recent_signups,
			COUNT(CASE WHEN last_login_at > NOW() - INTERVAL '24 hours' THEN 1 END) as recent_logins
		FROM users`

	err := r.db.QueryRowContext(ctx, query).Scan(
		&stats.TotalUsers,
		&stats.ActiveUsers,
		&stats.InactiveUsers,
		&stats.SuspendedUsers,
		&stats.DeletedUsers,
		&stats.LockedUsers,
		&stats.UsersWithTelegram,
		&stats.RecentSignups,
		&stats.RecentLogins,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}

	// Get counts by role
	roleQuery := `
		SELECT role, COUNT(*) 
		FROM users 
		WHERE status != 'deleted'
		GROUP BY role`

	rows, err := r.db.QueryContext(ctx, roleQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to get role stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var roleStr string
		var count int
		if err := rows.Scan(&roleStr, &count); err != nil {
			return nil, fmt.Errorf("failed to scan role stats: %w", err)
		}
		stats.UsersByRole[domain.UserRole(roleStr)] = count
	}

	return stats, nil
}

// GetLockedUsers returns users with locked accounts
func (r *UserRepository) GetLockedUsers(ctx context.Context) ([]*domain.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, role, status,
			   created_at, updated_at, last_login_at, login_attempts, locked_until,
			   phone, telegram_username, telegram_chat_id, metadata
		FROM users 
		WHERE locked_until IS NOT NULL AND locked_until > NOW()
		ORDER BY locked_until DESC`

	users, err := r.scanUsers(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get locked users: %w", err)
	}

	return users, nil
}

// GetRecentUsers returns recently created users
func (r *UserRepository) GetRecentUsers(ctx context.Context, limit int) ([]*domain.User, error) {
	query := `
		SELECT id, email, password_hash, first_name, last_name, role, status,
			   created_at, updated_at, last_login_at, login_attempts, locked_until,
			   phone, telegram_username, telegram_chat_id, metadata
		FROM users 
		WHERE status != 'deleted'
		ORDER BY created_at DESC
		LIMIT $1`

	users, err := r.scanUsers(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent users: %w", err)
	}

	return users, nil
}

// DeleteInactiveUsers deletes users inactive since a given time
func (r *UserRepository) DeleteInactiveUsers(ctx context.Context, inactiveSince time.Time) (int, error) {
	query := `
		UPDATE users 
		SET status = 'deleted', updated_at = NOW()
		WHERE status = 'inactive' 
		  AND (last_login_at IS NULL OR last_login_at < $1)
		  AND created_at < $1`

	result, err := r.db.ExecContext(ctx, query, inactiveSince)
	if err != nil {
		return 0, fmt.Errorf("failed to delete inactive users: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// GetUsersForCleanup returns users that match cleanup criteria
func (r *UserRepository) GetUsersForCleanup(ctx context.Context, criteria interfaces.CleanupCriteria) ([]*domain.User, error) {
	whereParts := []string{"1=1"}
	args := []interface{}{}
	argIndex := 1

	if !criteria.InactiveSince.IsZero() {
		whereParts = append(whereParts, fmt.Sprintf("(last_login_at IS NULL OR last_login_at < $%d)", argIndex))
		args = append(args, criteria.InactiveSince)
		argIndex++
	}

	if !criteria.DeletedBefore.IsZero() {
		whereParts = append(whereParts, fmt.Sprintf("status = 'deleted' AND updated_at < $%d", argIndex))
		args = append(args, criteria.DeletedBefore)
		argIndex++
	}

	if criteria.NeverLoggedIn {
		whereParts = append(whereParts, "last_login_at IS NULL")
	}

	if criteria.Status != nil {
		whereParts = append(whereParts, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, string(*criteria.Status))
		argIndex++
	}

	if !criteria.IncludeTestUsers {
		whereParts = append(whereParts, "email NOT LIKE '%@example.com' AND email NOT LIKE '%@test.com'")
	}

	query := fmt.Sprintf(`
		SELECT id, email, password_hash, first_name, last_name, role, status,
			   created_at, updated_at, last_login_at, login_attempts, locked_until,
			   phone, telegram_username, telegram_chat_id, metadata
		FROM users 
		WHERE %s
		ORDER BY created_at ASC`,
		strings.Join(whereParts, " AND "))

	users, err := r.scanUsers(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get users for cleanup: %w", err)
	}

	return users, nil
}

// Helper functions

// scanUser scans a single user from a query result
func (r *UserRepository) scanUser(ctx context.Context, query string, args ...interface{}) (*domain.User, error) {
	user := &domain.User{}
	var metadataJSON []byte

	err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.FirstName,
		&user.LastName,
		&user.Role,
		&user.Status,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
		&user.LoginAttempts,
		&user.LockedUntil,
		&user.Phone,
		&user.TelegramUsername,
		&user.TelegramChatID,
		&metadataJSON,
	)
	if err != nil {
		return nil, err
	}

	// Unmarshal metadata
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &user.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	} else {
		user.Metadata = make(map[string]string)
	}

	return user, nil
}

// scanUsers scans multiple users from a query result
func (r *UserRepository) scanUsers(ctx context.Context, query string, args ...interface{}) ([]*domain.User, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*domain.User
	for rows.Next() {
		user := &domain.User{}
		var metadataJSON []byte

		err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.PasswordHash,
			&user.FirstName,
			&user.LastName,
			&user.Role,
			&user.Status,
			&user.CreatedAt,
			&user.UpdatedAt,
			&user.LastLoginAt,
			&user.LoginAttempts,
			&user.LockedUntil,
			&user.Phone,
			&user.TelegramUsername,
			&user.TelegramChatID,
			&metadataJSON,
		)
		if err != nil {
			return nil, err
		}

		// Unmarshal metadata
		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &user.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		} else {
			user.Metadata = make(map[string]string)
		}

		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return users, nil
}

// buildWhereClause builds the WHERE clause for filtering
func (r *UserRepository) buildWhereClause(filter interfaces.UserFilter) (string, []interface{}) {
	whereParts := []string{"status != 'deleted'"}
	args := []interface{}{}
	argIndex := 1

	if filter.Role != nil {
		whereParts = append(whereParts, fmt.Sprintf("role = $%d", argIndex))
		args = append(args, string(*filter.Role))
		argIndex++
	}

	if filter.Status != nil {
		whereParts[0] = "1=1" // Remove the default status filter
		whereParts = append(whereParts, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, string(*filter.Status))
		argIndex++
	}

	if filter.CreatedAfter != nil {
		whereParts = append(whereParts, fmt.Sprintf("created_at >= $%d", argIndex))
		args = append(args, *filter.CreatedAfter)
		argIndex++
	}

	if filter.CreatedBefore != nil {
		whereParts = append(whereParts, fmt.Sprintf("created_at <= $%d", argIndex))
		args = append(args, *filter.CreatedBefore)
		argIndex++
	}

	if filter.LastLoginAfter != nil {
		whereParts = append(whereParts, fmt.Sprintf("last_login_at >= $%d", argIndex))
		args = append(args, *filter.LastLoginAfter)
		argIndex++
	}

	if filter.LastLoginBefore != nil {
		whereParts = append(whereParts, fmt.Sprintf("last_login_at <= $%d", argIndex))
		args = append(args, *filter.LastLoginBefore)
		argIndex++
	}

	if filter.IsLocked != nil {
		if *filter.IsLocked {
			whereParts = append(whereParts, "locked_until IS NOT NULL AND locked_until > NOW()")
		} else {
			whereParts = append(whereParts, "(locked_until IS NULL OR locked_until <= NOW())")
		}
	}

	where := ""
	if len(whereParts) > 0 {
		where = "WHERE " + strings.Join(whereParts, " AND ")
	}

	return where, args
}

// buildOrderByClause builds the ORDER BY clause
func (r *UserRepository) buildOrderByClause(sortBy, sortOrder string) string {
	validSortFields := map[string]string{
		"created_at":    "created_at",
		"updated_at":    "updated_at",
		"last_login_at": "last_login_at",
		"email":         "email",
		"name":          "first_name, last_name",
		"first_name":    "first_name",
		"last_name":     "last_name",
	}

	orderBy := "ORDER BY created_at DESC" // Default

	if field, ok := validSortFields[sortBy]; ok {
		direction := "DESC"
		if sortOrder == "asc" {
			direction = "ASC"
		}
		orderBy = fmt.Sprintf("ORDER BY %s %s", field, direction)
	}

	return orderBy
}
