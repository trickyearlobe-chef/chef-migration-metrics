// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import "testing"

func TestApplyTransforms(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		transforms []Transform
		want       string
	}{
		{"no transforms is identity", " Alice ", nil, " Alice "},
		{"trim", "  alice  ", []Transform{{Kind: "trim"}}, "alice"},
		{"lowercase", "ALICE", []Transform{{Kind: "lowercase"}}, "alice"},
		{"uppercase", "alice", []Transform{{Kind: "uppercase"}}, "ALICE"},
		{"prefix", "alice", []Transform{{Kind: "prefix", Value: "team-"}}, "team-alice"},
		{"suffix", "alice", []Transform{{Kind: "suffix", Value: "-team"}}, "alice-team"},
		{"replace", "a_b_c", []Transform{{Kind: "replace", From: "_", To: "-"}}, "a-b-c"},
		{
			"chain applies left to right",
			"  ALICE@EXAMPLE.COM ",
			[]Transform{{Kind: "trim"}, {Kind: "lowercase"}, {Kind: "strip_domain"}},
			"alice",
		},
		{
			"order matters — strip_domain before lowercase leaves case",
			"ALICE@EXAMPLE.COM",
			[]Transform{{Kind: "strip_domain"}},
			"ALICE",
		},
		{"default fills an empty value", "", []Transform{{Kind: "default", Value: "unassigned"}}, "unassigned"},
		{"default leaves a non-empty value alone", "alice", []Transform{{Kind: "default", Value: "unassigned"}}, "alice"},
		{"default does not treat whitespace as empty", " ", []Transform{{Kind: "default", Value: "unassigned"}}, " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, err := CompileTransforms(tt.transforms)
			if err != nil {
				t.Fatalf("CompileTransforms: %v", err)
			}
			if got := chain.Apply(tt.in); got != tt.want {
				t.Errorf("Apply(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// strip_domain must leave an address literal alone. Truncating one produces a
// value that looks plausible and is wrong, which is the worst kind of import
// defect because nothing downstream can detect it.
func TestStripDomain_LeavesIPLiteralsUnchanged(t *testing.T) {
	tests := []struct{ in, want string }{
		{"alice@example.com", "alice"},
		{"alice@sub.example.com", "alice"},
		{"alice", "alice"},
		{"", ""},
		{"10.0.0.1", "10.0.0.1"},
		{"192.168.0.10", "192.168.0.10"},
		{"::1", "::1"},
		{"fe80::1", "fe80::1"},
		{"2001:db8::8a2e:370:7334", "2001:db8::8a2e:370:7334"},
		// An address with a user part is still an address on the right, and the
		// localpart is still what we want.
		{"alice@10.0.0.1", "alice"},
		// Several "@" — the localpart is everything before the first.
		{"a@b@example.com", "a"},
	}
	chain, err := CompileTransforms([]Transform{{Kind: "strip_domain"}})
	if err != nil {
		t.Fatalf("CompileTransforms: %v", err)
	}
	for _, tt := range tests {
		if got := chain.Apply(tt.in); got != tt.want {
			t.Errorf("strip_domain(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// regex_extract yields the empty string when it does not match. It must never
// pass the input through: unextracted raw text reaching an owner name creates
// owners nobody recognises, and the mapping looks like it worked.
func TestRegexExtract_EmptyOnNoMatchOrNoCaptureGroup(t *testing.T) {
	tests := []struct {
		name, pattern, in, want string
	}{
		{"first capture group", `^([a-z]+)-`, "platform-team", "platform"},
		{"no match yields empty", `^([a-z]+)-`, "12345", ""},
		{"pattern with no capture group yields empty", `[a-z]+`, "platform", ""},
		{"empty input yields empty", `^([a-z]+)-`, "", ""},
		{"only the first capture group is used", `(\w+)\.(\w+)`, "alice.smith", "alice"},
		{"unanchored match is found mid-string", `(\d+)`, "node number 42 here", "42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, err := CompileTransforms([]Transform{{Kind: "regex_extract", Pattern: tt.pattern}})
			if err != nil {
				t.Fatalf("CompileTransforms(%q): %v", tt.pattern, err)
			}
			if got := chain.Apply(tt.in); got != tt.want {
				t.Errorf("regex_extract(%q, %q) = %q, want %q", tt.pattern, tt.in, got, tt.want)
			}
		})
	}
}

func TestCompileTransforms_Rejects(t *testing.T) {
	tests := []struct {
		name       string
		transforms []Transform
	}{
		{"unknown kind", []Transform{{Kind: "titlecase"}}},
		{"empty kind", []Transform{{Kind: ""}}},
		{"uncompilable pattern", []Transform{{Kind: "regex_extract", Pattern: "([a-z"}}},
		{"regex_extract without a pattern", []Transform{{Kind: "regex_extract"}}},
		{"replace without a from", []Transform{{Kind: "replace", To: "-"}}},
		{"an unknown kind later in the chain is still caught", []Transform{{Kind: "trim"}, {Kind: "nope"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := CompileTransforms(tt.transforms); err == nil {
				t.Errorf("CompileTransforms(%+v) = nil error, want an error", tt.transforms)
			}
		})
	}
}

// A compiled chain is reused across every row of an import, so it must not
// accumulate state between calls.
func TestCompiledChain_IsReusable(t *testing.T) {
	chain, err := CompileTransforms([]Transform{{Kind: "trim"}, {Kind: "lowercase"}, {Kind: "strip_domain"}})
	if err != nil {
		t.Fatalf("CompileTransforms: %v", err)
	}
	for i := 0; i < 3; i++ {
		if got := chain.Apply(" Alice@Example.com "); got != "alice" {
			t.Fatalf("call %d: got %q, want %q", i, got, "alice")
		}
		if got := chain.Apply(" Bob@Example.com "); got != "bob" {
			t.Fatalf("call %d: got %q, want %q", i, got, "bob")
		}
	}
}
