// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
)

// Scheduler runs backups on a cron schedule.
type Scheduler struct {
	svc    *Service
	logger func(level, msg string)
	stopCh chan struct{}
	doneCh chan struct{}
	clock  func() time.Time

	// mu protects schedule: the loop reads it under mu each iteration and
	// Reschedule swaps it live.
	mu       sync.Mutex
	schedule collector.CronParser

	// reschedule wakes the loop when the schedule is swapped live so it stops
	// the pending timer and recomputes the next fire from the new schedule.
	// Buffered (size 1) with non-blocking sends, so bursts coalesce.
	reschedule chan struct{}
}

// NewScheduler creates a scheduler from a cron expression (5-field standard cron).
func NewScheduler(svc *Service, cronExpr string, logger func(level, msg string)) (*Scheduler, error) {
	sched, err := collector.ParseSchedule(cronExpr)
	if err != nil {
		return nil, fmt.Errorf("backup: invalid schedule %q: %w", cronExpr, err)
	}
	if logger == nil {
		logger = func(_, _ string) {}
	}
	return &Scheduler{
		svc:        svc,
		schedule:   sched,
		logger:     logger,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
		reschedule: make(chan struct{}, 1),
		clock:      time.Now,
	}, nil
}

// Start begins the scheduled backup loop. It runs until Stop is called or the
// context is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	go s.run(ctx)
}

// Stop signals the scheduler to stop and waits for it to finish.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
}

// Reschedule swaps the active cron schedule and wakes the loop so the new
// schedule drives the next backup immediately — no scheduler restart. Safe to
// call concurrently with the running loop. A nil schedule is ignored.
func (s *Scheduler) Reschedule(schedule collector.CronParser) {
	if schedule == nil {
		return
	}
	s.mu.Lock()
	s.schedule = schedule
	s.mu.Unlock()
	// Non-blocking signal — the buffered channel coalesces bursts; the loop
	// re-reads s.schedule on its next iteration.
	select {
	case s.reschedule <- struct{}{}:
	default:
	}
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.doneCh)

	for {
		now := s.clock()
		s.mu.Lock()
		schedule := s.schedule
		s.mu.Unlock()
		next := schedule.Next(now)
		if next.IsZero() {
			s.logger("error", "backup: schedule will never fire — stopping scheduler")
			return
		}

		wait := next.Sub(now)
		s.logger("info", fmt.Sprintf("backup: next scheduled backup at %s", next.Format(time.RFC3339)))

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.stopCh:
			timer.Stop()
			return
		case <-s.reschedule:
			// Schedule changed live — drop the pending timer and recompute the
			// next fire from the new schedule.
			timer.Stop()
			s.logger("info", "backup: schedule updated — recalculating next run")
			continue
		case <-timer.C:
			s.runBackup(ctx)
		}
	}
}

func (s *Scheduler) runBackup(ctx context.Context) {
	if s.svc.IsActive() {
		s.logger("info", "backup: scheduler skipping — another operation in progress")
		return
	}

	s.logger("info", "backup: scheduled backup starting")
	m, err := s.svc.Create(ctx, "scheduler")
	if err != nil {
		s.logger("error", "backup: scheduled create failed: "+err.Error())
		return
	}

	s.svc.RunCreate(ctx, &m)

	if m.Status == StatusSucceeded {
		s.logger("info", "backup: scheduled backup completed successfully")
		if pruned, err := s.svc.Prune(); err != nil {
			s.logger("error", "backup: prune failed: "+err.Error())
		} else if len(pruned) > 0 {
			s.logger("info", fmt.Sprintf("backup: pruned %d old backups", len(pruned)))
		}
	} else {
		s.logger("error", "backup: scheduled backup failed: "+m.Error)
	}
}
