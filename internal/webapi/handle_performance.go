// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/perf"
)

// WithPerformance sets the perf.Recorder used by the request timing
// middleware and the performance stats endpoint.
func WithPerformance(rec *perf.Recorder) RouterOption {
	return func(r *Router) {
		r.recorder = rec
	}
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/performance — per-endpoint request latency stats
// DELETE /api/v1/admin/performance — reset request stats
// ---------------------------------------------------------------------------

func (r *Router) handlePerformance(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handlePerformanceGet(w, req)
	case http.MethodDelete:
		r.handlePerformanceDelete(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and DELETE.")
	}
}

// endpointStat is the JSON shape for a single endpoint in the request stats
// response. Field names match the spec.
type endpointStat struct {
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Count      int     `json:"count"`
	ErrorCount int     `json:"error_count"`
	P50Ms      float64 `json:"p50_ms"`
	P95Ms      float64 `json:"p95_ms"`
	P99Ms      float64 `json:"p99_ms"`
	MaxMs      float64 `json:"max_ms"`
}

type performanceResponse struct {
	WindowSeconds int            `json:"window_seconds"`
	Endpoints     []endpointStat `json:"endpoints"`
}

func (r *Router) handlePerformanceGet(w http.ResponseWriter, _ *http.Request) {
	snap := r.recorder.Snapshot()

	endpoints := make([]endpointStat, 0, len(snap))
	for _, ks := range snap {
		method, path := splitEndpointKey(ks.Key)
		endpoints = append(endpoints, endpointStat{
			Method:     method,
			Path:       path,
			Count:      ks.Count,
			ErrorCount: ks.ErrorCount,
			P50Ms:      durationMs(ks.P50),
			P95Ms:      durationMs(ks.P95),
			P99Ms:      durationMs(ks.P99),
			MaxMs:      durationMs(ks.Max),
		})
	}

	WriteJSON(w, http.StatusOK, performanceResponse{
		WindowSeconds: r.cfg.Performance.WindowSeconds,
		Endpoints:     endpoints,
	})
}

func (r *Router) handlePerformanceDelete(w http.ResponseWriter, _ *http.Request) {
	r.recorder.Reset()
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// GET /api/v1/admin/performance/db — PostgreSQL stats
// DELETE /api/v1/admin/performance/db — reset PG stats
// ---------------------------------------------------------------------------

func (r *Router) handlePerformanceDB(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		r.handlePerformanceDBGet(w, req)
	case http.MethodDelete:
		r.handlePerformanceDBDelete(w, req)
	default:
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed,
			"This endpoint supports GET and DELETE.")
	}
}

type performanceDBResponse struct {
	PgStatStatementsAvailable bool `json:"pg_stat_statements_available"`
	TopQueries                any  `json:"top_queries"`
	TableStats                any  `json:"table_stats"`
	IndexStats                any  `json:"index_stats"`
	ActiveQueries             any  `json:"active_queries"`
}

func (r *Router) handlePerformanceDBGet(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	available := r.db.PgStatStatementsAvailable(ctx)

	topQueries, err := r.db.TopQueryStats(ctx, 20)
	if err != nil {
		WriteInternalError(w, "Failed to query top query stats.")
		return
	}

	tableStats, err := r.db.TableStats(ctx)
	if err != nil {
		WriteInternalError(w, "Failed to query table stats.")
		return
	}

	indexStats, err := r.db.IndexStats(ctx)
	if err != nil {
		WriteInternalError(w, "Failed to query index stats.")
		return
	}

	activeQueries, err := r.db.ActiveQueries(ctx)
	if err != nil {
		WriteInternalError(w, "Failed to query active queries.")
		return
	}

	resp := performanceDBResponse{
		PgStatStatementsAvailable: available,
		TopQueries:                emptySliceIfNil(topQueries),
		TableStats:                emptySliceIfNil(tableStats),
		IndexStats:                emptySliceIfNil(indexStats),
		ActiveQueries:             emptySliceIfNil(activeQueries),
	}

	WriteJSON(w, http.StatusOK, resp)
}

func (r *Router) handlePerformanceDBDelete(w http.ResponseWriter, req *http.Request) {
	if err := r.db.ResetPgStats(req.Context()); err != nil {
		WriteInternalError(w, "Failed to reset PostgreSQL stats.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// splitEndpointKey splits "GET /api/v1/nodes" into method and path. If there
// is no space, the whole key is treated as the path with an empty method.
func splitEndpointKey(key string) (method, path string) {
	if idx := strings.IndexByte(key, ' '); idx >= 0 {
		return key[:idx], key[idx+1:]
	}
	return "", key
}

// durationMs converts a time.Duration to fractional milliseconds.
func durationMs(d interface{ Seconds() float64 }) float64 {
	return d.Seconds() * 1000
}

// emptySliceIfNil returns an empty JSON-safe slice ([]) when the input is
// nil, preventing "null" in the JSON output.
func emptySliceIfNil[T any](s []T) any {
	if s == nil {
		return []T{}
	}
	return s
}

// ---------------------------------------------------------------------------
// POST /api/v1/admin/performance/vacuum — run VACUUM FULL
// ---------------------------------------------------------------------------

func (r *Router) handleVacuumFull(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, ErrCodeMethodNotAllowed, "VACUUM endpoint requires POST.")
		return
	}

	if err := r.db.VacuumFull(req.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}
