// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

//go:build functional

package ownershipsql

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// pingWith opens a connection and does nothing else with it. Used by the TLS
// tests, where whether the handshake happens at all is the whole question.
func pingWith(t *testing.T, dsn string) error {
	t.Helper()

	db, err := sql.Open(DriverSQLServer, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}
