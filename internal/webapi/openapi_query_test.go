// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"strings"
	"testing"
)

// Which addresses accept page and per_page.
//
// This was measured against a running instance rather than reasoned about, and
// the measurement is why it is declared per address rather than derived. Three
// derivations were tried and all of them over-report:
//
//   - Reachability from the registered handler gives 36 patterns against the 22
//     the service actually paginates, because a subtree handler serves many
//     addresses and only some of them page.
//   - Restricting that to non-subtree routes still over-reports by seven:
//     handleOwnershipIntake and handleOwnershipEndpoints are each registered at
//     several exact patterns and dispatch on the path inside.
//   - Looking for a "pagination" object in the answer misses one address —
//     /run-events/nodes/{organisation}/{name} honours per_page and echoes no
//     metadata at all.
//
// So the declaration is per (method, address), like sub(), and what holds it
// honest is the pair of checks below: nothing may claim pagination whose
// handler cannot reach it, and no handler that reaches it may go unclaimed.
//
// The measurement is repeatable rather than a claim in a comment:
// tools/api-probe/probe.py walks every GET in the served description against a
// running instance and reports which honour per_page. Re-run it after changing
// anything here.

// paginationReach reports which handler functions can reach ParsePagination,
// following calls within this package.
//
// This is an over-approximation on purpose. As an upper bound it is sound —
// a handler that cannot reach ParsePagination definitely does not paginate —
// and that is exactly what the two tests below need it for.
func paginationReach(t *testing.T) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	calls := map[string]map[string]bool{}

	for _, path := range handlerSources(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("cannot read %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if calls[fn.Name.Name] == nil {
				calls[fn.Name.Name] = map[string]bool{}
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					calls[fn.Name.Name][fun.Name] = true
				case *ast.SelectorExpr:
					calls[fn.Name.Name][fun.Sel.Name] = true
				}
				return true
			})
		}
	}
	if len(calls) == 0 {
		t.Fatal("no functions were read, so nothing was checked")
	}

	var reaches func(string, map[string]bool) bool
	reaches = func(name string, seen map[string]bool) bool {
		if seen[name] {
			return false
		}
		seen[name] = true
		for callee := range calls[name] {
			if callee == "ParsePagination" {
				return true
			}
			if reaches(callee, seen) {
				return true
			}
		}
		return false
	}

	out := map[string]bool{}
	for name := range calls {
		if name != "ParsePagination" && reaches(name, map[string]bool{}) {
			out[name] = true
		}
	}
	return out
}

// registeredHandlers maps each route pattern to the handler it was registered
// with, read from registerRoutes.
func registeredHandlers(t *testing.T) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "router.go", nil, 0)
	if err != nil {
		t.Fatalf("cannot read the router: %v", err)
	}

	// A pattern can be registered more than once — /api/v1/admin/users is
	// registered with the real handler or with handleNotImplemented depending
	// on whether accounts are configured. Keeping only the last one read makes
	// the live handler invisible, so every one is kept.
	out := map[string][]string{}
	var found bool
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "registerRoutes" {
			return true
		}
		found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "public", "protect", "adminOnly", "operatorOnly":
			default:
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			handler, ok := call.Args[1].(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pattern := strings.Trim(lit.Value, `"`)
			out[pattern] = append(out[pattern], handler.Sel.Name)
			return true
		})
		return false
	})
	if !found {
		t.Fatal("registerRoutes was not found in router.go, so nothing was checked; if it " +
			"moved or was renamed, point this test at it rather than deleting it")
	}
	return out
}

// Nothing may claim to paginate whose handler cannot reach ParsePagination.
// That is a stale claim: the caller sends per_page, is ignored, and gets the
// whole list believing it got a page.
func TestQuery_NothingClaimsPaginationItCannotDo(t *testing.T) {
	reach := paginationReach(t)
	handlers := registeredHandlers(t)

	for _, rt := range newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes() {
		names, known := handlers[rt.Pattern]
		if !known {
			continue
		}
		anyReaches := false
		for _, n := range names {
			anyReaches = anyReaches || reach[n]
		}
		for _, addr := range describableAddresses(rt) {
			for method := range addr.paginated {
				if !anyReaches {
					t.Errorf("%s %s is described as taking page and per_page, but %v cannot "+
						"reach ParsePagination — a caller asking for a page gets the whole "+
						"list and believes it got a page",
						method, addr.path, names)
				}
			}
			for method := range addr.capped {
				if !anyReaches {
					t.Errorf("%s %s is described as taking per_page, but %v cannot reach "+
						"ParsePagination", method, addr.path, names)
				}
			}
		}
	}
}

