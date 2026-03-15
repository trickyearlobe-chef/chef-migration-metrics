// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"path/filepath"
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

// ---------------------------------------------------------------------------
// Path safety helpers
// ---------------------------------------------------------------------------

// safeName validates that a user-supplied name is a single, clean path
// component with no directory traversal. It returns the cleaned name and
// true if the name is safe, or ("", false) if the name is empty, contains
// path separators, or attempts traversal (e.g. "..", ".", "/etc/passwd").
//
// This MUST be used before incorporating any user-controlled value into a
// filesystem path (filepath.Join, os.Stat, os.RemoveAll, etc.).
func safeName(name string) (string, bool) {
	if name == "" {
		return "", false
	}

	// Clean the name to resolve any ".." or "." components.
	cleaned := filepath.Clean(name)

	// Reject if cleaning changed the value (indicates traversal attempt),
	// if it contains a path separator, or if it resolves to "." or "..".
	if cleaned != name ||
		strings.ContainsAny(cleaned, `/\`) ||
		cleaned == "." || cleaned == ".." {
		return "", false
	}

	return cleaned, true
}
