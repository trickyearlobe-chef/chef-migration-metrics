// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// What a call answers with is the half of the description a caller cannot work
// around by trying it once. A missing address is obvious and a missing request
// body is loud — the call is refused — but an undescribed answer is silent: a
// generated client has no model to decode into, so whoever wrote it reads the
// fields out of the browser's network tab and hard-codes what they saw.
//
// The same rule as everything else here: which type an address writes is
// declared at the registration site with answers(), and the shape is reflected
// off that type. Nothing writes a field list by hand.
//
// Which type an address really writes is measured, not reasoned about —
// tools/api-probe/probe.py asks a running instance and reports every address
// whose answer disagrees with the description. What the tests below add is the
// part that can run without an instance: the service may not send a field the
// description has never heard of, a page must be described as the envelope it
// really is, and the number of addresses answering undescribed may only fall.

// A described answer reaches the document a caller actually fetches, resolved
// through the reference rather than sitting in the route table.
func TestResponses_ADescribedAnswerReachesTheServedDocument(t *testing.T) {
	doc := testRouterForDescription(t).openAPIDocument()

	schema := responseSchemaIn(doc, "GET", "/api/v1/version")
	if schema == nil {
		t.Fatal("the version call is described as answering nothing, so a client generated " +
			"from this has no model to decode the answer into")
	}
	ref, _ := schema["$ref"].(string)
	if ref == "" {
		t.Fatalf("the answer carries no schema reference: %v", schema)
	}

	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	name := strings.TrimPrefix(ref, schemaRefPrefix)
	if schemas[name] == nil {
		t.Fatalf("the answer refers to %q, which is not defined in the document", name)
	}
}

// A page is described as the envelope it really is: the list under data, and
// the pagination metadata beside it read off PaginationResponse rather than
// written out here.
func TestResponses_APageIsDescribedAsTheEnvelope(t *testing.T) {
	doc := testRouterForDescription(t).openAPIDocument()

	schema := responseSchemaIn(doc, "GET", "/api/v1/nodes")
	if schema == nil {
		t.Fatal("listing machines is described as answering nothing")
	}
	props, _ := schema["properties"].(map[string]any)
	data, _ := props["data"].(map[string]any)
	if data == nil || data["type"] != "array" {
		t.Errorf("a page does not describe its rows as a list: %v", props["data"])
	}
	if props["pagination"] == nil {
		t.Error("a page does not describe the pagination metadata that comes with it, so a " +
			"caller cannot see how many pages there are without fetching one and looking")
	}
}

// Nothing may be described as a page that is not described as taking page and
// per_page. The two are declared separately and a caller reads them together:
// an envelope with no way to ask for the next page reads as a service that
// truncates.
func TestResponses_NothingIsDescribedAsAPageItCannotBeAskedFor(t *testing.T) {
	for _, rt := range testRouterForDescription(t).Routes() {
		for _, addr := range describableAddresses(rt) {
			for method, answer := range addr.answers {
				if !answer.Page {
					continue
				}
				if !addr.paginated[method] && !addr.capped[method] {
					t.Errorf("%s %s is described as answering a page, but not as accepting "+
						"page or per_page — a caller is shown a total and no way to walk it",
						method, addr.path)
				}
			}
		}
	}
}

// Every declared answer has a shape worth describing. A type with nothing on
// the wire describes an empty object, which tells a caller the answer is empty
// — a lie that reads exactly like the truth.
func TestResponses_EveryDeclaredAnswerHasAShape(t *testing.T) {
	declared := 0
	for _, rt := range testRouterForDescription(t).Routes() {
		for _, addr := range describableAddresses(rt) {
			for method, answer := range addr.answers {
				declared++
				if answer.Value == nil {
					t.Errorf("%s %s declares no type as its answer", method, addr.path)
					continue
				}
				reg := newSchemaRegistry()
				if !describesSomething(reg, reg.schemaFor(reflect.TypeOf(answer.Value))) {
					t.Errorf("%s %s is described as answering %s, which puts nothing on the "+
						"wire — the description says the answer is empty",
						method, addr.path, goTypeName(reflect.TypeOf(answer.Value)))
				}
			}
		}
	}
	if declared == 0 {
		t.Fatal("no address declares what it answers with, so nothing was checked")
	}
}

// The service may not send a field the description has never heard of.
//
// Asked of the service rather than of the declaration: every described address
// is called and the field names it really sends are compared with the ones the
// description carries. An extra field is a caller decoding into a generated
// model and silently dropping something, or a declaration pointing at the wrong
// type entirely.
//
// Only one direction can be checked here. A field the description names and the
// answer does not carry proves nothing — the store behind this test is empty,
// and an empty field is omitted rather than sent — which is why the other
// direction is measured against a running instance instead.
func TestResponses_NoFieldIsSentThatTheDescriptionDoesNotMention(t *testing.T) {
	router := testRouterForDescription(t)
	doc := router.openAPIDocument()
	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)

	asked := 0
	for _, rt := range router.Routes() {
		for _, addr := range describableAddresses(rt) {
			answer, declared := addr.answers[http.MethodGet]
			if !declared {
				continue
			}
			w := ask(router, fillPathParameters(addr.path))
			if w == nil || w.Code != http.StatusOK ||
				!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
				continue // says nothing about the shape; the live probe covers these
			}
			var body any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				continue
			}
			asked++
			described := responseSchemaIn(doc, http.MethodGet, addr.path)
			for _, extra := range fieldsNotDescribed(body, described, schemas, answer.Page) {
				t.Errorf("GET %s sends %q, which the description does not mention — a "+
					"generated client drops it on the floor", addr.path, extra)
			}
		}
	}
	if asked == 0 {
		t.Fatal("no described address answered, so nothing was checked")
	}
}

