// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package migrations embeds the SQL migration files into the Go binary so
// that deployments are fully self-contained — no external migrations
// directory is required at runtime.
//
// This package lives inside migrations/ (alongside the .sql files) because
// embed paths are relative to the source file and cannot use "..".
// Placing it here lets the directive reference *.sql directly.
//
// The internal/datastore package can accept an fs.FS for migration discovery,
// falling back to disk when no embedded FS is registered.
package migrations

import "embed"

// content holds every .sql file in the migrations directory. Both .up.sql
// and .down.sql files are included so the binary can support rollbacks.
//
//go:embed *.sql
var content embed.FS

// FS returns the embedded filesystem containing all migration SQL files.
// The returned fs.FS has files at its root (e.g. "0001_initial_schema.up.sql").
func FS() embed.FS {
	return content
}
