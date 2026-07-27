// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/ingest"
)

// handleIngest is the passive telemetry sink: POST /api/v1/ingest. It accepts
// Chef run telemetry from three producer shapes (node-direct run_converge, Chef
// Server proxy relay, Chef Automate Data Feed), gunzips + splits NDJSON,
// normalises each record, and persists the converge runs in ONE transaction.
//
// It is INTENTIONALLY UNAUTHENTICATED in the MVP (tech debt) — registered
// directly on the mux, not behind r.protect. It always answers in Automate's
// accepted set (200) on receipt: Automate drops a Data Feed destination that
// answers outside 200-204, so a malformed/oversize/failed body is dropped with
// a 200, never bounced back to the producer. See specifications/event-ingest.md.
func (r *Router) handleIngest(w http.ResponseWriter, req *http.Request) {
	if !requirePOST(w, req) {
		return
	}

	ic := r.liveConfig().Ingest
	if !ic.IsEnabled() {
		// Opt-in: when disabled the endpoint does not exist.
		WriteError(w, http.StatusNotFound, ErrCodeNotFound, "Ingest is not enabled.")
		return
	}

	maxBody := ic.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 32 << 20
	}
	maxRecords := ic.MaxRecordsPerBody
	if maxRecords <= 0 {
		maxRecords = 500
	}

	body, tooLarge, err := readCappedBody(w, req, maxBody)
	if err != nil {
		r.logf("ERROR", "ingest: reading body: %v", err)
		writeIngestAccepted(w, 0, 0)
		return
	}
	if tooLarge {
		r.logf("WARN", "ingest: body exceeded %d bytes (size/gzip-bomb guard) — dropped", maxBody)
		writeIngestAccepted(w, 0, 0)
		return
	}

	records, overLimit, err := decodeJSONStream(body, maxRecords)
	if err != nil {
		// Malformed/partial body → persist nothing.
		r.logf("WARN", "ingest: malformed body — dropped: %v", err)
		writeIngestAccepted(w, 0, 0)
		return
	}
	if overLimit {
		r.logf("WARN", "ingest: record count exceeds cap %d — dropped", maxRecords)
		writeIngestAccepted(w, 0, 0)
		return
	}

	failuresOnly := ic.IsFailuresOnly()
	runs := make([]ingest.ConvergeRun, 0, len(records))
	for _, rec := range records {
		run, err := ingest.Normalise(rec)
		if err != nil {
			// A record that decodes as JSON but cannot be mapped (e.g. a converge
			// with no end_time) taints the body — persist nothing.
			r.logf("WARN", "ingest: unmappable record — body dropped: %v", err)
			writeIngestAccepted(w, len(records), 0)
			return
		}
		if run == nil { // nil == accepted-but-ignored shape (run_start, attributes-only)
			continue
		}
		// Firehose-relief valve: when failures_only is set, discard success events
		// and keep only failures.
		if failuresOnly && run.Status != "failure" {
			continue
		}
		runs = append(runs, *run)
	}

	stored := 0
	if len(runs) > 0 {
		n, err := r.db.BulkUpsertConvergeRuns(req.Context(), runs)
		if err != nil {
			// Persist failed — accept and drop rather than bounce the producer.
			r.logf("ERROR", "ingest: persisting %d runs: %v", len(runs), err)
			writeIngestAccepted(w, len(records), 0)
			return
		}
		stored = n
	}

	writeIngestAccepted(w, len(records), stored)
}

// writeIngestAccepted always responds 200 (Automate's accepted set) with a small
// receipt of how many records arrived and how many were newly stored.
func writeIngestAccepted(w http.ResponseWriter, received, stored int) {
	WriteJSON(w, http.StatusOK, map[string]int{"received": received, "stored": stored})
}

// readCappedBody reads the request body under a hard byte cap, transparently
// gunzipping when Content-Encoding: gzip. The cap applies to the DECOMPRESSED
// size too, so a gzip bomb is caught. Returns tooLarge=true (not an error) when
// either side exceeds the cap.
func readCappedBody(w http.ResponseWriter, req *http.Request, max int64) (body []byte, tooLarge bool, err error) {
	limited := http.MaxBytesReader(w, req.Body, max)
	var reader io.Reader = limited

	if strings.EqualFold(req.Header.Get("Content-Encoding"), "gzip") {
		gz, gerr := gzip.NewReader(limited)
		if gerr != nil {
			return nil, false, gerr
		}
		defer func() { _ = gz.Close() }() // read path — Close only re-checks CRC of an already-read body
		reader = io.LimitReader(gz, max+1)
	}

	data, rerr := io.ReadAll(reader)
	if rerr != nil {
		var mbe *http.MaxBytesError
		if errors.As(rerr, &mbe) {
			return nil, true, nil // compressed side exceeded the cap
		}
		return nil, false, rerr
	}
	if int64(len(data)) > max {
		return nil, true, nil // decompressed side exceeded the cap (gzip bomb)
	}
	return data, false, nil
}

// decodeJSONStream parses a body as one-or-more successive JSON values (NDJSON,
// or a single object, LF/CRLF/whitespace-separated alike). Returns overLimit
// when the count exceeds max, or an error on the first malformed value.
func decodeJSONStream(body []byte, max int) (records []json.RawMessage, overLimit bool, err error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	for {
		var raw json.RawMessage
		if e := dec.Decode(&raw); e != nil {
			if errors.Is(e, io.EOF) {
				break
			}
			return records, false, e
		}
		records = append(records, raw)
		if len(records) > max {
			return records, true, nil
		}
	}
	return records, false, nil
}
