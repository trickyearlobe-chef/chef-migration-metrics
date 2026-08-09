// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package ingest normalises Chef run telemetry from three producer shapes
// (node-direct run_converge, Chef Server proxy relay, Chef Automate Data Feed)
// into a single ConvergeRun. It is the source of truth for that mapping, pinned
// by a contract test against the golden fixtures in testdata/event-ingest/. See
// journeys/run-history.md.
package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// runTime is a run timestamp decoded from either encoding Chef producers use:
//   - an RFC3339 string ("2026-07-16T09:01:12Z") — raw run_converge (node-direct
//     and Chef Server proxy), and
//   - a protobuf Timestamp object {"seconds":N,"nanos":M} — the Chef Automate
//     Data Feed's client_run (measured live 2026-07-18; the authored fixtures
//     wrongly used a string, so every real Data Feed record was silently dropped
//     on the zero-value end_time guard).
//
// A bare numeric epoch is also tolerated defensively. null / absent / empty
// decode to the zero time.
type runTime struct{ time.Time }

func (t *runTime) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	switch b[0] {
	case '"': // RFC3339 string
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			return nil
		}
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			// Tolerate fractional seconds.
			if parsed, err = time.Parse(time.RFC3339Nano, s); err != nil {
				return fmt.Errorf("ingest: parsing run timestamp %q: %w", s, err)
			}
		}
		t.Time = parsed
	case '{': // protobuf Timestamp {"seconds":N,"nanos":M}
		var ts struct {
			Seconds int64 `json:"seconds"`
			Nanos   int64 `json:"nanos"`
		}
		if err := json.Unmarshal(b, &ts); err != nil {
			return err
		}
		if ts.Seconds != 0 || ts.Nanos != 0 {
			t.Time = time.Unix(ts.Seconds, ts.Nanos).UTC()
		}
	default: // bare numeric epoch seconds
		var n int64
		if err := json.Unmarshal(b, &n); err != nil {
			return fmt.Errorf("ingest: unrecognised run timestamp %s", string(b))
		}
		if n != 0 {
			t.Time = time.Unix(n, 0).UTC()
		}
	}
	return nil
}

// maxBacktraceLines bounds a stored backtrace. Customer converge backtraces are
// typically ~40 lines; the cap guards against a pathological producer without
// discarding the useful head of the trace.
const maxBacktraceLines = 100

// ConvergeRun is the extract-and-discard subset of a converge we persist: the run
// summary, observed cookbooks, and (on failure) the error, bounded backtrace, and
// failing resource. The bulk node-attribute tree is discarded and never mapped here.
type ConvergeRun struct {
	RunID                string
	Organisation         string
	NodeName             string
	SourceFQDN           string
	ChefServerFQDN       string
	Status               string
	ChefVersion          string
	StartTime            time.Time
	EndTime              time.Time
	RunList              []string
	ExpandedRunList      json.RawMessage // kept verbatim; opaque to us
	Cookbooks            map[string]string
	TotalResourceCount   int
	UpdatedResourceCount int
	Error                *RunError
	FailedResource       *FailedResource
	Shape                string // "datafeed" | "run_converge"
}

// RunError is a normalised converge failure. Description is kept verbatim (its
// shape varies by producer); Backtrace is bounded to maxBacktraceLines.
type RunError struct {
	Class       string          `json:"class"`
	Message     string          `json:"message"`
	Description json.RawMessage `json:"description,omitempty"`
	Backtrace   []string        `json:"backtrace,omitempty"`
}

// FailedResource identifies the resource whose action raised the failure.
type FailedResource struct {
	CookbookName string `json:"cookbook_name"`
	RecipeName   string `json:"recipe_name"`
	Name         string `json:"name"`
	Type         string `json:"type"`
}

// Shape constants for ConvergeRun.Shape / provenance.
const (
	ShapeDataFeed = "datafeed"
	ShapeConverge = "run_converge"
)

// resource is the subset of a resources[] element we read — the failing one.
type resource struct {
	Type         string `json:"type"`
	Name         string `json:"name"`
	CookbookName string `json:"cookbook_name"`
	RecipeName   string `json:"recipe_name"`
	Status       string `json:"status"`
}

