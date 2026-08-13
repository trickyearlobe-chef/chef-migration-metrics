// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The ownership import's "Browse tables" button reported "Method not allowed."
// Two separate faults produced that: the path was never registered on the mux,
// and the frontend fallback that caught it answered on method before it
// noticed the path was an API path at all. The second is the general one — it
// made every unrouted non-GET API request report a method error instead of a
// missing endpoint, which reads as a permissions problem rather than a wiring
// one.

// TestIntakeListTables_IsRouted drives a POST through the mux and asserts it
// reaches handleIntakeListTables. The credential complaint is that handler's
// own, so receiving it proves the request arrived rather than being answered
// by the fallback.
func TestIntakeListTables_IsRouted(t *testing.T) {
	r := testRouter()

	req := intakeRequest(t, "/api/v1/ownership/import/tables", "", map[string]string{
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "no connection was named") {
		t.Errorf("body = %q, want the list-tables handler's own complaint about the connection",
			w.Body.String())
	}
}

// TestUnroutedAPIPath_IsNotFoundNotMethodNotAllowed covers the general fault.
// A POST to an API path nobody registered has to say the endpoint is missing;
// saying the method is wrong sends the reader looking for a permissions or
// verb problem on an endpoint that was never wired up.
func TestUnroutedAPIPath_IsNotFoundNotMethodNotAllowed(t *testing.T) {
	r := testRouter()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/ownership/import/no-such-endpoint", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assertStatus(t, w, http.StatusNotFound)
			assertErrorCode(t, w, ErrCodeNotFound)
		})
	}
}

// TestNonAPIPath_StillRejectsNonGET keeps the fix honest in the other
// direction: the frontend fallback serves a single-page app, and a POST to a
// page route is a method error, not a missing page.
func TestNonAPIPath_StillRejectsNonGET(t *testing.T) {
	r := testRouter()

	req := httptest.NewRequest(http.MethodPost, "/owners", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assertStatus(t, w, http.StatusMethodNotAllowed)
	assertErrorCode(t, w, ErrCodeMethodNotAllowed)
}

// TestOwnershipIntakeDispatchCasesAreRouted reads the paths handleOwnershipIntake
// dispatches on straight out of its source and asserts the mux carries each
// one. A registration list kept in step with a dispatch switch by hand is how
// "tables" went missing in the first place; this fails the next time the two
// drift rather than waiting for somebody to press the button.
func TestOwnershipIntakeDispatchCasesAreRouted(t *testing.T) {
	paths := dispatchPathsIn(t, "handle_ownership_intake.go", "handleOwnershipIntake")
	if len(paths) == 0 {
		t.Fatal("found no dispatch paths in handleOwnershipIntake — the test has lost its subject")
	}

	r := testRouter()
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			// A prefix case names a directory; ask about a child of it.
			probe := p
			if strings.HasSuffix(probe, "/") {
				probe += "probe"
			}
			req := httptest.NewRequest(http.MethodPost, probe, nil)
			_, pattern := r.mux.Handler(req)
			if pattern == "" || pattern == "/" {
				t.Errorf("%s is dispatched by handleOwnershipIntake but no route reaches it "+
					"(matched pattern %q — the frontend fallback)", p, pattern)
			}
		})
	}
}

// dispatchPathsIn returns the API paths the named function compares the request
// path against, taken from the AST so a comment or an unrelated string cannot
// be mistaken for a case.
func dispatchPathsIn(t *testing.T, file, funcName string) []string {
	t.Helper()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}

	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == funcName {
			fn = d
			break
		}
	}
	if fn == nil {
		t.Fatalf("%s not found in %s", funcName, file)
	}

	seen := map[string]bool{}
	var paths []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		v := strings.Trim(lit.Value, `"`)
		if strings.HasPrefix(v, "/api/") && !seen[v] {
			seen[v] = true
			paths = append(paths, v)
		}
		return true
	})
	return paths
}