// The addresses still answering undescribed may only get fewer.
//
// A list of them would be a second thing to keep true; the count is recomputed
// from the description every run. It fails in both directions on purpose —
// upwards is an address added without saying what it answers, downwards is work
// finished and the number not struck down, which is how a ratchet stops
// ratcheting.
func TestResponses_TheUndescribedAnswersOnlyGetFewer(t *testing.T) {
	const undescribed = 156

	count := 0
	doc := testRouterForDescription(t).openAPIDocument()
	paths, _ := doc["paths"].(map[string]any)
	for path, item := range paths {
		operations, _ := item.(map[string]any)
		for method := range operations {
			if responseSchemaIn(doc, strings.ToUpper(method), path) == nil {
				count++
			}
		}
	}

	switch {
	case count > undescribed:
		t.Errorf("%d operations answer with a shape nobody has described, up from %d — an "+
			"address was added without declaring what it answers, and a caller has to read "+
			"the browser's network traffic to find out", count, undescribed)
	case count < undescribed:
		t.Errorf("%d operations answer undescribed, down from %d. Strike the number down to "+
			"%d so the next one cannot creep back up", count, undescribed, count)
	}
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

// ask reads one address, or gives up on it.
//
// The store behind this test answers only where a test has taught it to, and
// panics otherwise. That is a gap in the fixture rather than anything about the
// address, so it is skipped: what this test can say about an address it could
// not reach is nothing either way.
func ask(router *Router, path string) (w *httptest.ResponseRecorder) {
	defer func() {
		if recover() != nil {
			w = nil
		}
	}()
	w = httptest.NewRecorder()
	router.ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodGet, path, nil)))
	return w
}

func testRouterForDescription(t *testing.T) *Router {
	t.Helper()
	return newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"))
}

// responseSchemaIn is the schema one operation says it answers with, or nil
// where it says nothing.
func responseSchemaIn(doc map[string]any, method, path string) map[string]any {
	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths[path].(map[string]any)
	op, _ := item[strings.ToLower(method)].(map[string]any)
	responses, _ := op["responses"].(map[string]any)
	ok, _ := responses["200"].(map[string]any)
	content, _ := ok["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	return schema
}

// describesSomething reports whether a schema tells a caller anything at all.
func describesSomething(reg *schemaRegistry, schema map[string]any) bool {
	if ref, ok := schema["$ref"].(string); ok {
		target, _ := reg.defs[strings.TrimPrefix(ref, schemaRefPrefix)].(map[string]any)
		if target == nil {
			return false
		}
		props, _ := target["properties"].(map[string]any)
		return len(props) > 0
	}
	if props, ok := schema["properties"].(map[string]any); ok {
		return len(props) > 0
	}
	return len(schema) > 0
}

// fillPathParameters puts something harmless in every named segment. The store
// behind this test holds nothing, so what goes in does not matter — an address
// that answers 404 is skipped by the caller of this.
func fillPathParameters(path string) string {
	out := []string{}
	for _, segment := range strings.Split(path, "/") {
		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			segment = "example"
		}
		out = append(out, segment)
	}
	return strings.Join(out, "/")
}

// fieldsNotDescribed lists the field names an answer carries that its schema
// does not, one level down into a page's rows.
func fieldsNotDescribed(body, schema any, schemas map[string]any, page bool) []string {
	resolved := resolveSchema(schema, schemas)
	if resolved == nil {
		return nil
	}
	object, ok := body.(map[string]any)
	if !ok {
		return nil
	}
	props, _ := resolved["properties"].(map[string]any)
	if props == nil {
		return nil // additionalProperties: anything may turn up
	}

	var extra []string
	for name, value := range object {
		field, described := props[name]
		if !described {
			extra = append(extra, name)
			continue
		}
		if page && name == "data" {
			extra = append(extra, rowFieldsNotDescribed(value, field, schemas)...)
		}
	}
	return extra
}

// rowFieldsNotDescribed checks the first row of a page against the item schema.
func rowFieldsNotDescribed(value, field any, schemas map[string]any) []string {
	rows, _ := value.([]any)
	if len(rows) == 0 {
		return nil
	}
	array := resolveSchema(field, schemas)
	if array == nil {
		return nil
	}
	var extra []string
	for _, name := range fieldsNotDescribed(rows[0], array["items"], schemas, false) {
		extra = append(extra, "data[]."+name)
	}
	return extra
}

// resolveSchema follows a reference into components/schemas.
func resolveSchema(schema any, schemas map[string]any) map[string]any {
	object, _ := schema.(map[string]any)
	for i := 0; object != nil && i < 8; i++ {
		ref, isRef := object["$ref"].(string)
		if !isRef {
			return object
		}
		object, _ = schemas[strings.TrimPrefix(ref, schemaRefPrefix)].(map[string]any)
	}
	return object
}
