// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"reflect"
	"sort"
	"strings"
)

// The API description, generated from the route table rather than written
// beside it.
//
// A hand-written description starts rotting the day it is committed — the
// failure that killed the document set the journeys replaced, and the reason
// three endpoints were written up here that were never built. Generating it
// means a renamed path changes the description in the same commit, and the
// drift tests fail if the two ever disagree.
//
// What is generated is the *set*: every address, its methods, and the access it
// requires. What each one is for is hand-written, in apidoc.go, because nothing
// can derive that from a route table. The set is enforced; the prose is
// reviewed.

const openAPIPath = "/api/v1/openapi.json"

// openAPIDocument builds the OpenAPI 3.1 description of everything served
// under /api/.
func (r *Router) openAPIDocument() map[string]any {
	paths := map[string]any{}
	schemas := newSchemaRegistry()

	for _, rt := range r.Routes() {
		for _, addr := range describableAddresses(rt) {
			item, _ := paths[addr.path].(map[string]any)
			if item == nil {
				item = map[string]any{}
				paths[addr.path] = item
			}
			for _, method := range addr.methods {
				op := r.operation(addr.path, method, rt.Role)
				if addr.paginated[method] {
					op["parameters"] = append(
						op["parameters"].([]any), paginationParameters()...)
				}
				if addr.capped[method] {
					op["parameters"] = append(
						op["parameters"].([]any), perPageParameter())
				}
				if body := describedInput(schemas, method, addr, method+" "+addr.path); body != nil {
					op["requestBody"] = body
				}
				if answer, ok := addr.answers[method]; ok {
					describeAnswer(schemas, op, answer)
				}
				item[strings.ToLower(method)] = op
			}
		}
	}

	doc := map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title": "Chef Migration Metrics",
			"description": "What is stopping this estate moving to the target Chef version, " +
				"and what to do first.",
			"version": r.version,
		},
		"paths": paths,
	}
	if defs := schemas.components(); len(defs) > 0 {
		doc["components"] = map[string]any{"schemas": defs}
	}
	return doc
}

// describedInput is what a caller sends, or nil where a call reads nothing.
//
// Three kinds of input exist here and they have to stay distinguishable. A JSON
// document is reflected off the type the handler really decodes into. An upload
// is said to be an upload. A body this service deliberately does not describe
// says so, and why. Only the fourth case — reading nothing at all — is absent
// from the description, because there a caller sending nothing is correct.
func describedInput(schemas *schemaRegistry, method string, addr address,
	key string) map[string]any {
	if bodies, ok := addr.bodies[method]; ok && len(bodies) > 0 {
		return jsonRequestBody(schemas, bodies)
	}
	if fields, ok := addr.forms[method]; ok && len(fields) > 0 {
		return formRequestBody(fields)
	}
	if uploadWrites[key] {
		return map[string]any{
			"required":    true,
			"description": "Sent as a form, not as a JSON document. The individual fields are not described yet.",
			"content": map[string]any{
				"multipart/form-data": map[string]any{
					"schema": map[string]any{"type": "object"},
				},
			},
		}
	}
	if why, ok := undescribedBodies[key]; ok {
		return map[string]any{
			"required":    true,
			"description": why,
			"content": map[string]any{
				"application/json": map[string]any{"schema": map[string]any{}},
			},
		}
	}
	return nil
}

// describeAnswer says what comes back, reflected off the type the handler
// really encodes.
//
// It replaces the content of the 200 rather than adding a second one: an
// operation says one thing about what it answers with, and the description of
// that answer stays where an undescribed one already was, so a reader sees the
// same shape of document either way.
func describeAnswer(schemas *schemaRegistry, op map[string]any, answer Answer) {
	schema := schemas.schemaFor(reflect.TypeOf(answer.Value))
	if answer.Page {
		schema = schemas.pageOf(reflect.TypeOf(answer.Value))
	}
	responses, _ := op["responses"].(map[string]any)
	ok, _ := responses["200"].(map[string]any)
	if ok == nil {
		return
	}
	ok["content"] = map[string]any{
		"application/json": map[string]any{"schema": schema},
	}
}

