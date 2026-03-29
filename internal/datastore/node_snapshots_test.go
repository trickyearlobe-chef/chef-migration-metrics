// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
    "context"
    "testing"
)

// ---------------------------------------------------------------------------
// UpsertNodeSnapshot — parameter validation
// ---------------------------------------------------------------------------

func TestUpsertNodeSnapshot_MissingCollectionRunID(t *testing.T) {
    db := &DB{}
    _, err := db.upsertNodeSnapshot(context.TODO(), nil, InsertNodeSnapshotParams{
        OrganisationID: "org-1",
        NodeName:       "web01",
    })
    if err == nil {
        t.Fatal("expected error for missing collection_run_id")
    }
    if got := err.Error(); got != "datastore: collection run ID is required to insert a node snapshot" {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertNodeSnapshot_MissingOrganisationID(t *testing.T) {
    db := &DB{}
    _, err := db.upsertNodeSnapshot(context.TODO(), nil, InsertNodeSnapshotParams{
        CollectionRunID: "run-1",
        NodeName:        "web01",
    })
    if err == nil {
        t.Fatal("expected error for missing organisation_id")
    }
    if got := err.Error(); got != "datastore: organisation ID is required to insert a node snapshot" {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertNodeSnapshot_MissingNodeName(t *testing.T) {
    db := &DB{}
    _, err := db.upsertNodeSnapshot(context.TODO(), nil, InsertNodeSnapshotParams{
        CollectionRunID: "run-1",
        OrganisationID:  "org-1",
    })
    if err == nil {
        t.Fatal("expected error for missing node_name")
    }
    if got := err.Error(); got != "datastore: node name is required to insert a node snapshot" {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertNodeSnapshot_ValidationOrder(t *testing.T) {
    db := &DB{}

    // All fields missing — should fail on collection_run_id first.
    _, err := db.upsertNodeSnapshot(context.TODO(), nil, InsertNodeSnapshotParams{})
    if err == nil {
        t.Fatal("expected error for empty params")
    }
    if got := err.Error(); got != "datastore: collection run ID is required to insert a node snapshot" {
        t.Errorf("expected collection_run_id error first, got: %v", err)
    }

    // collection_run_id present — should fail on organisation_id.
    _, err = db.upsertNodeSnapshot(context.TODO(), nil, InsertNodeSnapshotParams{
        CollectionRunID: "run-1",
    })
    if err == nil {
        t.Fatal("expected error for missing organisation_id")
    }
    if got := err.Error(); got != "datastore: organisation ID is required to insert a node snapshot" {
        t.Errorf("expected organisation_id error, got: %v", err)
    }

    // collection_run_id + organisation_id — should fail on node_name.
    _, err = db.upsertNodeSnapshot(context.TODO(), nil, InsertNodeSnapshotParams{
        CollectionRunID: "run-1",
        OrganisationID:  "org-1",
    })
    if err == nil {
        t.Fatal("expected error for missing node_name")
    }
    if got := err.Error(); got != "datastore: node name is required to insert a node snapshot" {
        t.Errorf("expected node_name error, got: %v", err)
    }
}

// ---------------------------------------------------------------------------
// InsertNodeSnapshotParams — zero-value defaults
// ---------------------------------------------------------------------------

func TestInsertNodeSnapshotParams_Defaults(t *testing.T) {
    var p InsertNodeSnapshotParams
    if p.CollectionRunID != "" {
        t.Errorf("zero-value CollectionRunID should be empty, got %q", p.CollectionRunID)
    }
    if p.OrganisationID != "" {
        t.Errorf("zero-value OrganisationID should be empty, got %q", p.OrganisationID)
    }
    if p.NodeName != "" {
        t.Errorf("zero-value NodeName should be empty, got %q", p.NodeName)
    }
    if p.ChefEnvironment != "" {
        t.Errorf("zero-value ChefEnvironment should be empty, got %q", p.ChefEnvironment)
    }
    if p.ChefVersion != "" {
        t.Errorf("zero-value ChefVersion should be empty, got %q", p.ChefVersion)
    }
    if p.Platform != "" {
        t.Errorf("zero-value Platform should be empty, got %q", p.Platform)
    }
    if p.PlatformVersion != "" {
        t.Errorf("zero-value PlatformVersion should be empty, got %q", p.PlatformVersion)
    }
    if p.PlatformFamily != "" {
        t.Errorf("zero-value PlatformFamily should be empty, got %q", p.PlatformFamily)
    }
    if p.IsStale {
        t.Error("zero-value IsStale should be false")
    }
    if !p.CollectedAt.IsZero() {
        t.Error("zero-value CollectedAt should be zero time")
    }
    if p.Filesystem != nil {
        t.Error("zero-value Filesystem should be nil")
    }
    if p.Cookbooks != nil {
        t.Error("zero-value Cookbooks should be nil")
    }
    if p.RunList != nil {
        t.Error("zero-value RunList should be nil")
    }
    if p.Roles != nil {
        t.Error("zero-value Roles should be nil")
    }
}

// ---------------------------------------------------------------------------
// NodeSnapshot struct — zero value
// ---------------------------------------------------------------------------

func TestNodeSnapshot_ZeroValue(t *testing.T) {
    var ns NodeSnapshot
    if ns.ID != "" {
        t.Errorf("zero-value ID should be empty, got %q", ns.ID)
    }
    if ns.CollectionRunID != "" {
        t.Errorf("zero-value CollectionRunID should be empty, got %q", ns.CollectionRunID)
    }
    if ns.OrganisationID != "" {
        t.Errorf("zero-value OrganisationID should be empty, got %q", ns.OrganisationID)
    }
    if ns.NodeName != "" {
        t.Errorf("zero-value NodeName should be empty, got %q", ns.NodeName)
    }
    if ns.ChefEnvironment != "" {
        t.Errorf("zero-value ChefEnvironment should be empty, got %q", ns.ChefEnvironment)
    }
    if ns.ChefVersion != "" {
        t.Errorf("zero-value ChefVersion should be empty, got %q", ns.ChefVersion)
    }
    if ns.Platform != "" {
        t.Errorf("zero-value Platform should be empty, got %q", ns.Platform)
    }
    if ns.PlatformVersion != "" {
        t.Errorf("zero-value PlatformVersion should be empty, got %q", ns.PlatformVersion)
    }
    if ns.PlatformFamily != "" {
        t.Errorf("zero-value PlatformFamily should be empty, got %q", ns.PlatformFamily)
    }
    if ns.IsStale {
        t.Error("zero-value IsStale should be false")
    }
    if !ns.CollectedAt.IsZero() {
        t.Error("zero-value CollectedAt should be zero time")
    }
    if !ns.CreatedAt.IsZero() {
        t.Error("zero-value CreatedAt should be zero time")
    }
    if ns.IsPolicyfileNode() {
        t.Error("zero-value IsPolicyfileNode() should be false")
    }
}

// ---------------------------------------------------------------------------
// IsPolicyfileNode — method tests
// ---------------------------------------------------------------------------

func TestNodeSnapshot_IsPolicyfileNode_BothSet(t *testing.T) {
    ns := NodeSnapshot{PolicyName: "base", PolicyGroup: "production"}
    if !ns.IsPolicyfileNode() {
        t.Error("expected IsPolicyfileNode() = true when both PolicyName and PolicyGroup are set")
    }
}

func TestNodeSnapshot_IsPolicyfileNode_OnlyPolicyName(t *testing.T) {
    ns := NodeSnapshot{PolicyName: "base"}
    if ns.IsPolicyfileNode() {
        t.Error("expected IsPolicyfileNode() = false when only PolicyName is set")
    }
}

func TestNodeSnapshot_IsPolicyfileNode_OnlyPolicyGroup(t *testing.T) {
    ns := NodeSnapshot{PolicyGroup: "production"}
    if ns.IsPolicyfileNode() {
        t.Error("expected IsPolicyfileNode() = false when only PolicyGroup is set")
    }
}

func TestNodeSnapshot_IsPolicyfileNode_NeitherSet(t *testing.T) {
    ns := NodeSnapshot{}
    if ns.IsPolicyfileNode() {
        t.Error("expected IsPolicyfileNode() = false when neither PolicyName nor PolicyGroup is set")
    }
}

// ---------------------------------------------------------------------------
// BulkUpsertNodeSnapshots — parameter validation
// ---------------------------------------------------------------------------

func TestBulkUpsertNodeSnapshots_EmptyParams(t *testing.T) {
    // BulkUpsert with an empty slice should succeed without touching the DB.
    db := &DB{}
    _, n, err := db.bulkUpsertNodeSnapshots(context.TODO(), nil, false)
    if err != nil {
        t.Fatalf("unexpected error for empty params: %v", err)
    }
    if n != 0 {
        t.Errorf("expected 0 inserted for empty params, got %d", n)
    }
}
