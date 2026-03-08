// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// CronParser is the interface for parsing and evaluating cron schedule
// expressions. This is satisfied by robfig/cron/v3's Parser or by a
// simple test double.
type CronParser interface {
	// Next returns the next activation time after the given reference time
	// for the configured schedule expression. Returns the zero time if the
	// expression will never activate again.
	Next(after time.Time) time.Time
}

// CronSchedule wraps a parsed cron expression and implements CronParser.
// This type is constructed by ParseSchedule.
type CronSchedule struct {
	expr   string
	fields []cronField
}

// Scheduler drives periodic collection runs using a cron-style schedule.
// It enforces the single-run constraint — if a run is still in progress
// when the next tick fires, the tick is skipped and a WARN is logged.
//
// Use Start to begin the scheduling loop and Stop to shut it down.
type Scheduler struct {
	collector *Collector
	schedule  CronParser
	logger    *logging.Logger

	// cancel and done coordinate shutdown.
	cancel context.CancelFunc
	done   chan struct{}

	// mu protects started.
	mu      sync.Mutex
	started bool

	// clock is used for time operations. Defaults to time.Now. Override
	// in tests via WithClock.
	clock func() time.Time

	// newTimer creates a timer that fires after the given duration. Override
	// in tests via WithTimerFactory.
	newTimer func(d time.Duration) (<-chan time.Time, func() bool)
}

// SchedulerOption configures optional behaviour on a Scheduler.
type SchedulerOption func(*Scheduler)

// WithClock overrides the default time.Now function. Intended for testing.
func WithClock(fn func() time.Time) SchedulerOption {
	return func(s *Scheduler) {
		if fn != nil {
			s.clock = fn
		}
	}
}

// WithTimerFactory overrides the default timer creation. The function must
// return a channel that fires after duration d and a stop function.
// Intended for testing.
func WithTimerFactory(fn func(d time.Duration) (<-chan time.Time, func() bool)) SchedulerOption {
	return func(s *Scheduler) {
		if fn != nil {
			s.newTimer = fn
		}
	}
}

// NewScheduler creates a Scheduler that drives the given Collector according
// to the provided cron schedule.
func NewScheduler(
	collector *Collector,
	schedule CronParser,
	logger *logging.Logger,
	opts ...SchedulerOption,
) *Scheduler {
	s := &Scheduler{
		collector: collector,
		schedule:  schedule,
		logger:    logger,
		clock:     time.Now,
		newTimer: func(d time.Duration) (<-chan time.Time, func() bool) {
			t := time.NewTimer(d)
			return t.C, t.Stop
		},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start begins the scheduling loop in a background goroutine. It returns
// immediately. Call Stop to shut down the scheduler.
//
// Start may only be called once. Subsequent calls return an error.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return fmt.Errorf("collector: scheduler already started")
	}
	s.started = true

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})

	go s.loop(ctx)
	return nil
}

// Stop signals the scheduling loop to exit and waits for it to finish.
// If the scheduler was never started, Stop returns immediately.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	s.cancel()
	<-s.done
}

// TriggerNow requests an immediate collection run outside the normal
// schedule. If a run is already in progress, it returns an error
// (the run is not queued).
func (s *Scheduler) TriggerNow(ctx context.Context) (*RunResult, error) {
	log := s.logger.WithScope(logging.ScopeCollectionRun)
	log.Info("manually triggered collection run")
	return s.collector.Run(ctx)
}

// loop is the main scheduling goroutine. It calculates the time until the
// next cron tick, sleeps, and then triggers a collection run.
func (s *Scheduler) loop(ctx context.Context) {
	defer close(s.done)

	log := s.logger.WithScope(logging.ScopeCollectionRun)

	for {
		now := s.clock()
		nextFire := s.schedule.Next(now)
		if nextFire.IsZero() {
			log.Error("cron schedule will never fire again — stopping scheduler")
			return
		}

		delay := nextFire.Sub(now)
		if delay < 0 {
			delay = 0
		}

		log.Debug(fmt.Sprintf("next collection scheduled at %s (in %s)",
			nextFire.Format(time.RFC3339), delay.Round(time.Second)))

		timerCh, timerStop := s.newTimer(delay)

		select {
		case <-ctx.Done():
			timerStop()
			log.Info("scheduler shutting down")
			return

		case <-timerCh:
			// Timer fired — attempt to run collection.
			if s.collector.IsRunning() {
				log.Warn("scheduled collection tick skipped — a run is already in progress")
				continue
			}

			s.runWithRecovery(ctx, log)
		}
	}
}

// runWithRecovery executes a collection run, recovering from panics so that
// a bug in the collector does not crash the scheduler goroutine.
func (s *Scheduler) runWithRecovery(ctx context.Context, log *logging.ScopedLogger) {
	var result *RunResult
	var err error

	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic during collection run: %v", r)
			}
		}()
		result, err = s.collector.Run(ctx)
	}()

	if err != nil {
		if ctx.Err() != nil {
			// Context was cancelled during the run — we're shutting down.
			log.Info("collection run interrupted by shutdown")
			return
		}
		log.Error(fmt.Sprintf("scheduled collection run failed: %v", err))
	} else if result != nil {
		log.Info(fmt.Sprintf("scheduled collection run completed: %d/%d orgs, %d nodes in %s",
			result.SucceededOrgs, result.TotalOrgs, result.TotalNodes,
			result.Duration.Round(time.Millisecond)))
	}
}

// ---------------------------------------------------------------------------
// Built-in cron parser (avoids external dependency for simple schedules)
// ---------------------------------------------------------------------------

