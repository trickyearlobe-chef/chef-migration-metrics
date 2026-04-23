// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package nodekitchen

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/chefapi"
)

// CookbookDownloader downloads a cookbook from the Chef Server into a directory.
type CookbookDownloader interface {
	// DownloadCookbook downloads all files for a cookbook version into destDir.
	DownloadCookbook(ctx context.Context, name, version, destDir string) error
}

// GitCookbookLocator finds a git-cloned cookbook's local directory.
type GitCookbookLocator interface {
	// LocateCookbook returns the absolute path to the cookbook directory
	// in the git clone area, or empty string if not available.
	LocateCookbook(name string) string
}

// AssemblyConfig holds configuration for cookbook assembly.
type AssemblyConfig struct {
	CookbookSource string // "server", "git", or "hybrid"
	OrgName        string
	GitCookbookDir string // base dir for git cookbook clones
	Concurrency    int    // max concurrent downloads
}

// WorkingDir represents the assembled working directory for a Node Kitchen run.
type WorkingDir struct {
	Path    string // absolute path to the temp directory
	Cleanup func() // removes the temp directory
}

// CreateWorkingDir creates a temporary working directory for a Node Kitchen run.
func CreateWorkingDir(nodeName string) (*WorkingDir, error) {
	ts := time.Now().Unix()
	dirName := fmt.Sprintf("cmm-node-kitchen-%s-%d", nodeName, ts)
	tmpDir := filepath.Join(os.TempDir(), dirName)

	if err := os.MkdirAll(filepath.Join(tmpDir, "cookbooks"), 0o755); err != nil {
		return nil, fmt.Errorf("nodekitchen: creating cookbooks dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "roles"), 0o755); err != nil {
		// Best-effort cleanup of partial creation.
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("nodekitchen: creating roles dir: %w", err)
	}

	return &WorkingDir{
		Path: tmpDir,
		Cleanup: func() {
			_ = os.RemoveAll(tmpDir)
		},
	}, nil
}

// AssembleCookbooks downloads or copies cookbooks into the working directory.
// The cookbooks map keys are cookbook names, values are version strings.
func AssembleCookbooks(
	ctx context.Context,
	workDir string,
	cookbooks map[string]string,
	cfg AssemblyConfig,
	downloader CookbookDownloader,
	gitLocator GitCookbookLocator,
) error {
	if len(cookbooks) == 0 {
		return nil
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}

	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var firstErr error

	var wg sync.WaitGroup
	for name, version := range cookbooks {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(name, version string) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			mu.Lock()
			if firstErr != nil {
				mu.Unlock()
				return
			}
			mu.Unlock()

			destDir := filepath.Join(workDir, "cookbooks", name)
			err := assembleSingleCookbook(ctx, name, version, destDir, cfg, downloader, gitLocator)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(name, version)
	}
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func assembleSingleCookbook(
	ctx context.Context,
	name, version, destDir string,
	cfg AssemblyConfig,
	downloader CookbookDownloader,
	gitLocator GitCookbookLocator,
) error {
	switch cfg.CookbookSource {
	case "server":
		return downloadFromServer(ctx, name, version, destDir, downloader)
	case "git":
		return copyFromGit(name, destDir, gitLocator)
	case "hybrid":
		if gitLocator != nil {
			src := gitLocator.LocateCookbook(name)
			if src != "" {
				return copyDir(src, destDir)
			}
		}
		return downloadFromServer(ctx, name, version, destDir, downloader)
	default:
		return fmt.Errorf("nodekitchen: unknown cookbook source %q", cfg.CookbookSource)
	}
}

func downloadFromServer(ctx context.Context, name, version, destDir string, downloader CookbookDownloader) error {
	if downloader == nil {
		return fmt.Errorf("nodekitchen: no downloader configured for cookbook %s/%s", name, version)
	}
	return downloader.DownloadCookbook(ctx, name, version, destDir)
}

func copyFromGit(name, destDir string, gitLocator GitCookbookLocator) error {
	if gitLocator == nil {
		return fmt.Errorf("nodekitchen: no git locator configured for cookbook %s", name)
	}
	src := gitLocator.LocateCookbook(name)
	if src == "" {
		return fmt.Errorf("nodekitchen: cookbook %s not found in git clone area", name)
	}
	return copyDir(src, destDir)
}

// roleJSON is the structure written to role JSON files.
type roleJSON struct {
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	RunList            []string               `json:"run_list"`
	EnvRunLists        map[string][]string    `json:"env_run_lists"`
	DefaultAttributes  map[string]interface{} `json:"default_attributes"`
	OverrideAttributes map[string]interface{} `json:"override_attributes"`
	JSONClass          string                 `json:"json_class"`
	ChefType           string                 `json:"chef_type"`
}

// WriteRoles fetches roles and writes them as JSON files into the working directory.
func WriteRoles(ctx context.Context, workDir string, roleNames []string, roleFetcher RoleFetcher) error {
	for _, name := range roleNames {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("nodekitchen: role writing cancelled: %w", err)
		}

		role, err := roleFetcher.GetRole(ctx, name)
		if err != nil {
			return fmt.Errorf("nodekitchen: fetching role %q for assembly: %w", name, err)
		}

		envRunLists := role.EnvRunLists
		if envRunLists == nil {
			envRunLists = make(map[string][]string)
		}
		runList := role.RunList
		if runList == nil {
			runList = []string{}
		}

		rj := roleJSON{
			Name:               role.Name,
			Description:        role.Description,
			RunList:            runList,
			EnvRunLists:        envRunLists,
			DefaultAttributes:  map[string]interface{}{},
			OverrideAttributes: map[string]interface{}{},
			JSONClass:          "Chef::Role",
			ChefType:           "role",
		}

		data, err := json.MarshalIndent(rj, "", "  ")
		if err != nil {
			return fmt.Errorf("nodekitchen: marshalling role %q: %w", name, err)
		}

		dest := filepath.Join(workDir, "roles", name+".json")
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return fmt.Errorf("nodekitchen: writing role file %q: %w", dest, err)
		}
	}
	return nil
}

