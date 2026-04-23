// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// RunRequest holds the input parameters for a Node Kitchen run.
type RunRequest struct {
	NodeName          string `json:"node_name"`
	OrganisationName  string `json:"organisation_name"`
	TargetChefVersion string `json:"target_chef_version"`
	CookbookSource    string `json:"cookbook_source"` // "server", "git", "hybrid"
}

// RunResult holds the outcome of a Node Kitchen run.
type RunResult struct {
	ID              string
	ConvergePassed  *bool
	VerifyPassed    *bool
	ConvergeOutput  string
	VerifyOutput    string
	DestroyOutput   string
	DurationSeconds *int
	ErrorMessage    string
	Error           error // non-nil for infrastructure errors (not kitchen failures)
}

// RunnerDeps bundles all external dependencies the runner needs.
// Using a struct avoids a constructor with 10+ parameters.
type RunnerDeps struct {
	DB             DataStore
	RoleFetcher    RoleFetcher
	DepResolver    CookbookDependencyResolver
	Downloader     CookbookDownloader
	GitLocator     GitCookbookLocator
	Executor       KitchenExecutor
	CredResolver   CredentialResolver
	Logger         Logger
	TKConfig       config.TestKitchenConfig
	GitCookbookDir string
	Concurrency    int
}

// DataStore is the subset of datastore.DB needed by the runner.
type DataStore interface {
	GetNodeSnapshotByName(ctx context.Context, organisationName, nodeName string) (datastore.NodeSnapshot, error)
	UpsertNodeKitchenRun(ctx context.Context, p datastore.UpsertNodeKitchenRunParams) (datastore.NodeKitchenRun, error)
	UpdateNodeKitchenRunResult(ctx context.Context, id string, p datastore.UpdateNodeKitchenRunResultParams) (datastore.NodeKitchenRun, error)
}

// KitchenExecutor is the same interface from analysis/kitchen.go.
// Redeclared here to avoid coupling the runner to the analysis package.
type KitchenExecutor interface {
	Run(ctx context.Context, dir string, extraEnv []string, args ...string) (stdout, stderr string, exitCode int, err error)
}

// CredentialResolver abstracts credential resolution for kitchen runs.
type CredentialResolver interface {
	ResolveKitchenCredentials(ctx context.Context, tkConfig config.TestKitchenConfig) (envVars map[string][]byte, cleanup func(), err error)
}

// Logger abstracts logging for the runner.
type Logger interface {
	Info(msg string)
	Warn(msg string)
	Error(msg string)
}

// Runner orchestrates a Node Kitchen run end-to-end.
type Runner struct {
	deps RunnerDeps
}

// NewRunner creates a Runner with the given dependencies.
func NewRunner(deps RunnerDeps) *Runner {
	return &Runner{deps: deps}
}

