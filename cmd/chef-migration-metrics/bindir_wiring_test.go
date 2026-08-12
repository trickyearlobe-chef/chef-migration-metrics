// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The configured directory reaches the thing that resolves the Chef tools.
//
// internal/embedded is tested on its own and can be entirely right about a
// directory it is handed while nothing hands it one — which is exactly the
// state this setting was in: a box on the settings screen, a resolver that
// only ever looked at PATH, and nothing between them. So the wiring itself is
// held here.
//
// Read out of the source because this happens inside setupCollector, which
// needs a database, a collector and a running process to call. The claim is a
// one-line one and reading it is enough to hold it.
func TestBinDirIsGivenToTheToolResolver(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("cannot read main.go: %v", err)
	}

	var configured bool
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "WithBinDir" {
			return true
		}
		// The argument has to be the setting, not a literal somebody left
		// behind while testing.
		if arg, ok := call.Args[0].(*ast.SelectorExpr); ok && arg.Sel.Name == "EmbeddedBinDir" {
			configured = true
		}
		return true
	})

	if !configured {
		t.Error("nothing gives the tool resolver the configured directory, so the setting on " +
			"the analysis-tools screen reaches nothing: an operator points at their Chef " +
			"tools, is told it was saved, and scanning stays off")
	}
}
