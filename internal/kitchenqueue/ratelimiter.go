// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package kitchenqueue

import (
	"context"
	"sync"
	"time"
)

// RateLimiterConfigFn returns the live limiter parameters, read fresh on every
// call so on-site tuning takes effect with no restart:
//
//   - window        — the DHCP lease time (e.g. 60m). A lease is assumed held
//     for this full duration regardless of when the VM finishes.
//   - maxPerWindow  — the usable IP pool size (e.g. 25).
//
// A window <= 0 or maxPerWindow < 1 disables the limiter: starts pass through
// immediately with no pacing.
type RateLimiterConfigFn func() (window time.Duration, maxPerWindow int)

// RateLimiter gates VM starts so that, in any trailing window, no more than
// maxPerWindow starts occur, and starts are evenly paced (minimum inter-start
// gap ~= window/maxPerWindow). It counts starts and charges each against the
// window for the full duration regardless of whether the VM finished or
// released its lease early — a hard worst-case guarantee against DHCP pool
// exhaustion that holds even if IP release fails.
//
// The limiter is global: a single window/max governs all VM starts.
type RateLimiter struct {
	cfgFn   RateLimiterConfigFn
	nowFn   func() time.Time
	sleepFn func(context.Context, time.Duration) error

	mu     sync.Mutex
	starts []time.Time // recorded start times, ascending, pruned to the window
}

// RateLimiterOption configures a RateLimiter (test seams for the clock).
type RateLimiterOption func(*RateLimiter)

// WithNowFunc overrides the time source (tests).
func WithNowFunc(fn func() time.Time) RateLimiterOption {
	return func(rl *RateLimiter) {
		if fn != nil {
			rl.nowFn = fn
		}
	}
}

// WithSleepFunc overrides the wait primitive (tests). It must return promptly
// with ctx.Err() if ctx is cancelled.
func WithSleepFunc(fn func(context.Context, time.Duration) error) RateLimiterOption {
	return func(rl *RateLimiter) {
		if fn != nil {
			rl.sleepFn = fn
		}
	}
}

// NewRateLimiter builds a limiter reading parameters via cfgFn.
func NewRateLimiter(cfgFn RateLimiterConfigFn, opts ...RateLimiterOption) *RateLimiter {
	rl := &RateLimiter{
		cfgFn:   cfgFn,
		nowFn:   time.Now,
		sleepFn: realSleep,
	}
	for _, opt := range opts {
		opt(rl)
	}
	return rl
}

// Wait blocks until a VM start is permitted, then records the start. It returns
// ctx.Err() if the context is cancelled before a slot is granted.
//
// The lock is held for the whole call: this serialises the decision-and-record
// step so concurrent workers cannot both observe a free slot and start
// together, which is exactly the pacing the limiter exists to enforce.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		window, max := rl.cfgFn()
		if window <= 0 || max < 1 {
			// Limiter disabled — pass through without recording (nothing to bound).
			return nil
		}

		now := rl.nowFn()
		rl.prune(now, window)

		next := rl.earliest(now, window, max)
		if !next.After(now) {
			rl.starts = append(rl.starts, now)
			return nil
		}

		if err := rl.sleepFn(ctx, next.Sub(now)); err != nil {
			return err
		}
	}
}

// prune drops starts that have aged out of the trailing window (start time at
// or before now-window no longer consumes a lease for our worst-case bound).
func (rl *RateLimiter) prune(now time.Time, window time.Duration) {
	cutoff := now.Add(-window)
	i := 0
	for i < len(rl.starts) && !rl.starts[i].After(cutoff) {
		i++
	}
	if i > 0 {
		rl.starts = rl.starts[i:]
	}
}

// earliest returns the earliest time a new start is permitted, as the later of
// two constraints:
//
//   - window: if max or more starts are still in the window, the start at index
//     len-max must age out first (handles a lowered max leaving an over-full
//     window).
//   - pacing: at least window/max after the most recent start.
func (rl *RateLimiter) earliest(now time.Time, window time.Duration, max int) time.Time {
	next := now

	if n := len(rl.starts); n >= max {
		windowClear := rl.starts[n-max].Add(window)
		if windowClear.After(next) {
			next = windowClear
		}
	}

	if n := len(rl.starts); n > 0 {
		gap := window / time.Duration(max)
		paced := rl.starts[n-1].Add(gap)
		if paced.After(next) {
			next = paced
		}
	}

	return next
}

// realSleep is the production wait: ctx-aware, no-op for non-positive d.
func realSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
