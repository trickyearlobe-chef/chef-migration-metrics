// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fullOffense extends the basic parsed offense with the correctable flag.
type fullOffense struct {
	CopName     string `json:"cop_name"`
	Severity    string `json:"severity"`
	Correctable bool   `json:"correctable"`

	// Location carries the repo-relative path, which decides whether this
	// offense is about the cookbook or about a helper task that never runs on a
	// converging node. Dropping it here is what let one copied Rakefile make
	// nearly every cookbook read as broken.
	Location struct {
		File string `json:"file"`
	} `json:"location"`
}

// parseFullOffenses parses the offences JSONB into a flat list that includes
// the correctable field. Handles both file-based and flat formats.
func parseFullOffenses(data []byte) []fullOffense {
	if len(data) == 0 {
		return nil
	}

	// Try file-based RuboCop format first.
	type fileOffense struct {
		CopName     string `json:"cop_name"`
		Severity    string `json:"severity"`
		Correctable bool   `json:"correctable"`
	}
	type fileEntry struct {
		Path     string        `json:"path"`
		Offenses []fileOffense `json:"offenses"`
	}

	var fileEntries []fileEntry
	if err := json.Unmarshal(data, &fileEntries); err == nil && len(fileEntries) > 0 && fileEntries[0].Path != "" {
		var result []fullOffense
		for _, fe := range fileEntries {
			for _, o := range fe.Offenses {
				// In this legacy shape the path lives on the group; scope is
				// decided per offense, so carry it down.
				item := fullOffense{CopName: o.CopName, Severity: o.Severity, Correctable: o.Correctable}
				item.Location.File = fe.Path
				result = append(result, item)
			}
		}
		return result
	}

	// Try flat format.
	var flat []fullOffense
	if err := json.Unmarshal(data, &flat); err == nil {
		return flat
	}

	return nil
}

// extractCopNameFromPath extracts the cop name from a path like
// /api/v1/cookstyle/cops/Chef/Deprecations/NodeSet/cookbooks
// The cop name is everything between "/cops/" and "/cookbooks".
func copNamespace(copName string) string {
	parts := strings.Split(copName, "/")
	if len(parts) < 3 {
		return copName
	}
	return strings.Join(parts[:2], "/") + "/"
}

func extractCopNameFromPath(path string) string {
	const prefix = "/api/v1/cookstyle/cops/"
	const suffix = "/cookbooks"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	start := len(prefix)
	end := len(path) - len(suffix)
	if end <= start {
		return ""
	}
	return path[start:end]
}

// extractCopNameFromClassificationPath extracts the cop name from
// /api/v1/cookstyle/cops/Chef/Deprecations/NodeSet/classification
func extractCopNameFromClassificationPath(path string) string {
	const prefix = "/api/v1/cookstyle/cops/"
	const suffix = "/classification"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	start := len(prefix)
	end := len(path) - len(suffix)
	if end <= start {
		return ""
	}
	return path[start:end]
}

// extractCustomCopName extracts the cop name from /api/v1/cookstyle/custom-cops/Custom/...
func extractCustomCopName(path string) string {
	const prefix = "/api/v1/cookstyle/custom-cops/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	name := path[len(prefix):]
	name = strings.TrimSuffix(name, "/")
	return name
}

// validateCustomCop validates a custom cop definition.
func validateCustomCop(c datastore.CustomCopDefinition) error {
	if c.CopName == "" {
		return errMissingField("cop_name")
	}
	if !strings.HasPrefix(c.CopName, "Custom/") {
		return errInvalidField("cop_name", "must start with 'Custom/'")
	}
	if c.Pattern == "" {
		return errMissingField("pattern")
	}
	switch c.PatternType {
	case "regex", "literal":
		// valid
	default:
		return errInvalidField("pattern_type", "must be 'regex' or 'literal'")
	}
	switch c.Classification {
	case "blocker", "review", "noise":
		// valid
	case "":
		return errMissingField("classification")
	default:
		return errInvalidField("classification", "must be one of: blocker, review, noise")
	}
	return nil
}

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func errMissingField(field string) error {
	return &validationError{msg: field + " is required"}
}

