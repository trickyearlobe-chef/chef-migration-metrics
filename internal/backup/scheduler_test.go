// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestScheduler_RunsBackup(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)

	var mu sync.Mutex
	var logs []string
	logger := func(level, msg string) {
		mu.Lock()
		logs = append(logs, fmt.Sprintf("[%s] %s", level, msg))
		mu.Unlock()
	}

	// Schedule every minute — we'll use a clock override to trigger immediately
	sched, err := NewScheduler(svc, "* * * * *", logger)
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	// Override clock to be just before the next minute boundary so the timer fires quickly
	now := time.Now().Truncate(time.Minute).Add(time.Minute).Add(-50 * time.Millisecond)
	sched.clock = func() time.Time { return now }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched.Start(ctx)

	// Wait for the backup to complete
	time.Sleep(300 * time.Millisecond)
	sched.Stop()

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) == 0 {
		t.Error("scheduler did not create any backups")
	}

	mu.Lock()
	hasStartLog := false
	for _, l := range logs {
		if l == "[info] backup: scheduled backup starting" {
			hasStartLog = true
		}
	}
	mu.Unlock()

	if !hasStartLog {
		t.Error("expected start log message")
	}
}

func TestScheduler_InvalidCron(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)

	_, err := NewScheduler(svc, "not a cron", nil)
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

func TestScheduler_Stop(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)

	sched, err := NewScheduler(svc, "0 2 * * *", nil)
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	ctx := context.Background()
	sched.Start(ctx)

	// Stop should return quickly
	done := make(chan struct{})
	go func() {
		sched.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2s")
	}
}

func TestScheduler_SkipsWhenActive(t *testing.T) {
	exec := &mockExecutor{
		pgDumpFn: func(ctx context.Context, _ ConnParams, outputPath string) error {
			// Block for a while to simulate long backup
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
			return writeTestFile(outputPath)
		},
	}
	svc := newTestService(t, exec)

	var mu sync.Mutex
	var skipCount int
	logger := func(_, msg string) {
		mu.Lock()
		if msg == "backup: scheduler skipping — another operation in progress" {
			skipCount++
		}
		mu.Unlock()
	}

	// Start a manual backup to block the scheduler
	ctx := context.Background()
	m, err := svc.Create(ctx, "manual")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	go svc.RunCreate(ctx, &m)

	// Schedule every minute, override clock to fire immediately
	sched, err := NewScheduler(svc, "* * * * *", logger)
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	now := time.Now().Truncate(time.Minute).Add(time.Minute).Add(-10 * time.Millisecond)
	sched.clock = func() time.Time { return now }

	sched.Start(ctx)
	time.Sleep(200 * time.Millisecond)
	sched.Stop()

	mu.Lock()
	skipped := skipCount
	mu.Unlock()

	if skipped == 0 {
		t.Error("scheduler should have skipped at least once while backup was active")
	}
}

// ---------------------------------------------------------------------------
// Reschedule — live schedule swap (mirrors collector scheduler, Chunk D)
// ---------------------------------------------------------------------------

// chanParser is a CronParser double that signals on every Next() call, so a
// test can observe which schedule the loop is currently driving from. It
// returns a fixed far-future time so the real timer never fires during the
// test — we only exercise scheduling, not backup execution.
type chanParser struct {
	queried chan struct{}
	next    time.Time
}

func (p *chanParser) Next(time.Time) time.Time {
	select {
	case p.queried <- struct{}{}:
	default:
	}
	return p.next
}

// Reschedule swaps the active schedule live and wakes the loop so the new
// schedule drives the next tick — no scheduler restart. Proven by observing the
// loop query the new parser after the swap.
func TestScheduler_Reschedule_PicksUpNewSchedule(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)

	future := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	parserA := &chanParser{queried: make(chan struct{}, 8), next: future}
	parserB := &chanParser{queried: make(chan struct{}, 8), next: future}

	sched, err := NewScheduler(svc, "0 2 * * *", nil)
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	// Drive from parserA with the clock well before the fixed next time, so the
	// timer (a couple of hours out) never fires during the test.
	sched.schedule = parserA
	sched.clock = func() time.Time { return time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC) }

	sched.Start(context.Background())
	defer sched.Stop()

	// The loop queries the initial schedule.
	select {
	case <-parserA.queried:
	case <-time.After(2 * time.Second):
		t.Fatal("initial schedule was never queried")
	}

	// Swap the schedule live.
	sched.Reschedule(parserB)

	// The loop must wake and recompute from the new schedule.
	select {
	case <-parserB.queried:
	case <-time.After(2 * time.Second):
		t.Fatal("rescheduled schedule never queried — loop did not pick up the new schedule")
	}
}

// A nil schedule passed to Reschedule is ignored (no panic, no swap).
func TestScheduler_Reschedule_NilIgnored(t *testing.T) {
	exec := &mockExecutor{}
	svc := newTestService(t, exec)

	sched, err := NewScheduler(svc, "0 2 * * *", nil)
	if err != nil {
		t.Fatalf("NewScheduler: %v", err)
	}
	sched.Reschedule(nil) // before Start — must not panic
}
