// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// The filters, as opposed to the pagination.
//
// page and per_page are declared with paginated() and their bounds read off the
// constants the service applies. Everything else in the query string is a
// string key read by hand, so there is nothing to reflect and the names have to
// be declared — and, as with pagination, the unit is the (method, address)
// rather than the handler, because one handler serves several addresses and
// reads different keys at each.
//
// What holds them honest is below: nothing may be described that no handler
// reads, and the number of keys nothing describes may only fall. The first is
// the one that matters — a described filter that is ignored is worse than an
// undescribed one, because the caller believes the answer was filtered.

// queryKeysRead collects every query-string key read anywhere in the handlers,
// with the functions that read it.
//
// Read syntactically, and deliberately narrowly: a key counts when it is read
// off a request's query directly, off a variable assigned from one, or off a
// parameter declared as url.Values — which is how the biggest filter set here
// is written, as a function taking the values rather than the request. Anything
// else that happens to be called Get is not a query parameter, and a header is
// certainly not one.
func queryKeysRead(t *testing.T) map[string]map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	out := map[string]map[string]bool{}

	record := func(key, fn string) {
		if out[key] == nil {
			out[key] = map[string]bool{}
		}
		out[key][fn] = true
	}

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
			for key := range queryKeysIn(fn) {
				record(key, fn.Name.Name)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no query parameters were found in the handlers, so nothing was checked; if " +
			"the idiom changed, point this test at the new one rather than deleting it")
	}
	return out
}

// queryKeysIn reads one function's query keys.
func queryKeysIn(fn *ast.FuncDecl) map[string]bool {
	values := map[string]bool{} // variables holding a url.Values
	for _, param := range fn.Type.Params.List {
		if typeName(param.Type) != "url.Values" {
			continue
		}
		for _, name := range param.Names {
			values[name.Name] = true
		}
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			if i < len(assign.Lhs) && isQueryCall(rhs) {
				if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
					values[ident.Name] = true
				}
			}
		}
		return true
	})

	keys := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if key, ok := queryKeyFromCall(node, values); ok {
				keys[key] = true
			}
		case *ast.IndexExpr:
			// req.URL.Query()["organisation"], or the same off a variable.
			if !isQueryCall(node.X) && !isQueryValue(node.X, values) {
				return true
			}
			if key, ok := stringLiteral(node.Index); ok {
				keys[key] = true
			}
		}
		return true
	})
	return keys
}

// queryKeyFromCall reads the key out of a Get or Has on a request's query, or
// out of the queryStringSlice helper that reads a repeated parameter.
func queryKeyFromCall(call *ast.CallExpr, values map[string]bool) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "queryStringSlice" &&
		len(call.Args) == 2 {
		return stringLiteral(call.Args[1])
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Get" && sel.Sel.Name != "Has") || len(call.Args) != 1 {
		return "", false
	}
	if !isQueryCall(sel.X) && !isQueryValue(sel.X, values) {
		return "", false
	}
	return stringLiteral(call.Args[0])
}

// isQueryCall reports whether an expression is a call to .URL.Query().
func isQueryCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Query" && strings.HasSuffix(typeName(sel.X), ".URL")
}

// isQueryValue reports whether an expression is a variable holding url.Values.
func isQueryValue(expr ast.Expr, values map[string]bool) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && values[ident.Name]
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(lit.Value, `"`), true
}

// typeName writes a selector or identifier back out as source, so "url.Values"
// and "req.URL" can be recognised without a type checker.
func typeName(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.SelectorExpr:
		return typeName(node.X) + "." + node.Sel.Name
	case *ast.StarExpr:
		return typeName(node.X)
	}
	return ""
}

// declaredQueryParams is every filter any address says it reads.
func declaredQueryParams(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, rt := range testRouterForDescription(t).Routes() {
		for _, addr := range describableAddresses(rt) {
			for method, params := range addr.queries {
				for _, param := range params {
					out[param.Name] = append(out[param.Name], method+" "+addr.path)
				}
			}
		}
	}
	return out
}

// Nothing may be described that no handler reads.
//
// This is the failure that matters. A caller sends a filter, the service
// ignores it, and the answer looks filtered — so they act on a subset that is
// really the whole estate, or a whole estate they believe is a subset.
func TestFilters_NothingIsDescribedThatNoHandlerReads(t *testing.T) {
	read := queryKeysRead(t)
	for param, addresses := range declaredQueryParams(t) {
		if len(read[param]) == 0 {
			t.Errorf("%s is described as taking %q, but no handler reads it — a caller filters "+
				"by it, is ignored, and believes the answer was narrowed",
				addresses[0], param)
		}
	}
}

// The filters nothing describes may only get fewer.
//
// Counted rather than listed, for the same reason the undescribed answers are:
// a list is a second thing to keep true, and the count is recomputed every run.
// pagination is excluded — it is declared separately and described already.
func TestFilters_TheUndescribedFiltersOnlyGetFewer(t *testing.T) {
	const undescribed = 20

	declared := declaredQueryParams(t)
	var missing []string
	for param := range queryKeysRead(t) {
		switch param {
		case "page", "per_page", "sort", "order":
			continue // declared with paginated(), and described with its bounds
		}
		if len(declared[param]) == 0 {
			missing = append(missing, param)
		}
	}
	sort.Strings(missing)

	switch {
	case len(missing) > undescribed:
		t.Errorf("%d filters are read and described nowhere, up from %d (%s) — a caller cannot "+
			"know to send them", len(missing), undescribed, strings.Join(missing, ", "))
	case len(missing) < undescribed:
		t.Errorf("%d filters are read and described nowhere, down from %d. Strike the number "+
			"down to %d so the next one cannot creep back up", len(missing), undescribed,
			len(missing))
	}
}
