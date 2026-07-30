// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// These tests read the captured cookstyle output in testdata/ rather than
// hand-authored JSON. Fabricated fixtures are how the original bug survived CI:
// tests invented a "correctable" key the pipeline never wrote, so decoding it
// appeared to work.

func loadCookstyleFixture(t *testing.T, name string) CookstyleOutput {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	var out CookstyleOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshalling fixture %s: %v", name, err)
	}
	return out
}

func fixtureOffenses(t *testing.T, name string) []CookstyleOffense {
	t.Helper()
	out := loadCookstyleFixture(t, name)
	var offs []CookstyleOffense
	for _, f := range out.Files {
		for _, o := range f.Offenses {
			o.File = f.Path
			offs = append(offs, o)
		}
	}
	return offs
}

// The scan struct must decode `correctable`. Before this fix it declared only
// `corrected`, so the capability was dropped at the very first hop.
func TestCookstyleOffense_DecodesCorrectable(t *testing.T) {
	offs := fixtureOffenses(t, "cookstyle_scan_mixed_plain.json")
	if len(offs) != 6 {
		t.Fatalf("expected 6 offences in the fixture, got %d", len(offs))
	}

	var correctable, corrected int
	for _, o := range offs {
		if o.Correctable {
			correctable++
		}
		if o.Corrected {
			corrected++
		}
	}
	if correctable != 3 {
		t.Errorf("correctable = %d, want 3", correctable)
	}
	// A plain scan has corrected nothing.
	if corrected != 0 {
		t.Errorf("corrected = %d on a plain scan, want 0", corrected)
	}
}

// correctable and corrected are independent. A correcting run flips corrected
// while correctable is unchanged — so neither can be derived from the other.
func TestCookstyleOffense_CorrectableAndCorrectedAreIndependent(t *testing.T) {
	before := fixtureOffenses(t, "cookstyle_scan_mixed_plain.json")
	after := fixtureOffenses(t, "cookstyle_scan_mixed_autocorrected.json")

	countCorrectable := func(offs []CookstyleOffense) (c int) {
		for _, o := range offs {
			if o.Correctable {
				c++
			}
		}
		return
	}
	countCorrected := func(offs []CookstyleOffense) (c int) {
		for _, o := range offs {
			if o.Corrected {
				c++
			}
		}
		return
	}

	if got, want := countCorrectable(before), countCorrectable(after); got != want {
		t.Errorf("correctable changed across a correcting run: %d then %d", got, want)
	}
	if countCorrected(before) != 0 || countCorrected(after) != 3 {
		t.Errorf("corrected should go 0 -> 3, got %d -> %d",
			countCorrected(before), countCorrected(after))
	}
}

// A cop can be correctable in principle yet left alone by `--auto-correct`,
// which is what CMM runs. Deriving "what we will fix" from the static flag
// therefore overstates it.
func TestCookstyleOffense_UnsafeCorrectionsAreCorrectableButNotCorrected(t *testing.T) {
	offs := fixtureOffenses(t, "cookstyle_scan_unsafe_correctable.json")
	if len(offs) == 0 {
		t.Fatal("fixture has no offences")
	}
	for _, o := range offs {
		if !o.Correctable {
			t.Errorf("%s: expected correctable=true", o.CopName)
		}
		if o.Corrected {
			t.Errorf("%s: --auto-correct must not have corrected an unsafe cop", o.CopName)
		}
	}
}

func TestCookstyleOffense_NonCorrectableDecodesFalse(t *testing.T) {
	offs := fixtureOffenses(t, "cookstyle_scan_noncorrectable.json")
	if len(offs) == 0 {
		t.Fatal("fixture has no offences")
	}
	for _, o := range offs {
		if o.Correctable {
			t.Errorf("%s: expected correctable=false", o.CopName)
		}
	}
}

// The persisted shape is where the bug lived: enrichOffenses dropped the field
// entirely, so every read-side handler decoded a key that was never written.
// This asserts the marshalled bytes, not a round-trip through the same struct —
// a round-trip cannot detect a field that is missing from both sides.
func TestEnrichOffenses_PersistsCorrectableInMarshalledJSON(t *testing.T) {
	offs := fixtureOffenses(t, "cookstyle_scan_mixed_plain.json")

	raw, err := json.Marshal(enrichOffenses(offs))
	if err != nil {
		t.Fatalf("marshalling enriched offences: %v", err)
	}

	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if len(decoded) != len(offs) {
		t.Fatalf("got %d enriched offences, want %d", len(decoded), len(offs))
	}

	var correctable int
	for i, m := range decoded {
		v, present := m["correctable"]
		if !present {
			t.Fatalf("offence %d: no \"correctable\" key in the persisted JSON", i)
		}
		if b, _ := v.(bool); b {
			correctable++
		}
	}
	if correctable != 3 {
		t.Errorf("persisted correctable count = %d, want 3", correctable)
	}
}

// Custom cops are constructed in Go and never appear in cookstyle's output, so
// nothing can auto-correct them. Leaving the zero value implicit would make this
// true by accident; pin it, because the preview's arithmetic depends on custom
// cop offences never being counted as correctable.
func TestCustomCopOffenses_AreNotCorrectable(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "default.rb"), []byte("package 'forbidden'\n"), 0o600); err != nil {
		t.Fatalf("writing probe recipe: %v", err)
	}

	offenses := ScanCustomCops(dir, []datastore.CustomCopDefinition{{
		CopName:        "Custom/NoForbiddenPackage",
		Description:    "forbidden package",
		PatternType:    "regex",
		Pattern:        `forbidden`,
		FileGlob:       "*.rb",
		Classification: "review",
		Enabled:        true,
	}})

	if len(offenses) == 0 {
		t.Fatal("expected the custom cop to match the probe recipe")
	}
	for _, off := range offenses {
		if off.Correctable {
			t.Errorf("custom cop %s must not be marked correctable", off.CopName)
		}
	}
}
