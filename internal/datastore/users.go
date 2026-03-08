// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// User represents a row in the users table.
type User struct {
	ID                  string
	Username            string
	DisplayName         string
	Email               string
	PasswordHash        string
	Role                string
	AuthProvider        string
	IsLocked            bool
	FailedLoginAttempts int
	LastLoginAt         time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// InsertUserParams holds the parameters for creating a new user.
type InsertUserParams struct {
	Username     string
	DisplayName  string
	Email        string
	PasswordHash string
	Role         string
	AuthProvider string
}

// UpdateUserParams holds the parameters for updating a user. Only non-zero
// fields are applied.
type UpdateUserParams struct {
	DisplayName *string
	Email       *string
	Role        *string
	IsLocked    *bool
}

// userColumns is the SELECT column list for the users table.
const userColumns = `id, username, display_name, email, password_hash, role,
	auth_provider, is_locked, failed_login_attempts, last_login_at,
	created_at, updated_at`

// scanUser scans a single user row into a User struct.
func scanUser(row interface{ Scan(dest ...any) error }) (User, error) {
	var u User
	var displayName, email sql.NullString
	var lastLogin sql.NullTime

	err := row.Scan(
		&u.ID,
		&u.Username,
		&displayName,
		&email,
		&u.PasswordHash,
		&u.Role,
		&u.AuthProvider,
		&u.IsLocked,
		&u.FailedLoginAttempts,
		&lastLogin,
		&u.CreatedAt,
		&u.UpdatedAt,
	)
	if err != nil {
		return User{}, err
	}
	u.DisplayName = stringFromNull(displayName)
	u.Email = stringFromNull(email)
	u.LastLoginAt = timeFromNull(lastLogin)
	return u, nil
}

// scanUsers scans multiple user rows.
func (db *DB) scanUsers(ctx context.Context, query string, args ...any) ([]User, error) {
	rows, err := db.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("datastore: querying users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, scanErr := scanUser(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("datastore: scanning user row: %w", scanErr)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: iterating user rows: %w", err)
	}
	return users, nil
}

// ---------------------------------------------------------------------------
// CRUD
// ---------------------------------------------------------------------------

// InsertUser creates a new user and returns the created record. Returns
// ErrAlreadyExists if a user with the same username already exists.
func (db *DB) InsertUser(ctx context.Context, p InsertUserParams) (User, error) {
	query := `
		INSERT INTO users (username, display_name, email, password_hash, role, auth_provider)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING ` + userColumns

	row := db.pool.QueryRowContext(ctx, query,
		p.Username,
		nullString(p.DisplayName),
		nullString(p.Email),
		p.PasswordHash,
		p.Role,
		p.AuthProvider,
	)

	u, err := scanUser(row)
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrAlreadyExists
		}
		return User{}, fmt.Errorf("datastore: inserting user: %w", err)
	}
	return u, nil
}

