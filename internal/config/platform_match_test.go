// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestMatchPlatform_ExactMatch(t *testing.T) {
	entries := []PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204"},
		{KitchenName: "centos-7", Image: "centos-7"},
		{KitchenName: "windows-2022", Image: "win2022"},
	}

	r := MatchPlatform("centos-7", entries)
	if r.Index != 1 {
		t.Fatalf("expected Index 1, got %d", r.Index)
	}
	if r.Entry == nil || r.Entry.Image != "centos-7" {
		t.Fatalf("expected Image centos-7, got %v", r.Entry)
	}
}

func TestMatchPlatform_GlobStar(t *testing.T) {
	entries := []PlatformMapEntry{
		{KitchenName: "rhel*", Image: "rhel-generic", IsPattern: true},
	}

	t.Run("matches rhel7", func(t *testing.T) {
		r := MatchPlatform("rhel7", entries)
		if r.Index != 0 {
			t.Fatalf("expected Index 0, got %d", r.Index)
		}
	})

	t.Run("matches rhel8-chef16", func(t *testing.T) {
		r := MatchPlatform("rhel8-chef16", entries)
		if r.Index != 0 {
			t.Fatalf("expected Index 0, got %d", r.Index)
		}
	})
}

func TestMatchPlatform_GlobQuestion(t *testing.T) {
	entries := []PlatformMapEntry{
		{KitchenName: "rhel?", Image: "rhel-single", IsPattern: true},
	}

	t.Run("matches rhel7", func(t *testing.T) {
		r := MatchPlatform("rhel7", entries)
		if r.Index != 0 {
			t.Fatalf("expected Index 0, got %d", r.Index)
		}
	})

	t.Run("does not match rhel10", func(t *testing.T) {
		r := MatchPlatform("rhel10", entries)
		if r.Index != -1 {
			t.Fatalf("expected no match, got Index %d", r.Index)
		}
	})
}

func TestMatchPlatform_FirstMatchWins(t *testing.T) {
	entries := []PlatformMapEntry{
		{KitchenName: "rhel*", Image: "first", IsPattern: true},
		{KitchenName: "rhel?", Image: "second", IsPattern: true},
	}

	r := MatchPlatform("rhel7", entries)
	if r.Index != 0 {
		t.Fatalf("expected Index 0 (first pattern), got %d", r.Index)
	}
	if r.Entry.Image != "first" {
		t.Fatalf("expected Image first, got %s", r.Entry.Image)
	}
}

func TestMatchPlatform_ExactPriorityOverPattern(t *testing.T) {
	entries := []PlatformMapEntry{
		{KitchenName: "rhel*", Image: "pattern-match", IsPattern: true},
		{KitchenName: "rhel7", Image: "exact-match"},
	}

	r := MatchPlatform("rhel7", entries)
	if r.Index != 1 {
		t.Fatalf("expected Index 1 (exact), got %d", r.Index)
	}
	if r.Entry.Image != "exact-match" {
		t.Fatalf("expected Image exact-match, got %s", r.Entry.Image)
	}
}

func TestMatchPlatform_SkipEntry(t *testing.T) {
	entries := []PlatformMapEntry{
		{KitchenName: "deprecated-os", Image: "", Skip: true},
	}

	r := MatchPlatform("deprecated-os", entries)
	if r.Index != 0 {
		t.Fatalf("expected Index 0, got %d", r.Index)
	}
	if r.Entry == nil || !r.Entry.Skip {
		t.Fatalf("expected Skip=true entry, got %v", r.Entry)
	}
}

func TestMatchPlatform_NoMatch(t *testing.T) {
	entries := []PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204"},
		{KitchenName: "centos*", Image: "centos-generic", IsPattern: true},
	}

	r := MatchPlatform("windows-2022", entries)
	if r.Index != -1 {
		t.Fatalf("expected Index -1, got %d", r.Index)
	}
	if r.Entry != nil {
		t.Fatalf("expected nil Entry, got %v", r.Entry)
	}
}

func TestMatchPlatform_EmptyName(t *testing.T) {
	entries := []PlatformMapEntry{
		{KitchenName: "ubuntu-22.04", Image: "ubuntu-2204"},
	}

	r := MatchPlatform("", entries)
	if r.Index != -1 {
		t.Fatalf("expected Index -1, got %d", r.Index)
	}
	if r.Entry != nil {
		t.Fatalf("expected nil Entry, got %v", r.Entry)
	}
}

func TestMatchPlatform_EmptyEntries(t *testing.T) {
	t.Run("nil slice", func(t *testing.T) {
		r := MatchPlatform("anything", nil)
		if r.Index != -1 {
			t.Fatalf("expected Index -1, got %d", r.Index)
		}
		if r.Entry != nil {
			t.Fatalf("expected nil Entry, got %v", r.Entry)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		r := MatchPlatform("anything", []PlatformMapEntry{})
		if r.Index != -1 {
			t.Fatalf("expected Index -1, got %d", r.Index)
		}
	})
}

func TestMatchPlatform_PatternWithImage(t *testing.T) {
	transport := &PlatformMapTransport{
		Username:           "admin",
		PasswordCredential: "win-cred",
	}
	entries := []PlatformMapEntry{
		{KitchenName: "windows-*", Image: "win-base", IsPattern: true, Transport: transport},
	}

	r := MatchPlatform("windows-2022", entries)
	if r.Index != 0 {
		t.Fatalf("expected Index 0, got %d", r.Index)
	}
	if r.Entry.Image != "win-base" {
		t.Fatalf("expected Image win-base, got %s", r.Entry.Image)
	}
	if r.Entry.Transport == nil {
		t.Fatal("expected non-nil Transport")
	}
	if r.Entry.Transport.Username != "admin" {
		t.Fatalf("expected Username admin, got %s", r.Entry.Transport.Username)
	}
	if r.Entry.Transport.PasswordCredential != "win-cred" {
		t.Fatalf("expected PasswordCredential win-cred, got %s", r.Entry.Transport.PasswordCredential)
	}
}

func TestMatchPlatform_MixedExactAndPattern(t *testing.T) {
	entries := []PlatformMapEntry{
		{KitchenName: "centos*", Image: "centos-pattern", IsPattern: true},
		{KitchenName: "rhel*", Image: "rhel-pattern", IsPattern: true},
		{KitchenName: "centos-7", Image: "centos-7-exact"},
	}

	t.Run("exact wins over earlier pattern", func(t *testing.T) {
		r := MatchPlatform("centos-7", entries)
		if r.Index != 2 {
			t.Fatalf("expected Index 2 (exact), got %d", r.Index)
		}
		if r.Entry.Image != "centos-7-exact" {
			t.Fatalf("expected Image centos-7-exact, got %s", r.Entry.Image)
		}
	})

	t.Run("pattern still works for non-exact", func(t *testing.T) {
		r := MatchPlatform("centos-8", entries)
		if r.Index != 0 {
			t.Fatalf("expected Index 0 (pattern), got %d", r.Index)
		}
		if r.Entry.Image != "centos-pattern" {
			t.Fatalf("expected Image centos-pattern, got %s", r.Entry.Image)
		}
	})
}
