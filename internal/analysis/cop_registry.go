// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"strings"
	"sync"
)

// CopRegistryEntry is one cop as reported by `cookstyle --show-cops`: the
// authoritative list of cops the running binary actually has. It carries the
// metadata the drift report and cop-list universe need — not the full config
// block (Include/Exclude/EnforcedStyle etc. are parsed but discarded).
type CopRegistryEntry struct {
	// CopName is the fully-qualified cop name, e.g. "Chef/Deprecations/NodeSet".
	CopName string
	// Department is the cop name minus its final segment, matching cookstyle's
	// department grouping, e.g. "Chef/Deprecations". Empty for a bare name.
	Department string
	// TopNamespace is the first path segment, e.g. "Chef" or "Style". Used to
	// separate migration-relevant Chef/* cops from generic-Ruby cops.
	TopNamespace string
	// Enabled reflects the cop's default Enabled flag in the ruleset.
	Enabled bool
	// Severity is the configured default severity ("warning"/"error"/…), if any.
	Severity string
	// Description is the free-text description, folded to a single line.
	Description string
	// VersionAdded is the gem version the cop was added in (NOT the Chef-Client
	// removal version — that is curated separately;).
	VersionAdded string
}

// ParseShowCops parses the textual output of `cookstyle --show-cops` into cop
// entries. The format is a header line, per-department `# Department '…'`
// comments, `# Supports --autocorrect` markers, then one column-0 `Dept/Name:`
// header per cop followed by 2-space-indented `Key: value` lines. Description
// values may wrap across further-indented continuation lines; list-valued keys
// (Include/Exclude) and unrecognised keys are ignored.
func ParseShowCops(output string) []CopRegistryEntry {
	var entries []CopRegistryEntry
	var cur *CopRegistryEntry
	var lastKey string

	flush := func() {
		if cur != nil {
			cur.Description = strings.TrimSpace(cur.Description)
			entries = append(entries, *cur)
			cur = nil
		}
		lastKey = ""
	}

	for line := range strings.SplitSeq(output, "\n") {
		if strings.TrimSpace(line) == "" {
			// Blank lines separate cops but also appear inside a block after
			// list values; a subsequent column-0 header starts the next cop, so
			// blanks alone don't flush.
			continue
		}
		// Comment/header lines (# Department, # Supports, # Available) — ignore.
		if strings.HasPrefix(line, "#") {
			continue
		}
		// Column-0 line ending in ':' is a cop header.
		if line[0] != ' ' && line[0] != '\t' {
			if name, ok := strings.CutSuffix(strings.TrimSpace(line), ":"); ok && strings.Contains(name, "/") {
				flush()
				cur = &CopRegistryEntry{
					CopName:      name,
					Department:   copDepartment(name),
					TopNamespace: copTopNamespace(name),
				}
			}
			continue
		}
		if cur == nil {
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		// A key line is at the block's base 2-space indent, "Key:" form, and not
		// a "- " list item. Deeper-indented or list lines are values.
		if indent == 2 && !strings.HasPrefix(trimmed, "- ") {
			if key, val, ok := strings.Cut(trimmed, ":"); ok && isBareKey(key) {
				lastKey = key
				applyCopKey(cur, key, strings.TrimSpace(val))
				continue
			}
		}
		// Continuation of a wrapped Description; ignore continuations/list items
		// for any other key.
		if lastKey == "Description" {
			if cur.Description != "" {
				cur.Description += " "
			}
			cur.Description += strings.TrimSpace(line)
		}
	}
	flush()
	return entries
}

// isBareKey reports whether s looks like a YAML key (letters/digits only), to
// avoid mistaking a wrapped Description sentence ("in {}. Wrapping…") — which
// can contain a colon — for a new key.
func isBareKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// applyCopKey records the keys the registry cares about; everything else is
// intentionally discarded.
func applyCopKey(e *CopRegistryEntry, key, val string) {
	switch key {
	case "Enabled":
		e.Enabled = val == "true"
	case "Severity":
		e.Severity = unquote(val)
	case "Description":
		e.Description = val
	case "VersionAdded":
		e.VersionAdded = unquote(val)
	}
}

// unquote strips surrounding single quotes cookstyle emits around some scalar
// values (e.g. VersionAdded: '0.46').
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// copDepartment returns the cop name minus its final segment.
func copDepartment(name string) string {
	if i := strings.LastIndex(name, "/"); i >= 0 {
		return name[:i]
	}
	return ""
}

// copTopNamespace returns the first path segment of a cop name.
func copTopNamespace(name string) string {
	top, _, _ := strings.Cut(name, "/")
	return top
}

// CopRegistry is a parsed, indexed snapshot of the cops a specific cookstyle
// binary reports.
type CopRegistry struct {
	Entries []CopRegistryEntry
	byName  map[string]CopRegistryEntry
	version string
}

// NewCopRegistry indexes the given entries under the reported cookstyle version.
func NewCopRegistry(entries []CopRegistryEntry, version string) *CopRegistry {
	byName := make(map[string]CopRegistryEntry, len(entries))
	for _, e := range entries {
		byName[e.CopName] = e
	}
	return &CopRegistry{Entries: entries, byName: byName, version: version}
}

// Version returns the cookstyle version this registry was captured from.
func (r *CopRegistry) Version() string { return r.version }

// Has reports whether the binary emits a cop by this exact name.
func (r *CopRegistry) Has(name string) bool {
	_, ok := r.byName[name]
	return ok
}

// Lookup returns the entry for a cop and whether it was found.
func (r *CopRegistry) Lookup(name string) (CopRegistryEntry, bool) {
	e, ok := r.byName[name]
	return e, ok
}

// ChefCops returns only the migration-relevant Chef/* cops, excluding generic
// Ruby departments (Style/Layout/Lint/Metrics/Bundler/…).
func (r *CopRegistry) ChefCops() []CopRegistryEntry {
	var out []CopRegistryEntry
	for _, e := range r.Entries {
		if e.TopNamespace == "Chef" {
			out = append(out, e)
		}
	}
	return out
}

// CopRegistryProvider lazily runs `cookstyle --show-cops` once and caches the
// parsed registry, keyed implicitly by the executor/version it was built with
// (the binary is re-detected at startup, so a version change rebuilds the
// provider). A failed load is not cached, so a transient failure can be retried.
type CopRegistryProvider struct {
	executor CookstyleExecutor
	version  string

	mu     sync.Mutex
	cached *CopRegistry
}

// NewCopRegistryProvider builds a provider over the given cookstyle executor and
// reported version.
func NewCopRegistryProvider(executor CookstyleExecutor, version string) *CopRegistryProvider {
	return &CopRegistryProvider{executor: executor, version: version}
}

// Registry returns the cached cop registry, loading it on first use. On a load
// or parse failure it returns the error and caches nothing, so callers degrade
// to the static universe and a later call can retry.
func (p *CopRegistryProvider) Registry(ctx context.Context) (*CopRegistry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached != nil {
		return p.cached, nil
	}
	stdout, _, _, err := p.executor.Run(ctx, "", "--show-cops")
	if err != nil {
		return nil, err
	}
	reg := NewCopRegistry(ParseShowCops(stdout), p.version)
	p.cached = reg
	return reg, nil
}
