// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
    "context"
    "testing"
)

// ---------------------------------------------------------------------------
// UpsertNodeReadiness — parameter validation
// ---------------------------------------------------------------------------

func TestUpsertNodeReadiness_MissingNodeSnapshotID(t *testing.T) {
    db := &DB{}
    _, err := db.upsertNodeReadiness(context.TODO(), nil, UpsertNodeReadinessParams{
        OrganisationID:    "org-1",
        NodeName:          "web01",
        TargetChefVersion: "18.0.0",
    })
    if err == nil {
        t.Fatal("expected error for missing node_snapshot_id")
    }
    if got := err.Error(); got != "datastore: node_snapshot_id is required" {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertNodeReadiness_MissingOrganisationID(t *testing.T) {
    db := &DB{}
    _, err := db.upsertNodeReadiness(context.TODO(), nil, UpsertNodeReadinessParams{
        NodeSnapshotID:    "snap-1",
        NodeName:          "web01",
        TargetChefVersion: "18.0.0",
    })
    if err == nil {
        t.Fatal("expected error for missing organisation_id")
    }
    if got := err.Error(); got != "datastore: organisation_id is required" {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertNodeReadiness_MissingNodeName(t *testing.T) {
    db := &DB{}
    _, err := db.upsertNodeReadiness(context.TODO(), nil, UpsertNodeReadinessParams{
        NodeSnapshotID:    "snap-1",
        OrganisationID:    "org-1",
        TargetChefVersion: "18.0.0",
    })
    if err == nil {
        t.Fatal("expected error for missing node_name")
    }
    if got := err.Error(); got != "datastore: node_name is required" {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertNodeReadiness_MissingTargetChefVersion(t *testing.T) {
    db := &DB{}
    _, err := db.upsertNodeReadiness(context.TODO(), nil, UpsertNodeReadinessParams{
        NodeSnapshotID: "snap-1",
        OrganisationID: "org-1",
        NodeName:       "web01",
    })
    if err == nil {
        t.Fatal("expected error for missing target_chef_version")
    }
    if got := err.Error(); got != "datastore: target_chef_version is required" {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertNodeReadiness_ValidationOrder(t *testing.T) {
    db := &DB{}

    // All fields missing — should fail on node_snapshot_id first.
    _, err := db.upsertNodeReadiness(context.TODO(), nil, UpsertNodeReadinessParams{})
    if err == nil {
        t.Fatal("expected error for empty params")
    }
    if got := err.Error(); got != "datastore: node_snapshot_id is required" {
        t.Errorf("expected node_snapshot_id error first, got: %v", err)
    }

    // node_snapshot_id present — should fail on organisation_id.
    _, err = db.upsertNodeReadiness(context.TODO(), nil, UpsertNodeReadinessParams{
        NodeSnapshotID: "snap-1",
    })
    if err == nil {
        t.Fatal("expected error for missing organisation_id")
    }
    if got := err.Error(); got != "datastore: organisation_id is required" {
        t.Errorf("expected organisation_id error, got: %v", err)
    }

    // node_snapshot_id + organisation_id — should fail on node_name.
    _, err = db.upsertNodeReadiness(context.TODO(), nil, UpsertNodeReadinessParams{
        NodeSnapshotID: "snap-1",
        OrganisationID: "org-1",
    })
    if err == nil {
        t.Fatal("expected error for missing node_name")
    }
    if got := err.Error(); got != "datastore: node_name is required" {
        t.Errorf("expected node_name error, got: %v", err)
    }

    // node_snapshot_id + organisation_id + node_name — should fail on target_chef_version.
    _, err = db.upsertNodeReadiness(context.TODO(), nil, UpsertNodeReadinessParams{
        NodeSnapshotID: "snap-1",
        OrganisationID: "org-1",
        NodeName:       "web01",
    })
    if err == nil {
        t.Fatal("expected error for missing target_chef_version")
    }
    if got := err.Error(); got != "datastore: target_chef_version is required" {
        t.Errorf("expected target_chef_version error, got: %v", err)
    }
}

// ---------------------------------------------------------------------------
// UpsertNodeReadinessParams — boolean and optional field defaults
// ---------------------------------------------------------------------------

func TestUpsertNodeReadinessParams_Defaults(t *testing.T) {
    var p UpsertNodeReadinessParams

    if p.IsReady {
        t.Error("zero-value IsReady should be false")
    }
    if p.AllCookbooksCompatible {
        t.Error("zero-value AllCookbooksCompatible should be false")
    }
    if p.SufficientDiskSpace != nil {
        t.Error("zero-value SufficientDiskSpace should be nil")
    }
    if p.AvailableDiskMB != nil {
        t.Error("zero-value AvailableDiskMB should be nil")
    }
    if p.RequiredDiskMB != nil {
        t.Error("zero-value RequiredDiskMB should be nil")
    }
    if p.BlockingCookbooks != nil {
        t.Error("zero-value BlockingCookbooks should be nil")
    }
    if p.StaleData {
        t.Error("zero-value StaleData should be false")
    }
}

// ---------------------------------------------------------------------------
// NodeReadiness struct — zero value
// ---------------------------------------------------------------------------

func TestNodeReadiness_ZeroValue(t *testing.T) {
    var nr NodeReadiness
    if nr.ID != "" {
        t.Errorf("zero-value ID should be empty, got %q", nr.ID)
    }
    if nr.NodeSnapshotID != "" {
        t.Errorf("zero-value NodeSnapshotID should be empty, got %q", nr.NodeSnapshotID)
    }
    if nr.IsReady {
        t.Error("zero-value IsReady should be false")
    }
    if nr.AllCookbooksCompatible {
        t.Error("zero-value AllCookbooksCompatible should be false")
    }
    if nr.SufficientDiskSpace != nil {
        t.Error("zero-value SufficientDiskSpace should be nil")
    }
    if nr.AvailableDiskMB != nil {
        t.Error("zero-value AvailableDiskMB should be nil")
    }
    if nr.RequiredDiskMB != nil {
        t.Error("zero-value RequiredDiskMB should be nil")
    }
    if nr.StaleData {
        t.Error("zero-value StaleData should be false")
    }
    if nr.BlockingCookbooks != nil {
        t.Error("zero-value BlockingCookbooks should be nil")
    }
}
