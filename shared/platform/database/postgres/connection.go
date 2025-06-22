package postgres

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver

	"github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// Config holds PostgreSQL connection configuration
type Config struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	User            string        `json:"user"`
	Password        string        `json:"password"`
	DBName          string        `json:"db_name"`
	SSLMode         string        `json:"ssl_mode"`
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	ConnectTimeout  time.Duration `json:"connect_timeout"`
}

// DefaultConfig returns a default PostgreSQL configuration
func DefaultConfig() Config {
	return Config{
		Host:            "localhost",
		Port:            5432,
		User:            "postgres",
		Password:        "password",
		DBName:          "postgres",
		SSLMode:         "disable",
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnectTimeout:  30 * time.Second,
	}
}

// DSN returns the PostgreSQL connection string
func (c Config) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode, int(c.ConnectTimeout.Seconds()))
}

// Connection manages a PostgreSQL database connection
type Connection struct {
	DB     *sqlx.DB
	config Config
	logger logging.Logger
}

// NewConnection creates a new PostgreSQL connection
func NewConnection(config Config, logger logging.Logger) (*Connection, error) {
	// Create context with timeout for connection
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	// Open database connection
	db, err := sqlx.ConnectContext(ctx, "postgres", config.DSN())
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to PostgreSQL")
	}

	// Configure connection pool
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.Wrap(err, "failed to ping PostgreSQL database")
	}

	logger.Info(ctx, "PostgreSQL connection established", map[string]interface{}{
		"host":              config.Host,
		"port":              config.Port,
		"database":          config.DBName,
		"max_open_conns":    config.MaxOpenConns,
		"max_idle_conns":    config.MaxIdleConns,
		"conn_max_lifetime": config.ConnMaxLifetime,
	})

	return &Connection{
		DB:     db,
		config: config,
		logger: logger,
	}, nil
}

// Close closes the database connection
func (c *Connection) Close() error {
	if c.DB != nil {
		err := c.DB.Close()
		if err != nil {
			c.logger.Error(nil, "Failed to close PostgreSQL connection", err)
			return err
		}
		c.logger.Info(nil, "PostgreSQL connection closed")
	}
	return nil
}

// HealthCheck performs a health check on the database
func (c *Connection) HealthCheck(ctx context.Context) error {
	if c.DB == nil {
		return errors.NewInternal("database connection is nil")
	}

	// Check if we can ping the database
	if err := c.DB.PingContext(ctx); err != nil {
		return errors.Wrap(err, "database ping failed")
	}

	// Check if we can execute a simple query
	var result int
	if err := c.DB.GetContext(ctx, &result, "SELECT 1"); err != nil {
		return errors.Wrap(err, "database query failed")
	}

	return nil
}

// GetStats returns database connection statistics
func (c *Connection) GetStats() map[string]interface{} {
	if c.DB == nil {
		return map[string]interface{}{
			"status": "disconnected",
		}
	}

	stats := c.DB.Stats()
	return map[string]interface{}{
		"status":               "connected",
		"max_open_conns":       stats.MaxOpenConnections,
		"open_conns":           stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration":        stats.WaitDuration,
		"max_idle_closed":      stats.MaxIdleClosed,
		"max_idle_time_closed": stats.MaxIdleTimeClosed,
		"max_lifetime_closed":  stats.MaxLifetimeClosed,
	}
}

// Migrator handles database migrations
type Migrator struct {
	db         *sqlx.DB
	logger     logging.Logger
	migrations embed.FS
}

// NewMigrator creates a new database migrator
func NewMigrator(db *sqlx.DB, migrations embed.FS, logger logging.Logger) *Migrator {
	return &Migrator{
		db:         db,
		logger:     logger,
		migrations: migrations,
	}
}

// RunMigrations executes all pending migrations
func (m *Migrator) RunMigrations(ctx context.Context) error {
	// Create migrations table if it doesn't exist
	if err := m.createMigrationsTable(ctx); err != nil {
		return errors.Wrap(err, "failed to create migrations table")
	}

	// Get list of migration files
	upFiles, err := m.getUpMigrationFiles()
	if err != nil {
		return errors.Wrap(err, "failed to get migration files")
	}

	if len(upFiles) == 0 {
		m.logger.Info(ctx, "No migration files found")
		return nil
	}

	// Get already applied migrations
	appliedMigrations, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get applied migrations")
	}

	// Apply pending migrations
	pendingCount := 0
	for _, file := range upFiles {
		migrationName := m.getMigrationName(file)

		// Skip if already applied
		if appliedMigrations[migrationName] {
			continue
		}

		if err := m.applyMigration(ctx, file, migrationName); err != nil {
			return errors.Wrap(err, fmt.Sprintf("failed to apply migration %s", migrationName))
		}
		pendingCount++
	}

	if pendingCount > 0 {
		m.logger.Info(ctx, "Migrations completed", map[string]interface{}{
			"applied_count": pendingCount,
			"total_count":   len(upFiles),
		})
	} else {
		m.logger.Info(ctx, "No pending migrations")
	}

	return nil
}

