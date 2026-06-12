// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/analysis"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// ClientFactory creates a Chef API client for a given organisation name.
type ClientFactory func(ctx context.Context, orgName string) (*chefapi.Client, error)

// RunnerFactory creates per-request Node Kitchen runners with org-specific
// Chef API clients. It satisfies the webapi.NodeKitchenRunner interface.
type RunnerFactory struct {
	DB             DataStore
	DepResolver    CookbookDependencyResolver
	ClientFactory  ClientFactory
	Executor       KitchenExecutor
	CredResolver   CredentialResolver
	Logger         Logger
	TKConfigFn     func() config.TestKitchenConfig
	GitCookbookDir string
	// ConcurrencyFn returns the live max-concurrent-downloads value, read on
	// each Run so concurrency.cookbook_download applies at the next node-kitchen
	// run without a restart. When nil or <= 0, assembly falls back to its default.
	ConcurrencyFn func() int
}

// concurrency resolves the live cookbook-download concurrency for a run,
// returning 0 when no provider is wired (assembly clamps 0 to its default).
func (f *RunnerFactory) concurrency() int {
	if f.ConcurrencyFn != nil {
		return f.ConcurrencyFn()
	}
	return 0
}

// Run creates an org-specific runner and delegates the run to it.
func (f *RunnerFactory) Run(ctx context.Context, req RunRequest) RunResult {
	client, err := f.ClientFactory(ctx, req.OrganisationName)
	if err != nil {
		return RunResult{
			Error:        fmt.Errorf("nodekitchen: creating Chef API client for %s: %w", req.OrganisationName, err),
			ErrorMessage: fmt.Sprintf("Failed to create Chef API client for %s: %v", req.OrganisationName, err),
		}
	}

	runner := NewRunner(RunnerDeps{
		DB:             f.DB,
		RoleFetcher:    &chefAPIRoleFetcher{client: client},
		DepResolver:    f.DepResolver,
		Downloader:     &ChefServerDownloader{Client: client},
		GitLocator:     &FSGitCookbookLocator{BaseDir: f.GitCookbookDir},
		Executor:       f.Executor,
		CredResolver:   f.CredResolver,
		Logger:         f.Logger,
		TKConfigFn:     f.TKConfigFn,
		GitCookbookDir: f.GitCookbookDir,
		Concurrency:    f.concurrency(),
	})

	return runner.Run(ctx, req)
}

// chefAPIRoleFetcher adapts a chefapi.Client to the RoleFetcher interface.
type chefAPIRoleFetcher struct {
	client *chefapi.Client
}

func (f *chefAPIRoleFetcher) GetRole(ctx context.Context, name string) (*chefapi.RoleDetail, error) {
	return f.client.GetRole(ctx, name)
}

// DBCookbookDependencyResolver implements CookbookDependencyResolver by
// reading the dependencies JSON column from the server_cookbooks table.
type DBCookbookDependencyResolver struct {
	DB *datastore.DB
}

// GetCookbookDependencies returns the dependency map for a cookbook version
// by looking up its metadata in the server_cookbooks table.
func (r *DBCookbookDependencyResolver) GetCookbookDependencies(ctx context.Context, orgName, cookbookName, version string) (map[string]string, error) {
	cb, err := r.DB.GetServerCookbookByKey(ctx, orgName, cookbookName, version)
	if err != nil {
		return nil, fmt.Errorf("nodekitchen: looking up cookbook %s/%s/%s: %w", orgName, cookbookName, version, err)
	}

	if len(cb.Dependencies) == 0 || string(cb.Dependencies) == "null" {
		return nil, nil
	}

	var deps map[string]string
	if err := json.Unmarshal(cb.Dependencies, &deps); err != nil {
		return nil, fmt.Errorf("nodekitchen: parsing dependencies for %s/%s/%s: %w", orgName, cookbookName, version, err)
	}
	return deps, nil
}

// AnalysisCredentialAdapter adapts the analysis.ResolveKitchenCredentials
// function to the nodekitchen.CredentialResolver interface.
type AnalysisCredentialAdapter struct {
	Resolver *secrets.CredentialResolver
}

// ResolveKitchenCredentials resolves credentials via the analysis package
// and returns env vars + cleanup function matching the runner interface.
func (a *AnalysisCredentialAdapter) ResolveKitchenCredentials(ctx context.Context, tkConfig config.TestKitchenConfig) (map[string][]byte, func(), error) {
	kc, err := analysis.ResolveKitchenCredentials(ctx, a.Resolver, tkConfig)
	if err != nil {
		return nil, nil, err
	}
	return kc.EnvVars, kc.Cleanup, nil
}

// ScopedLoggerAdapter adapts a logging.ScopedLogger (which returns error
// and accepts variadic options) to the nodekitchen.Logger interface.
type ScopedLoggerAdapter struct {
	Info_  func(msg string)
	Warn_  func(msg string)
	Error_ func(msg string)
}

func (a *ScopedLoggerAdapter) Info(msg string)  { a.Info_(msg) }
func (a *ScopedLoggerAdapter) Warn(msg string)  { a.Warn_(msg) }
func (a *ScopedLoggerAdapter) Error(msg string) { a.Error_(msg) }

// DefaultExecutor implements KitchenExecutor by shelling out to the
// kitchen binary. It sanitises the environment to prevent Bundler
// interference from cookbook Gemfiles.
type DefaultExecutor struct {
	Path string // absolute path to the kitchen binary
}

// Run executes the kitchen binary with the given arguments.
func (e *DefaultExecutor) Run(ctx context.Context, dir string, extraEnv []string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, e.Path, args...)
	cmd.Dir = dir
	cmd.Env = append(sanitiseKitchenEnv(os.Environ()), extraEnv...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if ok := errors.As(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
			err = nil // process ran to completion, just non-zero exit
		}
	}
	return stdoutBuf.String(), stderrBuf.String(), exitCode, err
}

// sanitiseKitchenEnv removes Bundler-related variables that can interfere
// with the system-installed Test Kitchen binary.
func sanitiseKitchenEnv(environ []string) []string {
	drop := map[string]bool{
		"BUNDLE_GEMFILE":  true,
		"BUNDLE_BIN_PATH": true,
		"BUNDLE_PATH":     true,
		"RUBYOPT":         true,
	}
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if idx := strings.IndexByte(kv, '='); idx > 0 {
			if drop[kv[:idx]] {
				continue
			}
		}
		out = append(out, kv)
	}
	return out
}
