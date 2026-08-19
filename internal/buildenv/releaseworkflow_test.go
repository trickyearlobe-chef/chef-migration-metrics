// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package buildenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The release workflow publishes a zip of the repository including its .git
// directory. Two lines in that job decide what ends up inside it, and both are
// the kind of line an editor or a dependency bump removes without comment.
// These tests fail when that happens, and say what it costs.

const sourceArchiveJob = "source-archive"

func releaseWorkflow(t *testing.T) string {
	t.Helper()

	path := filepath.Join("..", "..", ".github", "workflows", "release.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

// jobBlock returns the lines of one job, from its `  <name>:` header to the
// next job at the same indent. Text rather than YAML parsing: this needs no
// dependency, and the checks below are about lines being present at all.
func jobBlock(t *testing.T, name string) string {
	t.Helper()

	src := releaseWorkflow(t)
	header := "  " + name + ":"

	lines := strings.Split(src, "\n")
	start := -1
	for i, line := range lines {
		if line == header {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("release.yml has no %q job.\n"+
			"It produces the only archive carrying the repository's history and tags; the\n"+
			"generated source zips hold the tagged tree alone. If it was removed on purpose,\n"+
			"remove these tests in the same change.", name)
	}

	for i := start + 1; i < len(lines); i++ {
		l := lines[i]
		if len(l) > 2 && l[0] == ' ' && l[1] == ' ' && l[2] != ' ' && strings.HasSuffix(strings.TrimSpace(l), ":") {
			return strings.Join(lines[start:i], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// TestSourceArchiveDoesNotPersistCredentials fails when the checkout in the
// source-archive job stops disabling credential persistence.
//
// actions/checkout otherwise writes an http.extraheader holding a token into
// .git/config. That file is inside the published archive, so the token would be
// published with it — and the archive is meant to be copied onto other servers.
func TestSourceArchiveDoesNotPersistCredentials(t *testing.T) {
	block := jobBlock(t, sourceArchiveJob)

	if !strings.Contains(block, "persist-credentials: false") {
		t.Errorf("the %s job's checkout no longer sets `persist-credentials: false`.\n"+
			"actions/checkout writes a token into .git/config, and this job publishes .git.\n"+
			"Restore the line rather than relying on `git clone` to drop it.", sourceArchiveJob)
	}
}

// TestSourceArchiveClonesWithNoLocal fails when the clone loses --no-local.
//
// Cloning a path without it copies the object store wholesale, so every other
// branch's commits travel inside the archive with no ref pointing at them.
// `git branch` in the unpacked copy still shows only main, which is what makes
// this worth a test: the archive looks correct and is not.
func TestSourceArchiveClonesWithNoLocal(t *testing.T) {
	block := jobBlock(t, sourceArchiveJob)

	if !strings.Contains(block, "git clone") {
		t.Fatalf("the %s job no longer clones; these assertions describe a job that "+
			"no longer exists", sourceArchiveJob)
	}
	if !strings.Contains(block, "--no-local") {
		t.Errorf("the %s job's `git clone` no longer passes --no-local.\n"+
			"Without it git copies the whole object store, so branches other than main end\n"+
			"up in the archive as unreferenced objects — invisible to `git branch`, and\n"+
			"recoverable by anyone holding the file.", sourceArchiveJob)
	}
	if !strings.Contains(block, "--single-branch") {
		t.Errorf("the %s job's `git clone` no longer passes --single-branch, so the archive\n"+
			"carries every branch rather than main alone.", sourceArchiveJob)
	}
}
