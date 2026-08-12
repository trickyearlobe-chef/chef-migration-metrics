// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// What a write call expects is the part of the description a caller cannot work
// around. A missing path is obvious — nothing answers at all. A missing body is
// not: the address answers, refuses, and complains about a field the caller has
// never heard of.
//
// The handlers are the record here, as the route table is for addresses. These
// tests read them, so the set of bodies the service decodes and the set it
// describes cannot drift apart without a red build.

// A body decoded into a struct declared inline in the handler is invisible to
// everything else here: it has no name, so no address can declare it, so the
// description says the call takes nothing.
func TestBodies_NoRequestBodyIsDecodedIntoAnAnonymousStruct(t *testing.T) {
	sites := requestBodyDecodeSites(t)
	if len(sites) == 0 {
		t.Fatal("no request-body decode sites were found in the handlers, so nothing was " +
			"checked; if the decode idiom changed, point this test at the new one rather than " +
			"deleting it")
	}

	for _, site := range sites {
		if site.typeName == "" {
			t.Errorf("%s decodes a request body into a struct with no name, so nothing can "+
				"describe it: the address reads as taking no input while the handler refuses "+
				"every call that sends none. Give the type a name and declare it with takes()",
				site.pos)
		}
	}
}

// Every named type a handler decodes from the request body is declared by some
// address. Without this, lifting a body to a named type and forgetting to
// declare it leaves the call described as taking nothing, and no other test
// here notices.
func TestBodies_EveryDecodedRequestBodyIsDescribed(t *testing.T) {
	declared := map[string]bool{}
	for _, decl := range declaredBodies(t) {
		declared[goTypeName(reflect.TypeOf(decl.value))] = true
	}

	for _, site := range requestBodyDecodeSites(t) {
		if site.typeName == "" {
			continue // reported by the anonymous-struct test above
		}
		if !declared[site.typeName] {
			t.Errorf("%s decodes a request body into %s, but no address declares it — the call "+
				"is described as taking no input, so a generated client cannot make it",
				site.pos, site.typeName)
		}
	}
}

// The other direction. A declared type nothing decodes describes a body the
// service ignores, which is worse than silence: the caller builds it, sends it,
// gets a 200 and believes it worked.
func TestBodies_EveryDescribedBodyIsReallyDecoded(t *testing.T) {
	decoded := map[string]bool{}
	for _, site := range requestBodyDecodeSites(t) {
		decoded[site.typeName] = true
	}

	for _, decl := range declaredBodies(t) {
		name := goTypeName(reflect.TypeOf(decl.value))
		if name == "" || strings.HasSuffix(name, ".") {
			t.Errorf("%s %s declares an unnamed type as its body, which cannot be described",
				decl.method, decl.path)
			continue
		}
		if !decoded[name] {
			t.Errorf("%s %s is described as taking a %s, but no handler decodes one from the "+
				"request body — a caller sends it and it is dropped without a word",
				decl.method, decl.path, name)
		}
	}
}

// The description a caller actually fetches carries the body, resolved through
// the reference, not just the route table in memory.
func TestBodies_ADescribedBodyReachesTheServedDocument(t *testing.T) {
	doc := newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).openAPIDocument()

	paths, _ := doc["paths"].(map[string]any)
	item, _ := paths["/api/v1/auth/login"].(map[string]any)
	post, _ := item["post"].(map[string]any)
	if post == nil {
		t.Fatal("signing in is not described at all")
	}

	body, _ := post["requestBody"].(map[string]any)
	if body == nil {
		t.Fatal("signing in is described as taking no input, so nobody can work out what to send")
	}
	content, _ := body["content"].(map[string]any)
	media, _ := content["application/json"].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	ref, _ := schema["$ref"].(string)
	if ref == "" {
		t.Fatalf("the body carries no schema reference: %v", schema)
	}

	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	target, _ := schemas[strings.TrimPrefix(ref, schemaRefPrefix)].(map[string]any)
	if target == nil {
		t.Fatalf("the body refers to %q, which the document does not define — the reference "+
			"dangles and standard tooling will not load it", ref)
	}
	props, _ := target["properties"].(map[string]any)
	for _, field := range []string{"username", "password"} {
		if _, ok := props[field]; !ok {
			t.Errorf("signing in is described without %q, so a generated client cannot do it",
				field)
		}
	}
}

