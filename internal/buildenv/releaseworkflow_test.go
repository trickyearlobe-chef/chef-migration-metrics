// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package buildenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Guards on the source-archive job: what the published zip may contain.

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

// jobBlock returns one job's lines, from its header to the next job.
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
		t.Fatalf("release.yml has no %q job; remove these tests with it", name)
	}

	for i := start + 1; i < len(lines); i++ {
		l := lines[i]
		if len(l) > 2 && l[0] == ' ' && l[1] == ' ' && l[2] != ' ' && strings.HasSuffix(strings.TrimSpace(l), ":") {
			return strings.Join(lines[start:i], "\n")
		}
	}
	return strings.Join(lines[start:], "\n")
}

// actions/checkout writes a token into .git/config, and .git is published.
func TestSourceArchiveDoesNotPersistCredentials(t *testing.T) {
	block := jobBlock(t, sourceArchiveJob)

	if !strings.Contains(block, "persist-credentials: false") {
		t.Errorf("%s: checkout must set `persist-credentials: false`; .git is published",
			sourceArchiveJob)
	}
}

// Without --no-local, clone copies the whole object store: every branch ships,
// unreferenced and invisible to `git branch`.
func TestSourceArchiveClonesWithNoLocal(t *testing.T) {
	block := jobBlock(t, sourceArchiveJob)

	if !strings.Contains(block, "git clone") {
		t.Fatalf("%s: no `git clone`", sourceArchiveJob)
	}
	if !strings.Contains(block, "--no-local") {
		t.Errorf("%s: `git clone` must pass --no-local, or every branch ships as "+
			"unreferenced objects", sourceArchiveJob)
	}
	if !strings.Contains(block, "--single-branch") {
		t.Errorf("%s: `git clone` must pass --single-branch", sourceArchiveJob)
	}
}

// Tag auto-following varies by git version, so the job must not rely on it.
func TestSourceArchiveTakesTagsExplicitly(t *testing.T) {
	block := jobBlock(t, sourceArchiveJob)

	if !strings.Contains(block, "refs/tags/*:refs/tags/*") {
		t.Errorf("%s: tags must be fetched by refspec, not left to clone", sourceArchiveJob)
	}
}

// A stray-tags-only check passes an archive that is missing tags.
func TestSourceArchiveChecksTagsInBothDirections(t *testing.T) {
	block := jobBlock(t, sourceArchiveJob)

	if !strings.Contains(block, "--no-merged main") {
		t.Errorf("%s: must reject tags not reachable from main", sourceArchiveJob)
	}
	if !strings.Contains(block, "is missing tags that are on main") {
		t.Errorf("%s: must reject an archive that is missing tags", sourceArchiveJob)
	}
}