// copyDir recursively copies directory contents from src to dst, skipping
// .git directories.
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("nodekitchen: stat source %q: %w", src, err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("nodekitchen: source %q is not a directory", src)
	}

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return fmt.Errorf("nodekitchen: creating destination %q: %w", dst, err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("nodekitchen: reading directory %q: %w", src, err)
	}

	for _, entry := range entries {
		if entry.Name() == ".git" {
			continue
		}

		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if err := copyFile(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("nodekitchen: opening %q: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("nodekitchen: creating %q: %w", dst, err)
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("nodekitchen: copying %q to %q: %w", src, dst, err)
	}
	return out.Close()
}

// ChefServerDownloader implements CookbookDownloader using the Chef Server API.
type ChefServerDownloader struct {
	Client *chefapi.Client
}

// DownloadCookbook downloads all files for a cookbook version into destDir.
func (d *ChefServerDownloader) DownloadCookbook(ctx context.Context, name, version, destDir string) error {
	manifest, err := d.Client.GetCookbookVersionManifest(ctx, name, version)
	if err != nil {
		return fmt.Errorf("nodekitchen: getting manifest for %s/%s: %w", name, version, err)
	}

	for _, ref := range manifest.AllFiles() {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("nodekitchen: download cancelled for %s/%s: %w", name, version, err)
		}

		data, err := d.Client.DownloadFileContent(ctx, ref.URL, ref.Checksum)
		if err != nil {
			return fmt.Errorf("nodekitchen: downloading file %q for %s/%s: %w", ref.Path, name, version, err)
		}

		filePath := filepath.Join(destDir, ref.Path)
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return fmt.Errorf("nodekitchen: creating dir for %q: %w", filePath, err)
		}
		if err := os.WriteFile(filePath, data, 0o644); err != nil {
			return fmt.Errorf("nodekitchen: writing file %q: %w", filePath, err)
		}
	}
	return nil
}

// FSGitCookbookLocator implements GitCookbookLocator using the filesystem.
type FSGitCookbookLocator struct {
	BaseDir string // e.g. /data/git-cookbooks
}

// LocateCookbook returns the absolute path to the cookbook directory in the
// git clone area, or empty string if not available.
func (l *FSGitCookbookLocator) LocateCookbook(name string) string {
	if l.BaseDir == "" {
		return ""
	}
	p := filepath.Join(l.BaseDir, name)
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return ""
	}
	return p
}