// Every write is described as taking something, or is listed as taking nothing.
// The two tests above hold the described set honest; this one is about
// coverage — a write that reads nothing from its body is a real and common
// thing here (running a scan, resolving an entry), and the only way to tell it
// apart from one somebody forgot is to say so.
func TestBodies_EveryWriteEitherDeclaresABodyOrIsListedAsTakingNone(t *testing.T) {
	declared := map[string]bool{}
	for _, decl := range declaredBodies(t) {
		declared[decl.method+" "+decl.path] = true
	}

	for _, rt := range newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes() {
		for _, addr := range describableAddresses(rt) {
			for _, method := range addr.methods {
				if !writesSomething(method) {
					continue
				}
				key := method + " " + addr.path
				if declared[key] || bodylessWrites[key] ||
					uploadWrites[key] || undescribedBodies[key] != "" {
					continue
				}
				t.Errorf("%s takes no described input. Declare the type it decodes with "+
					"takes(); or say plainly which of the other three it is — reads nothing "+
					"(bodylessWrites), takes a form (uploadWrites), or takes a body we "+
					"deliberately do not describe (undescribedBodies, with the reason)", key)
			}
		}
	}
}

// Anything listed as reading no body must still be served, or the list becomes
// where stale claims go to hide.
func TestBodies_NothingListedAsBodylessHasBeenRemoved(t *testing.T) {
	served := map[string]bool{}
	for _, rt := range newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes() {
		for _, addr := range describableAddresses(rt) {
			for _, method := range addr.methods {
				served[method+" "+addr.path] = true
			}
		}
	}

	for _, list := range []struct {
		name string
		keys []string
	}{
		{"bodylessWrites", keysOf(bodylessWrites)},
		{"uploadWrites", keysOf(uploadWrites)},
		{"undescribedBodies", reasonKeysOf(undescribedBodies)},
	} {
		for _, key := range list.keys {
			if !served[key] {
				t.Errorf("%q is in %s, but nothing serves it — remove the entry rather than "+
					"leaving it to be read as a live claim", key, list.name)
			}
		}
	}
}

// A write cannot be both described as taking a body and listed as taking none.
func TestBodies_NothingIsBothDescribedAndListedAsBodyless(t *testing.T) {
	for _, decl := range declaredBodies(t) {
		key := decl.method + " " + decl.path
		if bodylessWrites[key] {
			t.Errorf("%s declares a request body and is also listed as reading none; one of the "+
				"two is wrong and a caller cannot tell which", key)
		}
	}
	for key := range uploadWrites {
		if bodylessWrites[key] {
			t.Errorf("%s is listed both as taking a form and as reading nothing", key)
		}
		if undescribedBodies[key] != "" {
			t.Errorf("%s is listed both as taking a form and as taking an undescribed body", key)
		}
	}
	for key := range undescribedBodies {
		if bodylessWrites[key] {
			t.Errorf("%s is listed both as taking an undescribed body and as reading nothing", key)
		}
	}
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func reasonKeysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writesSomething(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	default:
		// DELETE is left out on purpose: every one of these identifies what to
		// remove in the address, and none reads a body.
		return false
	}
}

type declaredBody struct {
	method string
	path   string
	value  any
}

func declaredBodies(t *testing.T) []declaredBody {
	t.Helper()
	var out []declaredBody
	for _, rt := range newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes() {
		for method, vs := range rt.Bodies {
			for _, v := range vs {
				out = append(out, declaredBody{method: method, path: rt.Pattern, value: v})
			}
		}
		for _, sp := range rt.SubPaths {
			for method, vs := range sp.Bodies {
				for _, v := range vs {
					out = append(out, declaredBody{
						method: method, path: rt.Pattern + sp.Suffix, value: v})
				}
			}
		}
	}
	return out
}

// decodeSite is one place a handler reads the request body.
type decodeSite struct {
	pos string
	// typeName is the named type decoded into, or empty when the variable was
	// declared as a struct with no name.
	typeName string
}

// requestBodyDecodeSites finds every `json.NewDecoder(req.Body).Decode(&x)` in
// the package and resolves x back to how it was declared.
//
// Deliberately narrow: only the request body. Decoding a column of stored JSON,
// or an uploaded file, is a different thing with different rules — the shapes
// under ingest are genuinely flexible and naming them would turn a real-world
// change into a decode failure.
func requestBodyDecodeSites(t *testing.T) []decodeSite {
	t.Helper()
	fset := token.NewFileSet()
	var out []decodeSite

	for _, path := range handlerSources(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("cannot read %s: %v", path, err)
		}
		// One function at a time. Nearly every handler calls its body `body`,
		// so resolving names across a whole file makes two handlers in one file
		// look like the same type — which reads as "already described" and is
		// the quietest possible way to lose a body.
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			out = append(out, decodeSitesIn(fset, fn)...)
		}
	}
	return out
}

