// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// A connection proposed on screen must be one this code can actually send.
//
// This replaces a check on a TLS control that no longer exists. That control
// was an override which silently won over whatever the connection already said,
// and its stated reason — that changing a TLS setting meant retyping a whole
// encrypted connection, password included — disappeared when the connection
// became something the administrator can read and edit.
//
// The knowledge it held did not go with it. The four friendly SQL Server modes
// mapped onto pairs of driver options, measured against a real server because
// "encrypt=false" and saying nothing parse identically and then behave
// differently. That mapping is now in the proposed connections and the note
// beside them, which is where somebody writing a connection is looking.
//
// So what is checked here is the proposal itself: that each one composes and
// names its database. A proposal that is usually wrong is worse than none,
// because it turns setting a connection up into correcting one — and one that
// cannot even be sent is the worst of that kind.

var connectionPanel = filepath.Join("..", "..", "frontend", "src", "pages",
	"OwnershipConnectionPanel.tsx")

// proposedConnection matches one entry of the PROPOSED record. Each is written
// as a join round the marker rather than as one string, so that no line in that
// file has the shape of a real connection carrying a password — so this reads
// the expression and puts it back together.
//
//	sqlserver:
//	  "sqlserver://user:" + PASSWORD_MARKER + "@host:1433?database=cmdb",
var proposedConnection = regexp.MustCompile(`(?m)^\s*(postgres|sqlserver):\s*\n?\s*("[^\n]+),\s*$`)

// proposedBlock isolates the PROPOSED record, so the note beside it — which
// talks about the same two databases in prose — is not read as a connection.
var proposedBlock = regexp.MustCompile(`(?s)const PROPOSED[^{]*\{(.*?)\n\};`)

// asWritten puts a joined proposal back into the string a browser would render.
func asWritten(expression string) string {
	joined := strings.ReplaceAll(expression, `" + PASSWORD_MARKER + "`, PasswordMarker)
	return strings.Trim(strings.TrimSpace(joined), `"`)
}

func TestTheConnectionsTheScreenProposesCanActuallyBeSent(t *testing.T) {
	content, err := os.ReadFile(connectionPanel)
	if err != nil {
		t.Fatalf("cannot read the screen that proposes these connections — if it moved, "+
			"point this test at its new home rather than deleting it: %v", err)
	}

	block := proposedBlock.FindStringSubmatch(string(content))
	if block == nil {
		t.Fatal("the screen no longer declares the connections it proposes; if they moved, " +
			"update this test rather than deleting the case")
	}
	matches := proposedConnection.FindAllStringSubmatch(block[1], -1)
	if len(matches) == 0 {
		t.Fatal("the screen proposes no connection at all; if the proposals moved, update " +
			"this test rather than deleting the case")
	}

	for _, match := range matches {
		driver, proposal := match[1], asWritten(match[2])

		if err := secrets.ValidateDatabaseURL(proposal); err != nil {
			t.Errorf("the %s connection offered as a starting point names no database: %v",
				driver, err)
		}
		composed, err := Compose(driver, proposal, "irrelevant")
		if err != nil {
			t.Errorf("the %s connection offered as a starting point cannot be composed, so "+
				"anybody starting from it is refused: %v", driver, err)
			continue
		}
		// The scheme has to agree with the database it is offered for, or the
		// screen proposes a connection that reads as the other database and
		// fails as a login error somewhere else entirely.
		if composed.Form == FormURL && !strings.HasPrefix(strings.ToLower(proposal), driver) {
			t.Errorf("the connection offered for %s does not begin with its own scheme: %s",
				driver, proposal)
		}
	}
}
