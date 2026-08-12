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
// It has to reach it through an accessor rather than as a value read once.
// Handing over the string at startup is how this setting came to need a
// restart, and it is the shape a file-based configuration left behind.
func TestBinDirIsGivenToTheToolResolver(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", nil, 0)
	if err != nil {
		t.Fatalf("cannot read main.go: %v", err)
	}

	var live, frozen bool
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		switch sel.Sel.Name {
		case "WithBinDirFunc":
			// The accessor has to actually read the setting, not close over a
			// value somebody captured earlier.
			ast.Inspect(call.Args[0], func(inner ast.Node) bool {
				if s, ok := inner.(*ast.SelectorExpr); ok && s.Sel.Name == "EmbeddedBinDir" {
					live = true
				}
				return true
			})
		case "WithBinDir":
			// The static form, given the configured value: a startup snapshot
			// wearing the same clothes.
			if arg, ok := call.Args[0].(*ast.SelectorExpr); ok && arg.Sel.Name == "EmbeddedBinDir" {
				frozen = true
			}
		}
		return true
	})

	if !live {
		t.Error("nothing gives the tool resolver the configured directory through an accessor, " +
			"so the setting on the analysis-tools screen reaches nothing until a restart: an " +
			"operator points at their Chef tools, is told it was saved, and scanning stays off")
	}
	if frozen {
		t.Error("the configured directory is handed over as a value read once at startup, so " +
			"changing it on the screen does nothing until somebody restarts the service — " +
			"which is what the accessor exists to stop")
	}
}
