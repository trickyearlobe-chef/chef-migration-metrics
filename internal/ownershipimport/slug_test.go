// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package ownershipimport

import (
	"regexp"
	"strings"
	"testing"
	"testing/quick"
)

// ownerNameConstraint mirrors the CHECK on owners.name:
//
//	CONSTRAINT owners_name_format CHECK (name ~ '^[a-z0-9][a-z0-9._-]*$')
//
// migrations/0001_initial_schema.up.sql:729. The slugifier's output is asserted
// against this rather than against the Go-side regex alone, because a value that
// satisfies one and not the other fails at write time, not at import time.
var ownerNameConstraint = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func TestSlugifyOwnerName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"already legal", "alice", "alice"},
		{"uppercase folds down", "Alice", "alice"},
		{"spaces become hyphens", "Alice Smith", "alice-smith"},
		{"email localpart is legal", "alice.smith", "alice.smith"},
		{"underscores are permitted", "alice_smith", "alice_smith"},
		{"runs of separators collapse", "Alice   Smith", "alice-smith"},
		{"mixed illegal runes collapse to one hyphen", "alice (smith)!", "alice-smith"},
		{"leading and trailing hyphens are trimmed", "  alice  ", "alice"},
		{"leading dot is stripped — the constraint forbids it first", ".alice", "alice"},
		{"leading underscore is stripped", "_alice", "alice"},
		{"trailing dot survives — legal after the first rune", "alice.", "alice."},
		{"digits may lead", "3rd-party", "3rd-party"},
		{"accents fold to hyphens, not to ASCII bases", "Renée", "ren-e"},
		{"non-latin scripts fold to hyphens", "日本", ""},
		{"slash separated", "acme/platform", "acme-platform"},
		{"whole email", "Alice.Smith@example.com", "alice.smith-example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SlugifyOwnerName(tt.input)
			if tt.want == "" {
				if err == nil {
					t.Fatalf("SlugifyOwnerName(%q) = %q, want an error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("SlugifyOwnerName(%q) returned error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("SlugifyOwnerName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSlugifyOwnerName_RejectsWhatCannotBecomeAName(t *testing.T) {
	// These are the class the report calls invalid_owner_name: a real value that
	// is neither missing nor malformed, but cannot become a legal owner name.
	for _, input := range []string{"", "   ", "???", "---", "...", "___", "!@#$%"} {
		got, err := SlugifyOwnerName(input)
		if err == nil {
			t.Errorf("SlugifyOwnerName(%q) = %q, want an error", input, got)
		}
	}
}

// TestSlugifyOwnerName_PropertyAgainstConstraint is the assertion that matters:
// for any input at all, the slugifier either errors or returns something the
// database will accept. Fixed cases cannot establish that.
func TestSlugifyOwnerName_PropertyAgainstConstraint(t *testing.T) {
	f := func(s string) bool {
		got, err := SlugifyOwnerName(s)
		if err != nil {
			return got == ""
		}
		return ownerNameConstraint.MatchString(got) && OwnerNameRe.MatchString(got)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 2000}); err != nil {
		t.Error(err)
	}
}

// The property test above generates mostly non-ASCII noise. This one covers the
// shape real ownership exports actually carry.
func TestSlugifyOwnerName_PropertyOverRealisticInput(t *testing.T) {
	alphabet := []rune(" .-_@/\\,;:()[]'\"abcXYZ019éüß")
	f := func(idx []uint8) bool {
		var b strings.Builder
		for _, i := range idx {
			b.WriteRune(alphabet[int(i)%len(alphabet)])
		}
		got, err := SlugifyOwnerName(b.String())
		if err != nil {
			return got == ""
		}
		return ownerNameConstraint.MatchString(got) && OwnerNameRe.MatchString(got)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 3000}); err != nil {
		t.Error(err)
	}
}

func TestSlugifyOwnerName_IsIdempotent(t *testing.T) {
	for _, input := range []string{"Alice Smith", "Renée", "acme/platform", "Alice.Smith@example.com"} {
		once, err := SlugifyOwnerName(input)
		if err != nil {
			t.Fatalf("SlugifyOwnerName(%q): %v", input, err)
		}
		twice, err := SlugifyOwnerName(once)
		if err != nil {
			t.Fatalf("SlugifyOwnerName(%q): %v", once, err)
		}
		if once != twice {
			t.Errorf("not idempotent for %q: %q then %q", input, once, twice)
		}
	}
}