// runError mirrors the on-the-wire error object (both producers agree on shape).
type runError struct {
	Class       string          `json:"class"`
	Message     string          `json:"message"`
	Description json.RawMessage `json:"description"`
	Backtrace   []string        `json:"backtrace"`
}

// The run payload — a raw run_converge at top level, or the client_run section of
// a Data Feed record — is NOT decoded into a fixed struct. Chef node data is
// freeform and producers vary field shapes (cookbooks object-vs-list, timestamps
// string-vs-protobuf-object, counts, …); a struct decode fails the WHOLE record
// on the first mismatched field and drops it. Instead the body is read as
// map[string]json.RawMessage and each field we keep is extracted best-effort (see
// fromRunBody), so an unexpected shape yields a zero value rather than dropping a
// real converge. Only a missing end_time (the partition key) rejects a record.

// strField / intField best-effort-extract a scalar; a wrong shape yields the zero
// value (never an error), so one odd field cannot drop the record.
func strField(f map[string]json.RawMessage, key string) string {
	var s string
	_ = json.Unmarshal(f[key], &s)
	return s
}

func intField(f map[string]json.RawMessage, key string) int {
	var n int
	_ = json.Unmarshal(f[key], &n)
	return n
}

// rawField returns a present, non-null raw value, else nil.
func rawField(f map[string]json.RawMessage, key string) json.RawMessage {
	if v, ok := f[key]; ok && len(v) > 0 && string(v) != "null" {
		return v
	}
	return nil
}

// parseRunTime decodes a run timestamp (RFC3339 string or protobuf object) from
// raw, returning the zero time on any unrecognised shape.
func parseRunTime(raw json.RawMessage) time.Time {
	var t runTime
	_ = json.Unmarshal(raw, &t)
	return t.Time
}

// chefVersionFromNode digs chef_version out of the node attribute tree
// (run_converge has no top-level chef_version). Best-effort; "" on any shape.
func chefVersionFromNode(raw json.RawMessage) string {
	var n struct {
		Automatic struct {
			ChefPackages struct {
				Chef struct {
					Version string `json:"version"`
				} `json:"chef"`
			} `json:"chef_packages"`
		} `json:"automatic"`
	}
	_ = json.Unmarshal(raw, &n)
	return n.Automatic.ChefPackages.Chef.Version
}

// parseRunError decodes the failure detail, returning nil for an absent, empty
// ({}), or unrecognised error (so a success with error:{} is not a failure).
func parseRunError(raw json.RawMessage) *RunError {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var e runError
	if json.Unmarshal(raw, &e) != nil {
		return nil
	}
	if e.Class == "" && e.Message == "" && len(e.Backtrace) == 0 {
		return nil
	}
	bt := e.Backtrace
	if len(bt) > maxBacktraceLines {
		bt = bt[:maxBacktraceLines]
	}
	return &RunError{Class: e.Class, Message: e.Message, Description: e.Description, Backtrace: bt}
}

// firstFailedFrom decodes resources best-effort and returns the first failed one.
func firstFailedFrom(raw json.RawMessage) *FailedResource {
	var rs []resource
	_ = json.Unmarshal(raw, &rs)
	return firstFailed(rs)
}

