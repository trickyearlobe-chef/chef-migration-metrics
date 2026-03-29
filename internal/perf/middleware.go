// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package perf

import (
	"net/http"
	"time"
)

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Middleware records request latency to a Recorder.
type Middleware struct {
	rec *Recorder
}

// NewMiddleware creates a Middleware backed by the given Recorder.
func NewMiddleware(rec *Recorder) *Middleware {
	return &Middleware{rec: rec}
}

// Wrap returns a new http.Handler that records the latency of each request
// to the Recorder, keyed by "METHOD pattern" where pattern comes from
// req.Pattern() (Go 1.22+ ServeMux pattern). Falls back to req.URL.Path
// if Pattern() returns "".
// It also records errors (status >= 500) via rec.RecordError.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()

		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, req)

		elapsed := time.Since(start)

		key := req.Pattern
		if key == "" {
			key = req.Method + " " + req.URL.Path
		}

		m.rec.Record(key, elapsed)

		if sr.status >= 500 {
			m.rec.RecordError(key)
		}
	})
}
