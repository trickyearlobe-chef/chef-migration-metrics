// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
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

	for _, rt := range r.Routes() {
		for _, addr := range describableAddresses(rt) {
			item, _ := paths[addr.path].(map[string]any)
			if item == nil {
				item = map[string]any{}
				paths[addr.path] = item
			}
			for _, method := range addr.methods {
				item[strings.ToLower(method)] = r.operation(addr.path, method, rt.Role)
			}
		}
	}

	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title": "Chef Migration Metrics",
			"description": "What is stopping this estate moving to the target Chef version, " +
				"and what to do first.",
			"version": r.version,
		},
		"paths": paths,
	}
}

// address is one describable endpoint: a concrete path and the methods it
// answers.
type address struct {
	path    string
	methods []string
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
		return []address{{path: rt.Pattern, methods: rt.Methods}}
	}
	out := make([]address, 0, len(rt.SubPaths))
	for _, sp := range rt.SubPaths {
		out = append(out, address{path: rt.Pattern + sp.Suffix, methods: sp.Methods})
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
	op["x-required-role"] = string(role)
	if role != RolePublic {
		op["security"] = []any{map[string]any{"bearerAuth": []any{}}}
	}

	if doc, ok := apiDocs[method+" "+path]; ok {
		op["summary"] = doc
	}
	return op
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
