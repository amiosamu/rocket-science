package migrations

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)


var migrationFiles embed.FS

// Migrator handles database migrations
type Migrator struct {
	db *sqlx.DB
}

// NewMigrator creates a new migrator instance
func NewMigrator(db *sqlx.DB) *Migrator {
	return &Migrator{db: db}
}

// Up runs all pending migrations
func (m *Migrator) Up(ctx context.Context) error {
	// Create migrations table if it doesn't exist
	if err := m.createMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get list of migration files
	upFiles, err := m.getUpMigrationFiles()
	if err != nil {
		return fmt.Errorf("failed to get migration files: %w", err)
	}

	// Get already applied migrations
	appliedMigrations, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Apply pending migrations
	for _, file := range upFiles {
		migrationName := m.getMigrationName(file)

		// Skip if already applied
		if appliedMigrations[migrationName] {
			continue
		}

		if err := m.applyMigration(ctx, file, migrationName); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", migrationName, err)
		}
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
	entries, err := migrationFiles.ReadDir(".")
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
	content, err := migrationFiles.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", filename, err)
	}

	// Start transaction
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migration SQL
	_, err = tx.ExecContext(ctx, string(content))
	if err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration as applied
	_, err = tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (migration) VALUES ($1)",
		migrationName)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	fmt.Printf("Applied migration: %s\n", migrationName)
	return nil
}

// Down runs migrations down (rollback) - for development use
func (m *Migrator) Down(ctx context.Context, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("steps must be positive")
	}

	// Get applied migrations in reverse order
	appliedMigrations, err := m.getAppliedMigrationsInOrder(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Limit to requested steps
	if steps > len(appliedMigrations) {
		steps = len(appliedMigrations)
	}

	// Rollback migrations
	for i := 0; i < steps; i++ {
		migration := appliedMigrations[i]
		downFile := migration + ".down.sql"

		if err := m.rollbackMigration(ctx, downFile, migration); err != nil {
			return fmt.Errorf("failed to rollback migration %s: %w", migration, err)
		}
	}

	return nil
}

// getAppliedMigrationsInOrder returns applied migrations in order
func (m *Migrator) getAppliedMigrationsInOrder(ctx context.Context, reverse bool) ([]string, error) {
	orderBy := "ASC"
	if reverse {
		orderBy = "DESC"
	}

	query := fmt.Sprintf("SELECT migration FROM schema_migrations ORDER BY applied_at %s", orderBy)

	var migrations []string
	err := m.db.SelectContext(ctx, &migrations, query)
	return migrations, err
}

// rollbackMigration rolls back a single migration
func (m *Migrator) rollbackMigration(ctx context.Context, filename, migrationName string) error {
	// Read down migration file
	content, err := migrationFiles.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read down migration file %s: %w", filename, err)
	}

	// Start transaction
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute down migration SQL
	_, err = tx.ExecContext(ctx, string(content))
	if err != nil {
		return fmt.Errorf("failed to execute down migration SQL: %w", err)
	}

	// Remove migration record
	_, err = tx.ExecContext(ctx,
		"DELETE FROM schema_migrations WHERE migration = $1",
		migrationName)
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rollback: %w", err)
	}

	fmt.Printf("Rolled back migration: %s\n", migrationName)
	return nil
}
