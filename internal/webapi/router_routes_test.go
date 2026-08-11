// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
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
