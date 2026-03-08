// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/logging"
)

// ---------------------------------------------------------------------------
// Git operation result types
// ---------------------------------------------------------------------------

// GitFetchResult summarises the outcome of a git cookbook fetching pass across
// all configured base URLs and active cookbook names.
type GitFetchResult struct {
	// Total is the number of cookbook repositories that were candidates for
	// clone or pull.
	Total int

	// Cloned is the number of repositories freshly cloned.
	Cloned int

	// Pulled is the number of repositories that were already cloned and
	// successfully pulled (fetched + reset).
	Pulled int

	// Unchanged is the number of repositories whose HEAD SHA did not change
	// after pull.
	Unchanged int

	// Failed is the number of repositories whose clone or pull failed.
	Failed int

	// Duration is the wall-clock time spent on the fetching pass.
	Duration time.Duration

	// Errors collects per-repository error details for logging.
	Errors []GitFetchError
}

// GitFetchError records a single git repository operation failure.
type GitFetchError struct {
	CookbookName string
	RepoURL      string
	Err          error
}

func (e GitFetchError) Error() string {
	return fmt.Sprintf("%s (%s): %v", e.CookbookName, e.RepoURL, e.Err)
}

// GitRepoResult holds the outcome of a single repository clone-or-pull
// operation.
type GitRepoResult struct {
	CookbookName  string
	RepoURL       string
	DefaultBranch string
	HeadCommitSHA string
	HasTestSuite  bool
	WasCloned     bool // true if freshly cloned, false if pulled
	Changed       bool // true if HEAD SHA changed (or first clone)
	Err           error
}

// ---------------------------------------------------------------------------
// Git command executor (interface for testing)
// ---------------------------------------------------------------------------

// GitExecutor abstracts running git commands so that tests can substitute a
// fake implementation without touching the filesystem or requiring git.
type GitExecutor interface {
	// Run executes a git command in the given working directory and returns
	// the combined stdout output. If the command fails, it returns an error
	// that includes stderr content.
	Run(ctx context.Context, dir string, args ...string) (string, error)
}

// defaultGitExecutor shells out to the real git binary.
type defaultGitExecutor struct{}

func (d *defaultGitExecutor) Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		if stderrStr != "" {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, stderrStr)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// ---------------------------------------------------------------------------
// Git cookbook manager
// ---------------------------------------------------------------------------

// GitCookbookManager handles cloning and pulling cookbook git repositories.
// It is safe for concurrent use.
type GitCookbookManager struct {
	// baseDir is the root directory under which cookbook repositories are
	// stored. Each repository is placed in a subdirectory named after the
	// cookbook (e.g. baseDir/cookbook-name).
	baseDir string

	// executor runs git commands. Defaults to the real git binary but can
	// be replaced for testing.
	executor GitExecutor
}

// NewGitCookbookManager creates a new manager that stores repositories under
// baseDir. If executor is nil, the default executor (real git binary) is used.
func NewGitCookbookManager(baseDir string, executor GitExecutor) *GitCookbookManager {
	if executor == nil {
		executor = &defaultGitExecutor{}
	}
	return &GitCookbookManager{
		baseDir:  baseDir,
		executor: executor,
	}
}

// RepoDir returns the local filesystem path for a given cookbook name.
func (m *GitCookbookManager) RepoDir(cookbookName string) string {
	return filepath.Join(m.baseDir, cookbookName)
}

// ---------------------------------------------------------------------------
// Core operations — clone, pull, detect branch, read HEAD, detect test suite
// ---------------------------------------------------------------------------