func errInvalidField(field, reason string) error {
	return &validationError{msg: field + ": " + reason}
}

// ---------------------------------------------------------------------------
// Re-evaluation propagation + audit helpers
// ---------------------------------------------------------------------------

// enqueueReclassification schedules an asynchronous, coalesced reassessment for a
// cop × target. The scoped recompute closure is O(repos) and slow at scale, so it
// must not run on the request path — the classification write has already
// succeeded synchronously. Lazily starts the queue worker on first use.
func (r *Router) enqueueReclassification(copName, targetVersion string) {
	r.reclassQueueOnce.Do(func() {
		r.reclassQueue = newReclassificationQueue(r.runReclassification)
	})
	r.reclassQueue.enqueue(copName, targetVersion)
}

// runReclassification is the queue worker body: run the scoped recompute closure
// and emit the WebSocket recompute event so the UI refreshes when it completes.
func (r *Router) runReclassification(ctx context.Context, copName, targetVersion string) {
	prop := r.propagateCop(ctx, copName, targetVersion)
	r.emitCookstyleRecomputed(copName, targetVersion, prop)
}

// propagateCop runs the scoped recompute closure for a single cop × target
// version. Best-effort: a nil propagator or an error is logged, never fatal —
// the classification write has already succeeded. Returns the result (zero value
// when no propagator is wired) for inclusion in the response and audit details.
func (r *Router) propagateCop(ctx context.Context, copName, targetVersion string) PropagationResult {
	if r.cookstylePropagator == nil {
		return PropagationResult{Target: targetVersion}
	}
	res, err := r.cookstylePropagator.PropagateReclassification(ctx, copName, targetVersion)
	if err != nil {
		r.logf("ERROR", "cookstyle propagation for cop %q target %q: %v", copName, targetVersion, err)
	}
	return res
}

// propagateCustomCop runs the recompute closure for a custom-cop change across
// every configured target version (custom-cop classification is target-agnostic)
// and records a criteria-change audit entry.
func (r *Router) propagateCustomCop(ctx context.Context, req *http.Request, action, copName string) {
	var results []PropagationResult
	if r.cookstylePropagator != nil {
		for _, t := range r.liveConfig().TargetChefVersionList() {
			results = append(results, r.propagateCop(ctx, copName, t))
		}
	}
	r.auditCookstyle(req, action, copName, "", map[string]any{"propagation": results})
	r.emitCookstyleRecomputed(copName, "", PropagationResult{})
}

// emitCookstyleRecomputed broadcasts a status-changed event so open UI pages
// refresh after a criteria change propagates (spec: Re-evaluation & Propagation
// step 6). Nil-safe when no event hub is wired.
func (r *Router) emitCookstyleRecomputed(copName, targetVersion string, prop PropagationResult) {
	if r.hub == nil {
		return
	}
	r.hub.Broadcast(NewEvent(EventCookbookStatusChanged, map[string]any{
		"cause":               "cookstyle_reclassification",
		"cop_name":            copName,
		"target_chef_version": targetVersion,
		"propagation":         prop,
	}))
}

// auditCookstyle records a CookStyle criteria-change event for explainability.
// Best-effort — a write failure is logged but never blocks the request.
func (r *Router) auditCookstyle(req *http.Request, action, copName, targetVersion string, details map[string]any) {
	var raw json.RawMessage
	if details != nil {
		if b, err := json.Marshal(details); err == nil {
			raw = b
		}
	}
	if err := r.db.InsertCookstyleAuditEntry(req.Context(), datastore.InsertCookstyleAuditParams{
		Action:            action,
		Actor:             adminUsername(req),
		CopName:           copName,
		TargetChefVersion: targetVersion,
		Details:           raw,
	}); err != nil {
		r.logf("WARN", "cookstyle: failed to write audit log: %v", err)
	}
}
