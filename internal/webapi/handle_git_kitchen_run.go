// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/batch"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// GitKitchenRunner abstracts the batch kitchen runner so the handler can
// trigger a single git kitchen instance without importing the batch package
// in the router struct.
type GitKitchenRunner interface {
	RunInstance(ctx context.Context, req batch.RunInstanceRequest) batch.RunInstanceResult
}

type gitKitchenRunRequest struct {
	GitRepoName       string `json:"git_repo_name"`
	TargetChefVersion string `json:"target_chef_version"`
	PlatformName      string `json:"platform_name"`
	SuiteName         string `json:"suite_name"`
}

// handleGitKitchenRun starts an asynchronous Git Kitchen run for a single
// repo/platform/suite combination.
//
//	POST /api/v1/kitchen/git-run
func (r *Router) handleGitKitchenRun(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	var body gitKitchenRunRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteBadRequest(w, fmt.Sprintf("Invalid JSON body: %v", err))
		return
	}

	if body.GitRepoName == "" {
		WriteBadRequest(w, "git_repo_name is required.")
		return
	}
	if body.TargetChefVersion == "" {
		WriteBadRequest(w, "target_chef_version is required.")
		return
	}
	if body.PlatformName == "" {
		WriteBadRequest(w, "platform_name is required.")
		return
	}
	if body.SuiteName == "" {
		WriteBadRequest(w, "suite_name is required.")
		return
	}

	if r.gitKitchenRunner == nil {
		WriteError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable,
			"Git Kitchen runner is not configured.")
		return
	}

	ctx := req.Context()
	repos, err := r.db.ListGitReposByName(ctx, body.GitRepoName)
	if err != nil {
		r.logf("ERROR", "git-kitchen-run: listing repos for %s: %v", body.GitRepoName, err)
		WriteInternalError(w, "Failed to look up git repo.")
		return
	}
	if len(repos) == 0 {
		WriteNotFound(w, "Git repo not found.")
		return
	}

	var repo *datastore.GitRepo
	for i := range repos {
		if repos[i].IsCloned() {
			repo = &repos[i]
			break
		}
	}
	if repo == nil {
		WriteError(w, http.StatusConflict, "conflict",
			"Git repo exists but is not cloned yet.")
		return
	}

	go func() {
		result := r.gitKitchenRunner.RunInstance(context.Background(), batch.RunInstanceRequest{
			GitRepoName:       body.GitRepoName,
			GitRepoURL:        repo.GitRepoURL,
			CommitSHA:         repo.HeadCommitSHA,
			TargetChefVersion: body.TargetChefVersion,
			PlatformName:      body.PlatformName,
			SuiteName:         body.SuiteName,
		})
		convergePassed := result.ConvergePassed
		testsPassed := result.TestsPassed
		_, err := r.db.UpsertGitKitchenResult(context.Background(), datastore.UpsertGitKitchenResultParams{
			GitRepoName:       body.GitRepoName,
			GitRepoURL:        repo.GitRepoURL,
			TargetChefVersion: body.TargetChefVersion,
			CommitSHA:         repo.HeadCommitSHA,
			PlatformName:      body.PlatformName,
			SuiteName:         body.SuiteName,
			DriverUsed:        result.DriverUsed,
			TemplateUsed:      result.TemplateUsed,
			ConvergePassed:    convergePassed,
			TestsPassed:       testsPassed,
			TimedOut:          result.TimedOut,
			ConvergeOutput:    result.ConvergeOutput,
			VerifyOutput:      result.VerifyOutput,
			DestroyOutput:     result.DestroyOutput,
			DurationSeconds:   result.DurationSeconds,
			ErrorMessage:      result.ErrorMessage,
			StartedAt:         result.StartedAt,
			CompletedAt:       result.CompletedAt,
		})
		if err != nil {
			r.logf("ERROR", "git-kitchen-run: storing result for %s: %v", body.GitRepoName, err)
		}
	}()

	WriteJSON(w, http.StatusAccepted, map[string]string{
		"status": "started",
		"message": fmt.Sprintf("Git Kitchen run started for %s (platform: %s, suite: %s, version: %s).",
			body.GitRepoName, body.PlatformName, body.SuiteName, body.TargetChefVersion),
	})
}
