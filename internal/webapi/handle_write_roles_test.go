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

func writeRoleRequest(t *testing.T, method, path string,
	session func(*http.Request) *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0")).
		ServeHTTP(w, session(req))
	return w
}

func TestNodeKitchenRun_ViewerCannotDelete(t *testing.T) {
	w := writeRoleRequest(t, http.MethodDelete, "/api/v1/kitchen/node-runs/run-1", withViewerSession)
	if w.Code != http.StatusForbidden {
		t.Errorf("a viewer deleting a test run got %d, not 403 — that record is the evidence "+
			"that a cookbook does or does not converge on a real machine", w.Code)
	}
}

func TestNodeKitchenRun_OperatorCanStillDelete(t *testing.T) {
	w := writeRoleRequest(t, http.MethodDelete, "/api/v1/kitchen/node-runs/run-1", withOperatorSession)
	if w.Code == http.StatusForbidden {
		t.Error("an operator can no longer delete a test run, but triggering one is already " +
			"theirs to do")
	}
}

func TestNodeKitchenRun_ViewerCanStillRead(t *testing.T) {
	for _, path := range []string{"/api/v1/kitchen/node-runs", "/api/v1/kitchen/node-runs/run-1"} {
		if w := writeRoleRequest(t, http.MethodGet, path, withViewerSession); w.Code == http.StatusForbidden {
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
		if w := writeRoleRequest(t, c.method, c.path, withViewerSession); w.Code != http.StatusForbidden {
			t.Errorf("a viewer doing %s %s got %d, not 403 — a custom check moves verdicts "+
				"across the estate, and reclassifying a shipped one already needs an admin",
				c.method, c.path, w.Code)
		}
	}
}

// An operator is not enough here, deliberately: this is the same power as
// reclassifying a shipped check, which is admin.
func TestCustomCops_OperatorCannotWrite(t *testing.T) {
	if w := writeRoleRequest(t, http.MethodDelete, "/api/v1/cookstyle/custom-cops/Example/Cop",
		withOperatorSession); w.Code != http.StatusForbidden {
		t.Errorf("an operator deleting a custom check got %d, not 403", w.Code)
	}
}

func TestCustomCops_AdminCanStillWrite(t *testing.T) {
	if w := writeRoleRequest(t, http.MethodDelete, "/api/v1/cookstyle/custom-cops/Example/Cop",
		withAdminSession); w.Code == http.StatusForbidden {
		t.Error("an admin can no longer delete a custom check")
	}
}

func TestCustomCops_ViewerCanStillRead(t *testing.T) {
	for _, path := range []string{"/api/v1/cookstyle/custom-cops", "/api/v1/cookstyle/custom-cops/Example/Cop"} {
		if w := writeRoleRequest(t, http.MethodGet, path, withViewerSession); w.Code == http.StatusForbidden {
			t.Errorf("a viewer can no longer read %s", path)
		}
	}
}
