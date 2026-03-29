// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
    "context"
    "testing"
)

// ---------------------------------------------------------------------------
// UpsertOrganisationFromConfig — parameter validation
// ---------------------------------------------------------------------------

func TestUpsertOrganisationFromConfig_MissingName(t *testing.T) {
    db := &DB{}
    _, err := db.upsertOrganisationFromConfig(context.TODO(), nil, UpsertOrganisationParams{
        ChefServerURL: "https://chef.example.com",
        OrgName:       "myorg",
        ClientName:    "pivotal",
    })
    if err == nil {
        t.Fatal("expected error for missing name")
    }
    if got := err.Error(); got != "datastore: organisation name is required" {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertOrganisationFromConfig_MissingChefServerURL(t *testing.T) {
    db := &DB{}
    _, err := db.upsertOrganisationFromConfig(context.TODO(), nil, UpsertOrganisationParams{
        Name:       "prod",
        OrgName:    "myorg",
        ClientName: "pivotal",
    })
    if err == nil {
        t.Fatal("expected error for missing chef_server_url")
    }
    expected := `datastore: chef server URL is required for organisation "prod"`
    if got := err.Error(); got != expected {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertOrganisationFromConfig_MissingOrgName(t *testing.T) {
    db := &DB{}
    _, err := db.upsertOrganisationFromConfig(context.TODO(), nil, UpsertOrganisationParams{
        Name:          "prod",
        ChefServerURL: "https://chef.example.com",
        ClientName:    "pivotal",
    })
    if err == nil {
        t.Fatal("expected error for missing org_name")
    }
    expected := `datastore: org name is required for organisation "prod"`
    if got := err.Error(); got != expected {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertOrganisationFromConfig_MissingClientName(t *testing.T) {
    db := &DB{}
    _, err := db.upsertOrganisationFromConfig(context.TODO(), nil, UpsertOrganisationParams{
        Name:          "prod",
        ChefServerURL: "https://chef.example.com",
        OrgName:       "myorg",
    })
    if err == nil {
        t.Fatal("expected error for missing client_name")
    }
    expected := `datastore: client name is required for organisation "prod"`
    if got := err.Error(); got != expected {
        t.Errorf("unexpected error: %v", err)
    }
}

func TestUpsertOrganisationFromConfig_ValidationOrder(t *testing.T) {
    db := &DB{}

    // All fields missing — should fail on name first.
    _, err := db.upsertOrganisationFromConfig(context.TODO(), nil, UpsertOrganisationParams{})
    if err == nil {
        t.Fatal("expected error for empty params")
    }
    if got := err.Error(); got != "datastore: organisation name is required" {
        t.Errorf("expected name error first, got: %v", err)
    }

    // Name present — should fail on chef_server_url.
    _, err = db.upsertOrganisationFromConfig(context.TODO(), nil, UpsertOrganisationParams{
        Name: "prod",
    })
    if err == nil {
        t.Fatal("expected error for missing chef_server_url")
    }
    expected := `datastore: chef server URL is required for organisation "prod"`
    if got := err.Error(); got != expected {
        t.Errorf("expected chef_server_url error, got: %v", err)
    }

    // Name + ChefServerURL — should fail on org_name.
    _, err = db.upsertOrganisationFromConfig(context.TODO(), nil, UpsertOrganisationParams{
        Name:          "prod",
        ChefServerURL: "https://chef.example.com",
    })
    if err == nil {
        t.Fatal("expected error for missing org_name")
    }
    expected = `datastore: org name is required for organisation "prod"`
    if got := err.Error(); got != expected {
        t.Errorf("expected org_name error, got: %v", err)
    }

    // Name + ChefServerURL + OrgName — should fail on client_name.
    _, err = db.upsertOrganisationFromConfig(context.TODO(), nil, UpsertOrganisationParams{
        Name:          "prod",
        ChefServerURL: "https://chef.example.com",
        OrgName:       "myorg",
    })
    if err == nil {
        t.Fatal("expected error for missing client_name")
    }
    expected = `datastore: client name is required for organisation "prod"`
    if got := err.Error(); got != expected {
        t.Errorf("expected client_name error, got: %v", err)
    }
}

// ---------------------------------------------------------------------------
// UpsertOrganisationParams — zero-value defaults
// ---------------------------------------------------------------------------

func TestUpsertOrganisationParams_Defaults(t *testing.T) {
    var p UpsertOrganisationParams
    if p.Name != "" {
        t.Errorf("zero-value Name should be empty, got %q", p.Name)
    }
    if p.ChefServerURL != "" {
        t.Errorf("zero-value ChefServerURL should be empty, got %q", p.ChefServerURL)
    }
    if p.OrgName != "" {
        t.Errorf("zero-value OrgName should be empty, got %q", p.OrgName)
    }
    if p.ClientName != "" {
        t.Errorf("zero-value ClientName should be empty, got %q", p.ClientName)
    }
    if p.ClientKeyCredentialID != "" {
        t.Errorf("zero-value ClientKeyCredentialID should be empty, got %q", p.ClientKeyCredentialID)
    }
}

// ---------------------------------------------------------------------------
// Organisation struct — zero value
// ---------------------------------------------------------------------------

func TestOrganisation_ZeroValue(t *testing.T) {
    var o Organisation
    if o.ID != "" {
        t.Errorf("zero-value ID should be empty, got %q", o.ID)
    }
    if o.Name != "" {
        t.Errorf("zero-value Name should be empty, got %q", o.Name)
    }
}
