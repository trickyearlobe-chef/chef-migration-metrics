// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// If we put a format in front of somebody, we have to accept that format.
//
// A colleague was given the on-screen example, asked whether extra options could
// be added, was told to try it, and was refused — by a check that named the
// driver when the driver was right. The example and the validator disagreeing is
// the whole failure, so it is not enough for both to be tested separately: the
// examples themselves are read out of the files that display them and put
// through the validator.
//
// This fails if somebody changes an on-screen example to a shape we refuse, or
// moves it somewhere this test cannot see — both of which are worth a red build,
// because the example is where people get the format from.

// documentedExample matches a connection string as written in a source file,
// stopping at a quote, whitespace or a "\n" escape.
var documentedExample = regexp.MustCompile(
	`(?:jdbc:)?(?:postgres|postgresql|sqlserver|mssql)://[^"'` + "`" + `\s\\]+`)

// filesThatShowAFormat are the places a connection format is put in front of a
// person. Paths are repo-relative; a missing one fails rather than skips.
var filesThatShowAFormat = []string{
	"frontend/src/pages/OwnershipMappedImport.tsx",
	"frontend/src/pages/credentials/ValueField.tsx",
}

func TestDatabaseURL_AcceptsEveryFormatWeShowOnScreen(t *testing.T) {
	root := repoRoot(t)

	var found int
	for _, rel := range filesThatShowAFormat {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("cannot read a file that shows a connection format — if it moved, "+
				"point this test at its new home rather than deleting the case: %v", err)
			continue
		}
		examples := documentedExample.FindAllString(string(content), -1)
		if len(examples) == 0 {
			t.Errorf("%s showed a connection format when this was written and now shows none; "+
				"if the example moved, update this test", rel)
			continue
		}
		for _, example := range examples {
			found++
			// The examples use placeholder words where a real value goes. They
			// are validated exactly as displayed, because that is what somebody
			// copying them will paste.
			if err := ValidateDatabaseURL(example); err != nil {
				t.Errorf("we display a format we then refuse\n  file:  %s\n  shows: %s\n  error: %v",
					rel, example, err)
			}
		}
	}
	if found == 0 {
		t.Fatal("no on-screen format was checked, so this test proves nothing")
	}
}

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

// repoRoot walks up to the directory holding go.mod, so this survives the
// package being moved.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot find the working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", strings.TrimSpace(dir))
		}
		dir = parent
	}
}