// CloneOrPull ensures the cookbook repository is present and up to date.
// If the repo directory does not exist, it is cloned. If it already exists,
// the latest changes are fetched and the working tree is reset to the remote
// HEAD of the default branch.
//
// Per the specification, we use fetch + hard reset instead of pull to avoid
// merge conflicts. All git commands use machine-parseable flags.
func (m *GitCookbookManager) CloneOrPull(ctx context.Context, cookbookName, repoURL string) (*GitRepoResult, error) {
	repoDir := m.RepoDir(cookbookName)

	result := &GitRepoResult{
		CookbookName: cookbookName,
		RepoURL:      repoURL,
	}

	// If the target directory exists but is not a valid git repo, remove it.
	// This can happen when a previous clone attempt created the directory but
	// failed before completing (e.g. bad URL or network error), leaving
	// behind an empty or partial directory that would cause git clone to fail
	// with "fatal: could not create work tree dir '...': File exists".
	//
	// This is safe because callers (fetchGitCookbooks) ensure that only one
	// goroutine operates on a given cookbook at a time.
	if _, err := os.Stat(repoDir); err == nil && !isGitRepo(repoDir) {
		if err := os.RemoveAll(repoDir); err != nil {
			result.Err = fmt.Errorf("removing stale directory %s: %w", repoDir, err)
			return result, result.Err
		}
	}

	if isGitRepo(repoDir) {
		// Repository exists — fetch + reset.
		if err := m.fetch(ctx, repoDir); err != nil {
			result.Err = fmt.Errorf("fetch: %w", err)
			return result, result.Err
		}

		branch, err := m.detectDefaultBranch(ctx, repoDir)
		if err != nil {
			result.Err = fmt.Errorf("detect default branch: %w", err)
			return result, result.Err
		}
		result.DefaultBranch = branch

		// Read HEAD SHA before reset to detect changes.
		oldSHA, _ := m.readHeadSHA(ctx, repoDir)

		if err := m.resetToRemote(ctx, repoDir, branch); err != nil {
			result.Err = fmt.Errorf("reset: %w", err)
			return result, result.Err
		}

		newSHA, err := m.readHeadSHA(ctx, repoDir)
		if err != nil {
			result.Err = fmt.Errorf("read HEAD SHA: %w", err)
			return result, result.Err
		}
		result.HeadCommitSHA = newSHA
		result.Changed = oldSHA != newSHA
		result.WasCloned = false
	} else {
		// Repository does not exist — clone.
		if err := m.clone(ctx, repoURL, repoDir); err != nil {
			result.Err = fmt.Errorf("clone: %w", err)
			return result, result.Err
		}

		branch, err := m.detectDefaultBranch(ctx, repoDir)
		if err != nil {
			result.Err = fmt.Errorf("detect default branch: %w", err)
			return result, result.Err
		}
		result.DefaultBranch = branch

		sha, err := m.readHeadSHA(ctx, repoDir)
		if err != nil {
			result.Err = fmt.Errorf("read HEAD SHA: %w", err)
			return result, result.Err
		}
		result.HeadCommitSHA = sha
		result.WasCloned = true
		result.Changed = true
	}

	// Detect test suite presence.
	result.HasTestSuite = m.detectTestSuite(ctx, repoDir)

	return result, nil
}

// clone runs: git clone --quiet <url> <dir>
func (m *GitCookbookManager) clone(ctx context.Context, repoURL, repoDir string) error {
	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	_, err := m.executor.Run(ctx, "", "clone", "--quiet", repoURL, repoDir)
	return err
}

// fetch runs: git fetch --quiet origin
func (m *GitCookbookManager) fetch(ctx context.Context, repoDir string) error {
	_, err := m.executor.Run(ctx, repoDir, "fetch", "--quiet", "origin")
	return err
}

// detectDefaultBranch determines the default branch of a repository.
//
// Strategy (per specification):
//  1. Try: git symbolic-ref refs/remotes/origin/HEAD --short
//     This emits "origin/main" or "origin/master". Strip the "origin/" prefix.
//  2. If that fails, fall back to checking for main then master via:
//     git rev-parse --verify origin/main
//     git rev-parse --verify origin/master
func (m *GitCookbookManager) detectDefaultBranch(ctx context.Context, repoDir string) (string, error) {
	// Strategy 1: symbolic-ref
	out, err := m.executor.Run(ctx, repoDir, "symbolic-ref", "refs/remotes/origin/HEAD", "--short")
	if err == nil && out != "" {
		// Output is e.g. "origin/main" — strip the "origin/" prefix.
		branch := strings.TrimPrefix(out, "origin/")
		if branch != "" {
			return branch, nil
		}
	}

	// Strategy 2: fall back to checking main, then master.
	if _, err := m.executor.Run(ctx, repoDir, "rev-parse", "--verify", "origin/main"); err == nil {
		return "main", nil
	}
	if _, err := m.executor.Run(ctx, repoDir, "rev-parse", "--verify", "origin/master"); err == nil {
		return "master", nil
	}

	return "", fmt.Errorf("could not detect default branch: neither origin/main nor origin/master exists")
}

