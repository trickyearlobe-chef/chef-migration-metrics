// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Everything the web interface sends is a field this service reads.
//
// This is the check that could not be made before. "The interface tests are
// green" says nothing about it: 31 of the 45 page test files mock the API
// module outright, and nothing there drives a real body into a real handler.
// So the interface could send a field no handler had — and did, in three
// places, for as long as anybody can tell:
//
//   - a reclassification sent a target version nothing read;
//   - the analysis tools screen had a text box for a path this service had
//     never had a field for, so an operator set it, was told it saved, and
//     scanning stayed off;
//   - the server settings screen handed back the certificate chain and the
//     ACME status, which are read-only things the GET attaches.
//
// All three were dropped in silence. Now they would be refused, which is why
// this has to hold: a field the interface sends and no handler reads is a
// screen that has stopped working.
//
// The fixture is measured, not written. Regenerate it with the TypeScript
// compiler after changing anything a screen sends:
//
//	cd frontend && node ../tools/frontend-fields/record.cjs \
//	  > ../internal/webapi/testdata/frontend_request_fields.json

const frontendFieldsPath = "testdata/frontend_request_fields.json"

// namedSegment matches a path parameter in either vocabulary — the interface
// records them as {p}, the description names them.
var namedSegment = regexp.MustCompile(`\{[^}]*\}`)

func TestFrontend_EverythingTheInterfaceSendsIsAFieldWeRead(t *testing.T) {
	raw, err := os.ReadFile(frontendFieldsPath)
	if err != nil {
		t.Fatalf("nothing records what the interface sends, so a screen can start sending a "+
			"field no handler reads and nothing notices: %v", err)
	}
	var sends map[string][]string
	if err := json.Unmarshal(raw, &sends); err != nil {
		t.Fatalf("reading %s: %v", frontendFieldsPath, err)
	}
	if len(sends) == 0 {
		t.Fatal("the record of what the interface sends is empty, so this test checked nothing")
	}

	described := describedRequestFields(t)
	// The baseline. Without it, a description that had stopped carrying request
	// bodies at all would make every comparison below vacuously pass.
	if len(described) == 0 {
		t.Fatal("the description carries no request bodies at all, so nothing was compared")
	}

	matched := 0
	for call, fields := range sends {
		reads, ok := described[normaliseCall(call)]
		if !ok {
			// An address the description does not give a JSON body: an upload,
			// or one deliberately left undescribed. Nothing to compare.
			continue
		}
		matched++
		var unread []string
		for _, field := range fields {
			if !reads[field] {
				unread = append(unread, field)
			}
		}
		if len(unread) > 0 {
			sort.Strings(unread)
			t.Errorf("the interface sends %v to %s, which reads none of them — the service "+
				"refuses a body carrying anything it does not read, so that screen fails to "+
				"save. Either the handler should take these, or the screen should stop "+
				"sending them", unread, call)
		}
	}

	if matched == 0 {
		t.Error("none of the addresses the interface writes to matched a described body, so " +
			"this test compared nothing; the two vocabularies for a path parameter have " +
			"probably drifted apart")
	} else {
		t.Logf("%d of %d addresses the interface writes to were compared", matched, len(sends))
	}
}

// normaliseCall puts a "METHOD /path" into one vocabulary, so {p} and {name}
// are the same segment.
func normaliseCall(call string) string {
	method, path, ok := strings.Cut(call, " ")
	if !ok {
		return call
	}
	return method + " " + namedSegment.ReplaceAllString(path, "{}")
}

// describedRequestFields is every JSON body the description carries, by call,
// resolved through its references.
func describedRequestFields(t *testing.T) map[string]map[string]bool {
	t.Helper()
	doc := newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).openAPIDocument()

	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)

	out := map[string]map[string]bool{}
	paths, _ := doc["paths"].(map[string]any)
	for path, item := range paths {
		operations, _ := item.(map[string]any)
		for method, op := range operations {
			fields, _ := op.(map[string]any)
			body, _ := fields["requestBody"].(map[string]any)
			content, _ := body["content"].(map[string]any)
			json, _ := content["application/json"].(map[string]any)
			if json == nil {
				continue
			}
			schema, _ := json["schema"].(map[string]any)
			props := resolveProperties(schemas, schema, 0)
			if len(props) == 0 {
				// A body deliberately left undescribed, or one that is not an
				// object. Neither says anything about field names.
				continue
			}
			out[normaliseCall(strings.ToUpper(method)+" "+path)] = props
		}
	}
	return out
}

// resolveProperties follows a reference or an allOf to the field names beneath.
func resolveProperties(schemas map[string]any, schema map[string]any, depth int) map[string]bool {
	if schema == nil || depth > 6 {
		return nil
	}
	if ref, ok := schema["$ref"].(string); ok {
		target, _ := schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any)
		return resolveProperties(schemas, target, depth+1)
	}
	if parts, ok := schema["allOf"].([]any); ok {
		out := map[string]bool{}
		for _, part := range parts {
			partSchema, _ := part.(map[string]any)
			for name := range resolveProperties(schemas, partSchema, depth+1) {
				out[name] = true
			}
		}
		return out
	}
	properties, _ := schema["properties"].(map[string]any)
	if len(properties) == 0 {
		return nil
	}
	out := make(map[string]bool, len(properties))
	for name := range properties {
		out[name] = true
	}
	return out
}
