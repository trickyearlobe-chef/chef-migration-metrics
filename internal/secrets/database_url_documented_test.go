// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"regexp"
	"testing"
)

// The refusal has to recommend a shape it would accept.
//
// The companion check — that the connections the screen proposes can actually be
// sent — lives next to the code that composes them, as
// TestTheConnectionsTheScreenProposesCanActuallyBeSent in internal/ownershipsql.
// The screen proposes a whole connection joined round the password marker, so no
// line of it has the shape of a real connection carrying a password; reading it
// needs to know how it is written.

// documentedExample matches a connection string as written in prose, stopping
// at a quote, whitespace or a "\n" escape.
var documentedExample = regexp.MustCompile(
	`(?:jdbc:)?(?:postgres|postgresql|sqlserver|mssql)://[^"'` + "`" + `\s\\]+`)

// The refusal names two shapes as the way to fix it. Somebody reading it will
// copy one, so both have to be accepted.
func TestDatabaseURL_AcceptsTheShapesItsOwnRefusalRecommends(t *testing.T) {
	recommended := documentedExample.FindAllString(ErrDatabaseURLNamesNoDatabase.Error(), -1)
	if len(recommended) == 0 {
		t.Fatal("the refusal recommends no shape, so it tells the reader nothing to do")
	}
	for _, dsn := range recommended {
		if err := ValidateDatabaseURL(dsn); err != nil {
			t.Errorf("the refusal recommends a shape it would refuse\n  shows: %s\n  error: %v", dsn, err)
		}
	}
}