// formRequestBody describes a form submission, field by field.
//
// A file is a string of format binary, which is how OpenAPI says "upload"; a
// generated client turns that into a file part rather than a text one. Which
// fields are required is left unsaid, as it is for a JSON body: the handlers
// enforce it by hand, and several of these addresses accept either a file or a
// database connection, so no single field is required on its own.
func formRequestBody(fields []formField) map[string]any {
	props := map[string]any{}
	for _, field := range fields {
		schema := map[string]any{"type": "string"}
		if field.File {
			schema["format"] = "binary"
		}
		props[field.Name] = schema
	}
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"multipart/form-data": map[string]any{
				"schema": map[string]any{"type": "object", "properties": props},
			},
		},
	}
}

// jsonRequestBody describes a JSON document, reflected off the type the handler
// really decodes into.
//
// Marked required because a body was declared at all: every address that
// declares one refuses the call outright when the JSON will not parse, so an
// empty request is never a valid one. Which *fields* are required is a
// different question, and one nothing here can answer honestly — see
// openapi_schema.go.
func jsonRequestBody(schemas *schemaRegistry, bodies []any) map[string]any {
	schema := schemas.schemaFor(reflect.TypeOf(bodies[0]))
	if len(bodies) > 1 {
		parts := make([]any, 0, len(bodies))
		for _, body := range bodies {
			parts = append(parts, schemas.schemaFor(reflect.TypeOf(body)))
		}
		schema = map[string]any{"allOf": parts}
	}
	return map[string]any{
		"required": true,
		"content": map[string]any{
			"application/json": map[string]any{"schema": schema},
		},
	}
}

// address is one describable endpoint: a concrete path, the methods it answers,
// and the type each of those decodes its request body into.
type address struct {
	path      string
	methods   []string
	bodies    map[string][]any
	paginated map[string]bool
	capped    map[string]bool
	answers   map[string]Answer
	forms     map[string][]formField
}

// describableAddresses turns a recorded route into the addresses a caller can
// actually ask for.
//
// A subtree pattern is not an address — nothing is served at
// "/api/v1/git-repos/" itself — so it contributes its declared sub-paths and
// not itself. A subtree that declares none contributes nothing, which is
// deliberate: describing a bare prefix would tell a caller an address exists
// that answers nothing.
func describableAddresses(rt Route) []address {
	if !strings.HasPrefix(rt.Pattern, "/api/") {
		return nil
	}
	if !rt.IsSubtree() {
		return []address{{path: rt.Pattern, methods: rt.Methods, bodies: rt.Bodies,
			paginated: rt.Paginated, answers: rt.Answers, forms: rt.Forms}}
	}
	out := make([]address, 0, len(rt.SubPaths))
	for _, sp := range rt.SubPaths {
		out = append(out, address{
			path: rt.Pattern + sp.Suffix, methods: sp.Methods, bodies: sp.Bodies,
			paginated: sp.Paginated, capped: sp.Capped, answers: sp.Answers})
	}
	return out
}

// operation describes one method on one path.
func (r *Router) operation(path, method string, role RouteRole) map[string]any {
	op := map[string]any{
		"operationId": operationID(path, method),
		"parameters":  pathParameters(path),
		"responses": map[string]any{
			"200": map[string]any{"description": "The answer."},
		},
	}

	// The access level is part of what a caller needs to know before trying,
	// and it is the one thing here that is enforced rather than described.
	effective := effectiveRole(method, path, role)
	op["x-required-role"] = string(effective)
	if effective != RolePublic {
		op["security"] = []any{map[string]any{"bearerAuth": []any{}}}
	}

	if doc, ok := apiDocs[method+" "+path]; ok {
		op["summary"] = doc
	}
	return op
}