// The other direction. A handler that pages but has no address saying so leaves
// a caller with no way to bound an answer except by finding out the hard way.
func TestQuery_NoPaginatingHandlerIsLeftUnclaimed(t *testing.T) {
	reach := paginationReach(t)
	handlers := registeredHandlers(t)

	claimed := map[string]bool{}
	routes := newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes()
	for _, rt := range routes {
		for _, addr := range describableAddresses(rt) {
			if len(addr.paginated) > 0 || len(addr.capped) > 0 {
				claimed[rt.Pattern] = true
			}
		}
	}

	for _, rt := range routes {
		names, known := handlers[rt.Pattern]
		if !known || claimed[rt.Pattern] {
			continue
		}
		anyReaches := false
		for _, n := range names {
			anyReaches = anyReaches || reach[n]
		}
		if !anyReaches {
			continue
		}
		if !strings.HasPrefix(rt.Pattern, "/api/") {
			continue
		}
		if unpaginatedDespiteReaching[rt.Pattern] {
			continue
		}
		t.Errorf("%s is served by %v, which pages, but no address under it says so. Either "+
			"declare it with paginated()/subPaginated(), or record it in "+
			"unpaginatedDespiteReaching — measured against a running instance, not guessed",
			rt.Pattern, names)
	}
}

// A pattern recorded as not paginating must still be served, or the record is
// somewhere stale claims hide.
func TestQuery_NothingRecordedAsUnpaginatedHasBeenRemoved(t *testing.T) {
	served := map[string]bool{}
	for _, rt := range newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes() {
		served[rt.Pattern] = true
	}
	for pattern := range unpaginatedDespiteReaching {
		if !served[pattern] {
			t.Errorf("%q is recorded as not paginating, but nothing serves it — remove the "+
				"entry rather than leaving it to be read as a live claim", pattern)
		}
	}
}

// The parameters reach the served document, described well enough to generate a
// client that can actually walk a list.
func TestQuery_PaginationReachesTheServedDocument(t *testing.T) {
	doc := newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).openAPIDocument()
	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths["/api/v1/cookbooks"].(map[string]any)
	get, _ := item["get"].(map[string]any)
	if get == nil {
		t.Fatal("listing cookbooks is not described at all")
	}

	params, _ := get["parameters"].([]any)
	found := map[string]map[string]any{}
	for _, p := range params {
		param, _ := p.(map[string]any)
		if param["in"] == "query" {
			found[param["name"].(string)] = param
		}
	}

	for _, name := range []string{"page", "per_page"} {
		if found[name] == nil {
			t.Errorf("listing cookbooks does not describe %q, so a generated client has no way "+
				"to ask for the second page", name)
		}
	}
	if perPage := found["per_page"]; perPage != nil {
		schema, _ := perPage["schema"].(map[string]any)
		if schema["maximum"] != maxPerPage {
			t.Errorf("per_page is described with maximum %v, but the service clamps at %d — a "+
				"caller asking for more gets fewer than it was told it could",
				schema["maximum"], maxPerPage)
		}
		if schema["default"] != defaultPerPage {
			t.Errorf("per_page is described as defaulting to %v rather than %d",
				schema["default"], defaultPerPage)
		}
	}
}

// A write is never described as taking page and per_page.
func TestQuery_OnlyReadsArePaginated(t *testing.T) {
	for _, rt := range newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes() {
		for _, addr := range describableAddresses(rt) {
			for method := range addr.paginated {
				if method != http.MethodGet {
					t.Errorf("%s %s is described as paginated, but pagination is a property of "+
						"reading a list", method, addr.path)
				}
			}
			for method := range addr.capped {
				if method != http.MethodGet {
					t.Errorf("%s %s is described as capped, but that is a property of reading "+
						"a list", method, addr.path)
				}
			}
		}
	}
}
