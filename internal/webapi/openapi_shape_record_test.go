// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A recording of what every call answered with, so a change in shape fails a
// build rather than somebody's numbers three weeks later.
//
// Generating the description stops it going stale; it does nothing about it
// changing. A renamed field describes itself just as confidently under its new
// name, and an integration built on the old one breaks silently — see
// journeys/api-integration.md, "answers that keep their shape".
//
// What is recorded is the shape the description carries, resolved through its
// references: field names and types, with no instance anywhere near it. The
// live probe's recording is deliberately NOT what is kept here. It is read off
// a running service, and an object's keys there can be data — a map keyed by
// organisation or by version puts customer names in a file that then goes to
// git. This is derived from the Go types, so it cannot carry any.
//
// Changing it is meant to be possible and meant to be deliberate: re-record
// with `go test ./internal/webapi -run TestResponses_TheRecordedShapesStillHold
// -update` and the diff is the list of callers being broken.

var updateShapes = flag.Bool("update", false,
	"re-record the response shapes in testdata")

const shapeRecordPath = "testdata/response_shapes.json"

// The recorded shapes still hold.
//
// Failing here does not mean anything is wrong — a deliberate change to what a
// call answers with is ordinary work. It means the change reaches somebody
// outside, and the re-recording is where that gets noticed.
func TestResponses_TheRecordedShapesStillHold(t *testing.T) {
	current := recordedShapes(t)

	if *updateShapes {
		writeShapeRecord(t, current)
		t.Log("re-recorded " + shapeRecordPath)
		return
	}

	raw, err := os.ReadFile(shapeRecordPath)
	if err != nil {
		t.Fatalf("nothing records what these calls answer with, so nothing can fail when one "+
			"changes under a caller: %v. Record it with -update", err)
	}
	var recorded map[string]any
	if err := json.Unmarshal(raw, &recorded); err != nil {
		t.Fatalf("the recording cannot be read: %v", err)
	}

	for _, op := range sortedKeys(recorded) {
		was, _ := json.Marshal(recorded[op])
		now, ok := current[op]
		if !ok {
			t.Errorf("%s used to answer with a described shape and no longer does — a caller "+
				"generated against the last release decodes into a model of a shape nothing "+
				"sends any more", op)
			continue
		}
		if isNow, _ := json.Marshal(now); string(isNow) != string(was) {
			t.Errorf("%s answers with a different shape than was recorded.\n  was: %s\n  now: %s\n"+
				"Anything built against the old one breaks, and it breaks quietly. If that is "+
				"intended, re-record with -update and let the diff say who is affected.",
				op, was, isNow)
		}
	}

	// New operations are not a failure — describing one that was undescribed
	// is the work in hand — but they belong in the recording, or the next
	// change to them goes unnoticed.
	var added []string
	for _, op := range sortedKeys(current) {
		if _, known := recorded[op]; !known {
			added = append(added, op)
		}
	}
	if len(added) > 0 {
		t.Errorf("%d operations describe an answer that is not recorded yet (%s). Re-record "+
			"with -update, or the first change to them passes unnoticed",
			len(added), strings.Join(firstFew(added), ", "))
	}
}

// recordedShapes is every described answer, resolved through its references.
func recordedShapes(t *testing.T) map[string]any {
	t.Helper()
	doc := testRouterForDescription(t).openAPIDocument()
	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)

	out := map[string]any{}
	paths, _ := doc["paths"].(map[string]any)
	for path, item := range paths {
		operations, _ := item.(map[string]any)
		for method := range operations {
			schema := responseSchemaIn(doc, strings.ToUpper(method), path)
			if schema == nil {
				continue
			}
			out[strings.ToUpper(method)+" "+path] = flattenSchema(schema, schemas, nil)
		}
	}
	return out
}

// flattenSchema resolves references into the shape they name, so a change
// inside a shared type shows up at every call that answers with it. That is
// the point: renaming a field on one type breaks every caller of every address
// that sends it, and a recording that kept the reference would show one line
// changing and hide all of them.
//
// seen carries the types already being expanded on this path, so a type that
// contains itself records as a reference to itself rather than recursing.
func flattenSchema(schema any, schemas map[string]any, seen []string) any {
	object, _ := schema.(map[string]any)
	if object == nil {
		return schema
	}
	if ref, isRef := object["$ref"].(string); isRef {
		name := strings.TrimPrefix(ref, schemaRefPrefix)
		for _, already := range seen {
			if already == name {
				return "(recursive) " + name
			}
		}
		target, _ := schemas[name].(map[string]any)
		if target == nil {
			return "(undefined) " + name
		}
		return flattenSchema(target, schemas, append(seen, name))
	}

	out := map[string]any{}
	for key, value := range object {
		switch key {
		case "properties":
			props, _ := value.(map[string]any)
			flat := map[string]any{}
			for name, field := range props {
				flat[name] = flattenSchema(field, schemas, seen)
			}
			out[key] = flat
		case "items", "additionalProperties":
			out[key] = flattenSchema(value, schemas, seen)
		case "allOf":
			parts, _ := value.([]any)
			flat := make([]any, 0, len(parts))
			for _, part := range parts {
				flat = append(flat, flattenSchema(part, schemas, seen))
			}
			out[key] = flat
		default:
			out[key] = value
		}
	}
	return out
}

func writeShapeRecord(t *testing.T, shapes map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(shapeRecordPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(shapes, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shapeRecordPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func firstFew(all []string) []string {
	if len(all) > 5 {
		return append(all[:5:5], "…")
	}
	return all
}
