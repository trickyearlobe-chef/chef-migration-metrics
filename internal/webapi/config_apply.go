// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
)

// ReloadGranularity describes how much of the running system must re-apply a
// stored config change for it to take effect. Values are ordered by severity
// (least to most disruptive) so the worst granularity across a set of appliers
// can be taken with a simple max — see worstGranularity.
//
// Only ReloadProcess requires a full supervisor re-exec; everything below it is
// applied without dropping the process, so restart_required derives solely from
// "is the worst granularity process?" (configuration-live-reload.md).
type ReloadGranularity int

const (
	// ReloadApplied: read live per request via r.liveConfig(); nothing to
	// re-apply once the config holder has reloaded.
	ReloadApplied ReloadGranularity = iota
	// ReloadSubsystem: the owning component re-applies in place (reschedule a
	// cron, swap a log level, resize a worker pool, reconcile a table).
	ReloadSubsystem
	// ReloadListener: rebind the HTTP/TLS listener only; the process stays up.
	ReloadListener
	// ReloadProcess: supervisor re-exec required (last resort). Also the
	// pessimistic default for a section that has registered no applier.
	ReloadProcess
)

func (g ReloadGranularity) String() string {
	switch g {
	case ReloadApplied:
		return "applied"
	case ReloadSubsystem:
		return "subsystem"
	case ReloadListener:
		return "listener"
	case ReloadProcess:
		return "process"
	default:
		return "unknown"
	}
}

// ApplyResult is what an Applier reports after re-applying a stored change.
type ApplyResult struct {
	// Reload is the granularity that was actually needed to make the change
	// take effect.
	Reload ReloadGranularity
}

// Applier re-applies a freshly-stored config section to the running system and
// reports the granularity it needed. The subsystem that owns the change
// registers it; the web layer never declares restart_required itself, it
// derives the flag from what the appliers report. An Applier runs after the
// section is persisted and the config holder has reloaded.
type Applier func(context.Context) (ApplyResult, error)

// appliedApplier is the no-op applier for a section that is read live per
// request (r.liveConfig()): once the holder has reloaded there is nothing more
// to do, so it reports ReloadApplied and the section needs no restart.
func appliedApplier(context.Context) (ApplyResult, error) {
	return ApplyResult{Reload: ReloadApplied}, nil
}

// subsystemApplier adapts a plain reconcile func (the legacy postReload shape)
// into an Applier that reports ReloadSubsystem on success.
func subsystemApplier(fn func(context.Context) error) Applier {
	return func(ctx context.Context) (ApplyResult, error) {
		if err := fn(ctx); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Reload: ReloadSubsystem}, nil
	}
}

// applyCookstyleRescore re-evaluates all stored cookstyle results against the
// currently-active failure rules (subsystem). The rules are read from the
// reloaded live config. Reports ReloadSubsystem so the caller knows verdicts
// may have changed without a process restart.
func (r *Router) applyCookstyleRescore(ctx context.Context) (ApplyResult, error) {
	cfg := r.liveConfig()
	rules := analysis.EffectiveRules(
		cfg.AnalysisTools.CookstyleFailurePreset,
		cfg.AnalysisTools.CookstyleFailureRules,
	)
	_, err := RescoreCookstyleResults(ctx, r.db, rules, r.logger)
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Reload: ReloadSubsystem}, nil
}

// applyKitchenWorkerCount resizes the kitchen queue worker pool to match the
// live MaxConcurrentVMs (subsystem). When no kitchen queue is wired there is
// nothing to resize and the change is still live for the rest of the section
// (applied). Mirrors the inline resize in the dedicated test-kitchen handler.
func (r *Router) applyKitchenWorkerCount(context.Context) (ApplyResult, error) {
	if r.kitchenQueue == nil {
		return ApplyResult{Reload: ReloadApplied}, nil
	}
	r.kitchenQueue.SetWorkerCount(r.liveConfig().AnalysisTools.TestKitchen.EffectiveMaxConcurrentVMs())
	return ApplyResult{Reload: ReloadSubsystem}, nil
}

// logLevelApplier re-applies the validated log level to the running logger via
// the wired setter and reports ReloadSubsystem. Only registered when a setter
// is wired; without one the logging section has no applier and stays at the
// pessimistic process default. The level string is already validated by the
// handler (DEBUG/INFO/WARN/ERROR).
func (r *Router) logLevelApplier(level string) Applier {
	return func(context.Context) (ApplyResult, error) {
		if err := r.logLevelSetter(level); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Reload: ReloadSubsystem}, nil
	}
}

// collectionScheduleApplier reschedules the running collection scheduler to the
// validated cron string via the wired rescheduler and reports ReloadSubsystem.
// Only registered when a rescheduler is wired; without one the schedule half of
// the collection section has no applier and the change only takes effect on
// restart. The cron string is already validated by the handler (5-field cron).
func (r *Router) collectionScheduleApplier(schedule string) Applier {
	return func(context.Context) (ApplyResult, error) {
		if err := r.collectionRescheduler(schedule); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Reload: ReloadSubsystem}, nil
	}
}

// backupApplier reconciles the running backup scheduler to the freshly-stored
// backup config via the wired reconciler and reports ReloadSubsystem. Only
// registered when a reconciler is wired; without one the backup section has no
// applier and stays at the pessimistic process default. The reconciler reads the
// reloaded live config to decide start/stop/reschedule (so webapi stays
// decoupled from the backup subsystem and the schedule default lives in one
// place), hence it takes no arguments.
func (r *Router) backupApplier() Applier {
	return func(context.Context) (ApplyResult, error) {
		if err := r.backupReconciler(); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Reload: ReloadSubsystem}, nil
	}
}

// samlApplier rebuilds the SAML provider from the freshly-stored auth config via
// the wired reconciler and reports ReloadSubsystem. Only registered when a
// reconciler is wired; without one the auth section's SAML half has no applier
// (the rest of the section — session/lockout/min-password — still reloads as
// applied reads). The reconciler reads the reloaded live config itself, so it
// takes no arguments.
func (r *Router) samlApplier() Applier {
	return func(context.Context) (ApplyResult, error) {
		if err := r.samlReconciler(); err != nil {
			return ApplyResult{}, err
		}
		return ApplyResult{Reload: ReloadSubsystem}, nil
	}
}

// worstGranularity returns the most severe granularity among results. With no
// results it returns ReloadProcess: a section that registered no applier is
// assumed to need a restart (pessimistic — at worst over-prompts, never
// silently claims a change is live when it is not).
func worstGranularity(results []ApplyResult) ReloadGranularity {
	worst := ReloadProcess
	if len(results) == 0 {
		return worst
	}
	worst = ReloadApplied
	for _, res := range results {
		if res.Reload > worst {
			worst = res.Reload
		}
	}
	return worst
}
