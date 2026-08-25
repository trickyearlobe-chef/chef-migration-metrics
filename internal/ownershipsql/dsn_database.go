// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import "github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"

// A connection must name the database it reads. See journeys/ownership-intake.md.
//
// One implementation, in internal/secrets next to the credential type that
// applies it when a connection is stored; this calls it. A second copy drifts,
// and then a connection is accepted when stored and refused when used, which
// puts the refusal in front of somebody who cannot fix it.
//
// Checked here as well as at storage time because a connection stored as a
// generic secret reaches the database without ever having been looked at.
func validateDSNNamesDatabase(driver, dsn string) error {
	return secrets.ValidateDatabaseURL(dsn)
}
