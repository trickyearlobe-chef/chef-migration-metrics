// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import "net/http"

// RouteRole is the level of access a route requires.
type RouteRole string

const (
	// RolePublic is served without a session. Health, version, login and the
	// telemetry sink are the whole list, and each is public deliberately.
	RolePublic RouteRole = "public"
	// RoleAuthenticated is served to any valid session.
	RoleAuthenticated RouteRole = "authenticated"
	// RoleOperator is served to operators and above.
	RoleOperator RouteRole = "operator"
	// RoleAdmin is served to administrators only.
	RoleAdmin RouteRole = "admin"
)

// Route is one address this service serves, recorded at the moment it is
// registered. The record is the routes rather than a copy of them, so it cannot
// fall behind: a route that is not registered through the funnel below is not
// served at all.
type Route struct {
	// Pattern is the mux pattern. A pattern ending in a slash is a subtree:
	// the addresses under it are dispatched inside the handler and are
	// invisible here, so a subtree carries its own sub-paths (see SubPaths).
	Pattern string
	// Role is the access level enforced at registration.
	Role RouteRole
	// Methods are the HTTP methods this address answers. GET when nothing is
	// declared, which is what most of them are. A wrong one here would describe
	// an address a caller cannot use, so a test asks the service whether each
	// described method is actually accepted rather than trusting the default.
	Methods []string
	// SubPaths are the addresses a subtree handler dispatches by segment.
	// Empty for an ordinary route. Declared where the dispatch happens, because
	// nothing else can see them.
	SubPaths []SubPath
	// Bodies names the Go types each method decodes its request body into,
	// keyed by method. The *shape* is never written here — only which types —
	// so the fields and their names on the wire are reflected out of the types
	// the handler really uses and cannot drift from them. See takes.
	Bodies map[string][]any
	// Paginated records which methods accept page and per_page. See paginated.
	Paginated map[string]bool
}

// RouteOption declares something about a route that the registration itself
// cannot show: which methods it answers, or which addresses live under it.
type RouteOption func(*Route)

// methods declares the HTTP methods a route answers.
func methods(ms ...string) RouteOption {
	return func(rt *Route) { rt.Methods = ms }
}

// SubPath is one address under a subtree pattern, dispatched inside the
// handler rather than by the mux.
//
// The mux registers `/api/v1/git-repos/` and knows nothing more; the handler
// then reads the segments and answers `:name`, `:name/rescan`,
// `:name/committers/assign` and a dozen others. Describing only the subtree
// would describe a third of the real surface and read as complete, so these are
// declared alongside the registration and checked like any other route.
type SubPath struct {
	// Suffix follows the subtree pattern, with named segments in braces —
	// "{name}/rescan" under "/api/v1/git-repos/".
	Suffix string
	// Methods are the HTTP methods the handler answers at this address.
	Methods []string
	// Bodies names the Go types each method decodes its request body into,
	// keyed by method. See takes.
	Bodies map[string][]any
	// Paginated records which methods accept page and per_page. See paginated.
	Paginated map[string]bool
	// Capped records which methods accept per_page but ignore page. See
	// subCappedNotPaged.
	Capped map[string]bool
}

// IsSubtree reports whether the pattern matches everything beneath it.
func (rt Route) IsSubtree() bool {
	return len(rt.Pattern) > 0 && rt.Pattern[len(rt.Pattern)-1] == '/'
}

// Routes returns every route the router registered, in registration order.
func (r *Router) Routes() []Route {
	return r.routes
}

// handle is the single door onto the mux. Every registration goes through it so
// the record cannot disagree with what is served — a route added any other way
// would be served and undescribed, which is the failure the record exists to
// prevent. A test reads registerRoutes and fails if anything reaches the mux
// directly.
func (r *Router) handle(pattern string, role RouteRole, handler http.Handler, opts ...RouteOption) {
	rt := Route{Pattern: pattern, Role: role, Methods: []string{http.MethodGet}}
	for _, opt := range opts {
		opt(&rt)
	}
	r.routes = append(r.routes, rt)
	r.mux.Handle(pattern, handler)
}

// public registers a route served without a session.
func (r *Router) public(pattern string, handler http.HandlerFunc, opts ...RouteOption) {
	r.handle(pattern, RolePublic, handler, opts...)
}

// protect registers a route that requires authentication (any valid session).
// When authMiddleware is nil (auth not configured), the handler is registered
// without session enforcement so the API remains usable in development. The
// route is still recorded as requiring a session: the record describes what the
// service serves, not how one deployment happens to be configured.
func (r *Router) protect(pattern string, handler http.HandlerFunc, opts ...RouteOption) {
	guarded := http.Handler(handler)
	if r.authMiddleware != nil {
		guarded = r.authMiddleware.RequireAuth(enforceCredentialScope(handler))
	}
	r.handle(pattern, RoleAuthenticated, guarded, opts...)
}

// adminOnly registers a route that requires authentication AND the admin role.
func (r *Router) adminOnly(pattern string, handler http.HandlerFunc, opts ...RouteOption) {
	guarded := http.Handler(handler)
	if r.authMiddleware != nil {
		guarded = r.authMiddleware.RequireAuth(
			r.authMiddleware.RequireAdmin(enforceCredentialScope(handler)))
	}
	r.handle(pattern, RoleAdmin, guarded, opts...)
}

// operatorOnly registers a route that requires authentication AND at least
// operator role.
func (r *Router) operatorOnly(pattern string, handler http.HandlerFunc, opts ...RouteOption) {
	guarded := http.Handler(handler)
	if r.authMiddleware != nil {
		guarded = r.authMiddleware.RequireAuth(
			r.authMiddleware.RequireOperator(enforceCredentialScope(handler)))
	}
	r.handle(pattern, RoleOperator, guarded, opts...)
}

