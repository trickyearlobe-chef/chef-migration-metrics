// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package datastore

import (
	"testing"
)

func TestInsertOwnerAlias_Validation(t *testing.T) {
	db := &DB{} // nil pool — we only test validation, not DB calls

	tests := []struct {
		name    string
		params  InsertOwnerAliasParams
		wantErr string
	}{
		{
			name:    "missing owner_name",
			params:  InsertOwnerAliasParams{AliasType: "email", AliasValue: "x@example.com"},
			wantErr: "owner_name is required",
		},
		{
			name:    "missing alias_type",
			params:  InsertOwnerAliasParams{OwnerName: "team-a", AliasValue: "x@example.com"},
			wantErr: "alias_type is required",
		},
		{
			name:    "missing alias_value",
			params:  InsertOwnerAliasParams{OwnerName: "team-a", AliasType: "email"},
			wantErr: "alias_value is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := db.InsertOwnerAlias(t.Context(), tt.params)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); got != "datastore: "+tt.wantErr {
				t.Errorf("error = %q, want contains %q", got, tt.wantErr)
			}
		})
	}
}

func TestOwnerAlias_ZeroValue(t *testing.T) {
	var a OwnerAlias
	if a.ID != "" || a.OwnerName != "" || a.AliasType != "" || a.AliasValue != "" {
		t.Error("zero-value OwnerAlias should have empty fields")
	}
}

func TestInsertOwnerAliasParams_DefaultSource(t *testing.T) {
	p := InsertOwnerAliasParams{
		OwnerName:  "team-a",
		AliasType:  "email",
		AliasValue: "test@example.com",
	}
	if p.Source != "" {
		t.Errorf("zero-value Source should be empty, got %q", p.Source)
	}
}