// decodeSitesIn finds the request-body decodes in one function, resolving each
// against how that function declared the variable.
func decodeSitesIn(fset *token.FileSet, fn *ast.FuncDecl) []decodeSite {
	wholeBody := readsWholeRequestBody(fn)
	named := map[string]string{}
	anonymous := map[string]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		value, ok := n.(*ast.ValueSpec)
		if !ok || value.Type == nil {
			return true
		}
		for _, name := range value.Names {
			if _, inline := value.Type.(*ast.StructType); inline {
				anonymous[name.Name] = true
				continue
			}
			if rendered := renderTypeExpr(value.Type); rendered != "" {
				named[name.Name] = rendered
			}
		}
		return true
	})

	var out []decodeSite
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isBodyDecodeCall(call) {
			return true
		}
		if isUnmarshalCall(call) && !wholeBody {
			return true
		}
		unary, ok := call.Args[len(call.Args)-1].(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			return true
		}
		ident, ok := unary.X.(*ast.Ident)
		if !ok {
			return true
		}
		site := decodeSite{pos: fset.Position(call.Pos()).String()}
		if anonymous[ident.Name] {
			out = append(out, site)
			return true
		}
		if name, ok := named[ident.Name]; ok {
			site.typeName = name
			out = append(out, site)
		}
		return true
	})
	return out
}

// isBodyDecodeCall reports whether a call reads the request body into its last
// argument. Three idioms exist here and all have to be recognised: missing one
// hides a whole area of writes from every check in this file, which is the
// quietest way to lose a guard.
func isBodyDecodeCall(call *ast.CallExpr) bool {
	// json.NewDecoder(req.Body).Decode(&x)
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok &&
		sel.Sel.Name == "Decode" && len(call.Args) == 1 && decodesTheRequestBody(sel.X) {
		return true
	}
	// decodeAdminConfigBody(w, req, &x) — the settings sections, read by the
	// YAML decoder so that a caller may send either YAML or JSON.
	if fn, ok := call.Fun.(*ast.Ident); ok &&
		fn.Name == "decodeAdminConfigBody" && len(call.Args) == 3 {
		return true
	}
	// json.Unmarshal(body, &x) / yaml.Unmarshal(body, &x), where body was read
	// off the request. Only counted inside a handler that reads the request
	// body whole — see readsWholeRequestBody — because these two functions are
	// used all over the package on stored JSON, which is a different thing
	// under different rules.
	if isUnmarshalCall(call) && len(call.Args) == 2 {
		if sel := call.Fun.(*ast.SelectorExpr); true {
			if pkg, ok := sel.X.(*ast.Ident); ok && (pkg.Name == "json" || pkg.Name == "yaml") {
				return true
			}
		}
	}
	return false
}

func isUnmarshalCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Unmarshal"
}

// readsWholeRequestBody reports whether a function reads the request body in
// one go, which is what makes a later Unmarshal in the same function a
// request-body decode rather than a decode of something we stored earlier.
func readsWholeRequestBody(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "ReadAll" {
			return true
		}
		if arg, ok := call.Args[0].(*ast.SelectorExpr); ok && arg.Sel.Name == "Body" {
			if ident, ok := arg.X.(*ast.Ident); ok && ident.Name == "req" {
				found = true
			}
		}
		return true
	})
	return found
}

// renderTypeExpr writes a type expression the way goTypeName writes a
// reflect.Type, so the two can be compared.
func renderTypeExpr(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		if isBuiltinTypeName(t.Name) {
			return t.Name
		}
		return "webapi." + t.Name
	case *ast.SelectorExpr:
		pkg, ok := t.X.(*ast.Ident)
		if !ok {
			return ""
		}
		return pkg.Name + "." + t.Sel.Name
	case *ast.ArrayType:
		if inner := renderTypeExpr(t.Elt); inner != "" {
			return "[]" + inner
		}
	case *ast.StarExpr:
		return renderTypeExpr(t.X)
	}
	return ""
}

func isBuiltinTypeName(name string) bool {
	switch name {
	case "string", "bool", "byte", "rune", "error", "any",
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64":
		return true
	}
	return false
}

// decodesTheRequestBody reports whether a decoder was built over req.Body.
func decodesTheRequestBody(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "NewDecoder" {
		return false
	}
	arg, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok || arg.Sel.Name != "Body" {
		return false
	}
	ident, ok := arg.X.(*ast.Ident)
	return ok && ident.Name == "req"
}

// handlerSources lists the non-test Go files in this package.
func handlerSources(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("cannot list the package: %v", err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, filepath.Join(".", name))
	}
	if len(out) == 0 {
		t.Fatal("no handler sources were found, so nothing was checked")
	}
	return out
}
