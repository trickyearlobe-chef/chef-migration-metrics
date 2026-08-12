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