// paginationParameters describes page and per_page.
//
// Every number here is the constant ParsePagination actually applies, so the
// description cannot tell a caller a limit the service does not keep. Asking
// for more than the clamp is not an error — it quietly gives less — which is
// exactly the kind of thing a caller has to be told rather than discover.
func paginationParameters() []any {
	return []any{
		map[string]any{
			"name": "page", "in": "query", "required": false,
			"description": "Which page to read. Counts from 1.",
			"schema": map[string]any{
				"type": "integer", "minimum": 1, "default": defaultPage,
			},
		},
		perPageParameter(),
	}
}

// perPageParameter is how many to return. On its own — with no page beside it —
// it says the answer is capped but cannot be walked, which is the truth for the
// two addresses declared with cappedNotPaged.
func perPageParameter() map[string]any {
	return map[string]any{
		"name": "per_page", "in": "query", "required": false,
		"description": "How many to return. Asking for more than the maximum is not " +
			"refused — it returns the maximum.",
		"schema": map[string]any{
			"type": "integer", "minimum": 1,
			"default": defaultPerPage, "maximum": maxPerPage,
		},
	}
}

// pathParameters declares the named segments in a path, which is what tells a
// client generator these are variables rather than literals.
func pathParameters(path string) []any {
	var params []any
	for _, segment := range strings.Split(path, "/") {
		if !strings.HasPrefix(segment, "{") || !strings.HasSuffix(segment, "}") {
			continue
		}
		params = append(params, map[string]any{
			"name":     strings.Trim(segment, "{}"),
			"in":       "path",
			"required": true,
			"schema":   map[string]any{"type": "string"},
		})
	}
	return params
}

// operationID is the stable name a generated client gives this call.
func operationID(path, method string) string {
	id := strings.ToLower(method)
	for _, segment := range strings.Split(strings.TrimPrefix(path, "/api/v1/"), "/") {
		if segment == "" {
			continue
		}
		clean := strings.Trim(segment, "{}")
		for _, sep := range []string{"-", "."} {
			parts := strings.Split(clean, sep)
			for i := range parts {
				parts[i] = title(parts[i])
			}
			clean = strings.Join(parts, "")
		}
		id += clean
	}
	return id
}

func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// handleOpenAPI serves the description.
func (r *Router) handleOpenAPI(w http.ResponseWriter, req *http.Request) {
	if !requireGET(w, req) {
		return
	}
	WriteJSON(w, http.StatusOK, r.openAPIDocument())
}

// DescribedAddresses lists every path the description claims, sorted. Used by
// the drift tests, which compare it against the route table in both directions.
func (r *Router) DescribedAddresses() []string {
	doc := r.openAPIDocument()
	paths, _ := doc["paths"].(map[string]any)
	out := make([]string, 0, len(paths))
	for p := range paths {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// effectiveRole is what a caller actually needs to use one operation.
//
// Access is enforced in two places: the wrapper a route is registered with,
// which covers every method on it, and a role check inside the handler, which
// can apply to one method only — reading a batch is open to anybody while
// running it is not. Only the wrapper is visible in the route table, so a
// description built from that alone understates more than fifty operations,
// and an integration built on a viewer credential meets refusals the document
// did not predict.
//
// The stricter of the two wins. Handler-level requirements are declared in
// apiRoles, and a test probes every operation and fails when a declaration and
// the service disagree — so this cannot drift the way a second hand-written
// list would.
func effectiveRole(method, path string, wrapper RouteRole) RouteRole {
	declared, ok := apiRoles[method+" "+path]
	if !ok {
		return wrapper
	}
	if roleRank(declared) > roleRank(wrapper) {
		return declared
	}
	return wrapper
}

// roleRank orders the roles from least to most demanding.
func roleRank(role RouteRole) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleAuthenticated:
		return 1
	default:
		return 0
	}
}
