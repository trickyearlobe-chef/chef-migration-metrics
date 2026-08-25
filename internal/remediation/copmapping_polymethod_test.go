// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// LookupCopForOffense — poly-method cops (one cop_name, several unrelated
// deprecations discriminated by the offence message). See
// journeys/scan-trust.md.
// ---------------------------------------------------------------------------

func TestLookupCopForOffense_DeprecatedClassMethods_Removed(t *testing.T) {
	// File.exists?/Dir.exists? were removed in Ruby 3.2 → Blocker (RemovedIn set),
	// with the File.exist?/Dir.exist? guidance.
	cases := []struct {
		name    string
		message string
		wantPat string // token that must appear in the ReplacementPattern
	}{
		{"File.exists?", "`File.exists?` is deprecated in favor of `File.exist?`.", "File.exist?"},
		{"Dir.exists?", "`Dir.exists?` is deprecated in favor of `Dir.exist?`.", "Dir.exist?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := LookupCopForOffense("Lint/DeprecatedClassMethods", tc.message)
			if m == nil {
				t.Fatal("expected non-nil variant mapping")
			}
			if m.CopName != "Lint/DeprecatedClassMethods" {
				t.Errorf("CopName = %q, want the base cop name", m.CopName)
			}
			if m.RemovedIn != "19.0" {
				t.Errorf("RemovedIn = %q, want 19.0 (removed → Blocker)", m.RemovedIn)
			}
			if !strings.Contains(m.ReplacementPattern, tc.wantPat) {
				t.Errorf("ReplacementPattern %q does not mention %q", m.ReplacementPattern, tc.wantPat)
			}
			if !strings.Contains(m.Description, tc.name) {
				t.Errorf("Description %q does not mention the deprecated method %q", m.Description, tc.name)
			}
		})
	}
}

func TestLookupCopForOffense_DeprecatedClassMethods_DeprecationOnly(t *testing.T) {
	// Socket.gethostby* are deprecation-only (not removed) → no RemovedIn (Review),
	// with Addrinfo guidance, NOT File.exist?.
	cases := []struct {
		name    string
		message string
	}{
		{"Socket.gethostbyname", "`Socket.gethostbyname` is deprecated in favor of `Addrinfo.getaddrinfo`."},
		{"Socket.gethostbyaddr", "`Socket.gethostbyaddr` is deprecated in favor of `Addrinfo#getnameinfo`."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := LookupCopForOffense("Lint/DeprecatedClassMethods", tc.message)
			if m == nil {
				t.Fatal("expected non-nil variant mapping")
			}
			if m.RemovedIn != "" {
				t.Errorf("RemovedIn = %q, want empty (deprecation-only → Review)", m.RemovedIn)
			}
			if !strings.Contains(m.ReplacementPattern, "Addrinfo") {
				t.Errorf("ReplacementPattern %q does not mention Addrinfo", m.ReplacementPattern)
			}
			if strings.Contains(m.ReplacementPattern, "File.exist") {
				t.Errorf("ReplacementPattern %q wrongly carries the File.exist? guidance", m.ReplacementPattern)
			}
		})
	}
}

// Cookstyle messages spell the method with namespace qualifiers (::File.exists?,
// ::File::exists?, ::Socket.gethostbyname). Every spelling must resolve to the
// same variant as its bare form, not fall through to the base mapping.
func TestLookupCopForOffense_NamespaceQualifiedVariants(t *testing.T) {
	cases := []struct {
		name      string
		message   string
		wantRemIn string // "19.0" for a removed (Blocker) variant, "" for deprecation-only
		wantTok   string // grouping token, so all spellings of a method share one group
	}{
		{"bare File.exists?", "`File.exists?` is deprecated in favor of `File.exist?`.", "19.0", "File.exists?"},
		{"::File.exists?", "`::File.exists?` is deprecated in favor of `::File.exist?`.", "19.0", "File.exists?"},
		{"::File::exists?", "`::File::exists?` is deprecated in favor of `::File.exist?`.", "19.0", "File.exists?"},
		{"bare Socket.gethostbyname", "`Socket.gethostbyname` is deprecated in favor of `Addrinfo.getaddrinfo`.", "", "Socket.gethostbyname"},
		{"::Socket.gethostbyname", "`::Socket.gethostbyname` is deprecated in favor of `Addrinfo.getaddrinfo`.", "", "Socket.gethostbyname"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := LookupCopForOffense("Lint/DeprecatedClassMethods", tc.message)
			if m == nil {
				t.Fatal("expected a variant mapping, got nil (fell through to base?)")
			}
			if m.RemovedIn != tc.wantRemIn {
				t.Errorf("RemovedIn = %q, want %q", m.RemovedIn, tc.wantRemIn)
			}
			if tok := OffenseVariantToken("Lint/DeprecatedClassMethods", tc.message); tok != tc.wantTok {
				t.Errorf("group token = %q, want %q (all spellings of a method must share one group)", tok, tc.wantTok)
			}
		})
	}
}