// resetToRemote runs: git reset --hard origin/<branch>
func (m *GitCookbookManager) resetToRemote(ctx context.Context, repoDir, branch string) error {
	_, err := m.executor.Run(ctx, repoDir, "reset", "--hard", "origin/"+branch)
	return err
}

// readHeadSHA runs: git rev-parse HEAD
// Returns the 40-character SHA.
func (m *GitCookbookManager) readHeadSHA(ctx context.Context, repoDir string) (string, error) {
	out, err := m.executor.Run(ctx, repoDir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	// Validate it looks like a SHA.
	if len(out) < 40 {
		return "", fmt.Errorf("unexpected rev-parse output: %q", out)
	}
	return out[:40], nil
}

// detectTestSuite checks whether the repository contains a test suite.
// A cookbook is considered to have a test suite if any of the following
// paths exist at HEAD:
//   - .kitchen.yml or .kitchen.yaml (Test Kitchen)
//   - kitchen.yml or kitchen.yaml (Test Kitchen)
//   - test/ directory (InSpec profiles, integration tests)
//   - spec/ directory (ChefSpec)
func (m *GitCookbookManager) detectTestSuite(ctx context.Context, repoDir string) bool {
	testIndicators := []string{
		".kitchen.yml",
		".kitchen.yaml",
		"kitchen.yml",
		"kitchen.yaml",
		"test",
		"spec",
	}

	for _, path := range testIndicators {
		// Use git ls-tree to check if the path exists at HEAD without
		// depending on the working tree state.
		out, err := m.executor.Run(ctx, repoDir, "ls-tree", "--name-only", "HEAD", "--", path)
		if err == nil && out != "" {
			return true
		}
	}

	return false
}

// isGitRepo checks whether the given directory looks like a git repository
// by verifying the .git subdirectory exists.
func isGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

// ---------------------------------------------------------------------------
// Resolving cookbook names to git repository URLs
// ---------------------------------------------------------------------------

// ResolveGitCookbookURL attempts to find a git repository for a cookbook name
// by probing each configured base URL. For each base URL, it tries to clone
// (or verify existence of) a repository at <baseURL>/<cookbookName>.
//
// This is a best-effort heuristic — not all cookbooks will have a git repo.
// The function returns the first base URL that succeeds, or an empty string
// if none match.
func ResolveGitCookbookURL(cookbookName string, gitBaseURLs []string) string {
	for _, baseURL := range gitBaseURLs {
		baseURL = strings.TrimRight(baseURL, "/")
		repoURL := baseURL + "/" + cookbookName
		// We could probe with git ls-remote here but that would be slow for
		// many cookbooks. Instead, we attempt the clone in CloneOrPull and
		// handle failures gracefully. The resolved URL is a candidate.
		// For now, we simply construct it.
		if repoURL != "" {
			return repoURL
		}
	}
	return ""
}

// BuildGitCookbookURLs constructs candidate git repository URLs for a
// cookbook name across all configured base URLs. Unlike ResolveGitCookbookURL,
// this returns all candidates rather than just the first.
func BuildGitCookbookURLs(cookbookName string, gitBaseURLs []string) []string {
	urls := make([]string, 0, len(gitBaseURLs))
	for _, baseURL := range gitBaseURLs {
		baseURL = strings.TrimRight(baseURL, "/")
		urls = append(urls, baseURL+"/"+cookbookName)
	}
	return urls
}

// ---------------------------------------------------------------------------
// Orchestrator — parallel git fetch across multiple cookbooks
// ---------------------------------------------------------------------------

// fetchGitCookbooks clones or pulls git repositories for active cookbooks
// that have a matching git base URL configuration. It runs operations in
// parallel bounded by the given concurrency limit, and upserts results into
// the datastore.
//
// The function:
//  1. Determines which active cookbook names should be checked for git repos
//     by building candidate URLs from the configured git_base_urls.
//  2. For each candidate, attempts a clone or pull via GitCookbookManager.
//  3. On success, upserts the cookbook record with source = 'git', recording
//     the HEAD commit SHA, default branch, and test suite presence.
//  4. On failure, logs the error and continues — git failures are non-fatal.
//
// The activeCookbookNames set comes from the node collection phase and
// ensures we only fetch cookbooks that are in active use.
func fetchGitCookbooks(
	ctx context.Context,
	mgr *GitCookbookManager,
	db *datastore.DB,
	log *logging.ScopedLogger,
	gitBaseURLs []string,
	activeCookbookNames map[string]bool,
	concurrency int,
) GitFetchResult {
	start := time.Now()

	if concurrency <= 0 {
		concurrency = 1
	}
	if len(gitBaseURLs) == 0 {
		return GitFetchResult{Duration: time.Since(start)}
	}

	// Normalise base URLs once.
	trimmedURLs := make([]string, len(gitBaseURLs))
	for i, u := range gitBaseURLs {
		trimmedURLs[i] = strings.TrimRight(u, "/")
	}

	// Build a list of cookbook names to process. Each cookbook will try base
	// URLs sequentially within a single goroutine, which prevents concurrent
	// clone operations targeting the same local directory.
	cookbooks := make([]string, 0, len(activeCookbookNames))
	for name := range activeCookbookNames {
		cookbooks = append(cookbooks, name)
	}

	result := GitFetchResult{
		Total: len(cookbooks) * len(trimmedURLs),
	}

	if len(cookbooks) == 0 {
		result.Duration = time.Since(start)
		return result
	}

	log.Info(fmt.Sprintf("checking %d git cookbook candidate(s) across %d base URL(s)",
		result.Total, len(trimmedURLs)))

	// Use a buffered channel as a semaphore to bound concurrency.
	sem := make(chan struct{}, concurrency)

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, cbName := range cookbooks {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(cbName string) {
			defer wg.Done()

			// Acquire semaphore slot.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			// Try each base URL in order until one succeeds.
			var lastErr error
			for _, baseURL := range trimmedURLs {
				if ctx.Err() != nil {
					return
				}

				repoURL := baseURL + "/" + cbName
				repoResult, err := mgr.CloneOrPull(ctx, cbName, repoURL)

				if err != nil {
					lastErr = err
					// Log at DEBUG because a missing repo for one base URL
					// is expected when multiple base URLs are configured.
					log.Debug(fmt.Sprintf("git cookbook candidate failed: %s (%s): %v",
						cbName, repoURL, err))
					continue
				}

				// Success — record the result and stop trying further URLs.
				mu.Lock()
				if repoResult.WasCloned {
					result.Cloned++
				} else if repoResult.Changed {
					result.Pulled++
				} else {
					result.Unchanged++
				}
				mu.Unlock()

				// Upsert into the datastore.
				now := time.Now().UTC()
				_, upsertErr := db.UpsertGitCookbook(ctx, datastore.UpsertGitCookbookParams{
					Name:            cbName,
					GitRepoURL:      repoURL,
					HeadCommitSHA:   repoResult.HeadCommitSHA,
					DefaultBranch:   repoResult.DefaultBranch,
					HasTestSuite:    repoResult.HasTestSuite,
					IsActive:        true,
					IsStaleCookbook: false,
					FirstSeenAt:     now,
					LastFetchedAt:   now,
				})
				if upsertErr != nil {
					log.Warn(fmt.Sprintf("git cookbook upsert failed for %s: %v", cbName, upsertErr))
				} else {
					verb := "pulled"
					if repoResult.WasCloned {
						verb = "cloned"
					}
					log.Info(fmt.Sprintf("git cookbook %s: %s branch=%s sha=%s test_suite=%v",
						verb, cbName, repoResult.DefaultBranch,
						repoResult.HeadCommitSHA[:minInt(8, len(repoResult.HeadCommitSHA))],
						repoResult.HasTestSuite),
						logging.WithCookbook(cbName, ""),
						logging.WithCommitSHA(repoResult.HeadCommitSHA))
				}
				lastErr = nil
				break
			}

			// If all base URLs failed, count a single failure for the cookbook.
			if lastErr != nil {
				mu.Lock()
				result.Failed++
				result.Errors = append(result.Errors, GitFetchError{
					CookbookName: cbName,
					RepoURL:      trimmedURLs[len(trimmedURLs)-1] + "/" + cbName,
					Err:          lastErr,
				})
				mu.Unlock()
			}
		}(cbName)
	}

	wg.Wait()
	result.Duration = time.Since(start)
	return result
}

// minInt returns the smaller of a and b.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