// GetUserByUsername returns the user with the given username. Returns
// ErrNotFound if no such user exists.
func (db *DB) GetUserByUsername(ctx context.Context, username string) (User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE username = $1`
	row := db.pool.QueryRowContext(ctx, query, username)

	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("datastore: getting user by username: %w", err)
	}
	return u, nil
}

// GetUserByID returns the user with the given ID. Returns ErrNotFound if no
// such user exists.
func (db *DB) GetUserByID(ctx context.Context, id string) (User, error) {
	query := `SELECT ` + userColumns + ` FROM users WHERE id = $1`
	row := db.pool.QueryRowContext(ctx, query, id)

	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("datastore: getting user by id: %w", err)
	}
	return u, nil
}

// ListUsers returns all users ordered by username.
func (db *DB) ListUsers(ctx context.Context) ([]User, error) {
	query := `SELECT ` + userColumns + ` FROM users ORDER BY username`
	return db.scanUsers(ctx, query)
}

// CountUsers returns the total number of users.
func (db *DB) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := db.pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("datastore: counting users: %w", err)
	}
	return count, nil
}

// UpdateUser applies the non-nil fields of UpdateUserParams to the user
// identified by username. Returns the updated User record. Returns
// ErrNotFound if no such user exists.
func (db *DB) UpdateUser(ctx context.Context, username string, p UpdateUserParams) (User, error) {
	// Build a dynamic SET clause — only update provided fields.
	sets := []string{"updated_at = now()"}
	args := []any{}
	argIdx := 1

	if p.DisplayName != nil {
		sets = append(sets, fmt.Sprintf("display_name = $%d", argIdx))
		args = append(args, nullString(*p.DisplayName))
		argIdx++
	}
	if p.Email != nil {
		sets = append(sets, fmt.Sprintf("email = $%d", argIdx))
		args = append(args, nullString(*p.Email))
		argIdx++
	}
	if p.Role != nil {
		sets = append(sets, fmt.Sprintf("role = $%d", argIdx))
		args = append(args, *p.Role)
		argIdx++
	}
	if p.IsLocked != nil {
		sets = append(sets, fmt.Sprintf("is_locked = $%d", argIdx))
		args = append(args, *p.IsLocked)
		argIdx++
		// When unlocking, also reset the failed login counter.
		if !*p.IsLocked {
			sets = append(sets, "failed_login_attempts = 0")
		}
	}

	query := fmt.Sprintf(
		`UPDATE users SET %s WHERE username = $%d RETURNING %s`,
		joinStrings(sets, ", "), argIdx, userColumns,
	)
	args = append(args, username)

	row := db.pool.QueryRowContext(ctx, query, args...)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, fmt.Errorf("datastore: updating user: %w", err)
	}
	return u, nil
}

// UpdateUserPassword changes a user's password hash.
func (db *DB) UpdateUserPassword(ctx context.Context, username, passwordHash string) error {
	res, err := db.pool.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE username = $2`,
		passwordHash, username,
	)
	if err != nil {
		return fmt.Errorf("datastore: updating user password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUser removes a user by username. Returns ErrNotFound if no such
// user exists. Associated sessions are removed via CASCADE.
func (db *DB) DeleteUser(ctx context.Context, username string) error {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM users WHERE username = $1`, username,
	)
	if err != nil {
		return fmt.Errorf("datastore: deleting user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Login helpers
// ---------------------------------------------------------------------------

// IncrementFailedLoginAttempts bumps the failed_login_attempts counter and
// returns the new count.
func (db *DB) IncrementFailedLoginAttempts(ctx context.Context, username string) (int, error) {
	var count int
	err := db.pool.QueryRowContext(ctx, `
		UPDATE users
		   SET failed_login_attempts = failed_login_attempts + 1,
		       updated_at = now()
		 WHERE username = $1
		 RETURNING failed_login_attempts
	`, username).Scan(&count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("datastore: incrementing failed login attempts: %w", err)
	}
	return count, nil
}

// LockUser sets is_locked = true for the given user.
func (db *DB) LockUser(ctx context.Context, username string) error {
	res, err := db.pool.ExecContext(ctx, `
		UPDATE users SET is_locked = TRUE, updated_at = now() WHERE username = $1
	`, username)
	if err != nil {
		return fmt.Errorf("datastore: locking user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordLoginSuccess resets failed_login_attempts and updates last_login_at.
func (db *DB) RecordLoginSuccess(ctx context.Context, username string) error {
	res, err := db.pool.ExecContext(ctx, `
		UPDATE users
		   SET failed_login_attempts = 0,
		       last_login_at = now(),
		       updated_at = now()
		 WHERE username = $1
	`, username)
	if err != nil {
		return fmt.Errorf("datastore: recording login success: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// Seed helper
// ---------------------------------------------------------------------------

// EnsureDefaultAdmin creates a default admin user if no users exist.
// The caller must provide a pre-hashed password. Returns true if the user
// was created, false if users already exist.
func (db *DB) EnsureDefaultAdmin(ctx context.Context, passwordHash string) (bool, error) {
	count, err := db.CountUsers(ctx)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	_, err = db.InsertUser(ctx, InsertUserParams{
		Username:     "admin",
		DisplayName:  "Administrator",
		PasswordHash: passwordHash,
		Role:         "admin",
		AuthProvider: "local",
	})
	if err != nil {
		// If another goroutine/process raced us, treat as success.
		if errors.Is(err, ErrAlreadyExists) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// joinStrings joins a slice of strings with a separator. This is a trivial
// helper to avoid importing strings just for this.
func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}

// isUniqueViolation checks whether the error is a PostgreSQL unique
// violation (SQLSTATE 23505). We check by string matching to avoid
// importing the pq package in this file — the pq driver is already
// registered by the main package.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// lib/pq wraps PostgreSQL errors; the error string contains the SQLSTATE.
	return contains(err.Error(), "duplicate key value violates unique constraint") ||
		contains(err.Error(), "23505")
}

// contains is a simple substring check to avoid importing strings.
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
