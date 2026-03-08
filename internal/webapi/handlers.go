// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Path parameter extraction
// ---------------------------------------------------------------------------

// pathParam extracts a named segment from a URL path. For patterns registered
// with Go 1.22+ ServeMux, we could use req.PathValue(), but since the project
// uses Go 1.25 we use that directly when a wildcard is registered. For
// manually-routed prefix patterns (e.g. "/api/v1/nodes/by-version/") we fall
// back to trimming the prefix.
//
// Example:
//
//	pathParam(req, "/api/v1/nodes/by-version/") => "17.10.0"
func pathParam(req *http.Request, prefix string) string {
	return strings.TrimPrefix(req.URL.Path, prefix)
}

// pathSegments splits the path after the given prefix into segments.
// Leading and trailing slashes are ignored.
//
// Example:
//
//	pathSegments("/api/v1/nodes/prod/web01", "/api/v1/nodes/") => ["prod", "web01"]
func pathSegments(path, prefix string) []string {
	remainder := strings.TrimPrefix(path, prefix)
	remainder = strings.Trim(remainder, "/")
	if remainder == "" {
		return nil
	}
	return strings.Split(remainder, "/")
}

// ---------------------------------------------------------------------------
// Method routing helpers
// ---------------------------------------------------------------------------

// requireMethod checks that the request method matches the given method.
// If not, it writes a 405 response and returns false.
func requireMethod(w http.ResponseWriter, req *http.Request, method string) bool {
	if req.Method != method {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint requires "+method+".")
		return false
	}
	return true
}

// requireGET is a shorthand for requireMethod(w, req, "GET").
func requireGET(w http.ResponseWriter, req *http.Request) bool {
	return requireMethod(w, req, http.MethodGet)
}

// ---------------------------------------------------------------------------
// Query parameter helpers
// ---------------------------------------------------------------------------

// queryString returns the value of a query parameter, or the default if the
// parameter is missing or empty.
func queryString(req *http.Request, name, defaultValue string) string {
	v := req.URL.Query().Get(name)
	if v == "" {
		return defaultValue
	}
	return v
}

// queryStringSlice returns the values of a repeated query parameter. If the
// parameter is not present, nil is returned.
func queryStringSlice(req *http.Request, name string) []string {
	values, ok := req.URL.Query()[name]
	if !ok {
		return nil
	}
	// Filter out empty values.
	var result []string
	for _, v := range values {
		if v != "" {
			result = append(result, v)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// queryBool returns the boolean value of a query parameter. Accepted truthy
// values are "true", "1", "yes". Everything else (including missing) returns
// the default.
func queryBool(req *http.Request, name string, defaultValue bool) bool {
	v := req.URL.Query().Get(name)
	if v == "" {
		return defaultValue
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return defaultValue
	}
}
