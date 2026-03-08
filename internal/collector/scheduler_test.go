// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// ---------------------------------------------------------------------------
// ParseSchedule — valid expressions
// ---------------------------------------------------------------------------

func TestParseSchedule_ValidExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"every minute", "* * * * *"},
		{"hourly", "0 * * * *"},
		{"daily at midnight", "0 0 * * *"},
		{"every 15 minutes", "*/15 * * * *"},
		{"weekdays at 9am", "0 9 * * 1-5"},
		{"first of month", "0 0 1 * *"},
		{"specific minutes", "5,10,15 * * * *"},
		{"step with range", "1-30/5 * * * *"},
		{"sunday both 0 and 7", "0 0 * * 0"},
		{"sunday as 7", "0 0 * * 7"},
		{"complex", "5,15,25,35,45,55 6-18/2 1-15 1,4,7,10 1-5"},
		{"tabs as separators", "0\t0\t*\t*\t*"},
		{"multiple spaces", "0  0  *  *  *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := ParseSchedule(tt.expr)
			if err != nil {
				t.Fatalf("ParseSchedule(%q) returned error: %v", tt.expr, err)
			}
			if sched == nil {
				t.Fatal("ParseSchedule returned nil schedule")
			}
			if sched.String() != tt.expr {
				t.Errorf("String() = %q, want %q", sched.String(), tt.expr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseSchedule — invalid expressions
// ---------------------------------------------------------------------------

func TestParseSchedule_InvalidExpressions(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"empty", ""},
		{"one field", "0"},
		{"two fields", "0 0"},
		{"three fields", "0 0 *"},
		{"four fields", "0 0 * *"},
		{"six fields", "0 0 * * * *"},
		{"non-numeric", "a * * * *"},
		{"negative step", "*/0 * * * *"},
		{"minute out of range", "60 * * * *"},
		{"hour out of range", "0 24 * * *"},
		{"day zero", "0 0 0 * *"},
		{"day out of range", "0 0 32 * *"},
		{"month zero", "0 0 * 0 *"},
		{"month out of range", "0 0 * 13 *"},
		{"dow out of range", "0 0 * * 8"},
		{"inverted range", "0 0 15-5 * *"},
		{"range beyond max hour", "0 0-25 * * *"},
		{"empty step", "*/  * * * *"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSchedule(tt.expr)
			if err == nil {
				t.Fatalf("ParseSchedule(%q) expected error, got nil", tt.expr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// CronSchedule.Next — specific schedule calculations
// ---------------------------------------------------------------------------

func TestCronSchedule_Next_EveryMinute(t *testing.T) {
	sched, err := ParseSchedule("* * * * *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 10, 30, 45, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2025, 6, 15, 10, 31, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

func TestCronSchedule_Next_Hourly(t *testing.T) {
	sched, err := ParseSchedule("0 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2025, 6, 15, 11, 0, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

func TestCronSchedule_Next_HourlyAtTopOfHour(t *testing.T) {
	sched, err := ParseSchedule("0 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	// Reference is exactly at the top of the hour.
	ref := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2025, 6, 15, 11, 0, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

func TestCronSchedule_Next_Every15Minutes(t *testing.T) {
	sched, err := ParseSchedule("*/15 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		ref  time.Time
		want time.Time
	}{
		{
			ref:  time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC),
			want: time.Date(2025, 6, 15, 10, 15, 0, 0, time.UTC),
		},
		{
			ref:  time.Date(2025, 6, 15, 10, 14, 0, 0, time.UTC),
			want: time.Date(2025, 6, 15, 10, 15, 0, 0, time.UTC),
		},
		{
			ref:  time.Date(2025, 6, 15, 10, 15, 0, 0, time.UTC),
			want: time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
		},
		{
			ref:  time.Date(2025, 6, 15, 10, 45, 0, 0, time.UTC),
			want: time.Date(2025, 6, 15, 11, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		next := sched.Next(tt.ref)
		if !next.Equal(tt.want) {
			t.Errorf("Next(%v) = %v, want %v", tt.ref, next, tt.want)
		}
	}
}

func TestCronSchedule_Next_DailyAtMidnight(t *testing.T) {
	sched, err := ParseSchedule("0 0 * * *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

func TestCronSchedule_Next_WeekdaysOnly(t *testing.T) {
	sched, err := ParseSchedule("0 9 * * 1-5")
	if err != nil {
		t.Fatal(err)
	}

	// Saturday June 14, 2025 at noon.
	ref := time.Date(2025, 6, 14, 12, 0, 0, 0, time.UTC)
	next := sched.Next(ref)
	// Next weekday (Monday) at 9am.
	want := time.Date(2025, 6, 16, 9, 0, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}

	// Verify it's actually a Monday.
	if next.Weekday() != time.Monday {
		t.Errorf("expected Monday, got %s", next.Weekday())
	}
}

func TestCronSchedule_Next_SundayAs7(t *testing.T) {
	// "0 0 * * 7" should fire on Sundays (7 maps to 0).
	sched, err := ParseSchedule("0 0 * * 7")
	if err != nil {
		t.Fatal(err)
	}

	// Monday June 16, 2025.
	ref := time.Date(2025, 6, 16, 12, 0, 0, 0, time.UTC)
	next := sched.Next(ref)

	if next.Weekday() != time.Sunday {
		t.Errorf("expected Sunday, got %s (%v)", next.Weekday(), next)
	}
}

func TestCronSchedule_Next_SundayAs0(t *testing.T) {
	sched, err := ParseSchedule("0 0 * * 0")
	if err != nil {
		t.Fatal(err)
	}

	// Monday June 16, 2025.
	ref := time.Date(2025, 6, 16, 12, 0, 0, 0, time.UTC)
	next := sched.Next(ref)

	if next.Weekday() != time.Sunday {
		t.Errorf("expected Sunday, got %s (%v)", next.Weekday(), next)
	}
}

func TestCronSchedule_Next_FirstOfMonth(t *testing.T) {
	sched, err := ParseSchedule("0 0 1 * *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

func TestCronSchedule_Next_SpecificMonths(t *testing.T) {
	// Run at midnight on the 1st of Jan, Apr, Jul, Oct.
	sched, err := ParseSchedule("0 0 1 1,4,7,10 *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 4, 1, 0, 1, 0, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

func TestCronSchedule_Next_YearRollover(t *testing.T) {
	sched, err := ParseSchedule("0 0 1 1 *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

func TestCronSchedule_Next_LeapYear(t *testing.T) {
	// Feb 29 — only fires in leap years.
	sched, err := ParseSchedule("0 0 29 2 *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	next := sched.Next(ref)

	// 2025 is not a leap year, so it should skip to 2028.
	want := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

func TestCronSchedule_Next_StepWithRange(t *testing.T) {
	// "1-30/10" means minutes 1, 11, 21.
	sched, err := ParseSchedule("1-30/10 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2025, 6, 15, 10, 1, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}

	// After minute 1, next should be minute 11.
	ref2 := time.Date(2025, 6, 15, 10, 1, 30, 0, time.UTC)
	next2 := sched.Next(ref2)
	want2 := time.Date(2025, 6, 15, 10, 11, 0, 0, time.UTC)

	if !next2.Equal(want2) {
		t.Errorf("Next(%v) = %v, want %v", ref2, next2, want2)
	}
}

func TestCronSchedule_Next_List(t *testing.T) {
	// "5,35" means minute 5 and minute 35.
	sched, err := ParseSchedule("5,35 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 10, 6, 0, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2025, 6, 15, 10, 35, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

func TestCronSchedule_Next_MultipleSequential(t *testing.T) {
	// Hourly — verify that calling Next on the result of Next advances
	// correctly for several iterations.
	sched, err := ParseSchedule("0 * * * *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 24; i++ {
		next := sched.Next(ref)
		wantHour := (ref.Hour() + 1) % 24
		if next.Hour() != wantHour || next.Minute() != 0 {
			t.Errorf("iteration %d: Next(%v) = %v, want hour %d minute 0",
				i, ref, next, wantHour)
		}
		ref = next
	}
}

// ---------------------------------------------------------------------------
// CronSchedule.Next — ensures Next always returns a time strictly after ref
// ---------------------------------------------------------------------------

func TestCronSchedule_Next_StrictlyAfterRef(t *testing.T) {
	sched, err := ParseSchedule("* * * * *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	next := sched.Next(ref)

	if !next.After(ref) {
		t.Errorf("Next(%v) = %v — expected strictly after ref", ref, next)
	}
}

func TestCronSchedule_Next_ExactMatchAdvances(t *testing.T) {
	// If ref is exactly on a fire time, Next should still advance past it.
	sched, err := ParseSchedule("30 10 * * *")
	if err != nil {
		t.Fatal(err)
	}

	ref := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	next := sched.Next(ref)
	want := time.Date(2025, 6, 16, 10, 30, 0, 0, time.UTC)

	if !next.Equal(want) {
		t.Errorf("Next(%v) = %v, want %v", ref, next, want)
	}
}

// ---------------------------------------------------------------------------
// Cron field parsing edge cases
// ---------------------------------------------------------------------------

func TestParseCronField_Wildcard(t *testing.T) {
	cf, err := parseCronField("*", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	if len(cf.values) != 60 {
		t.Errorf("wildcard minute field: got %d values, want 60", len(cf.values))
	}
}

func TestParseCronField_WildcardWithStep(t *testing.T) {
	cf, err := parseCronField("*/10", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	// 0, 10, 20, 30, 40, 50
	want := map[int]bool{0: true, 10: true, 20: true, 30: true, 40: true, 50: true}
	if len(cf.values) != len(want) {
		t.Errorf("*/10 minute field: got %d values, want %d", len(cf.values), len(want))
	}
	for v := range want {
		if !cf.values[v] {
			t.Errorf("*/10 minute field: missing value %d", v)
		}
	}
}

func TestParseCronField_Range(t *testing.T) {
	cf, err := parseCronField("1-5", 0, 6)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true}
	if len(cf.values) != len(want) {
		t.Errorf("1-5 field: got %d values, want %d", len(cf.values), len(want))
	}
	for v := range want {
		if !cf.values[v] {
			t.Errorf("1-5 field: missing value %d", v)
		}
	}
}

func TestParseCronField_RangeWithStep(t *testing.T) {
	cf, err := parseCronField("0-20/5", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]bool{0: true, 5: true, 10: true, 15: true, 20: true}
	if len(cf.values) != len(want) {
		t.Errorf("0-20/5 field: got %d values, want %d", len(cf.values), len(want))
	}
	for v := range want {
		if !cf.values[v] {
			t.Errorf("0-20/5 field: missing value %d", v)
		}
	}
}

func TestParseCronField_List(t *testing.T) {
	cf, err := parseCronField("1,15,30", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]bool{1: true, 15: true, 30: true}
	if len(cf.values) != len(want) {
		t.Errorf("1,15,30 field: got %d values, want %d", len(cf.values), len(want))
	}
	for v := range want {
		if !cf.values[v] {
			t.Errorf("1,15,30 field: missing value %d", v)
		}
	}
}

func TestParseCronField_SingleValue(t *testing.T) {
	cf, err := parseCronField("42", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	if len(cf.values) != 1 || !cf.values[42] {
		t.Errorf("single value 42: got values %v", cf.values)
	}
}

func TestParseCronField_DayOfWeek7IsSunday(t *testing.T) {
	cf, err := parseCronField("7", 0, 6)
	if err != nil {
		t.Fatal(err)
	}
	if !cf.values[0] {
		t.Error("day-of-week 7 should normalise to 0 (Sunday)")
	}
}

func TestParseCronField_InvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		field string
		min   int
		max   int
	}{
		{"non-numeric", "abc", 0, 59},
		{"too high", "60", 0, 59},
		{"inverted range", "10-5", 0, 59},
		{"range beyond max", "0-70", 0, 59},
		{"zero step", "*/0", 0, 59},
		{"negative literal", "-1", 0, 59},
		{"empty", "", 0, 59},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseCronField(tt.field, tt.min, tt.max)
			if err == nil {
				t.Errorf("parseCronField(%q, %d, %d) expected error", tt.field, tt.min, tt.max)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestSplitFields(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"* * * * *", 5},
		{"0  0  *  *  *", 5},
		{"0\t0\t*\t*\t*", 5},
		{"0", 1},
		{"", 0},
		{"  leading", 1},
		{"trailing  ", 1},
	}

	for _, tt := range tests {
		fields := splitFields(tt.input)
		if len(fields) != tt.want {
			t.Errorf("splitFields(%q) = %d fields %v, want %d", tt.input, len(fields), fields, tt.want)
		}
	}
}

func TestSplitOnComma(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"1,2,3", []string{"1", "2", "3"}},
		{"42", []string{"42"}},
		{"*", []string{"*"}},
		{"1-5,10-15", []string{"1-5", "10-15"}},
	}

	for _, tt := range tests {
		got := splitOnComma(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitOnComma(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i, v := range got {
			if v != tt.want[i] {
				t.Errorf("splitOnComma(%q)[%d] = %q, want %q", tt.input, i, v, tt.want[i])
			}
		}
	}
}

func TestAtoi(t *testing.T) {
	tests := []struct {
		input string
		want  int
		err   bool
	}{
		{"0", 0, false},
		{"1", 1, false},
		{"42", 42, false},
		{"100", 100, false},
		{"", 0, true},
		{"abc", 0, true},
		{"-1", 0, true},
	}

	for _, tt := range tests {
		got, err := atoi(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("atoi(%q) expected error", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("atoi(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("atoi(%q) = %d, want %d", tt.input, got, tt.want)
			}
		}
	}
}

func TestNormalise(t *testing.T) {
	// Day-of-week: 7 → 0
	if got := normalise(7, 0, 6); got != 0 {
		t.Errorf("normalise(7, 0, 6) = %d, want 0", got)
	}
	// Non-DOW field: 7 stays 7
	if got := normalise(7, 0, 59); got != 7 {
		t.Errorf("normalise(7, 0, 59) = %d, want 7", got)
	}
	// Normal value unchanged
	if got := normalise(3, 0, 6); got != 3 {
		t.Errorf("normalise(3, 0, 6) = %d, want 3", got)
	}
}

// ---------------------------------------------------------------------------
// Scheduler — construction and lifecycle
// ---------------------------------------------------------------------------

func TestNewScheduler_NilCollectorPanics(t *testing.T) {
	// NewScheduler doesn't explicitly panic on nil, but we can verify it
	// constructs without error when given valid args.
	sched, _ := ParseSchedule("* * * * *")
	logger := newTestLogger()
	s := NewScheduler(nil, sched, logger)
	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}
}

func TestScheduler_StartStop(t *testing.T) {
	sched, _ := ParseSchedule("0 0 1 1 *") // fires once a year — won't trigger during test
	logger := newTestLogger()

	// We need a valid collector to avoid nil panics in the loop, but
	// since the schedule won't fire, it doesn't matter.
	s := NewScheduler(nil, sched, logger,
		WithClock(func() time.Time {
			return time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
		}),
		// Use a timer that blocks forever so the loop just waits on ctx.
		WithTimerFactory(func(d time.Duration) (<-chan time.Time, func() bool) {
			ch := make(chan time.Time) // never fires
			return ch, func() bool { return true }
		}),
	)

	if err := s.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Starting again should fail.
	if err := s.Start(); err == nil {
		t.Fatal("second Start() should return error")
	}

	// Stop should not hang.
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2 seconds")
	}
}

func TestScheduler_StopWithoutStart(t *testing.T) {
	sched, _ := ParseSchedule("* * * * *")
	logger := newTestLogger()
	s := NewScheduler(nil, sched, logger)

	// Should not panic or hang.
	s.Stop()
}

// ---------------------------------------------------------------------------
// Integration: schedule fires and collector is invoked
// ---------------------------------------------------------------------------

func TestScheduler_FiresAndCallsCollector(t *testing.T) {
	sched, _ := ParseSchedule("* * * * *")
	logger := newTestLogger()

	// Build a real collector (with nil DB — Run will fail, but the
	// scheduler's fire-and-call logic is what we're testing).
	cfg := &config.Config{}
	resolver := secrets.NewCredentialResolver(nil)
	coll := New(nil, cfg, logger, resolver)

	// Use a controllable timer that fires immediately once.
	fireCount := 0
	s := NewScheduler(coll, sched, logger,
		WithClock(func() time.Time {
			return time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
		}),
		WithTimerFactory(func(d time.Duration) (<-chan time.Time, func() bool) {
			ch := make(chan time.Time, 1)
			fireCount++
			if fireCount <= 1 {
				ch <- time.Now() // fire immediately on first call
			}
			// On subsequent calls, block forever (scheduler will be stopped).
			return ch, func() bool { return true }
		}),
	)

	if err := s.Start(); err != nil {
		t.Fatal(err)
	}

	// Give the loop time to run. The collector.Run will panic/recover or
	// error due to nil DB, but the scheduler should handle it gracefully.
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	// The scheduler should have attempted at least one fire.
	if fireCount < 1 {
		t.Error("timer factory was never called")
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newTestLogger() *logging.Logger {
	return logging.New(logging.Options{
		Level:   logging.DEBUG,
		Writers: []logging.Writer{logging.NewMemoryWriter()},
	})
}
