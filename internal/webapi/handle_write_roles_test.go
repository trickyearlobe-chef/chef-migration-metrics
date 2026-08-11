// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Two write paths were reachable by anybody with a session, while the things
// they sit next to were not: deleting a test run is evidence about whether a
// cookbook converges, and a custom static check moves verdicts across the whole
// estate exactly as reclassifying a shipped one does.
//
// Each is held to the role its own neighbour already required — operator to
// touch a node kitchen run, because triggering one is operatorOnly; admin to
// touch a custom check, because changing a shipped check's classification is
// requireAdminRole.
//
// Reading stays open to everybody. These endpoints answer GET on the same
// pattern, so the check belongs to the method and not to the route.

// writeRoleStatus issues a request under a given session and returns the status.
//
// A handler that gets as far as the empty mock store panics, and that is a
// meaningful answer here rather than a failure: it means the request was not
// refused, which is the only thing these tests ask. Returning a sentinel keeps
// that case readable instead of needing every store method stubbed to prove a
// role check did not fire.
const reachedTheStore = -1

func writeRoleStatus(t *testing.T, method, path string,
	session func(*http.Request) *http.Request) (code int) {
	t.Helper()
	defer func() {
		if recover() != nil {
			code = reachedTheStore
		}
	}()
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0")).
		ServeHTTP(w, session(req))
	return w.Code
}

func TestNodeKitchenRun_ViewerCannotDelete(t *testing.T) {
	code := writeRoleStatus(t, http.MethodDelete, "/api/v1/kitchen/node-runs/run-1", withViewerSession)
	if code != http.StatusForbidden {
		t.Errorf("a viewer deleting a test run got %d, not 403 — that record is the evidence "+
			"that a cookbook does or does not converge on a real machine", code)
	}
}

func TestNodeKitchenRun_OperatorCanStillDelete(t *testing.T) {
	code := writeRoleStatus(t, http.MethodDelete, "/api/v1/kitchen/node-runs/run-1", withOperatorSession)
	if code == http.StatusForbidden {
		t.Error("an operator can no longer delete a test run, but triggering one is already " +
			"theirs to do")
	}
}

func TestNodeKitchenRun_ViewerCanStillRead(t *testing.T) {
	for _, path := range []string{"/api/v1/kitchen/node-runs", "/api/v1/kitchen/node-runs/run-1"} {
		if code := writeRoleStatus(t, http.MethodGet, path, withViewerSession); code == http.StatusForbidden {
			t.Errorf("a viewer can no longer read %s; guarding a delete must not close the "+
				"screen everybody uses", path)
		}
	}
}

func TestCustomCops_ViewerCannotWrite(t *testing.T) {
	for _, c := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/cookstyle/custom-cops"},
		{http.MethodPut, "/api/v1/cookstyle/custom-cops/Example/Cop"},
		{http.MethodDelete, "/api/v1/cookstyle/custom-cops/Example/Cop"},
	} {
		if code := writeRoleStatus(t, c.method, c.path, withViewerSession); code != http.StatusForbidden {
			t.Errorf("a viewer doing %s %s got %d, not 403 — a custom check moves verdicts "+
				"across the estate, and reclassifying a shipped one already needs an admin",
				c.method, c.path, code)
		}
	}
}

// An operator is not enough here, deliberately: this is the same power as
// reclassifying a shipped check, which is admin.
func TestCustomCops_OperatorCannotWrite(t *testing.T) {
	if code := writeRoleStatus(t, http.MethodDelete, "/api/v1/cookstyle/custom-cops/Example/Cop",
		withOperatorSession); code != http.StatusForbidden {
		t.Errorf("an operator deleting a custom check got %d, not 403", code)
	}
}

func TestCustomCops_AdminCanStillWrite(t *testing.T) {
	if code := writeRoleStatus(t, http.MethodDelete, "/api/v1/cookstyle/custom-cops/Example/Cop",
		withAdminSession); code == http.StatusForbidden {
		t.Error("an admin can no longer delete a custom check")
	}
}

