// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The route table is what makes a generated API description possible: it is the
// set of addresses this service actually serves, recorded as they are
// registered rather than written down beside them. Every claim the description
// makes is checked against it, in both directions, so a renamed path breaks a
// build here instead of a customer's client.

func TestRoutes_AreRecordedAsTheyAreRegistered(t *testing.T) {
	routes := newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes()

	if len(routes) == 0 {
		t.Fatal("the router records no routes, so nothing can say what this service serves")
	}

	for _, want := range []struct {
		pattern string
		role    RouteRole
	}{
		{"/api/v1/health", RolePublic},
		{"/api/v1/cookbooks", RoleAuthenticated},
		{"/api/v1/admin/users/", RoleAdmin},
	} {
		got, ok := findRoute(routes, want.pattern)
		if !ok {
			t.Errorf("%s is served but not recorded, so the description would omit it",
				want.pattern)
			continue
		}
		if got.Role != want.role {
			t.Errorf("%s is recorded as needing %q, but it is registered as %q — a description "+
				"built from this would tell a caller the wrong thing about access",
				want.pattern, got.Role, want.role)
		}
	}
}

// A recorded route that is not served is the same lie as a described path that
// is not served — it just gets there one step earlier.
func TestRoutes_EveryRecordedRouteIsServed(t *testing.T) {
	router := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"))

	for _, rt := range router.Routes() {
		// The frontend fallback answers anything the API does not, so a
		// recorded-but-unserved route would be indistinguishable from a served
		// one unless the matched pattern itself is compared.
		_, matched := router.mux.Handler(httptest.NewRequest(http.MethodGet, rt.Pattern, nil))
		if matched != rt.Pattern {
			t.Errorf("%s is recorded, but a request for it is served by %q instead",
				rt.Pattern, matched)
		}
	}
}

// A declared sub-path that does not land on the subtree that declared it is a
// typo describing an address nobody serves — and it would read as authoritative
// precisely because it was declared next to the registration.
//
// This proves where the address lands, not that the handler dispatches it. The
// second half is only answerable by asking the handler for real, which is what
// the journey suite does against the served description.
func TestRoutes_EveryDeclaredSubPathLandsOnItsSubtree(t *testing.T) {
	router := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"))

	for _, rt := range router.Routes() {
		for _, sp := range rt.SubPaths {
			if !rt.IsSubtree() {
				t.Errorf("%s declares sub-path %q but is not a subtree, so nothing dispatches it",
					rt.Pattern, sp.Suffix)
				continue
			}
			address := rt.Pattern + strings.ReplaceAll(
				strings.ReplaceAll(sp.Suffix, "{", ""), "}", "")
			_, matched := router.mux.Handler(httptest.NewRequest(http.MethodGet, address, nil))
			if matched != rt.Pattern {
				t.Errorf("%s declares %q, but a request for %s is served by %q instead",
					rt.Pattern, sp.Suffix, address, matched)
			}
			if len(sp.Methods) == 0 {
				t.Errorf("%s%s declares no methods, so a caller cannot tell how to reach it",
					rt.Pattern, sp.Suffix)
			}
		}
	}
}

// The recording only holds if every registration goes through it. Public routes
// used to be registered straight onto the mux, so a route added that way would
// be served and undescribed — which is the failure this whole mechanism exists
// to make impossible. Reading the source is the only way to check the funnel is
// the only door.
func TestRoutes_RegistrationGoesOnlyThroughTheFunnel(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "router.go", nil, 0)
	if err != nil {
		t.Fatalf("cannot read the router: %v", err)
	}

	var read bool
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "registerRoutes" {
			return true
		}
		read = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "mux" {
				return true
			}
			t.Errorf("registerRoutes touches the mux directly at %s; a route registered that "+
				"way is served but never recorded, so it cannot appear in the description and "+
				"nothing goes red", fset.Position(sel.Pos()))
			return true
		})
		return false
	})

	// Without this, renaming registerRoutes turns the check into a test that
	// inspects nothing and passes — the quietest way to lose a guard.
	if !read {
		t.Fatal("registerRoutes was not found in router.go, so nothing was checked; if it moved " +
			"or was renamed, point this test at it rather than deleting it")
	}
}

func findRoute(routes []Route, pattern string) (Route, bool) {
	for _, rt := range routes {
		if rt.Pattern == pattern {
			return rt, true
		}
	}
	return Route{}, false
}

// The point of generating the description: the set it claims and the set the
// service serves cannot drift apart, because they are computed from the same
// record. These two tests are what turn a rename into a red build here rather
// than a broken client at a customer.

func TestOpenAPI_DescribesEveryServedAddress(t *testing.T) {
	router := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"))

	described := map[string]bool{}
	for _, p := range router.DescribedAddresses() {
		described[p] = true
	}

	for _, rt := range router.Routes() {
		for _, addr := range describableAddresses(rt) {
			if !described[addr.path] {
				t.Errorf("%s is served but the description does not mention it, so anybody "+
					"working from the description does not know it exists", addr.path)
			}
		}
	}
}

func TestOpenAPI_DescribesNothingItDoesNotServe(t *testing.T) {
	router := newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0"))

	served := map[string]bool{}
	for _, rt := range router.Routes() {
		for _, addr := range describableAddresses(rt) {
			served[addr.path] = true
		}
	}

	for _, p := range router.DescribedAddresses() {
		if !served[p] {
			t.Errorf("the description promises %s, which nothing serves — a caller writes "+
				"against it and fails at three in the morning", p)
		}
	}
}

// A subtree that declares no sub-paths describes nothing, and would leave a
// whole area of the service invisible while everything here stayed green.
func TestOpenAPI_NoSubtreeIsLeftUndeclared(t *testing.T) {
	for _, rt := range newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes() {
		if !strings.HasPrefix(rt.Pattern, "/api/") || !rt.IsSubtree() {
			continue
		}
		if len(rt.SubPaths) == 0 {
			t.Errorf("%s dispatches addresses inside its handler but declares none, so that "+
				"whole area is served and undescribed", rt.Pattern)
		}
	}
}

// The description is only reachable if it is served where a client looks for it.
func TestOpenAPI_IsServed(t *testing.T) {
	w := httptest.NewRecorder()
	newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0")).
		ServeHTTP(w, withAdminSession(httptest.NewRequest(http.MethodGet, openAPIPath, nil)))

	if w.Code != http.StatusOK {
		t.Fatalf("the description is not served at %s (status %d)", openAPIPath, w.Code)
	}
	var doc struct {
		OpenAPI string                    `json:"openapi"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("what is served at %s is not readable as JSON: %v", openAPIPath, err)
	}
	if doc.OpenAPI == "" {
		t.Error("what is served is not an OpenAPI document, so standard tooling cannot read it")
	}
	if len(doc.Paths) == 0 {
		t.Error("the description names no paths, so it describes nothing")
	}
	if _, ok := doc.Paths[openAPIPath]; !ok {
		t.Error("the description does not describe itself, so a client cannot discover how it " +
			"was fetched")
	}
}