// sub declares one address under a subtree, with the methods its handler
// answers. Read from the handler's dispatch, never from the comment above it:
// the comments were checked and found both incomplete (the bare `{name}`
// detail case is undocumented in several) and stale (one names a sub-path that
// was removed), which is the rot this whole mechanism exists to end.
func sub(suffix string, ms ...string) RouteOption {
	if len(ms) == 0 {
		ms = []string{http.MethodGet}
	}
	return func(rt *Route) {
		rt.SubPaths = append(rt.SubPaths, SubPath{Suffix: suffix, Methods: ms})
	}
}

// takes declares which type a method on this address decodes its request body
// into. A zero value of the type is enough — nothing is read from it but its
// shape.
//
// More than one may be named, for the one address that reads a single body into
// several types: the settings for the server itself carry certificates and ACME
// credentials that are routed to encrypted storage rather than into the
// settings section. A caller has to be told about all of them, so the
// description is every named type at once.
//
// This is the one thing about a body that is written down, and it is one token
// per address rather than a table of fields. Everything a caller needs to know
// — which fields exist, what they are called on the wire, what they hold — is
// reflected off the type in openapi_schema.go, so a field added to a handler
// appears in the description in the same commit and a renamed one cannot leave
// a stale name behind.
//
// TestBodies_EveryDecodedRequestBodyIsDescribed reads the handlers and fails
// when one decodes a body no address declares, so this cannot quietly fall
// behind the way a hand-kept list would.
func takes(method string, vs ...any) RouteOption {
	return func(rt *Route) {
		if rt.Bodies == nil {
			rt.Bodies = map[string][]any{}
		}
		rt.Bodies[method] = vs
	}
}

// subTakes is takes for one address under a subtree.
//
// It panics when the suffix has not been declared with sub first, because the
// alternative is a body silently attached to nothing — an address that reads as
// taking no input while the handler refuses every call that sends none. The
// panic happens as the router is built, so every test that builds one catches
// it.
func subTakes(suffix, method string, vs ...any) RouteOption {
	return func(rt *Route) {
		for i := range rt.SubPaths {
			if rt.SubPaths[i].Suffix != suffix {
				continue
			}
			if rt.SubPaths[i].Bodies == nil {
				rt.SubPaths[i].Bodies = map[string][]any{}
			}
			rt.SubPaths[i].Bodies[method] = vs
			return
		}
		panic("subTakes(" + suffix + ", " + method + ") on " + rt.Pattern +
			": no such sub-path is declared, so the body would describe nothing. " +
			"Declare it with sub() first, and keep sub() before subTakes().")
	}
}

// paginated declares that reading this address accepts page and per_page.
//
// Measured against a running instance with tools/api-probe/probe.py, not
// derived and not guessed. Three derivations were tried and every one
// over-reports, because the handler a
// route is registered with is not the unit that pages: a subtree handler serves
// many addresses and only some of them page, and two handlers here are
// registered at several exact patterns and dispatch on the path inside. A
// wrongly attached page parameter is a caller asking for fifty rows, being
// ignored, and believing the whole estate was fifty rows.
//
// What the parameters themselves say — the default, the minimum, the clamp — is
// read off the constants ParsePagination actually applies, so this cannot
// disagree with the service about the numbers. Only *which addresses* is
// written down, and two tests in openapi_query_test.go hold that honest from
// both sides.
func paginated() RouteOption {
	return func(rt *Route) {
		if rt.Paginated == nil {
			rt.Paginated = map[string]bool{}
		}
		rt.Paginated[http.MethodGet] = true
	}
}

// subPaginated is paginated for one address under a subtree. Panics when the
// suffix has not been declared with sub first — see subTakes for why.
func subPaginated(suffix string) RouteOption {
	return func(rt *Route) {
		for i := range rt.SubPaths {
			if rt.SubPaths[i].Suffix != suffix {
				continue
			}
			if rt.SubPaths[i].Paginated == nil {
				rt.SubPaths[i].Paginated = map[string]bool{}
			}
			rt.SubPaths[i].Paginated[http.MethodGet] = true
			return
		}
		panic("subPaginated(" + suffix + ") on " + rt.Pattern +
			": no such sub-path is declared, so the parameters would describe nothing. " +
			"Declare it with sub() first, and keep sub() before subPaginated().")
	}
}

// subCappedNotPaged declares that reading one address under a subtree honours
// per_page but ignores page.
//
// This is not a description choice, it is a defect being described accurately.
// Both addresses that carry it read their runs through a store call that takes
// a limit and no offset, so asking for the second page returns the first one
// again. Describing them as paginated would tell a caller to walk pages that
// silently repeat — worse than an error, because it looks like duplicate data
// at the far end rather than like a failure.
//
// Recorded in plans/todo-snagging.md. When the store learns an offset these
// become subPaginated() and this loses its last user, along with Capped.
// There is deliberately no route-level version: both addresses are sub-paths,
// and an option nothing calls is a shape somebody later fills in by accident.
func subCappedNotPaged(suffix string) RouteOption {
	return func(rt *Route) {
		for i := range rt.SubPaths {
			if rt.SubPaths[i].Suffix != suffix {
				continue
			}
			if rt.SubPaths[i].Capped == nil {
				rt.SubPaths[i].Capped = map[string]bool{}
			}
			rt.SubPaths[i].Capped[http.MethodGet] = true
			return
		}
		panic("subCappedNotPaged(" + suffix + ") on " + rt.Pattern +
			": no such sub-path is declared. Declare it with sub() first.")
	}
}
