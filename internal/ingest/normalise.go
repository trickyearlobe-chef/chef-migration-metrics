// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package ingest normalises Chef run telemetry from three producer shapes
// (node-direct run_converge, Chef Server proxy relay, Chef Automate Data Feed)
// into a single ConvergeRun. It is the source of truth for that mapping, pinned
// by a contract test against the golden fixtures in testdata/event-ingest/. See
// specifications/event-ingest.md.
package ingest

import (
	"encoding/json"
	"fmt"
	"time"
)

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
	Class       string
	Message     string
	Description json.RawMessage
	Backtrace   []string
}

// FailedResource identifies the resource whose action raised the failure.
type FailedResource struct {
	CookbookName string
	RecipeName   string
	Name         string
	Type         string
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

// runBody is the common run payload — a raw run_converge at top level, or the
// client_run section of a Data Feed record. The two carry the same run under
// different envelopes and a couple of differently-named identity fields.
type runBody struct {
	// run_converge identity
	RunID            string `json:"run_id"`
	NodeName         string `json:"node_name"`
	OrganizationName string `json:"organization_name"`
	ChefServerFQDN   string `json:"chef_server_fqdn"`
	// Data Feed client_run identity
	ID           string `json:"id"`
	Organization string `json:"organization"`
	SourceFQDN   string `json:"source_fqdn"`
	// shared run fields
	Status               string            `json:"status"`
	ChefVersion          string            `json:"chef_version"`
	StartTime            time.Time         `json:"start_time"`
	EndTime              time.Time         `json:"end_time"`
	RunList              []string          `json:"run_list"`
	ExpandedRunList      json.RawMessage   `json:"expanded_run_list"`
	Cookbooks            map[string]string `json:"cookbooks"`
	TotalResourceCount   int               `json:"total_resource_count"`
	UpdatedResourceCount int               `json:"updated_resource_count"`
	Resources            []resource        `json:"resources"`
	Error                *runError         `json:"error"`
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
	var b runBody
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, fmt.Errorf("ingest: decoding %s body: %w", shape, err)
	}

	run := &ConvergeRun{
		Status:               b.Status,
		ChefVersion:          b.ChefVersion,
		StartTime:            b.StartTime,
		EndTime:              b.EndTime,
		RunList:              b.RunList,
		ExpandedRunList:      b.ExpandedRunList,
		Cookbooks:            b.Cookbooks,
		TotalResourceCount:   b.TotalResourceCount,
		UpdatedResourceCount: b.UpdatedResourceCount,
		Shape:                shape,
	}

	// Identity fields differ by envelope.
	switch shape {
	case ShapeDataFeed:
		run.RunID = b.ID
		run.NodeName = b.NodeName
		run.Organisation = b.Organization
		run.SourceFQDN = b.SourceFQDN
	default: // run_converge (direct or proxy relay)
		run.RunID = b.RunID
		run.NodeName = b.NodeName
		run.Organisation = b.OrganizationName
		run.ChefServerFQDN = b.ChefServerFQDN
	}

	if run.Cookbooks == nil {
		run.Cookbooks = map[string]string{}
	}
	if run.EndTime.IsZero() {
		return nil, fmt.Errorf("ingest: %s run %q has no end_time (cannot partition)", shape, run.RunID)
	}

	if b.Error != nil {
		bt := b.Error.Backtrace
		if len(bt) > maxBacktraceLines {
			bt = bt[:maxBacktraceLines]
		}
		run.Error = &RunError{
			Class:       b.Error.Class,
			Message:     b.Error.Message,
			Description: b.Error.Description,
			Backtrace:   bt,
		}
		run.FailedResource = firstFailed(b.Resources)
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
