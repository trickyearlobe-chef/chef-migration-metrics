// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Addon cops are operator-supplied RuboCop cop files (real `.rb` cop classes)
// placed on the app host and referenced from config. They are require:'d into
// the scan sidecar so cookstyle loads them alongside its own cops. The trust
// boundary is deploying the app — addon cops are never web-uploaded.
//
// This file resolves the configured path entries (files, directories, globs)
// into concrete `.rb` cop files, and parses each cop's name so the sidecar can
// enable it explicitly. A required-but-unconfigured custom cop is registered by
// RuboCop but does NOT run (neither AllCops.NewCops nor --enable-pending-cops
// turns it on) — only an explicit `<CopName>: { Enabled: true }` entry does, and
// that needs the cop's name.
//
// Resolution is isolated: an entry that doesn't resolve (missing path, non-.rb
// file, empty directory, zero-match glob, malformed glob) or a .rb with no
// recognisable cop class is reported as a problem for the caller to surface
// (admin/log) and never aborts resolution of the other entries.

// AddonCop is a resolved operator addon cop file plus the cop name(s) it
// defines, so the sidecar can both require the file and enable its cops.
type AddonCop struct {
	// Path is the absolute path to the `.rb` cop file to require.
	Path string
	// CopNames are the RuboCop cop names defined in the file (e.g.
	// "Cmm/NoNodeRegexMatch"), used to enable the otherwise-pending cops.
	CopNames []string
}

// AddonCopProblem describes a configured addon-cop path entry that could not be
// fully resolved. It is advisory — the scan still runs, just without that
// entry's cops (or without enabling a cop whose name could not be parsed).
type AddonCopProblem struct {
	// Path is the configured entry (or resolved file) that had a problem.
	Path string
	// Reason is a human-readable explanation for surfacing.
	Reason string
}

func (p AddonCopProblem) String() string { return fmt.Sprintf("%s (%s)", p.Path, p.Reason) }

// hasGlobMeta reports whether s contains shell-glob metacharacters.
func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// isRubyFile reports whether path has a .rb extension.
func isRubyFile(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".rb")
}

// ResolveAddonCopFiles expands the configured path entries into deduplicated,
// sorted addon cop files with their parsed cop names.
//
// Each entry may be:
//   - a glob pattern (containing *, ?, or [) — expanded; only matching .rb
//     files are kept;
//   - a directory — expanded to its top-level *.rb files;
//   - a plain .rb file — used directly.
//
// Entries that resolve to nothing usable, and resolved files with no
// recognisable cop class, are returned as problems; they do not abort
// resolution. Blank entries are ignored silently.
func ResolveAddonCopFiles(paths []string) (cops []AddonCop, problems []AddonCopProblem) {
	seen := make(map[string]struct{})
	var files []string
	addFile := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if _, dup := seen[abs]; dup {
			return
		}
		seen[abs] = struct{}{}
		files = append(files, abs)
	}

	for _, raw := range paths {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}

		if hasGlobMeta(entry) {
			matches, err := filepath.Glob(entry)
			if err != nil {
				problems = append(problems, AddonCopProblem{Path: entry, Reason: "invalid glob pattern: " + err.Error()})
				continue
			}
			var rbCount int
			for _, m := range matches {
				if isRubyFile(m) {
					if fi, statErr := os.Stat(m); statErr == nil && !fi.IsDir() {
						addFile(m)
						rbCount++
					}
				}
			}
			if rbCount == 0 {
				problems = append(problems, AddonCopProblem{Path: entry, Reason: "glob matched no .rb files"})
			}
			continue
		}

		fi, err := os.Stat(entry)
		if err != nil {
			problems = append(problems, AddonCopProblem{Path: entry, Reason: "path does not exist"})
			continue
		}

		if fi.IsDir() {
			rbFiles, dirProblem := rubyFilesInDir(entry)
			if dirProblem != "" {
				problems = append(problems, AddonCopProblem{Path: entry, Reason: dirProblem})
				continue
			}
			for _, f := range rbFiles {
				addFile(f)
			}
			continue
		}

		if !isRubyFile(entry) {
			problems = append(problems, AddonCopProblem{Path: entry, Reason: "not a .rb file"})
			continue
		}
		addFile(entry)
	}

	sort.Strings(files)
	for _, f := range files {
		names := ParseAddonCopNames(f)
		if len(names) == 0 {
			problems = append(problems, AddonCopProblem{Path: f, Reason: "no RuboCop cop class found; the file is loaded but its cops cannot be enabled"})
		}
		cops = append(cops, AddonCop{Path: f, CopNames: names})
	}
	return cops, problems
}

// AddonCopPaths returns just the require paths from a slice of resolved addon
// cops (handy for callers that only need the file list).
func AddonCopPaths(cops []AddonCop) []string {
	if len(cops) == 0 {
		return nil
	}
	paths := make([]string, len(cops))
	for i, c := range cops {
		paths[i] = c.Path
	}
	return paths
}

// rubyFilesInDir lists the top-level *.rb files in dir. The returned slice is
// sorted. A directory with no .rb files yields an empty slice and a non-empty
// problem reason.
func rubyFilesInDir(dir string) (files []string, problem string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "cannot read directory: " + err.Error()
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if isRubyFile(e.Name()) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		return nil, "directory contains no .rb files"
	}
	sort.Strings(files)
	return files, ""
}

// ---------------------------------------------------------------------------
// Cop name parsing
// ---------------------------------------------------------------------------

var (
	addonModuleRe = regexp.MustCompile(`^\s*module\s+([A-Za-z0-9_:]+)`)
	addonClassRe  = regexp.MustCompile(`^\s*class\s+([A-Za-z0-9_:]+)\s*<\s*([A-Za-z0-9_:]+)`)
)

// ParseAddonCopNames reads a RuboCop cop file and returns the cop names it
// defines (e.g. "Cmm/NoNodeRegexMatch"), derived from the nested
// module/class declarations under RuboCop::Cop. It recognises both the
// conventional nested form and a flat `class RuboCop::Cop::Dept::Name < Base`
// form. Files that don't follow these conventions yield no names (the caller
// surfaces that as a problem). Returns nil on read error.
func ParseAddonCopNames(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var modules []string
	var names []string
	seen := make(map[string]bool)

	for _, line := range strings.Split(string(data), "\n") {
		if m := addonModuleRe.FindStringSubmatch(line); m != nil {
			modules = append(modules, strings.Split(m[1], "::")...)
			continue
		}
		if c := addonClassRe.FindStringSubmatch(line); c != nil {
			if !looksLikeCopParent(c[2]) {
				continue
			}
			segs := append(append([]string{}, modules...), strings.Split(c[1], "::")...)
			if name := copNameFromSegments(segs); name != "" && !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

// looksLikeCopParent reports whether a superclass token denotes a RuboCop cop
// base class (RuboCop::Cop::Base, Base, RuboCop::Cop::Cop, Cop, …).
func looksLikeCopParent(parent string) bool {
	return strings.Contains(parent, "Base") || parent == "Cop" || strings.HasSuffix(parent, "::Cop")
}

// copNameFromSegments turns a namespace segment list into a RuboCop cop name by
// dropping the leading RuboCop and Cop segments and joining the rest with "/".
func copNameFromSegments(segs []string) string {
	i := 0
	if i < len(segs) && segs[i] == "RuboCop" {
		i++
	}
	if i < len(segs) && segs[i] == "Cop" {
		i++
	}
	rest := segs[i:]
	if len(rest) == 0 {
		return ""
	}
	return strings.Join(rest, "/")
}
