// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// ---------------------------------------------------------------------------
// Pagination
// ---------------------------------------------------------------------------

// PaginationParams holds the parsed pagination parameters from a request.
type PaginationParams struct {
	Page    int
	PerPage int
}

const (
	defaultPage    = 1
	defaultPerPage = 50
	maxPerPage     = 500
)

// ParsePagination extracts page and per_page from the request query string.
// Missing or invalid values fall back to defaults. per_page is clamped to
// maxPerPage.
func ParsePagination(r *http.Request) PaginationParams {
	p := PaginationParams{
		Page:    defaultPage,
		PerPage: defaultPerPage,
	}

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			p.Page = n
		}
	}

	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			p.PerPage = n
		}
	}

	if p.PerPage > maxPerPage {
		p.PerPage = maxPerPage
	}

	return p
}

// Offset returns the zero-based offset for SQL OFFSET clauses.
func (p PaginationParams) Offset() int {
	return (p.Page - 1) * p.PerPage
}

// Limit returns the per_page value for SQL LIMIT clauses.
func (p PaginationParams) Limit() int {
	return p.PerPage
}

// PaginationResponse is the pagination metadata included in paginated API
// responses.
type PaginationResponse struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// NewPaginationResponse computes the pagination response metadata from the
// request parameters and the total number of items.
func NewPaginationResponse(params PaginationParams, totalItems int) PaginationResponse {
	totalPages := 0
	if params.PerPage > 0 {
		totalPages = (totalItems + params.PerPage - 1) / params.PerPage
	}
	return PaginationResponse{
		Page:       params.Page,
		PerPage:    params.PerPage,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}
}

// ---------------------------------------------------------------------------
// Paginated response envelope
// ---------------------------------------------------------------------------

// PaginatedResponse is the standard envelope for paginated list endpoints.
type PaginatedResponse struct {
	Data       any                `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
}

// ---------------------------------------------------------------------------
// Error response
// ---------------------------------------------------------------------------

// ErrorResponse is the standard structure for API error responses.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Standard error codes matching the spec.
const (
	ErrCodeBadRequest      = "bad_request"
	ErrCodeUnauthorized    = "unauthorized"
	ErrCodeForbidden       = "forbidden"
	ErrCodeNotFound        = "not_found"
	ErrCodeValidationError = "validation_error"
	ErrCodeRateLimited     = "rate_limited"
	ErrCodeInternalError   = "internal_error"
	ErrCodeMethodNotAllowed = "method_not_allowed"
	ErrCodeServiceUnavailable = "service_unavailable"
)

// ---------------------------------------------------------------------------
// Response writers
// ---------------------------------------------------------------------------

// WriteJSON writes a JSON response with the given status code. The response
// includes the Content-Type header set to application/json; charset=utf-8.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		// At this point headers are already sent — the best we can do is
		// log. The caller's logger middleware should catch this.
		_ = err
	}
}

// WriteError writes a JSON error response with the given status code, error
// code, and human-readable message.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorResponse{
		Error:   code,
		Message: message,
	})
}

// WriteErrorf writes a JSON error response with a formatted message.
func WriteErrorf(w http.ResponseWriter, status int, code, format string, args ...any) {
	WriteError(w, status, code, fmt.Sprintf(format, args...))
}

// WriteBadRequest writes a 400 Bad Request error response.
func WriteBadRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, ErrCodeBadRequest, message)
}

// WriteNotFound writes a 404 Not Found error response.
func WriteNotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, ErrCodeNotFound, message)
}

// WriteUnauthorized writes a 401 Unauthorized error response.
func WriteUnauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, ErrCodeUnauthorized, message)
}

// WriteForbidden writes a 403 Forbidden error response.
func WriteForbidden(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusForbidden, ErrCodeForbidden, message)
}

// WriteInternalError writes a 500 Internal Server Error response. The
// message should be generic for external clients; detailed error information
// should be logged server-side.
func WriteInternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, ErrCodeInternalError, message)
}

// WritePaginated writes a paginated JSON response using the standard
// envelope with data and pagination metadata.
func WritePaginated(w http.ResponseWriter, data any, params PaginationParams, totalItems int) {
	WriteJSON(w, http.StatusOK, PaginatedResponse{
		Data:       data,
		Pagination: NewPaginationResponse(params, totalItems),
	})
}

// ---------------------------------------------------------------------------
// Sorting
// ---------------------------------------------------------------------------

// SortParams holds the parsed sort parameters from a request.
type SortParams struct {
	Field string
	Order string // "asc" or "desc"
}

// ParseSort extracts sort and order from the request query string. The
// allowedFields parameter restricts which fields can be sorted on; if the
// requested field is not in the allowed set, the defaults are used.
func ParseSort(r *http.Request, defaultField string, allowedFields []string) SortParams {
	s := SortParams{
		Field: defaultField,
		Order: "asc",
	}

	if v := r.URL.Query().Get("sort"); v != "" {
		for _, f := range allowedFields {
			if v == f {
				s.Field = v
				break
			}
		}
	}

	if v := r.URL.Query().Get("order"); v == "desc" {
		s.Order = "desc"
	}

	return s
}