// cronField represents a parsed field in a cron expression. It stores
// the set of valid values for that field position.
type cronField struct {
	values map[int]bool
	min    int
	max    int
}

// ParseSchedule parses a standard 5-field cron expression (minute hour
// day-of-month month day-of-week) and returns a CronSchedule.
//
// Supported syntax:
//   - Literal numbers: "5" means value 5
//   - Wildcards: "*" means all valid values
//   - Ranges: "1-5" means 1, 2, 3, 4, 5
//   - Steps: "*/15" means every 15th value; "1-30/5" means 1, 6, 11, ...
//   - Lists: "1,15,30" means values 1, 15, and 30
//
// Day-of-week: 0 = Sunday, 6 = Saturday (7 is also accepted as Sunday).
func ParseSchedule(expr string) (*CronSchedule, error) {
	fields := splitFields(expr)
	if len(fields) != 5 {
		return nil, fmt.Errorf("collector: cron expression must have 5 fields, got %d: %q", len(fields), expr)
	}

	bounds := []struct{ min, max int }{
		{0, 59}, // minute
		{0, 23}, // hour
		{1, 31}, // day of month
		{1, 12}, // month
		{0, 6},  // day of week (0=Sun)
	}

	parsed := make([]cronField, 5)
	for i, f := range fields {
		cf, err := parseCronField(f, bounds[i].min, bounds[i].max)
		if err != nil {
			return nil, fmt.Errorf("collector: cron field %d (%q): %w", i+1, f, err)
		}
		parsed[i] = cf
	}

	return &CronSchedule{expr: expr, fields: parsed}, nil
}

// Next returns the next time the schedule fires after the given time.
func (cs *CronSchedule) Next(after time.Time) time.Time {
	// Start from the next minute boundary.
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Search forward up to 4 years (to handle leap years and edge cases).
	limit := after.Add(4 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		// Check month.
		if !cs.fields[3].values[int(t.Month())] {
			// Advance to next month.
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		// Check day of month.
		if !cs.fields[2].values[t.Day()] {
			t = t.AddDate(0, 0, 1)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}

		// Check day of week. Map Sunday=7 to 0 for consistency.
		dow := int(t.Weekday()) // 0=Sunday in Go
		if !cs.fields[4].values[dow] {
			t = t.AddDate(0, 0, 1)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}

		// Check hour.
		if !cs.fields[1].values[t.Hour()] {
			t = t.Add(time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
			continue
		}

		// Check minute.
		if !cs.fields[0].values[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}

		return t
	}

	return time.Time{} // Schedule will never fire (shouldn't happen for normal expressions)
}

// String returns the original cron expression.
func (cs *CronSchedule) String() string {
	return cs.expr
}

// ---------------------------------------------------------------------------
// Cron field parsing helpers
// ---------------------------------------------------------------------------

func splitFields(expr string) []string {
	var fields []string
	current := ""
	for _, ch := range expr {
		if ch == ' ' || ch == '\t' {
			if current != "" {
				fields = append(fields, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		fields = append(fields, current)
	}
	return fields
}

func parseCronField(field string, min, max int) (cronField, error) {
	cf := cronField{
		values: make(map[int]bool),
		min:    min,
		max:    max,
	}

	// Split on comma for lists: "1,15,30"
	parts := splitOnComma(field)
	for _, part := range parts {
		if err := parseCronPart(part, min, max, cf.values); err != nil {
			return cf, err
		}
	}

	if len(cf.values) == 0 {
		return cf, fmt.Errorf("no valid values in field %q", field)
	}

	return cf, nil
}

func parseCronPart(part string, min, max int, values map[int]bool) error {
	// Check for step: "*/15" or "1-30/5"
	step := 1
	if idx := indexByte(part, '/'); idx >= 0 {
		stepStr := part[idx+1:]
		part = part[:idx]
		s, err := atoi(stepStr)
		if err != nil || s <= 0 {
			return fmt.Errorf("invalid step %q", stepStr)
		}
		step = s
	}

	// Check for range: "1-30"
	if idx := indexByte(part, '-'); idx >= 0 {
		lo, err := atoi(part[:idx])
		if err != nil {
			return fmt.Errorf("invalid range start %q", part[:idx])
		}
		hi, err := atoi(part[idx+1:])
		if err != nil {
			return fmt.Errorf("invalid range end %q", part[idx+1:])
		}
		if lo < min || hi > max || lo > hi {
			return fmt.Errorf("range %d-%d out of bounds [%d, %d]", lo, hi, min, max)
		}
		for i := lo; i <= hi; i += step {
			values[normalise(i, min, max)] = true
		}
		return nil
	}

	// Check for wildcard: "*"
	if part == "*" {
		for i := min; i <= max; i += step {
			values[i] = true
		}
		return nil
	}

	// Literal value.
	v, err := atoi(part)
	if err != nil {
		return fmt.Errorf("invalid value %q", part)
	}
	v = normalise(v, min, max)
	if v < min || v > max {
		return fmt.Errorf("value %d out of bounds [%d, %d]", v, min, max)
	}
	values[v] = true
	return nil
}

func splitOnComma(s string) []string {
	var parts []string
	current := ""
	for _, ch := range s {
		if ch == ',' {
			if current != "" {
				parts = append(parts, current)
			}
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func atoi(s string) (int, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty string")
	}
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("non-digit %q", string(ch))
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}

// normalise handles day-of-week 7 → 0 (both represent Sunday).
func normalise(v, min, max int) int {
	if min == 0 && max == 6 && v == 7 {
		return 0
	}
	return v
}
