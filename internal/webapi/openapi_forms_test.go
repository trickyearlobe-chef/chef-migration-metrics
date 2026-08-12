// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// The addresses that take a form rather than a JSON document.
//
// A form field is a string key read by hand out of the request, so there is no
// type to reflect and the names have to be declared. What holds them honest is
// the pair below, in the same shape as the pagination checks: nothing may
// describe a field no handler reads, and no key a handler reads may go
// undescribed. Between them a renamed field breaks the build rather than a
// caller.

// formKeysRead collects every form field name read anywhere in the handlers,
// with the function that reads it.
func formKeysRead(t *testing.T) map[string]map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	out := map[string]map[string]bool{}

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
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 1 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "FormValue" && sel.Sel.Name != "FormFile") {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				key := strings.Trim(lit.Value, `"`)
				if out[key] == nil {
					out[key] = map[string]bool{}
				}
				out[key][fn.Name.Name] = true
				return true
			})
		}
	}
	if len(out) == 0 {
		t.Fatal("no form fields were found in the handlers, so nothing was checked; if the " +
			"idiom changed, point this test at the new one rather than deleting it")
	}
	return out
}

// declaredFormFields is every field any address says it takes.
func declaredFormFields(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, rt := range testRouterForDescription(t).Routes() {
		for _, addr := range describableAddresses(rt) {
			for method, fields := range addr.forms {
				for _, field := range fields {
					out[field.Name] = append(out[field.Name], method+" "+addr.path)
				}
			}
		}
	}
	return out
}

// Nothing may be described that no handler reads. A caller fills it in, sends
// it, and it is dropped without a word — the same failure as describing a body
// nothing decodes.
func TestForms_NothingIsDescribedThatNoHandlerReads(t *testing.T) {
	read := formKeysRead(t)
	for field, addresses := range declaredFormFields(t) {
		if len(read[field]) == 0 {
			t.Errorf("%s is described as taking a form field %q, but no handler reads one — a "+
				"caller sends it and it is silently ignored", addresses[0], field)
		}
	}
}

// The other direction. A field a handler reads and nothing describes is the
// failure this whole tail exists to end: the call is refused, and the message
// names a field the caller has never heard of.
func TestForms_NoFieldAHandlerReadsGoesUndescribed(t *testing.T) {
	// Read by the SAML handling rather than by an address a caller posts a
	// form to deliberately: the identity provider's browser POST carries it,
	// and its shape is the SAML binding rather than anything this service
	// decides.
	fromIdentityProvider := map[string]bool{"RelayState": true, "SAMLResponse": true}

	declared := declaredFormFields(t)
	for field, readers := range formKeysRead(t) {
		if fromIdentityProvider[field] {
			continue
		}
		if len(declared[field]) == 0 {
			t.Errorf("%v reads a form field %q that no address describes, so a caller cannot "+
				"know to send it and gets a refusal naming a field they have never seen",
				sortedFunctionNames(readers), field)
		}
	}
}

// Every address described as taking an upload now names its fields. The
// placeholder that said otherwise is what this replaced.
func TestForms_EveryUploadAddressNamesItsFields(t *testing.T) {
	described := map[string]bool{}
	for _, rt := range testRouterForDescription(t).Routes() {
		for _, addr := range describableAddresses(rt) {
			for method, fields := range addr.forms {
				if len(fields) > 0 {
					described[method+" "+addr.path] = true
				}
			}
		}
	}
	for address := range uploadWrites {
		if !described[address] {
			t.Errorf("%s is described as an upload whose fields nobody has written down, "+
				"which tells a caller only that the description is incomplete", address)
		}
	}
}

func sortedFunctionNames(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	return out
}