// createMigrationsTable creates the migrations tracking table
func (m *Migrator) createMigrationsTable(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id SERIAL PRIMARY KEY,
			migration VARCHAR(255) NOT NULL UNIQUE,
			applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)`

	_, err := m.db.ExecContext(ctx, query)
	return err
}

// getUpMigrationFiles returns all .up.sql files sorted by name
func (m *Migrator) getUpMigrationFiles() ([]string, error) {
	entries, err := m.migrations.ReadDir(".")
	if err != nil {
		return nil, err
	}

	var upFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			upFiles = append(upFiles, entry.Name())
		}
	}

	// Sort files to ensure consistent order
	sort.Strings(upFiles)
	return upFiles, nil
}

// getAppliedMigrations returns a map of already applied migrations
func (m *Migrator) getAppliedMigrations(ctx context.Context) (map[string]bool, error) {
	query := "SELECT migration FROM schema_migrations"

	var migrations []string
	err := m.db.SelectContext(ctx, &migrations, query)
	if err != nil {
		return nil, err
	}

	appliedMigrations := make(map[string]bool)
	for _, migration := range migrations {
		appliedMigrations[migration] = true
	}

	return appliedMigrations, nil
}

// getMigrationName extracts migration name from filename
func (m *Migrator) getMigrationName(filename string) string {
	// Remove .up.sql suffix
	name := strings.TrimSuffix(filename, ".up.sql")
	return name
}

// applyMigration applies a single migration file
func (m *Migrator) applyMigration(ctx context.Context, filename, migrationName string) error {
	// Read migration file
	content, err := m.migrations.ReadFile(filename)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to read migration file %s", filename))
	}

	// Start transaction
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to begin transaction")
	}
	defer tx.Rollback()

	// Execute migration SQL
	_, err = tx.ExecContext(ctx, string(content))
	if err != nil {
		return errors.Wrap(err, "failed to execute migration SQL")
	}

	// Record migration as applied
	_, err = tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (migration) VALUES ($1)",
		migrationName)
	if err != nil {
		return errors.Wrap(err, "failed to record migration")
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit migration")
	}

	m.logger.Info(ctx, "Applied migration", map[string]interface{}{
		"migration": migrationName,
		"file":      filename,
	})

	return nil
}

// Transaction helper functions

// WithTransaction executes a function within a database transaction
func (c *Connection) WithTransaction(ctx context.Context, fn func(tx *sqlx.Tx) error) error {
	tx, err := c.DB.BeginTxx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to begin transaction")
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit transaction")
	}

	return nil
}

// WithTransactionRetry executes a function with transaction retry logic
func (c *Connection) WithTransactionRetry(ctx context.Context, maxRetries int, fn func(tx *sqlx.Tx) error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := c.WithTransaction(ctx, fn)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable (serialization failure, deadlock, etc.)
		if !isRetryableError(err) {
			return err
		}

		// Exponential backoff
		if attempt < maxRetries {
			backoff := time.Duration(attempt+1) * 100 * time.Millisecond
			c.logger.Warn(ctx, "Transaction failed, retrying", map[string]interface{}{
				"attempt": attempt + 1,
				"backoff": backoff,
				"error":   err.Error(),
			})

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}
	}

	return errors.Wrap(lastErr, fmt.Sprintf("transaction failed after %d attempts", maxRetries+1))
}

// isRetryableError checks if a database error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for PostgreSQL specific error codes
	errorStr := err.Error()
	retryableErrors := []string{
		"serialization_failure",
		"deadlock_detected",
		"connection reset",
		"connection refused",
		"server closed the connection unexpectedly",
	}

	for _, retryableErr := range retryableErrors {
		if strings.Contains(strings.ToLower(errorStr), retryableErr) {
			return true
		}
	}

	return false
}
