// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth"
)

// A body carrying a field the service does not understand is refused.
//
// Until now one was accepted and silently dropped: the caller was told the
// call worked, and afterwards neither side could say what had been acted on.
// Three of those were live — a settings screen with a text box for a setting
// this service has never had, a reclassification sending a version nothing
// reads, and the server settings screen handing back the read-only status it
// had just been shown.
//
// The check that matters to somebody using an assistant is the message: an
// assistant told "unknown field X" corrects itself and tries again, while one
// told "invalid JSON" sends the same thing twice.

// strictBodyCases are addresses that decode a JSON body, one per decode path
// there is: an ordinary handler, one under a subtree, and a settings section
// (which is read by the YAML decoder so a caller may send either).
//
// Each carries a body the service accepts. That baseline is checked first and
// on its own, because a test that only sent the strange body would pass just as
// well against an address that refuses everything — and would then be proving
// nothing while reading as though it proved the point.
var strictBodyCases = []struct {
	name   string
	method string
	path   string
	good   string
}{
	{
		name:   "recording a finding",
		method: http.MethodPost,
		path:   "/api/v1/failure-register",
		good: `{"subject_name":"example-cookbook","subject_type":"git_repo",` +
			`"cookbook_name":"example-cookbook","verdict":"broken","reason":"it fails"}`,
	},
	{
		name:   "issuing oneself a credential",
		method: http.MethodPost,
		path:   "/api/v1/auth/me/tokens",
		good:   `{"name":"editor","can_write":false}`,
	},
	{
		name:   "reclassifying a static finding",
		method: http.MethodPut,
		path:   "/api/v1/cookstyle/cops/Lint%2FExample/classification",
		good:   `{"classification":"blocker","reason":"it breaks the upgrade"}`,
	},
	{
		name:   "a settings section",
		method: http.MethodPut,
		path:   "/api/v1/admin/config/logging",
		good:   `{"level":"info","retention_days":30}`,
	},
}

// The baseline: each body above is one the service takes. Kept separate so a
// failure says which of the two things went wrong.
func TestStrictBodies_TheBodiesTheseCasesVaryAreAccepted(t *testing.T) {
	for _, c := range strictBodyCases {
		t.Run(c.name, func(t *testing.T) {
			if w := strictBodySend(t, c.method, c.path, c.good); w.Code >= 400 {
				t.Fatalf("the body this case varies is not accepted to begin with (%d): %s — "+
					"so nothing it goes on to say about strictness means anything",
					w.Code, w.Body.String())
			}
		})
	}
}

// The same call, with one field nobody has ever heard of.
func TestStrictBodies_AFieldTheServiceDoesNotUnderstandIsRefused(t *testing.T) {
	for _, c := range strictBodyCases {
		t.Run(c.name, func(t *testing.T) {
			strange := strings.TrimSuffix(c.good, "}") + `,"totally_unknown_field":"ignored"}`
			w := strictBodySend(t, c.method, c.path, strange)

			if w.Code < 400 {
				t.Fatalf("a body carrying a field the service does not understand was accepted "+
					"(%d), so a caller who misspells one is told it worked and neither side can "+
					"say what was acted on", w.Code)
			}
			if !strings.Contains(w.Body.String(), "totally_unknown_field") {
				t.Errorf("the refusal does not name the field it could not understand: %s — a "+
					"caller told only that its body was invalid sends the same body again",
					w.Body.String())
			}
		})
	}
}

// Malformed JSON is still refused, and still says so rather than complaining
// about a field. Making a decoder stricter is a good way to lose this.
func TestStrictBodies_SomethingThatIsNotJSONIsStillRefused(t *testing.T) {
	for _, c := range strictBodyCases {
		t.Run(c.name, func(t *testing.T) {
			if w := strictBodySend(t, c.method, c.path, `{"not json`); w.Code < 400 {
				t.Errorf("a body that is not JSON at all was accepted (%d)", w.Code)
			}
		})
	}
}

// strictBodySend issues an authenticated request as an administrator.
func strictBodySend(t *testing.T, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	strictBodyRouter(t).ServeHTTP(w, withAdminSession(req))
	return w
}

// strictBodyRouter has everything the cases above need wired: a config store,
// because a settings section answers "not configured here" without one, and a
// credential manager, because issuing oneself a credential does.
func strictBodyRouter(t *testing.T) *Router {
	t.Helper()
	return newTestRouterForAdminConfig(
		testConfigWithTargetVersions("19.0"), newTestConfigStore(t), nil,
		WithCredentialManager(auth.NewCredentialManager(
			newMemCredentialStore().withUser("admin", "admin"))))
}

// The structural half. The cases above cover four addresses; there are
// fifty-odd, and writing a body the service accepts for each is a different
// piece of work from making them all strict.
//
// So this reads the handlers instead and holds the property everywhere: a
// request body is decoded through the strict helper or not at all. It is the
// same mechanism the description's drift tests use, and for the same reason —
// the handlers are the record, and a list kept beside them would fall behind.
func TestStrictBodies_NoHandlerDecodesABodyLeniently(t *testing.T) {
	fset := token.NewFileSet()
	lenient := 0
	strict := 0

	for _, path := range handlerSources(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("cannot read %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if fn, ok := call.Fun.(*ast.Ident); ok &&
				(fn.Name == strictBodyHelper || fn.Name == "decodeAdminConfigBody") {
				strict++
				return true
			}
			// json.NewDecoder(req.Body).Decode(&x) — lenient by default, which
			// is the whole problem.
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Decode" {
				return true
			}
			inner, ok := sel.X.(*ast.CallExpr)
			if !ok || len(inner.Args) != 1 {
				return true
			}
			if !readsFromRequestBody(inner.Args[0]) {
				return true
			}
			lenient++
			t.Errorf("%s decodes a request body with a decoder that accepts fields the "+
				"service does not understand, so a caller that sends one is told the call "+
				"worked. Decode it with %s instead",
				fset.Position(call.Pos()), strictBodyHelper)
			return true
		})
	}

	if strict == 0 {
		t.Fatalf("no handler was found decoding a body through %s, so this test checked "+
			"nothing; if the idiom changed, point it at the new one rather than deleting it",
			strictBodyHelper)
	}
	if lenient == 0 {
		t.Logf("%d body decodes, all strict", strict)
	}
}

// strictBodyHelper is the name this test looks for. Named once so the test
// reports the thing somebody should go and use.
const strictBodyHelper = "decodeJSONBody"
