// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package remediation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The preview's counts must come from the correcting run's own per-offence
// flags. Subtracting the correcting run's summary.offense_count from the scan's
// is always ~0, because a correcting run does not shrink its own offense_count,
// and every cookbook then reads as "0 correctable" beside a diff showing real
// changes.
//
// Fixtures are real cookstyle output; see internal/analysis/testdata/README.md.

func loadAutocorrectFixture(t *testing.T, name string) autocorrectJSONOutput {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "analysis", "testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	var out autocorrectJSONOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshalling fixture %s: %v", name, err)
	}
	return out
}

// The parser must retain per-offence flags. Parsing only `summary` is what made
// the correct counts unavailable in the first place.
func TestAutocorrectJSONOutput_ParsesPerOffenceFlags(t *testing.T) {
	out := loadAutocorrectFixture(t, "cookstyle_scan_mixed_autocorrected.json")

	if out.Summary.OffenseCount != 6 {
		t.Errorf("summary.offense_count = %d, want 6", out.Summary.OffenseCount)
	}
	if got := out.correctedCount(); got != 3 {
		t.Errorf("corrected count = %d, want 3", got)
	}
}

// The headline arithmetic, against the deployed toolchain's real output.
func TestAutocorrectCounts_FromCorrectingRun(t *testing.T) {
	out := loadAutocorrectFixture(t, "cookstyle_scan_mixed_autocorrected.json")

	corrected := out.correctedCount()
	remaining := out.Summary.OffenseCount - corrected

	if corrected != 3 {
		t.Errorf("correctable = %d, want 3", corrected)
	}
	if remaining != 3 {
		t.Errorf("remaining = %d, want 3", remaining)
	}
}

// CMM runs --auto-correct, which leaves unsafe corrections alone. Those
// offences are correctable=true but corrected=false, and must NOT be counted as
// fixed — the diff would not contain them.
func TestAutocorrectCounts_UnsafeCorrectionsCountAsRemaining(t *testing.T) {
	out := loadAutocorrectFixture(t, "cookstyle_scan_unsafe_correctable.json")

	corrected := out.correctedCount()
	remaining := out.Summary.OffenseCount - corrected

	if corrected != 0 {
		t.Errorf("corrected = %d, want 0 — --auto-correct fixes no unsafe cop", corrected)
	}
	if remaining != out.Summary.OffenseCount {
		t.Errorf("remaining = %d, want %d (all of them)", remaining, out.Summary.OffenseCount)
	}
}

// A plain scan corrected nothing, so the count must be zero rather than
// inferred from the static correctable flag.
func TestAutocorrectCounts_PlainScanCorrectsNothing(t *testing.T) {
	out := loadAutocorrectFixture(t, "cookstyle_scan_mixed_plain.json")
	if got := out.correctedCount(); got != 0 {
		t.Errorf("corrected = %d on a plain scan, want 0", got)
	}
}

// Guard on the arithmetic that must not be reinstated: subtracting the
// correcting run's summary from the scan's total yields zero, because the count
// does not shrink.
func TestAutocorrectCounts_SummarySubtractionIsAlwaysZero(t *testing.T) {
	plain := loadAutocorrectFixture(t, "cookstyle_scan_mixed_plain.json")
	after := loadAutocorrectFixture(t, "cookstyle_scan_mixed_autocorrected.json")

	if got := plain.Summary.OffenseCount - after.Summary.OffenseCount; got != 0 {
		t.Errorf("summary subtraction = %d; the fixtures no longer demonstrate the bug", got)
	}
}
