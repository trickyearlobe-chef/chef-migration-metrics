// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipsql

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The screen must not offer a TLS mode the server then refuses.
//
// This is the same rule that already cost a customer an evening in a different
// guise: a format was displayed, somebody used it, and it was rejected. The two
// lists live in different languages in different files, so the only way to keep
// them honest is to read the displayed one and check it.
//
// It fails if a mode is added on either side alone, or if the list moves
// somewhere this cannot see — both worth a red build, because the screen is where
// people get their choices from.

const importScreen = "frontend/src/pages/OwnershipMappedImport.tsx"

// tlsModeList matches one driver's array in the TLS_MODES record, e.g.
//
//	postgres: ["disable", "allow"],
var tlsModeList = regexp.MustCompile(`(?m)^\s*(postgres|sqlserver):\s*\[([^\]]*)\]`)

func TestTheScreenOffersOnlyModesTheServerAccepts(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(repoRoot(t), filepath.FromSlash(importScreen)))
	if err != nil {
		t.Fatalf("cannot read the screen that offers these modes — if it moved, point "+
			"this test at its new home rather than deleting it: %v", err)
	}

	matches := tlsModeList.FindAllStringSubmatch(string(content), -1)
	if len(matches) == 0 {
		t.Fatal("found no TLS mode lists on the import screen; if they moved, update this test")
	}

	checked := 0
	for _, match := range matches {
		driver, list := match[1], match[2]
		for _, quoted := range strings.Split(list, ",") {
			mode := strings.Trim(strings.TrimSpace(quoted), `"'`)
			if mode == "" {
				continue
			}
			checked++
			if !isTLSMode(driver, mode) {
				t.Errorf("the screen offers %q for %s, which the server refuses\n"+
					"  the server accepts: %s", mode, driver, strings.Join(TLSModesFor(driver), ", "))
			}
		}
	}
	if checked == 0 {
		t.Fatal("no mode was checked, so this test proves nothing")
	}

	// And the other direction: a mode the server accepts but nothing offers is
	// unreachable, which is its own kind of wrong.
	for _, driver := range SupportedDrivers {
		for _, mode := range TLSModesFor(driver) {
			if !strings.Contains(string(content), `"`+mode+`"`) {
				t.Errorf("the server accepts %q for %s but the screen never offers it",
					mode, driver)
			}
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
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}
}