// The full DeprecatedClassMethods surface (from the cop's PREFERRED_METHODS
// table) on CC19.3.15: ENV.clone/dup/freeze raise TypeError (Blocker);
// iterator?/attr are still present (Review). Guidance per cop.
func TestLookupCopForOffense_DeprecatedClassMethods_EnvAndKernel(t *testing.T) {
	cases := []struct {
		name      string
		message   string
		wantRemIn string
		wantGuide string
	}{
		{"ENV.clone", "`ENV.clone` is deprecated in favor of `ENV.to_h`.", "19.0", "ENV.to_h"},
		{"ENV.dup", "`ENV.dup` is deprecated in favor of `ENV.to_h`.", "19.0", "ENV.to_h"},
		{"ENV.freeze", "`ENV.freeze` is deprecated in favor of `ENV`.", "19.0", "cannot be frozen"},
		{"iterator?", "`iterator?` is deprecated in favor of `block_given?`.", "", "block_given?"},
		{"attr", "`attr :x, true` is deprecated in favor of `attr_accessor :x`.", "", "attr_accessor"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := LookupCopForOffense("Lint/DeprecatedClassMethods", tc.message)
			if m == nil {
				t.Fatal("expected a variant mapping, got nil")
			}
			if m.RemovedIn != tc.wantRemIn {
				t.Errorf("RemovedIn = %q, want %q", m.RemovedIn, tc.wantRemIn)
			}
			if !strings.Contains(m.ReplacementPattern, tc.wantGuide) {
				t.Errorf("ReplacementPattern %q does not mention %q", m.ReplacementPattern, tc.wantGuide)
			}
		})
	}
}

func TestLookupCopForOffense_PolyCop_UnknownMessageFallsBackToBase(t *testing.T) {
	// A poly cop message matching no variant falls back to the cop-name mapping.
	m := LookupCopForOffense("Lint/DeprecatedClassMethods", "some unrecognised deprecation")
	base := LookupCop("Lint/DeprecatedClassMethods")
	if m == nil || base == nil {
		t.Fatal("expected non-nil base mapping")
	}
	if m.RemovedIn != base.RemovedIn || m.ReplacementPattern != base.ReplacementPattern {
		t.Errorf("unmatched poly message should return the base cop mapping")
	}
}

func TestLookupCopForOffense_PolyCop_EmptyMessageFallsBackToBase(t *testing.T) {
	m := LookupCopForOffense("Lint/DeprecatedClassMethods", "")
	base := LookupCop("Lint/DeprecatedClassMethods")
	if m == nil || base == nil {
		t.Fatal("expected non-nil base mapping")
	}
	if m.RemovedIn != base.RemovedIn {
		t.Errorf("empty message should return the base cop mapping (RemovedIn %q, want %q)", m.RemovedIn, base.RemovedIn)
	}
}

func TestLookupCopForOffense_NonPolyCop_MatchesLookupCop(t *testing.T) {
	// A cop with no variants resolves identically to LookupCop, regardless of message.
	msg := "node.set is deprecated"
	m := LookupCopForOffense("Chef/Deprecations/NodeSet", msg)
	base := LookupCop("Chef/Deprecations/NodeSet")
	if m == nil || base == nil {
		t.Fatal("expected non-nil mapping")
	}
	if m.CopName != base.CopName || m.RemovedIn != base.RemovedIn {
		t.Errorf("non-poly cop should match LookupCop; got %+v want %+v", m, base)
	}
}

func TestLookupCopForOffense_UnknownCop_Nil(t *testing.T) {
	if m := LookupCopForOffense("Chef/Deprecations/CompletelyFakeCop", "whatever"); m != nil {
		t.Errorf("expected nil for unknown cop, got %+v", m)
	}
}
