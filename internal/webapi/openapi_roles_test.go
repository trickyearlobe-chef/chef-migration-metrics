// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

// The description must say what a caller actually needs, not what the route was
// registered with. Enforcement lives in two places — the wrapper, and a role
// check inside some handlers — and only the first is visible in the route
// table. Left alone, the document understates the requirement on more than
// fifty operations, and it drifts further every time anything is tightened.
//
// So the handler-level half is declared in apiRoles and checked here against
// the running service. The test router wires no wrapper middleware, so what a
// probe observes is exactly the handler's own opinion, which is the half that
// is declared.
//
// Only routes whose wrapper role is "authenticated" are probed. Anything the
// wrapper already guards must not be invoked with a session that would pass it:
// restarting the service is one such handler, and a probe has no business
// calling it to find out what it requires.
func TestOpenAPI_DescribedRoleIsTheRoleEnforced(t *testing.T) {
	var missing, spurious []string

	for _, rt := range newTestRouterWithMockAndConfig(&mockStore{},
		testConfigWithTargetVersions("19.0")).Routes() {
		if rt.Role != RoleAuthenticated {
			continue
		}
		for _, addr := range describableAddresses(rt) {
			for _, method := range addr.methods {
				key := method + " " + addr.path
				observed := roleEnforcedBy(method, addr.path)
				declared, isDeclared := apiRoles[key]

				switch {
				case observed == RoleAuthenticated && isDeclared:
					spurious = append(spurious, fmt.Sprintf("\t%q: %s,", key, roleConst(declared)))
				case observed != RoleAuthenticated && !isDeclared:
					missing = append(missing, fmt.Sprintf("\t%q: %s,", key, roleConst(observed)))
				case observed != declared && isDeclared:
					t.Errorf("%s is described as needing %q but the handler enforces %q, so the "+
						"description tells a caller the wrong thing about access",
						key, declared, observed)
				}
			}
		}
	}

	sort.Strings(missing)
	sort.Strings(spurious)
	if len(missing) > 0 {
		t.Errorf("%d operations refuse a caller the description says may use them. Add to "+
			"apiRoles in apidoc.go:\n%s", len(missing), strings.Join(missing, "\n"))
	}
	if len(spurious) > 0 {
		t.Errorf("%d operations are described as needing more than they enforce, which sends "+
			"people to raise access they do not need. Remove from apiRoles:\n%s",
			len(spurious), strings.Join(spurious, "\n"))
	}
}

// roleEnforcedBy reports the least demanding role the handler itself accepts.
func roleEnforcedBy(method, path string) RouteRole {
	concrete := strings.NewReplacer("{", "", "}", "").Replace(path)
	if !refuses(method, concrete, withViewerSession) {
		return RoleAuthenticated
	}
	if !refuses(method, concrete, withOperatorSession) {
		return RoleOperator
	}
	return RoleAdmin
}

// refuses reports whether the handler turned this caller away.
//
// A handler that reaches the empty mock store panics, and that is an answer
// rather than a failure: it means the caller was let through, which is all this
// asks. Recovering keeps the check honest without stubbing every store method
// to prove a role check did not fire.
func refuses(method, path string, session func(*http.Request) *http.Request) (denied bool) {
	defer func() {
		if recover() != nil {
			denied = false
		}
	}()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	newTestRouterWithMockAndConfig(&mockStore{}, testConfigWithTargetVersions("19.0")).
		ServeHTTP(w, session(req))
	return w.Code == http.StatusForbidden
}

func roleConst(role RouteRole) string {
	switch role {
	case RoleAdmin:
		return "RoleAdmin"
	case RoleOperator:
		return "RoleOperator"
	default:
		return "RoleAuthenticated"
	}
}