// Run executes a Node Kitchen run synchronously. This is typically called
// from a goroutine by the API handler. The context should have a timeout.
func (r *Runner) Run(ctx context.Context, req RunRequest) RunResult {
	startedAt := time.Now()
	var result RunResult

	// Step 1: Validate request.
	if err := validateRequest(req); err != nil {
		result.Error = err
		result.ErrorMessage = err.Error()
		return result
	}
	r.deps.Logger.Info(fmt.Sprintf("starting node kitchen run for %s/%s", req.OrganisationName, req.NodeName))

	// Step 2: Fetch node snapshot.
	r.deps.Logger.Info(fmt.Sprintf("fetching node snapshot for %s/%s", req.OrganisationName, req.NodeName))
	node, err := r.deps.DB.GetNodeSnapshotByName(ctx, req.OrganisationName, req.NodeName)
	if err != nil {
		result.Error = fmt.Errorf("nodekitchen: fetching node snapshot: %w", err)
		result.ErrorMessage = result.Error.Error()
		return result
	}

	// Step 3: Parse run_list.
	r.deps.Logger.Info("parsing run_list")
	entries, err := ParseRunList(node.RunList)
	if err != nil {
		result.Error = err
		result.ErrorMessage = err.Error()
		return result
	}

	// Step 4: Expand run_list.
	r.deps.Logger.Info("expanding run_list")
	expandedEntries, err := ExpandRunList(ctx, entries, r.deps.RoleFetcher)
	if err != nil {
		result.Error = err
		result.ErrorMessage = err.Error()
		return result
	}

	// Step 5: Parse node cookbooks.
	r.deps.Logger.Info("parsing node cookbooks")
	nodeCookbooks, err := ParseNodeCookbooks(node.Cookbooks)
	if err != nil {
		result.Error = err
		result.ErrorMessage = err.Error()
		return result
	}

	// Step 6: Resolve cookbook set.
	r.deps.Logger.Info("resolving cookbook set")
	cookbooks, err := ResolveCookbookSet(ctx, expandedEntries, nodeCookbooks, r.deps.DepResolver, req.OrganisationName)
	if err != nil {
		result.Error = err
		result.ErrorMessage = err.Error()
		return result
	}

	// Step 7: Upsert initial DB record.
	r.deps.Logger.Info("creating kitchen run record")
	runListStrings := formatEntries(expandedEntries)
	runListJSON, _ := json.Marshal(runListStrings)
	cookbookVersionsJSON, _ := json.Marshal(cookbooks)
	dbRun, err := r.deps.DB.UpsertNodeKitchenRun(ctx, datastore.UpsertNodeKitchenRunParams{
		NodeName:          req.NodeName,
		OrganisationName:  req.OrganisationName,
		TargetChefVersion: req.TargetChefVersion,
		CookbookSource:    req.CookbookSource,
		PlatformName:      node.Platform,
		RunList:           runListJSON,
		CookbookVersions:  cookbookVersionsJSON,
		StartedAt:         &startedAt,
	})
	if err != nil {
		result.Error = fmt.Errorf("nodekitchen: upserting kitchen run record: %w", err)
		result.ErrorMessage = result.Error.Error()
		return result
	}
	result.ID = dbRun.ID

	// From here on, always update the DB record before returning.

	// Step 8: Create working directory.
	r.deps.Logger.Info("creating working directory")
	workDir, err := CreateWorkingDir(req.NodeName)
	if err != nil {
		r.failRun(ctx, &result, startedAt, err)
		return result
	}
	defer workDir.Cleanup()

	// Step 9: Assemble cookbooks.
	r.deps.Logger.Info(fmt.Sprintf("assembling %d cookbooks (source=%s)", len(cookbooks), req.CookbookSource))
	assemblyCfg := AssemblyConfig{
		CookbookSource: req.CookbookSource,
		OrgName:        req.OrganisationName,
		GitCookbookDir: r.deps.GitCookbookDir,
		Concurrency:    r.deps.Concurrency,
	}
	if err := AssembleCookbooks(ctx, workDir.Path, cookbooks, assemblyCfg, r.deps.Downloader, r.deps.GitLocator); err != nil {
		r.failRun(ctx, &result, startedAt, fmt.Errorf("nodekitchen: assembling cookbooks: %w", err))
		return result
	}

	// Step 10: Write roles.
	roleNames := collectRoleNames(entries)
	if len(roleNames) > 0 {
		r.deps.Logger.Info(fmt.Sprintf("writing %d roles", len(roleNames)))
		if err := WriteRoles(ctx, workDir.Path, roleNames, r.deps.RoleFetcher); err != nil {
			r.failRun(ctx, &result, startedAt, err)
			return result
		}
	}

	// Step 11: Generate kitchen config.
	r.deps.Logger.Info("generating kitchen configuration")
	genCfg := KitchenGenConfig{
		NodeName:          req.NodeName,
		PlatformName:      node.Platform,
		PlatformVersion:   node.PlatformVersion,
		RunList:           runListStrings,
		TargetChefVersion: req.TargetChefVersion,
		CustomAttributes:  node.CustomAttributes,
		HasRoles:          len(roleNames) > 0,
	}
	kitchenYML, err := GenerateKitchenYML(genCfg)
	if err != nil {
		r.failRun(ctx, &result, startedAt, err)
		return result
	}

	// Step 12: Generate overlay.
	overlay, err := GenerateOverlay(&r.deps.TKConfig, node.Platform)
	if err != nil {
		r.failRun(ctx, &result, startedAt, err)
		return result
	}

	// Step 13: Write configs.
	r.deps.Logger.Info("writing kitchen configs")
	if err := WriteKitchenConfigs(workDir.Path, kitchenYML, overlay); err != nil {
		r.failRun(ctx, &result, startedAt, err)
		return result
	}

	// Step 14: Resolve credentials.
	var credEnv []string
	if r.deps.CredResolver != nil {
		r.deps.Logger.Info("resolving credentials")
		envVars, cleanup, err := r.deps.CredResolver.ResolveKitchenCredentials(ctx, r.deps.TKConfig)
		if err != nil {
			r.deps.Logger.Warn(fmt.Sprintf("credential resolution failed, proceeding without credentials: %v", err))
		} else {
			if cleanup != nil {
				defer cleanup()
			}
			credEnv = credEnvSlice(envVars)
		}
	}

	// Step 15: Run converge.
	r.deps.Logger.Info("running kitchen converge")
	convergeOutput, convergePassed, convergeErr := r.runPhase(ctx, workDir.Path, credEnv, "converge")
	result.ConvergeOutput = convergeOutput
	result.ConvergePassed = boolPtr(convergePassed)

	// Step 16: Run verify (only if converge passed).
	if convergePassed {
		r.deps.Logger.Info("running kitchen verify")
		verifyOutput, verifyPassed, _ := r.runPhase(ctx, workDir.Path, credEnv, "verify")
		result.VerifyOutput = verifyOutput
		result.VerifyPassed = boolPtr(verifyPassed)
	}

	// Step 17: Run destroy (always, fresh 5-minute context).
	r.deps.Logger.Info("running kitchen destroy")
	destroyCtx, destroyCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer destroyCancel()
	destroyOutput, _, _ := r.runPhase(destroyCtx, workDir.Path, credEnv, "destroy")
	result.DestroyOutput = destroyOutput

	// Step 18: Update DB record with final results.
	duration := int(time.Since(startedAt).Seconds())
	result.DurationSeconds = intPtr(duration)
	completedAt := time.Now()
	if convergeErr != nil {
		result.ErrorMessage = convergeErr.Error()
		result.Error = convergeErr
	}

	updateCtx, updateCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer updateCancel()
	_, _ = r.deps.DB.UpdateNodeKitchenRunResult(updateCtx, result.ID, datastore.UpdateNodeKitchenRunResultParams{
		ConvergePassed:  result.ConvergePassed,
		VerifyPassed:    result.VerifyPassed,
		ConvergeOutput:  result.ConvergeOutput,
		VerifyOutput:    result.VerifyOutput,
		DestroyOutput:   result.DestroyOutput,
		DurationSeconds: result.DurationSeconds,
		ErrorMessage:    result.ErrorMessage,
		CompletedAt:     &completedAt,
	})

	r.deps.Logger.Info(fmt.Sprintf("node kitchen run completed for %s/%s (converge=%v)", req.OrganisationName, req.NodeName, convergePassed))
	return result
}

