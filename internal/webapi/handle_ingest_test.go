// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ingest"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "event-ingest", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return b
}

func gzipBytes(t *testing.T, b []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// ingestRouter returns a router with the ingest endpoint enabled and a mock
// store that records the runs handed to BulkUpsertConvergeRuns.
func ingestRouter(t *testing.T, tweak func(*config.IngestConfig)) (*Router, *[]ingest.ConvergeRun, *int) {
	t.Helper()
	var captured []ingest.ConvergeRun
	calls := 0
	store := &mockStore{
		BulkUpsertConvergeRunsFn: func(ctx context.Context, runs []ingest.ConvergeRun) (int, error) {
			calls++
			captured = append(captured, runs...)
			return len(runs), nil
		},
	}
	cfg := testConfig()
	on := true
	cfg.Ingest.Enabled = &on
	if tweak != nil {
		tweak(&cfg.Ingest)
	}
	return newTestRouterWithMockAndConfig(store, cfg), &captured, &calls
}

func postIngest(r *Router, body []byte, gzipped bool) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if gzipped {
		req.Header.Set("Content-Encoding", "gzip")
	}
	r.ServeHTTP(w, req)
	return w
}

func decodeReceipt(t *testing.T, w *httptest.ResponseRecorder) (received, stored int) {
	t.Helper()
	var body struct {
		Received int `json:"received"`
		Stored   int `json:"stored"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding receipt %q: %v", w.Body.String(), err)
	}
	return body.Received, body.Stored
}

// A gzip NDJSON batch of two records (mixed shapes) → both normalised and stored.
func TestHandleIngest_GzipNDJSONBatch(t *testing.T) {
	r, captured, _ := ingestRouter(t, nil)
	ndjson := append(append([]byte{}, bytes.TrimSpace(fixture(t, "datafeed_success.json"))...), '\n')
	ndjson = append(ndjson, bytes.TrimSpace(fixture(t, "run_converge_success.json"))...)

	w := postIngest(r, gzipBytes(t, ndjson), true)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	received, stored := decodeReceipt(t, w)
	if received != 2 || stored != 2 {
		t.Errorf("receipt = received %d stored %d, want 2/2", received, stored)
	}
	if len(*captured) != 2 {
		t.Fatalf("captured %d runs, want 2", len(*captured))
	}
	shapes := map[string]int{}
	for _, run := range *captured {
		shapes[run.Shape]++
	}
	if shapes[ingest.ShapeDataFeed] != 1 || shapes[ingest.ShapeConverge] != 1 {
		t.Errorf("shapes = %v, want one of each", shapes)
	}
}

// A plain (uncompressed) single object body → one stored run.
func TestHandleIngest_PlainSingleObject(t *testing.T) {
	r, captured, _ := ingestRouter(t, nil)
	w := postIngest(r, fixture(t, "run_converge_failure.json"), false)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(*captured) != 1 {
		t.Fatalf("captured %d runs, want 1", len(*captured))
	}
	if (*captured)[0].Status != "failure" || (*captured)[0].Error == nil {
		t.Errorf("captured run = %+v, want a failure with error", (*captured)[0])
	}
}

// A malformed body must persist NOTHING and still answer 200 (never bounce the
// producer into a 4xx/5xx that makes Automate drop the destination).
func TestHandleIngest_MalformedPersistsNothing(t *testing.T) {
	r, captured, calls := ingestRouter(t, nil)
	body := append(bytes.TrimSpace(fixture(t, "datafeed_success.json")), []byte("\n{ this is not json")...)

	w := postIngest(r, body, false)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (accept-and-drop)", w.Code)
	}
	if *calls != 0 {
		t.Errorf("BulkUpsert called %d times, want 0 (one txn per body — malformed persists nothing)", *calls)
	}
	if len(*captured) != 0 {
		t.Errorf("captured %d runs, want 0", len(*captured))
	}
	_, stored := decodeReceipt(t, w)
	if stored != 0 {
		t.Errorf("stored = %d, want 0", stored)
	}
}

// Accepted-but-ignored shapes (run_start) yield a row-free 200 and no store call.
func TestHandleIngest_IgnoredShapeNoRows(t *testing.T) {
	r, captured, calls := ingestRouter(t, nil)
	w := postIngest(r, fixture(t, "run_start.json"), false)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *calls != 0 || len(*captured) != 0 {
		t.Errorf("calls=%d captured=%d, want 0/0 for an ignored shape", *calls, len(*captured))
	}
	received, stored := decodeReceipt(t, w)
	if received != 1 || stored != 0 {
		t.Errorf("receipt = %d/%d, want received 1 stored 0", received, stored)
	}
}

// Disabled endpoint → 404, store never touched.
func TestHandleIngest_DisabledReturns404(t *testing.T) {
	store := &mockStore{BulkUpsertConvergeRunsFn: func(context.Context, []ingest.ConvergeRun) (int, error) {
		t.Fatal("store must not be called when ingest is disabled")
		return 0, nil
	}}
	cfg := testConfig() // Ingest.Enabled nil → disabled
	r := newTestRouterWithMockAndConfig(store, cfg)

	w := postIngest(r, fixture(t, "run_converge_success.json"), false)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when disabled", w.Code)
	}
}

// GET is rejected (405) by requirePOST.
func TestHandleIngest_MethodNotAllowed(t *testing.T) {
	r, _, _ := ingestRouter(t, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405 for GET", w.Code)
	}
}

// Exceeding the per-body record cap drops the whole body (200, nothing stored).
func TestHandleIngest_RecordCapDrops(t *testing.T) {
	r, captured, calls := ingestRouter(t, func(ic *config.IngestConfig) { ic.MaxRecordsPerBody = 1 })
	ndjson := append(append([]byte{}, bytes.TrimSpace(fixture(t, "datafeed_success.json"))...), '\n')
	ndjson = append(ndjson, bytes.TrimSpace(fixture(t, "run_converge_success.json"))...)

	w := postIngest(r, ndjson, false)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if *calls != 0 || len(*captured) != 0 {
		t.Errorf("calls=%d captured=%d, want 0/0 (over cap dropped)", *calls, len(*captured))
	}
}
