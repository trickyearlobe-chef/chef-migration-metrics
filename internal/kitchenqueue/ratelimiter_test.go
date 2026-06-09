// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package kitchenqueue_test

import (
	"context"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/kitchenqueue"
)

// virtualClock drives the limiter deterministically: Now() returns the current
// virtual time and sleep() advances it instead of blocking, so a sequence of
// Wait() calls executes instantly while still exercising the real Wait logic.
type virtualClock struct {
	now time.Time
}

func (c *virtualClock) Now() time.Time { return c.now }

func (c *virtualClock) sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d > 0 {
		c.now = c.now.Add(d)
	}
	return nil
}

// fixedCfg returns a config accessor with constant values.
func fixedCfg(window time.Duration, max int) kitchenqueue.RateLimiterConfigFn {
	return func() (time.Duration, int) { return window, max }
}

// drive runs n Wait() calls and returns the virtual start time recorded for each.
func drive(t *testing.T, rl *kitchenqueue.RateLimiter, clk *virtualClock, n int) []time.Time {
	t.Helper()
	out := make([]time.Time, 0, n)
	for i := range n {
		if err := rl.Wait(context.Background()); err != nil {
			t.Fatalf("Wait %d returned error: %v", i, err)
		}
		out = append(out, clk.Now())
	}
	return out
}

// maxInTrailingWindow returns the largest number of starts that fall within any
// trailing half-open window of length w ending at one of the start times.
func maxInTrailingWindow(starts []time.Time, w time.Duration) int {
	worst := 0
	for _, end := range starts {
		lo := end.Add(-w)
		count := 0
		for _, s := range starts {
			// half-open (end-w, end]
			if s.After(lo) && !s.After(end) {
				count++
			}
		}
		if count > worst {
			worst = count
		}
	}
	return worst
}

func TestRateLimiter_PacesEvenly(t *testing.T) {
	clk := &virtualClock{now: time.Unix(0, 0).UTC()}
	window := 60 * time.Minute
	max := 4
	rl := kitchenqueue.NewRateLimiter(fixedCfg(window, max),
		kitchenqueue.WithNowFunc(clk.Now),
		kitchenqueue.WithSleepFunc(clk.sleep))

	starts := drive(t, rl, clk, 12)

	gap := window / time.Duration(max) // 15m
	for i := 1; i < len(starts); i++ {
		got := starts[i].Sub(starts[i-1])
		if got < gap {
			t.Fatalf("start %d too close: gap %v < min %v", i, got, gap)
		}
	}
}

func TestRateLimiter_NeverExceedsMaxInTrailingWindow(t *testing.T) {
	clk := &virtualClock{now: time.Unix(0, 0).UTC()}
	window := 90 * time.Minute
	max := 5
	rl := kitchenqueue.NewRateLimiter(fixedCfg(window, max),
		kitchenqueue.WithNowFunc(clk.Now),
		kitchenqueue.WithSleepFunc(clk.sleep))

	starts := drive(t, rl, clk, 25)

	if got := maxInTrailingWindow(starts, window); got > max {
		t.Fatalf("trailing-window starts %d exceeds max %d", got, max)
	}
}

// TestRateLimiter_WindowBackstopAfterMaxReduction proves the sliding-window
// constraint (not just pacing) holds when max is lowered mid-run: a burst
// admitted under a high max must wait for enough of it to age out of the
// window before another start is allowed under the lower max.
func TestRateLimiter_WindowBackstopAfterMaxReduction(t *testing.T) {
	clk := &virtualClock{now: time.Unix(0, 0).UTC()}
	window := 60 * time.Minute
	max := 100 // tiny pacing gap (0.6m) so the burst lands close together
	cfg := func() (time.Duration, int) { return window, max }
	rl := kitchenqueue.NewRateLimiter(cfg,
		kitchenqueue.WithNowFunc(clk.Now),
		kitchenqueue.WithSleepFunc(clk.sleep))

	burst := drive(t, rl, clk, 5)

	// Tighten to a small pool. The next start must wait until only max-1 of the
	// burst remain inside the trailing window.
	max = 3
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait after reduction: %v", err)
	}
	next := clk.Now()

	// With 5 in the burst and max now 3, start at index (5-3)=2 must exit the
	// window: next allowed = burst[2] + window.
	wantMin := burst[2].Add(window)
	if next.Before(wantMin) {
		t.Fatalf("next start %v before window backstop %v", next, wantMin)
	}

	// The guarantee is forward-looking: the limiter cannot un-start the burst
	// admitted under the old max, but the new start must not push the window
	// ending at its own admission time above the current max.
	all := append(append([]time.Time{}, burst...), next)
	lo := next.Add(-window)
	count := 0
	for _, s := range all {
		if s.After(lo) && !s.After(next) {
			count++
		}
	}
	if count > 3 {
		t.Fatalf("new start admitted with %d in its trailing window, exceeds max 3", count)
	}
}

func TestRateLimiter_MaxIncreaseTakesEffect(t *testing.T) {
	clk := &virtualClock{now: time.Unix(0, 0).UTC()}
	window := 60 * time.Minute
	max := 2 // gap 30m
	cfg := func() (time.Duration, int) { return window, max }
	rl := kitchenqueue.NewRateLimiter(cfg,
		kitchenqueue.WithNowFunc(clk.Now),
		kitchenqueue.WithSleepFunc(clk.sleep))

	first := drive(t, rl, clk, 1)[0]

	// Raise the cap: the pacing gap shrinks to 6m, so the next start should be
	// admitted only 6m later, not 30m.
	max = 10
	if err := rl.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	got := clk.Now().Sub(first)
	if want := 6 * time.Minute; got != want {
		t.Fatalf("after max increase, gap %v != %v", got, want)
	}
}

func TestRateLimiter_DisabledAllowsImmediately(t *testing.T) {
	clk := &virtualClock{now: time.Unix(0, 0).UTC()}
	rl := kitchenqueue.NewRateLimiter(fixedCfg(0, 0),
		kitchenqueue.WithNowFunc(clk.Now),
		kitchenqueue.WithSleepFunc(clk.sleep))

	starts := drive(t, rl, clk, 50)
	for i, s := range starts {
		if !s.Equal(clk.Now()) {
			t.Errorf("start %d admitted at %v, want %v (disabled limiter must not pace)", i, s, clk.Now())
		}
	}
	if !clk.Now().Equal(time.Unix(0, 0).UTC()) {
		t.Fatalf("disabled limiter advanced the clock to %v", clk.Now())
	}
}

func TestRateLimiter_ContextCancelled(t *testing.T) {
	clk := &virtualClock{now: time.Unix(0, 0).UTC()}
	rl := kitchenqueue.NewRateLimiter(fixedCfg(60*time.Minute, 1),
		kitchenqueue.WithNowFunc(clk.Now),
		kitchenqueue.WithSleepFunc(clk.sleep))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := rl.Wait(ctx); err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}
