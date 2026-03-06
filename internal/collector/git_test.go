// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package collector

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Fake GitExecutor for testing
// ---------------------------------------------------------------------------

// fakeGitCall records a single invocation of the fake executor.
type fakeGitCall struct {
	Dir  string
	Args []string
}

// fakeGitResponse defines the output and error for a specific command match.
type fakeGitResponse struct {
	// MatchArgs is compared against the actual args (joined with spaces)
	// using strings.Contains for flexible matching.
	MatchArgs string
	Output    string
	Err       error
}

// fakeGitExecutor records calls and returns pre-configured responses. It is
// safe for concurrent use.
type fakeGitExecutor struct {
	mu        sync.Mutex
	calls     []fakeGitCall
	responses []fakeGitResponse
	// defaultOutput is returned when no response matches.
	defaultOutput string
	// defaultErr is returned when no response matches and defaultOutput is empty.
	defaultErr error
}

func newFakeGitExecutor() *fakeGitExecutor {
	return &fakeGitExecutor{}
}

func (f *fakeGitExecutor) addResponse(matchArgs, output string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = append(f.responses, fakeGitResponse{
		MatchArgs: matchArgs,
		Output:    output,
		Err:       err,
	})
}

func (f *fakeGitExecutor) Run(_ context.Context, dir string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, fakeGitCall{Dir: dir, Args: args})

	joined := strings.Join(args, " ")
	for _, r := range f.responses {
		if strings.Contains(joined, r.MatchArgs) {
			return r.Output, r.Err
		}
	}

	if f.defaultErr != nil {
		return "", f.defaultErr
	}
	return f.defaultOutput, nil
}

func (f *fakeGitExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeGitExecutor) getCalls() []fakeGitCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]fakeGitCall, len(f.calls))
	copy(cp, f.calls)
	return cp
}

// ---------------------------------------------------------------------------
// isGitRepo tests
// ---------------------------------------------------------------------------

func TestIsGitRepo_NonExistentDir(t *testing.T) {
	if isGitRepo("/nonexistent/path/that/does/not/exist") {
		t.Error("expected isGitRepo to return false for non-existent directory")
	}
}

func TestIsGitRepo_ExistingDirWithoutGit(t *testing.T) {
	dir := t.TempDir()
	if isGitRepo(dir) {
		t.Error("expected isGitRepo to return false for directory without .git")
	}
}

// ---------------------------------------------------------------------------
// GitCookbookManager — detectDefaultBranch tests
// ---------------------------------------------------------------------------

