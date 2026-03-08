// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"os/exec"
)

// makeCommand creates an *exec.Cmd for the given program and arguments.
// It is a thin wrapper around exec.CommandContext, extracted into its own
// function so that the cookstyle scanner can reference it without importing
// os/exec directly in the same file (keeping imports tidy) and so that
// future enhancements (e.g. environment variable injection) have a single
// place to land.
func makeCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
