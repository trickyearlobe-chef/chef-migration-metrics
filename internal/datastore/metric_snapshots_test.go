// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// InsertMetricSnapshot — parameter validation
// ---------------------------------------------------------------------------

func TestInsertMetricSnapshot_MissingOrganisationName(t *testing.T) {
	db := &DB{}
	_, err := db.insertMetricSnapshot(context.TODO(), nil, InsertMetricSnapshotParams{
		SnapshotType: "chef_version_distribution",
		Data:         []byte(`{"total_nodes":0}`),
		SnapshotAt:   time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for missing organisation name")
	}
	if got := err.Error(); got != "datastore: organisation name is required to insert a metric snapshot" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInsertMetricSnapshot_MissingSnapshotType(t *testing.T) {
	db := &DB{}
	_, err := db.insertMetricSnapshot(context.TODO(), nil, InsertMetricSnapshotParams{
		OrganisationName: "org-1",
		Data:             []byte(`{"total_nodes":0}`),
		SnapshotAt:       time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for missing snapshot type")
	}
	if got := err.Error(); got != "datastore: snapshot type is required to insert a metric snapshot" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInsertMetricSnapshot_MissingData(t *testing.T) {
	db := &DB{}
	_, err := db.insertMetricSnapshot(context.TODO(), nil, InsertMetricSnapshotParams{
		OrganisationName: "org-1",
		SnapshotType:     "readiness_summary",
		SnapshotAt:       time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for missing data")
	}
	if got := err.Error(); got != "datastore: data is required to insert a metric snapshot" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInsertMetricSnapshot_EmptyData(t *testing.T) {
	db := &DB{}
	_, err := db.insertMetricSnapshot(context.TODO(), nil, InsertMetricSnapshotParams{
		OrganisationName: "org-1",
		SnapshotType:     "readiness_summary",
		Data:             []byte{},
		SnapshotAt:       time.Now(),
	})
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

// ---------------------------------------------------------------------------
// MetricSnapshot struct
// ---------------------------------------------------------------------------

func TestMetricSnapshot_ZeroValue(t *testing.T) {
	var ms MetricSnapshot
	if ms.ID != 0 {
		t.Errorf("zero-value ID should be 0, got %d", ms.ID)
	}
	if ms.SnapshotType != "" {
		t.Errorf("zero-value SnapshotType should be empty, got %q", ms.SnapshotType)
	}
	if ms.Data != nil {
		t.Error("zero-value Data should be nil")
	}
}

// ---------------------------------------------------------------------------
// InsertMetricSnapshotParams — validation order
// ---------------------------------------------------------------------------

func TestInsertMetricSnapshot_ValidationOrder(t *testing.T) {
	// All fields missing — should fail on organisation name first.
	db := &DB{}
	_, err := db.insertMetricSnapshot(context.TODO(), nil, InsertMetricSnapshotParams{})
	if err == nil {
		t.Fatal("expected error for empty params")
	}
	if got := err.Error(); got != "datastore: organisation name is required to insert a metric snapshot" {
		t.Errorf("expected organisation name error first, got: %v", err)
	}

	// Organisation name present, rest missing — should fail on snapshot type.
	_, err = db.insertMetricSnapshot(context.TODO(), nil, InsertMetricSnapshotParams{
		OrganisationName: "org-1",
	})
	if err == nil {
		t.Fatal("expected error for missing snapshot type")
	}
	if got := err.Error(); got != "datastore: snapshot type is required to insert a metric snapshot" {
		t.Errorf("expected snapshot type error, got: %v", err)
	}

	// Organisation name + snapshot type present, data missing — should fail on data.
	_, err = db.insertMetricSnapshot(context.TODO(), nil, InsertMetricSnapshotParams{
		OrganisationName: "org-1",
		SnapshotType:     "readiness_summary",
	})
	if err == nil {
		t.Fatal("expected error for missing data")
	}
	if got := err.Error(); got != "datastore: data is required to insert a metric snapshot" {
		t.Errorf("expected data error, got: %v", err)
	}
}