func TestDetectDefaultBranch_SymbolicRef(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("symbolic-ref", "origin/main", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	branch, err := mgr.detectDefaultBranch(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected branch %q, got %q", "main", branch)
	}
}

func TestDetectDefaultBranch_SymbolicRefMaster(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("symbolic-ref", "origin/master", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	branch, err := mgr.detectDefaultBranch(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "master" {
		t.Errorf("expected branch %q, got %q", "master", branch)
	}
}

func TestDetectDefaultBranch_FallbackMain(t *testing.T) {
	fake := newFakeGitExecutor()
	// symbolic-ref fails
	fake.addResponse("symbolic-ref", "", fmt.Errorf("not a symbolic ref"))
	// rev-parse --verify origin/main succeeds
	fake.addResponse("rev-parse --verify origin/main", "abc123", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	branch, err := mgr.detectDefaultBranch(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected branch %q, got %q", "main", branch)
	}
}

func TestDetectDefaultBranch_FallbackMaster(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("symbolic-ref", "", fmt.Errorf("not a symbolic ref"))
	fake.addResponse("rev-parse --verify origin/main", "", fmt.Errorf("unknown revision"))
	fake.addResponse("rev-parse --verify origin/master", "def456", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	branch, err := mgr.detectDefaultBranch(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "master" {
		t.Errorf("expected branch %q, got %q", "master", branch)
	}
}

func TestDetectDefaultBranch_NeitherExists(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("symbolic-ref", "", fmt.Errorf("not a symbolic ref"))
	fake.addResponse("rev-parse --verify origin/main", "", fmt.Errorf("unknown revision"))
	fake.addResponse("rev-parse --verify origin/master", "", fmt.Errorf("unknown revision"))

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	_, err := mgr.detectDefaultBranch(context.Background(), "/repo")
	if err == nil {
		t.Fatal("expected error when neither main nor master exists")
	}
	if !strings.Contains(err.Error(), "could not detect default branch") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GitCookbookManager — readHeadSHA tests
// ---------------------------------------------------------------------------

func TestReadHeadSHA_Success(t *testing.T) {
	sha := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	fake := newFakeGitExecutor()
	fake.addResponse("rev-parse HEAD", sha, nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	got, err := mgr.readHeadSHA(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != sha {
		t.Errorf("expected SHA %q, got %q", sha, got)
	}
}

func TestReadHeadSHA_TruncatesLongOutput(t *testing.T) {
	// Some git implementations may append extra data; we take only the first 40 chars.
	sha := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2extra-stuff"
	fake := newFakeGitExecutor()
	fake.addResponse("rev-parse HEAD", sha, nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	got, err := mgr.readHeadSHA(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 40 {
		t.Errorf("expected 40-char SHA, got %d chars: %q", len(got), got)
	}
}

func TestReadHeadSHA_TooShort(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("rev-parse HEAD", "short", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	_, err := mgr.readHeadSHA(context.Background(), "/repo")
	if err == nil {
		t.Fatal("expected error for short rev-parse output")
	}
	if !strings.Contains(err.Error(), "unexpected rev-parse output") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReadHeadSHA_GitError(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("rev-parse HEAD", "", fmt.Errorf("not a git repository"))

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	_, err := mgr.readHeadSHA(context.Background(), "/repo")
	if err == nil {
		t.Fatal("expected error from failed git command")
	}
}

// ---------------------------------------------------------------------------
// GitCookbookManager — detectTestSuite tests
// ---------------------------------------------------------------------------

func TestDetectTestSuite_KitchenYml(t *testing.T) {
	fake := newFakeGitExecutor()
	// First indicator (.kitchen.yml) matches
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yml", ".kitchen.yml", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	if !mgr.detectTestSuite(context.Background(), "/repo") {
		t.Error("expected test suite to be detected when .kitchen.yml exists")
	}
}

func TestDetectTestSuite_TestDir(t *testing.T) {
	fake := newFakeGitExecutor()
	// All indicators fail except "test"
	fake.defaultErr = fmt.Errorf("not found")
	// Override for "test" path
	fake.responses = nil
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yaml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- kitchen.yml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- kitchen.yaml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- test", "test", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	if !mgr.detectTestSuite(context.Background(), "/repo") {
		t.Error("expected test suite to be detected when test/ exists")
	}
}

func TestDetectTestSuite_SpecDir(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yaml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- kitchen.yml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- kitchen.yaml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- test", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- spec", "spec", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	if !mgr.detectTestSuite(context.Background(), "/repo") {
		t.Error("expected test suite to be detected when spec/ exists")
	}
}

func TestDetectTestSuite_None(t *testing.T) {
	fake := newFakeGitExecutor()
	// All indicators fail
	fake.defaultErr = fmt.Errorf("not found")

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	if mgr.detectTestSuite(context.Background(), "/repo") {
		t.Error("expected no test suite when none of the indicator paths exist")
	}
}

func TestDetectTestSuite_EmptyOutput(t *testing.T) {
	// git ls-tree succeeds but returns empty output — means path doesn't exist.
	fake := newFakeGitExecutor()
	fake.defaultOutput = ""

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	if mgr.detectTestSuite(context.Background(), "/repo") {
		t.Error("expected no test suite when ls-tree returns empty output")
	}
}

// ---------------------------------------------------------------------------
// GitCookbookManager — CloneOrPull tests (clone path)
// ---------------------------------------------------------------------------

func TestCloneOrPull_Clone_Success(t *testing.T) {
	sha := "abcdef1234567890abcdef1234567890abcdef12"
	fake := newFakeGitExecutor()
	fake.addResponse("clone", "", nil)
	fake.addResponse("symbolic-ref", "origin/main", nil)
	fake.addResponse("rev-parse HEAD", sha, nil)
	// No test suite indicators
	fake.addResponse("ls-tree", "", fmt.Errorf("not found"))

	baseDir := t.TempDir()
	mgr := NewGitCookbookManager(baseDir, fake)

	result, err := mgr.CloneOrPull(context.Background(), "nginx", "https://github.com/myorg/nginx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.CookbookName != "nginx" {
		t.Errorf("expected CookbookName %q, got %q", "nginx", result.CookbookName)
	}
	if result.RepoURL != "https://github.com/myorg/nginx" {
		t.Errorf("expected RepoURL %q, got %q", "https://github.com/myorg/nginx", result.RepoURL)
	}
	if !result.WasCloned {
		t.Error("expected WasCloned to be true")
	}
	if !result.Changed {
		t.Error("expected Changed to be true on fresh clone")
	}
	if result.DefaultBranch != "main" {
		t.Errorf("expected DefaultBranch %q, got %q", "main", result.DefaultBranch)
	}
	if result.HeadCommitSHA != sha {
		t.Errorf("expected HeadCommitSHA %q, got %q", sha, result.HeadCommitSHA)
	}
	if result.Err != nil {
		t.Errorf("expected nil Err, got %v", result.Err)
	}
}

func TestCloneOrPull_Clone_Failure(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("clone", "", fmt.Errorf("repository not found"))

	baseDir := t.TempDir()
	mgr := NewGitCookbookManager(baseDir, fake)

	result, err := mgr.CloneOrPull(context.Background(), "nonexistent", "https://example.com/nonexistent")
	if err == nil {
		t.Fatal("expected error on clone failure")
	}
	if result.Err == nil {
		t.Error("expected result.Err to be set")
	}
	if !strings.Contains(err.Error(), "clone") {
		t.Errorf("expected error to mention clone, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GitCookbookManager — CloneOrPull tests (pull path)
// ---------------------------------------------------------------------------

func TestCloneOrPull_Pull_Changed(t *testing.T) {
	oldSHA := "1111111111111111111111111111111111111111"
	newSHA := "2222222222222222222222222222222222222222"

	fake := newFakeGitExecutor()
	fake.addResponse("fetch", "", nil)
	fake.addResponse("symbolic-ref", "origin/main", nil)
	fake.addResponse("reset --hard", "", nil)
	fake.addResponse("ls-tree", "", fmt.Errorf("not found"))

	// rev-parse HEAD is called twice: once before reset (old SHA) and once after.
	// We use a counter to return different values.
	revParseCount := 0
	revParseMu := sync.Mutex{}
	originalRun := fake.Run
	_ = originalRun // suppress unused warning; we build a custom executor below

	// Build a more sophisticated fake that tracks rev-parse call count.
	customFake := &countingGitExecutor{
		base:     fake,
		oldSHA:   oldSHA,
		newSHA:   newSHA,
		revCount: &revParseCount,
		mu:       &revParseMu,
	}

	// Create a temp dir with a .git subdirectory to simulate existing repo.
	baseDir := t.TempDir()
	repoDir := baseDir + "/mybook"
	if err := createFakeGitDir(repoDir); err != nil {
		t.Fatalf("failed to create fake .git dir: %v", err)
	}

	mgr := NewGitCookbookManager(baseDir, customFake)
	result, err := mgr.CloneOrPull(context.Background(), "mybook", "https://github.com/myorg/mybook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.WasCloned {
		t.Error("expected WasCloned to be false for pull")
	}
	if !result.Changed {
		t.Error("expected Changed to be true when SHA changed")
	}
	if result.HeadCommitSHA != newSHA {
		t.Errorf("expected HeadCommitSHA %q, got %q", newSHA, result.HeadCommitSHA)
	}
}

func TestCloneOrPull_Pull_Unchanged(t *testing.T) {
	sameSHA := "3333333333333333333333333333333333333333"

	fake := &countingGitExecutor{
		base:     newFakeGitExecutor(),
		oldSHA:   sameSHA,
		newSHA:   sameSHA,
		revCount: new(int),
		mu:       &sync.Mutex{},
	}
	fake.base.addResponse("fetch", "", nil)
	fake.base.addResponse("symbolic-ref", "origin/main", nil)
	fake.base.addResponse("reset --hard", "", nil)
	fake.base.addResponse("ls-tree", "", fmt.Errorf("not found"))

	baseDir := t.TempDir()
	repoDir := baseDir + "/mybook"
	if err := createFakeGitDir(repoDir); err != nil {
		t.Fatalf("failed to create fake .git dir: %v", err)
	}

	mgr := NewGitCookbookManager(baseDir, fake)
	result, err := mgr.CloneOrPull(context.Background(), "mybook", "https://github.com/myorg/mybook")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.WasCloned {
		t.Error("expected WasCloned to be false")
	}
	if result.Changed {
		t.Error("expected Changed to be false when SHA unchanged")
	}
}

func TestCloneOrPull_Pull_FetchFails(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("fetch", "", fmt.Errorf("network error"))

	baseDir := t.TempDir()
	repoDir := baseDir + "/mybook"
	if err := createFakeGitDir(repoDir); err != nil {
		t.Fatalf("failed to create fake .git dir: %v", err)
	}

	mgr := NewGitCookbookManager(baseDir, fake)
	_, err := mgr.CloneOrPull(context.Background(), "mybook", "https://example.com/mybook")
	if err == nil {
		t.Fatal("expected error on fetch failure")
	}
	if !strings.Contains(err.Error(), "fetch") {
		t.Errorf("expected error to mention fetch, got: %v", err)
	}
}

func TestCloneOrPull_Pull_DetectBranchFails(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("fetch", "", nil)
	// All branch detection methods fail
	fake.addResponse("symbolic-ref", "", fmt.Errorf("no symbolic ref"))
	fake.addResponse("rev-parse --verify origin/main", "", fmt.Errorf("not found"))
	fake.addResponse("rev-parse --verify origin/master", "", fmt.Errorf("not found"))

	baseDir := t.TempDir()
	repoDir := baseDir + "/mybook"
	if err := createFakeGitDir(repoDir); err != nil {
		t.Fatalf("failed to create fake .git dir: %v", err)
	}

	mgr := NewGitCookbookManager(baseDir, fake)
	_, err := mgr.CloneOrPull(context.Background(), "mybook", "https://example.com/mybook")
	if err == nil {
		t.Fatal("expected error when default branch detection fails")
	}
	if !strings.Contains(err.Error(), "detect default branch") {
		t.Errorf("expected error to mention branch detection, got: %v", err)
	}
}

func TestCloneOrPull_Pull_ResetFails(t *testing.T) {
	fake := &countingGitExecutor{
		base:     newFakeGitExecutor(),
		oldSHA:   "1111111111111111111111111111111111111111",
		newSHA:   "2222222222222222222222222222222222222222",
		revCount: new(int),
		mu:       &sync.Mutex{},
	}
	fake.base.addResponse("fetch", "", nil)
	fake.base.addResponse("symbolic-ref", "origin/main", nil)
	fake.base.addResponse("reset --hard", "", fmt.Errorf("reset failed: dirty worktree"))

	baseDir := t.TempDir()
	repoDir := baseDir + "/mybook"
	if err := createFakeGitDir(repoDir); err != nil {
		t.Fatalf("failed to create fake .git dir: %v", err)
	}

	mgr := NewGitCookbookManager(baseDir, fake)
	_, err := mgr.CloneOrPull(context.Background(), "mybook", "https://example.com/mybook")
	if err == nil {
		t.Fatal("expected error on reset failure")
	}
	if !strings.Contains(err.Error(), "reset") {
		t.Errorf("expected error to mention reset, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// GitCookbookManager — CloneOrPull with test suite detection
// ---------------------------------------------------------------------------

func TestCloneOrPull_Clone_WithTestSuite(t *testing.T) {
	sha := "abcdef1234567890abcdef1234567890abcdef12"
	fake := newFakeGitExecutor()
	fake.addResponse("clone", "", nil)
	fake.addResponse("symbolic-ref", "origin/main", nil)
	fake.addResponse("rev-parse HEAD", sha, nil)
	// .kitchen.yml exists
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yml", ".kitchen.yml", nil)

	baseDir := t.TempDir()
	mgr := NewGitCookbookManager(baseDir, fake)

	result, err := mgr.CloneOrPull(context.Background(), "testcb", "https://github.com/org/testcb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasTestSuite {
		t.Error("expected HasTestSuite to be true when .kitchen.yml exists")
	}
}

// ---------------------------------------------------------------------------
// GitCookbookManager — context cancellation
// ---------------------------------------------------------------------------

func TestCloneOrPull_CancelledContext(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("clone", "", fmt.Errorf("signal: killed"))

	baseDir := t.TempDir()
	mgr := NewGitCookbookManager(baseDir, fake)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	_, err := mgr.CloneOrPull(ctx, "mycb", "https://example.com/mycb")
	// The clone may fail due to context or due to our fake error.
	// Either way, we should get an error.
	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// ---------------------------------------------------------------------------
// RepoDir test
// ---------------------------------------------------------------------------

func TestRepoDir(t *testing.T) {
	mgr := NewGitCookbookManager("/data/cookbooks", nil)
	got := mgr.RepoDir("nginx")
	want := "/data/cookbooks/nginx"
	if got != want {
		t.Errorf("RepoDir(%q) = %q, want %q", "nginx", got, want)
	}
}

// ---------------------------------------------------------------------------
// BuildGitCookbookURLs tests
// ---------------------------------------------------------------------------

func TestBuildGitCookbookURLs_MultipleBaseURLs(t *testing.T) {
	urls := BuildGitCookbookURLs("nginx", []string{
		"https://github.com/myorg",
		"https://gitlab.internal.com/chef-cookbooks",
	})

	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(urls))
	}
	if urls[0] != "https://github.com/myorg/nginx" {
		t.Errorf("urls[0] = %q, want %q", urls[0], "https://github.com/myorg/nginx")
	}
	if urls[1] != "https://gitlab.internal.com/chef-cookbooks/nginx" {
		t.Errorf("urls[1] = %q, want %q", urls[1], "https://gitlab.internal.com/chef-cookbooks/nginx")
	}
}

func TestBuildGitCookbookURLs_TrailingSlash(t *testing.T) {
	urls := BuildGitCookbookURLs("apache2", []string{
		"https://github.com/myorg/",
	})

	if len(urls) != 1 {
		t.Fatalf("expected 1 URL, got %d", len(urls))
	}
	if urls[0] != "https://github.com/myorg/apache2" {
		t.Errorf("urls[0] = %q, want %q", urls[0], "https://github.com/myorg/apache2")
	}
}

func TestBuildGitCookbookURLs_Empty(t *testing.T) {
	urls := BuildGitCookbookURLs("nginx", nil)
	if len(urls) != 0 {
		t.Errorf("expected 0 URLs, got %d", len(urls))
	}
}

// ---------------------------------------------------------------------------
// ResolveGitCookbookURL tests
// ---------------------------------------------------------------------------

func TestResolveGitCookbookURL_FirstBaseURL(t *testing.T) {
	got := ResolveGitCookbookURL("nginx", []string{
		"https://github.com/myorg",
		"https://gitlab.internal.com/cookbooks",
	})
	want := "https://github.com/myorg/nginx"
	if got != want {
		t.Errorf("ResolveGitCookbookURL = %q, want %q", got, want)
	}
}

func TestResolveGitCookbookURL_NoBaseURLs(t *testing.T) {
	got := ResolveGitCookbookURL("nginx", nil)
	if got != "" {
		t.Errorf("expected empty string for nil base URLs, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// GitFetchError tests
// ---------------------------------------------------------------------------

func TestGitFetchError_Error(t *testing.T) {
	gfe := GitFetchError{
		CookbookName: "nginx",
		RepoURL:      "https://github.com/myorg/nginx",
		Err:          fmt.Errorf("clone failed"),
	}

	got := gfe.Error()
	if !strings.Contains(got, "nginx") {
		t.Errorf("expected error to contain cookbook name, got: %s", got)
	}
	if !strings.Contains(got, "github.com") {
		t.Errorf("expected error to contain repo URL, got: %s", got)
	}
	if !strings.Contains(got, "clone failed") {
		t.Errorf("expected error to contain underlying error, got: %s", got)
	}
}

// ---------------------------------------------------------------------------
// GitRepoResult fields test
// ---------------------------------------------------------------------------

func TestGitRepoResult_Fields(t *testing.T) {
	result := GitRepoResult{
		CookbookName:  "mycb",
		RepoURL:       "https://example.com/mycb",
		DefaultBranch: "main",
		HeadCommitSHA: "abc123",
		HasTestSuite:  true,
		WasCloned:     false,
		Changed:       true,
	}

	if result.CookbookName != "mycb" {
		t.Error("CookbookName mismatch")
	}
	if result.DefaultBranch != "main" {
		t.Error("DefaultBranch mismatch")
	}
	if !result.HasTestSuite {
		t.Error("HasTestSuite should be true")
	}
	if result.WasCloned {
		t.Error("WasCloned should be false")
	}
	if !result.Changed {
		t.Error("Changed should be true")
	}
}

// ---------------------------------------------------------------------------
// GitFetchResult fields test
// ---------------------------------------------------------------------------

func TestGitFetchResult_ZeroValue(t *testing.T) {
	var r GitFetchResult
	if r.Total != 0 || r.Cloned != 0 || r.Pulled != 0 || r.Unchanged != 0 || r.Failed != 0 {
		t.Error("expected all counters to be zero")
	}
	if r.Duration != 0 {
		t.Error("expected zero Duration")
	}
	if len(r.Errors) != 0 {
		t.Error("expected empty Errors")
	}
}

// ---------------------------------------------------------------------------
// NewGitCookbookManager tests
// ---------------------------------------------------------------------------

func TestNewGitCookbookManager_DefaultExecutor(t *testing.T) {
	mgr := NewGitCookbookManager("/tmp/repos", nil)
	if mgr.executor == nil {
		t.Error("expected non-nil default executor")
	}
	if mgr.baseDir != "/tmp/repos" {
		t.Errorf("expected baseDir %q, got %q", "/tmp/repos", mgr.baseDir)
	}
}

func TestNewGitCookbookManager_CustomExecutor(t *testing.T) {
	fake := newFakeGitExecutor()
	mgr := NewGitCookbookManager("/data", fake)
	if mgr.executor != fake {
		t.Error("expected custom executor to be used")
	}
}

// ---------------------------------------------------------------------------
// minInt tests
// ---------------------------------------------------------------------------

func TestMinInt(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{5, 3, 3},
		{0, 0, 0},
		{-1, 1, -1},
		{42, 42, 42},
	}
	for _, tt := range tests {
		got := minInt(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("minInt(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// defaultGitExecutor — basic test (real git, if available)
// ---------------------------------------------------------------------------

func TestDefaultGitExecutor_Version(t *testing.T) {
	// This test verifies the defaultGitExecutor works with a real git binary.
	// It's skipped in environments without git.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH; skipping real executor test")
	}

	d := &defaultGitExecutor{}
	out, err := d.Run(context.Background(), "", "version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(out, "git version") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestDefaultGitExecutor_BadCommand(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH; skipping real executor test")
	}

	d := &defaultGitExecutor{}
	_, err := d.Run(context.Background(), "", "not-a-real-subcommand")
	if err == nil {
		t.Fatal("expected error for invalid git subcommand")
	}
}

// ---------------------------------------------------------------------------
// detectDefaultBranch — symbolic-ref returns empty after stripping prefix
// ---------------------------------------------------------------------------

func TestDetectDefaultBranch_SymbolicRefEmptyAfterStrip(t *testing.T) {
	fake := newFakeGitExecutor()
	// Weird edge case: symbolic-ref returns just "origin/" with nothing after.
	fake.addResponse("symbolic-ref", "origin/", nil)
	// Fallback should still work.
	fake.addResponse("rev-parse --verify origin/main", "abc123", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	branch, err := mgr.detectDefaultBranch(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "main" {
		t.Errorf("expected branch %q, got %q", "main", branch)
	}
}

// ---------------------------------------------------------------------------
// Test suite detection — kitchen.yaml variant
// ---------------------------------------------------------------------------

func TestDetectTestSuite_KitchenYaml(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yaml", ".kitchen.yaml", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	if !mgr.detectTestSuite(context.Background(), "/repo") {
		t.Error("expected test suite to be detected when .kitchen.yaml exists")
	}
}

func TestDetectTestSuite_KitchenYmlNoDot(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- .kitchen.yaml", "", fmt.Errorf("not found"))
	fake.addResponse("ls-tree --name-only HEAD -- kitchen.yml", "kitchen.yml", nil)

	mgr := NewGitCookbookManager(t.TempDir(), fake)
	if !mgr.detectTestSuite(context.Background(), "/repo") {
		t.Error("expected test suite to be detected when kitchen.yml exists")
	}
}

// ---------------------------------------------------------------------------
// CloneOrPull — clone with branch detection failure after clone
// ---------------------------------------------------------------------------

func TestCloneOrPull_Clone_BranchDetectionFails(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("clone", "", nil)
	fake.addResponse("symbolic-ref", "", fmt.Errorf("no ref"))
	fake.addResponse("rev-parse --verify origin/main", "", fmt.Errorf("nope"))
	fake.addResponse("rev-parse --verify origin/master", "", fmt.Errorf("nope"))

	baseDir := t.TempDir()
	mgr := NewGitCookbookManager(baseDir, fake)

	_, err := mgr.CloneOrPull(context.Background(), "badrepo", "https://example.com/badrepo")
	if err == nil {
		t.Fatal("expected error when branch detection fails after clone")
	}
	if !strings.Contains(err.Error(), "detect default branch") {
		t.Errorf("expected error about branch detection, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CloneOrPull — clone with HEAD read failure after clone
// ---------------------------------------------------------------------------

func TestCloneOrPull_Clone_HeadReadFails(t *testing.T) {
	fake := newFakeGitExecutor()
	fake.addResponse("clone", "", nil)
	fake.addResponse("symbolic-ref", "origin/main", nil)
	fake.addResponse("rev-parse HEAD", "", fmt.Errorf("fatal: bad revision"))

	baseDir := t.TempDir()
	mgr := NewGitCookbookManager(baseDir, fake)

	_, err := mgr.CloneOrPull(context.Background(), "broken", "https://example.com/broken")
	if err == nil {
		t.Fatal("expected error when HEAD read fails after clone")
	}
	if !strings.Contains(err.Error(), "read HEAD SHA") {
		t.Errorf("expected error about HEAD SHA, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Pull path — HEAD read failure after reset
// ---------------------------------------------------------------------------

func TestCloneOrPull_Pull_HeadReadFailsAfterReset(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	revCount := 0
	fake := &countingGitExecutor{
		base:      newFakeGitExecutor(),
		oldSHA:    sha,
		newSHA:    "", // will trigger error
		newSHAErr: fmt.Errorf("fatal: bad revision"),
		revCount:  &revCount,
		mu:        &sync.Mutex{},
	}
	fake.base.addResponse("fetch", "", nil)
	fake.base.addResponse("symbolic-ref", "origin/main", nil)
	fake.base.addResponse("reset --hard", "", nil)
	fake.base.addResponse("ls-tree", "", fmt.Errorf("not found"))

	baseDir := t.TempDir()
	repoDir := baseDir + "/mybook"
	if err := createFakeGitDir(repoDir); err != nil {
		t.Fatalf("failed to create fake .git dir: %v", err)
	}

	mgr := NewGitCookbookManager(baseDir, fake)
	_, err := mgr.CloneOrPull(context.Background(), "mybook", "https://example.com/mybook")
	if err == nil {
		t.Fatal("expected error when HEAD read fails after reset")
	}
	if !strings.Contains(err.Error(), "read HEAD SHA") {
		t.Errorf("expected error about HEAD SHA, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// createFakeGitDir creates a directory with a .git subdirectory to simulate
// an existing git repository.
func createFakeGitDir(dir string) error {
	return os.MkdirAll(dir+"/.git", 0o755)
}

// countingGitExecutor wraps a fakeGitExecutor but intercepts rev-parse HEAD
// calls to return different SHAs on successive invocations.
type countingGitExecutor struct {
	base      *fakeGitExecutor
	oldSHA    string
	newSHA    string
	newSHAErr error
	revCount  *int
	mu        *sync.Mutex
}

func (c *countingGitExecutor) Run(ctx context.Context, dir string, args ...string) (string, error) {
	joined := strings.Join(args, " ")
	if joined == "rev-parse HEAD" {
		c.mu.Lock()
		count := *c.revCount
		*c.revCount++
		c.mu.Unlock()

		if count == 0 {
			// First call — return old SHA.
			return c.oldSHA, nil
		}
		// Subsequent calls — return new SHA (or error).
		if c.newSHAErr != nil {
			return "", c.newSHAErr
		}
		return c.newSHA, nil
	}
	return c.base.Run(ctx, dir, args...)
}