func TestCustomCops_ViewerCanStillRead(t *testing.T) {
	for _, path := range []string{"/api/v1/cookstyle/custom-cops", "/api/v1/cookstyle/custom-cops/Example/Cop"} {
		if code := writeRoleStatus(t, http.MethodGet, path, withViewerSession); code == http.StatusForbidden {
			t.Errorf("a viewer can no longer read %s", path)
		}
	}
}

// Two asymmetries where an address was open while the thing beside it was not.
// Creating a kitchen batch is operatorOnly at the wrapper, so changing, running,
// cancelling or deleting one cannot be less. Excluding a repository from test
// runs is admin, so excluding one from scanning — which moves every verdict that
// repository feeds — cannot be less either.
//
// Reading is untouched in both: a batch and its progress, and the list of
// excluded repositories, stay open to anybody with a session.

func TestKitchenBatch_ViewerCannotChangeOrRunOne(t *testing.T) {
	for _, c := range []struct{ method, path string }{
		{http.MethodPut, "/api/v1/kitchen/batches/batch-1"},
		{http.MethodDelete, "/api/v1/kitchen/batches/batch-1"},
		{http.MethodPost, "/api/v1/kitchen/batches/batch-1/run"},
		{http.MethodPost, "/api/v1/kitchen/batches/batch-1/cancel"},
	} {
		if code := writeRoleStatus(t, c.method, c.path, withViewerSession); code != http.StatusForbidden {
			t.Errorf("a viewer doing %s %s got %d, not 403 — making a batch already needs an "+
				"operator, so running or deleting one cannot need less", c.method, c.path, code)
		}
	}
}

func TestKitchenBatch_OperatorCanStill(t *testing.T) {
	for _, c := range []struct{ method, path string }{
		{http.MethodDelete, "/api/v1/kitchen/batches/batch-1"},
		{http.MethodPost, "/api/v1/kitchen/batches/batch-1/cancel"},
	} {
		if code := writeRoleStatus(t, c.method, c.path, withOperatorSession); code == http.StatusForbidden {
			t.Errorf("an operator can no longer do %s %s, but creating the batch is theirs",
				c.method, c.path)
		}
	}
}

func TestKitchenBatch_ViewerCanStillRead(t *testing.T) {
	for _, path := range []string{
		"/api/v1/kitchen/batches/batch-1",
		"/api/v1/kitchen/batches/batch-1/progress",
		"/api/v1/kitchen/batches/batch-1/instances",
	} {
		if code := writeRoleStatus(t, http.MethodGet, path, withViewerSession); code == http.StatusForbidden {
			t.Errorf("a viewer can no longer read %s", path)
		}
	}
}

func TestGitRepoExclusion_ViewerAndOperatorCannotChangeIt(t *testing.T) {
	for _, session := range []struct {
		name string
		with func(*http.Request) *http.Request
	}{{"viewer", withViewerSession}, {"operator", withOperatorSession}} {
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			code := writeRoleStatus(t, method, "/api/v1/git-repos/example-repo/exclude", session.with)
			if code != http.StatusForbidden {
				t.Errorf("a %s doing %s on a repository exclusion got %d, not 403 — leaving a "+
					"repository out of scanning moves every verdict it feeds, and excluding one "+
					"from test runs already needs an admin", session.name, method, code)
			}
		}
	}
}

func TestGitRepoExclusion_AdminCanStillChangeIt(t *testing.T) {
	if code := writeRoleStatus(t, http.MethodDelete, "/api/v1/git-repos/example-repo/exclude",
		withAdminSession); code == http.StatusForbidden {
		t.Error("an admin can no longer clear a repository exclusion")
	}
}

func TestGitRepoExclusion_ViewerCanStillSeeWhatIsExcluded(t *testing.T) {
	if code := writeRoleStatus(t, http.MethodGet, "/api/v1/git-repos/excluded",
		withViewerSession); code == http.StatusForbidden {
		t.Error("a viewer can no longer see which repositories are excluded, which is the " +
			"screen that explains why something is missing from a list")
	}
}