// cookbookVersions normalises the run's cookbooks map to name -> version. Chef
// emits each entry as an object {"version": "x.y.z", ...} (measured against a real
// run_converge); a bare version string is also tolerated for robustness.
// cookbookVersions normalises the run's cookbooks to name -> version from either
// producer shape: run_converge / Server proxy send `cookbooks` as an OBJECT
// (name -> {"version":"x"} — a bare string is tolerated); the Automate Data Feed
// sends `cookbooks` as a LIST of names and the versions in `versioned_cookbooks`
// ([{name,version}]). The object shape wins when present; otherwise
// versioned_cookbooks is used. Both inputs are raw so an unexpected shape yields
// an empty map rather than failing the whole record decode.
func cookbookVersions(cookbooks, versioned json.RawMessage) map[string]string {
	out := map[string]string{}

	// run_converge / Server proxy: cookbooks is an object (name -> {version}).
	var asMap map[string]json.RawMessage
	if len(cookbooks) > 0 && json.Unmarshal(cookbooks, &asMap) == nil {
		for name, v := range asMap {
			var obj struct {
				Version string `json:"version"`
			}
			if err := json.Unmarshal(v, &obj); err == nil && obj.Version != "" {
				out[name] = obj.Version
				continue
			}
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				out[name] = s
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	// Data Feed: versioned_cookbooks is a list of {name, version}.
	var vc []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if len(versioned) > 0 && json.Unmarshal(versioned, &vc) == nil {
		for _, c := range vc {
			if c.Name != "" {
				out[c.Name] = c.Version
			}
		}
	}
	return out
}

// envelope peeks at the discriminating top-level keys without committing to a
// full shape. client_run present => Data Feed; message_type => a client message.
type envelope struct {
	ClientRun   json.RawMessage `json:"client_run"`
	MessageType string          `json:"message_type"`
}

// Normalise detects the producer shape of one decoded record and maps it to a
// ConvergeRun. It returns (nil, nil) for records the MVP accepts but ignores —
// run_start, action, and the attributes-only Data Feed record (the depsolve-abort
// gap). It returns an error only for a converge that cannot be mapped (e.g. a
// missing end_time, which cannot route to a time partition).
func Normalise(raw json.RawMessage) (*ConvergeRun, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("ingest: decoding record: %w", err)
	}

	switch {
	case len(env.ClientRun) > 0 && string(env.ClientRun) != "null":
		return fromRunBody(env.ClientRun, ShapeDataFeed)
	case env.MessageType == ShapeConverge:
		return fromRunBody(raw, ShapeConverge)
	default:
		// run_start / action / attributes-only — accepted, not persisted.
		return nil, nil
	}
}

func fromRunBody(raw json.RawMessage, shape string) (*ConvergeRun, error) {
	// Read the body as a field map — this only fails if it is not a JSON object.
	// Every field below is extracted best-effort so a producer's unexpected shape
	// on any single field cannot drop an otherwise-valid converge.
	var f map[string]json.RawMessage
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("ingest: decoding %s body: %w", shape, err)
	}

	chefVersion := strField(f, "chef_version")
	if chefVersion == "" {
		// run_converge has no top-level chef_version; fall back to the node tree.
		chefVersion = chefVersionFromNode(f["node"])
	}

	var runList []string
	_ = json.Unmarshal(f["run_list"], &runList) // wrong shape -> nil

	run := &ConvergeRun{
		Status:               strField(f, "status"),
		ChefVersion:          chefVersion,
		StartTime:            parseRunTime(f["start_time"]),
		EndTime:              parseRunTime(f["end_time"]),
		RunList:              runList,
		ExpandedRunList:      rawField(f, "expanded_run_list"),
		Cookbooks:            cookbookVersions(f["cookbooks"], f["versioned_cookbooks"]),
		TotalResourceCount:   intField(f, "total_resource_count"),
		UpdatedResourceCount: intField(f, "updated_resource_count"),
		Shape:                shape,
	}

	// Identity fields differ by envelope.
	switch shape {
	case ShapeDataFeed:
		run.RunID = strField(f, "id")
		run.NodeName = strField(f, "node_name")
		run.Organisation = strField(f, "organization")
		run.SourceFQDN = strField(f, "source_fqdn")
	default: // run_converge (direct or proxy relay)
		run.RunID = strField(f, "run_id")
		run.NodeName = strField(f, "node_name")
		run.Organisation = strField(f, "organization_name")
		run.ChefServerFQDN = strField(f, "chef_server_fqdn")
	}

	if run.Cookbooks == nil {
		run.Cookbooks = map[string]string{}
	}
	if run.EndTime.IsZero() {
		return nil, fmt.Errorf("ingest: %s run %q has no end_time (cannot partition)", shape, run.RunID)
	}

	// Failure detail — best-effort; a success (error absent or {}) yields nil.
	if e := parseRunError(f["error"]); e != nil {
		run.Error = e
		run.FailedResource = firstFailedFrom(f["resources"])
	}

	return run, nil
}

// firstFailed returns the first resource whose action failed, or nil.
func firstFailed(rs []resource) *FailedResource {
	for _, r := range rs {
		if r.Status == "failed" {
			return &FailedResource{
				CookbookName: r.CookbookName,
				RecipeName:   r.RecipeName,
				Name:         r.Name,
				Type:         r.Type,
			}
		}
	}
	return nil
}