// runPhase executes a single kitchen phase (converge, verify, or destroy).
// It returns the combined stdout+stderr output, whether the phase passed
// (exit code 0 with no infrastructure error), and any infrastructure error.
func (r *Runner) runPhase(ctx context.Context, dir string, extraEnv []string, phase string) (output string, passed bool, err error) {
	args := []string{phase, "--concurrency=1", "--log-level=info"}

	stdout, stderr, exitCode, execErr := r.deps.Executor.Run(ctx, dir, extraEnv, args...)

	combined := stdout
	if stderr != "" {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr
	}

	if execErr != nil {
		return combined, false, execErr
	}
	return combined, exitCode == 0, nil
}

// failRun records an infrastructure error in the result and updates the DB
// record. Used for failures in steps 8-13 (after the DB record exists).
func (r *Runner) failRun(ctx context.Context, result *RunResult, startedAt time.Time, err error) {
	result.Error = err
	result.ErrorMessage = err.Error()
	duration := int(time.Since(startedAt).Seconds())
	result.DurationSeconds = intPtr(duration)
	completedAt := time.Now()

	if result.ID != "" {
		updateCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = r.deps.DB.UpdateNodeKitchenRunResult(updateCtx, result.ID, datastore.UpdateNodeKitchenRunResultParams{
			ErrorMessage:    result.ErrorMessage,
			DurationSeconds: intPtr(duration),
			CompletedAt:     &completedAt,
		})
	}
}

// validateRequest checks that all required fields are present and valid.
func validateRequest(req RunRequest) error {
	if req.NodeName == "" {
		return fmt.Errorf("nodekitchen: node_name is required")
	}
	if req.OrganisationName == "" {
		return fmt.Errorf("nodekitchen: organisation_name is required")
	}
	if req.TargetChefVersion == "" {
		return fmt.Errorf("nodekitchen: target_chef_version is required")
	}
	switch req.CookbookSource {
	case "server", "git", "hybrid":
		// Valid.
	default:
		return fmt.Errorf("nodekitchen: invalid cookbook_source %q (must be server, git, or hybrid)", req.CookbookSource)
	}
	return nil
}

// collectRoleNames returns the unique role names from the original
// (unexpanded) run_list entries, preserving first-seen order.
func collectRoleNames(entries []RunListEntry) []string {
	seen := make(map[string]bool)
	var names []string
	for _, e := range entries {
		if e.Type == "role" && !seen[e.Name] {
			seen[e.Name] = true
			names = append(names, e.Name)
		}
	}
	return names
}

// formatEntries converts expanded RunListEntry values to formatted strings
// suitable for the kitchen run_list and DB storage.
func formatEntries(entries []RunListEntry) []string {
	s := make([]string, len(entries))
	for i, e := range entries {
		s[i] = FormatRunListEntry(e)
	}
	return s
}

// credEnvSlice converts credential env vars from map[string][]byte to a
// slice of KEY=VALUE strings for process injection.
func credEnvSlice(envVars map[string][]byte) []string {
	if len(envVars) == 0 {
		return nil
	}
	env := make([]string, 0, len(envVars))
	for k, v := range envVars {
		env = append(env, k+"="+string(v))
	}
	return env
}

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }
