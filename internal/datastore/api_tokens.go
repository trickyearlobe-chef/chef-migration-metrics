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

// APIToken is a credential a person made for a tool they are holding. It is
// another way into their own account, so it carries no role of its own — the
// role is read from the account at every request.
//
// The secret is not here and cannot be. Only its hash is stored, and the
// plaintext is returned once, at creation.
type APIToken struct {
	ID       string `json:"id"`
	Username string `json:"-"`

	// Name is what its owner called it, so a listing can be read and one of
	// them destroyed without guessing.
	Name string `json:"name"`

	// CanWrite is chosen when the credential is made. False means it can ask
	// questions and record nothing.
	CanWrite bool `json:"can_write"`

	CreatedAt time.Time `json:"created_at"`

	// LastUsedAt is nil until the credential is first used, and thereafter
	// accurate to about a minute — see TouchAPITokenLastUsed.
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// InsertAPITokenParams creates a credential. The caller hashes the secret;
// this layer never sees the plaintext.
type InsertAPITokenParams struct {
	Username  string
	Name      string
	TokenHash string
	CanWrite  bool
}

// apiTokenColumns is the SELECT column list for the api_tokens table. Note
// what is absent: there is no column holding the secret to select.
const apiTokenColumns = `id, username, name, can_write, created_at, last_used_at`

func scanAPIToken(row interface{ Scan(dest ...any) error }) (APIToken, error) {
	var t APIToken
	var lastUsed sql.NullTime

	err := row.Scan(&t.ID, &t.Username, &t.Name, &t.CanWrite, &t.CreatedAt, &lastUsed)
	if err != nil {
		return APIToken{}, err
	}
	if lastUsed.Valid {
		used := lastUsed.Time
		t.LastUsedAt = &used
	}
	return t, nil
}

// InsertAPIToken stores a new credential and returns it. What comes back never
// includes the secret, because the row does not hold one.
func (db *DB) InsertAPIToken(ctx context.Context, p InsertAPITokenParams) (APIToken, error) {
	query := `
		INSERT INTO api_tokens (username, name, token_hash, can_write)
		VALUES ($1, $2, $3, $4)
		RETURNING ` + apiTokenColumns

	row := db.pool.QueryRowContext(ctx, query, p.Username, p.Name, p.TokenHash, p.CanWrite)

	t, err := scanAPIToken(row)
	if err != nil {
		return APIToken{}, fmt.Errorf("datastore: inserting api token: %w", err)
	}
	return t, nil
}

// ListAPITokensByUsername returns the credentials belonging to one account,
// newest first. This is what its owner sees: enough to recognise one and
// destroy it, and nothing that could be used as one.
func (db *DB) ListAPITokensByUsername(ctx context.Context, username string) ([]APIToken, error) {
	query := `SELECT ` + apiTokenColumns + `
		FROM api_tokens WHERE username = $1 ORDER BY created_at DESC`

	rows, err := db.pool.QueryContext(ctx, query, username)
	if err != nil {
		return nil, fmt.Errorf("datastore: listing api tokens: %w", err)
	}
	defer func() { _ = rows.Close() }()

	tokens := []APIToken{}
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, fmt.Errorf("datastore: scanning api token: %w", err)
		}
		tokens = append(tokens, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("datastore: reading api tokens: %w", err)
	}
	return tokens, nil
}

// GetAPITokenByHash looks up a credential by the hash of the secret presented.
// Returns ErrNotFound when nothing matches, which is also what a destroyed
// credential looks like — immediately, because the row is gone rather than
// marked.
func (db *DB) GetAPITokenByHash(ctx context.Context, tokenHash string) (APIToken, error) {
	query := `SELECT ` + apiTokenColumns + ` FROM api_tokens WHERE token_hash = $1`
	row := db.pool.QueryRowContext(ctx, query, tokenHash)

	t, err := scanAPIToken(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return APIToken{}, ErrNotFound
		}
		return APIToken{}, fmt.Errorf("datastore: getting api token: %w", err)
	}
	return t, nil
}

// DeleteAPIToken destroys one credential belonging to the named account.
//
// Scoped by username as well as id so that "destroy mine" cannot reach
// somebody else's by guessing an id, and so a missing row and another
// person's row are refused identically.
func (db *DB) DeleteAPIToken(ctx context.Context, username, id string) error {
	res, err := db.pool.ExecContext(ctx,
		`DELETE FROM api_tokens WHERE id = $1 AND username = $2`, id, username)
	if err != nil {
		return fmt.Errorf("datastore: deleting api token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("datastore: deleting api token: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAPITokensByUsername destroys every credential belonging to an account.
// Called when the account is deleted: access made from an account leaves with
// it, or the account was never the thing granting access.
func (db *DB) DeleteAPITokensByUsername(ctx context.Context, username string) (int, error) {
	res, err := db.pool.ExecContext(ctx, `DELETE FROM api_tokens WHERE username = $1`, username)
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting api tokens for user: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("datastore: deleting api tokens for user: %w", err)
	}
	return int(n), nil
}

// TouchAPITokenLastUsed records that a credential was just used.
//
// At most one write a minute per credential. The authentication path runs on
// every request an assistant makes, and the question this answers is only "is
// this one still in use" — a write per request would put a row update in front
// of every read the service serves, to sharpen a timestamp nobody reads to the
// second.
func (db *DB) TouchAPITokenLastUsed(ctx context.Context, id string) error {
	_, err := db.pool.ExecContext(ctx, `
		UPDATE api_tokens SET last_used_at = now()
		WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - INTERVAL '1 minute')`, id)
	if err != nil {
		return fmt.Errorf("datastore: touching api token: %w", err)
	}
	return nil
}
