// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// tickStub is a thread-safe Hypervisor stub for ticker tests. It counts
// ListManagedVMs calls (one per sweep) and records destroyed IDs, so a
// test goroutine can observe the background ticker without a data race.
type tickStub struct {
	mu        sync.Mutex
	vms       []ManagedVM
	listCalls int
	destroyed []string
}

func (s *tickStub) ListTemplates(_ context.Context) ([]Template, error) { return nil, nil }

func (s *tickStub) ListManagedVMs(_ context.Context, _ string) ([]ManagedVM, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls++
	out := make([]ManagedVM, len(s.vms))
	copy(out, s.vms)
	return out, nil
}

func (s *tickStub) DestroyVM(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyed = append(s.destroyed, id)
	return nil
}

func (s *tickStub) Type() string { return "tick-stub" }

func (s *tickStub) sweepCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listCalls
}

func (s *tickStub) destroyCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.destroyed)
}

// oldVM returns a managed VM whose embedded timestamp is well past any
// realistic age threshold.
func oldVM(id string) ManagedVM {
	ts := time.Now().Add(-2 * time.Hour).Unix()
	return ManagedVM{HypervisorID: id, Name: fmt.Sprintf("cmm-cookbook-suite-ubuntu-%d", ts)}
}

// waitFor polls cond until it is true or the deadline elapses.
func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

// withFastRecheck shrinks the disabled-state recheck cadence for the test
// duration so disabled→enabled transitions are observable quickly.
func withFastRecheck(t *testing.T) {
	t.Helper()
	orig := sweepDisabledRecheck
	sweepDisabledRecheck = 5 * time.Millisecond
	t.Cleanup(func() { sweepDisabledRecheck = orig })
}

func TestSweepTicker_DisabledNeverSweeps(t *testing.T) {
	withFastRecheck(t)
	stub := &tickStub{vms: []ManagedVM{oldVM("vm-1")}}

	stop := StartSweepTicker(
		func() SweepParams { return SweepParams{Prefix: "cmm", Age: time.Hour, Interval: 0} },
		func(context.Context) (Hypervisor, error) { return stub, nil },
		nil,
	)
	defer stop()

	time.Sleep(40 * time.Millisecond)
	if n := stub.sweepCount(); n != 0 {
		t.Errorf("disabled ticker swept %d times, want 0", n)
	}
}

func TestSweepTicker_EnabledSweepsAndDestroys(t *testing.T) {
	withFastRecheck(t)
	stub := &tickStub{vms: []ManagedVM{oldVM("vm-1"), oldVM("vm-2")}}

	stop := StartSweepTicker(
		func() SweepParams { return SweepParams{Prefix: "cmm", Age: time.Hour, Interval: 5 * time.Millisecond} },
		func(context.Context) (Hypervisor, error) { return stub, nil },
		nil,
	)
	defer stop()

	if !waitFor(t, time.Second, func() bool { return stub.destroyCount() >= 2 }) {
		t.Fatalf("expected scheduled sweep to destroy old VMs, got %d destroyed", stub.destroyCount())
	}
}

func TestSweepTicker_ScheduledPathIsNeverDryRun(t *testing.T) {
	withFastRecheck(t)
	// An old VM under a real (non-dry-run) sweep must actually be destroyed.
	// Under a dry run it would only be flagged "would_destroy" and never
	// passed to DestroyVM.
	stub := &tickStub{vms: []ManagedVM{oldVM("vm-1")}}

	stop := StartSweepTicker(
		func() SweepParams { return SweepParams{Prefix: "cmm", Age: time.Hour, Interval: 5 * time.Millisecond} },
		func(context.Context) (Hypervisor, error) { return stub, nil },
		nil,
	)
	defer stop()

	if !waitFor(t, time.Second, func() bool { return stub.destroyCount() >= 1 }) {
		t.Fatal("scheduled sweep did not destroy an orphan — implies a dry run on the scheduled path")
	}
}

func TestSweepTicker_LiveDisableStopsSweeping(t *testing.T) {
	withFastRecheck(t)
	stub := &tickStub{vms: []ManagedVM{oldVM("vm-1")}}

	var mu sync.Mutex
	interval := 5 * time.Millisecond
	getParams := func() SweepParams {
		mu.Lock()
		defer mu.Unlock()
		return SweepParams{Prefix: "cmm", Age: time.Hour, Interval: interval}
	}

	stop := StartSweepTicker(getParams, func(context.Context) (Hypervisor, error) { return stub, nil }, nil)
	defer stop()

	// Confirm it is sweeping while enabled.
	if !waitFor(t, time.Second, func() bool { return stub.sweepCount() > 0 }) {
		t.Fatal("ticker never swept while enabled")
	}

	// Disable at runtime.
	mu.Lock()
	interval = 0
	mu.Unlock()

	// Allow the in-flight wait to elapse, then snapshot and confirm it settles.
	time.Sleep(40 * time.Millisecond)
	before := stub.sweepCount()
	time.Sleep(60 * time.Millisecond)
	if after := stub.sweepCount(); after != before {
		t.Errorf("ticker kept sweeping after live disable: %d -> %d", before, after)
	}
}

func TestSweepTicker_StopHaltsSweeping(t *testing.T) {
	withFastRecheck(t)
	stub := &tickStub{vms: []ManagedVM{oldVM("vm-1")}}

	stop := StartSweepTicker(
		func() SweepParams { return SweepParams{Prefix: "cmm", Age: time.Hour, Interval: 5 * time.Millisecond} },
		func(context.Context) (Hypervisor, error) { return stub, nil },
		nil,
	)

	waitFor(t, time.Second, func() bool { return stub.sweepCount() > 0 })
	stop()
	stop() // idempotent — must not panic.

	time.Sleep(20 * time.Millisecond)
	before := stub.sweepCount()
	time.Sleep(40 * time.Millisecond)
	if after := stub.sweepCount(); after != before {
		t.Errorf("ticker swept after stop: %d -> %d", before, after)
	}
}

func TestSweepTicker_NilHypervisorNoCrash(t *testing.T) {
	withFastRecheck(t)
	var sawErr bool
	var mu sync.Mutex

	stop := StartSweepTicker(
		func() SweepParams { return SweepParams{Prefix: "cmm", Age: time.Hour, Interval: 5 * time.Millisecond} },
		func(context.Context) (Hypervisor, error) { return nil, nil }, // not configured
		func(level, _ string, _ ...any) {
			if level == "ERROR" {
				mu.Lock()
				sawErr = true
				mu.Unlock()
			}
		},
	)
	defer stop()

	// Give it time to tick a few times; must not panic and must not log
	// errors for the legitimate "no hypervisor configured" case.
	time.Sleep(40 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if sawErr {
		t.Error("nil hypervisor (not configured) should be a quiet no-op, not an error")
	}
}

func TestSweepTicker_BuildErrorLogsAndContinues(t *testing.T) {
	withFastRecheck(t)
	var logs int
	var mu sync.Mutex

	stop := StartSweepTicker(
		func() SweepParams { return SweepParams{Prefix: "cmm", Age: time.Hour, Interval: 5 * time.Millisecond} },
		func(context.Context) (Hypervisor, error) { return nil, fmt.Errorf("creds unavailable") },
		func(level, _ string, _ ...any) {
			if level == "ERROR" {
				mu.Lock()
				logs++
				mu.Unlock()
			}
		},
	)
	defer stop()

	if !waitFor(t, time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return logs > 0 }) {
		t.Error("expected build error to be logged on the scheduled path")
	}
}
