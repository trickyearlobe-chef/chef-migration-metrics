// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package kitchenqueue

import (
	"context"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/gitkitchen"
)

// GitKitchenExecutor bridges queue items to the existing gitkitchen.RunInstance
// function for git-type kitchen runs.
type GitKitchenExecutor struct {
	kitchenExecutor gitkitchen.KitchenExecutor
	credResolver    gitkitchen.CredentialResolver
	store           gitkitchen.ResultStore
	repoDirFn       func(name, url string) string
	tkConfigFn      func() config.TestKitchenConfig
}

// GitKitchenExecutorConfig holds dependencies for creating a GitKitchenExecutor.
type GitKitchenExecutorConfig struct {
	KitchenExecutor gitkitchen.KitchenExecutor
	CredResolver    gitkitchen.CredentialResolver
	Store           gitkitchen.ResultStore
	RepoDirFn       func(name, url string) string
	TKConfigFn      func() config.TestKitchenConfig
}

// NewGitKitchenExecutor creates an executor for git kitchen queue items.
func NewGitKitchenExecutor(cfg GitKitchenExecutorConfig) *GitKitchenExecutor {
	return &GitKitchenExecutor{
		kitchenExecutor: cfg.KitchenExecutor,
		credResolver:    cfg.CredResolver,
		store:           cfg.Store,
		repoDirFn:       cfg.RepoDirFn,
		tkConfigFn:      cfg.TKConfigFn,
	}
}

// Execute runs a git kitchen instance from a queue item.
func (e *GitKitchenExecutor) Execute(ctx context.Context, item *datastore.KitchenQueueItem) (string, error) {
	if item.RunType != "git" {
		return "", fmt.Errorf("GitKitchenExecutor: unsupported run_type %q", item.RunType)
	}

	tkConfig := e.tkConfigFn()

	params := gitkitchen.RunInstanceParams{
		GitRepoName:       item.GitRepoName,
		GitRepoURL:        item.GitRepoURL,
		RepoDir:           e.repoDirFn(item.GitRepoName, item.GitRepoURL),
		InstanceName:      item.InstanceName,
		SuiteName:         item.SuiteName,
		PlatformName:      item.PlatformName,
		TargetChefVersion: item.TargetChefVersion,
		CommitSHA:         item.HeadCommitSHA,
	}

	result := gitkitchen.RunInstance(ctx, params, tkConfig, e.kitchenExecutor, e.credResolver)

	// Persist result to git_kitchen_results table
	upsertParams := datastore.UpsertGitKitchenResultParams{
		GitRepoName:       item.GitRepoName,
		GitRepoURL:        item.GitRepoURL,
		SuiteName:         item.SuiteName,
		PlatformName:      item.PlatformName,
		InstanceName:      item.InstanceName,
		TargetChefVersion: item.TargetChefVersion,
		CommitSHA:         item.HeadCommitSHA,
		Passed:            result.Passed,
		TimedOut:          result.TimedOut,
		Output:            result.Output,
		DurationSeconds:   result.DurationSeconds,
		ErrorMessage:      result.ErrorMessage,
		DriverUsed:        result.DriverUsed,
	}
	if _, err := e.store.UpsertGitKitchenResult(ctx, upsertParams); err != nil {
		return result.Output, fmt.Errorf("persisting result: %w", err)
	}

	if result.ErrorMessage != "" {
		return result.Output, fmt.Errorf("%s", result.ErrorMessage)
	}
	if result.Passed != nil && !*result.Passed {
		return result.Output, fmt.Errorf("kitchen test failed")
	}

	return result.Output, nil
}
