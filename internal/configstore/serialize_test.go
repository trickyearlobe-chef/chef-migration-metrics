// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// SerializeValue and DeserializeValue are inverses for a config sub-struct,
// using snake_case yaml tags for the stored field names.
func TestSerializeDeserializeValue_RoundTrip(t *testing.T) {
	in := config.TLSConfig{
		Mode:       "static",
		CertSource: "db",
		CertPath:   "/etc/cmm/cert.pem",
		KeyPath:    "/etc/cmm/key.pem",
		CAPath:     "/etc/cmm/ca.pem",
		MinVersion: "1.2",
	}

	raw, err := SerializeValue(in)
	if err != nil {
		t.Fatalf("SerializeValue: %v", err)
	}

	// Field names are stored snake_case (yaml tags), not Go PascalCase.
	if got := string(raw); !contains(got, `"cert_source"`) || !contains(got, `"ca_path"`) {
		t.Fatalf("serialised value missing snake_case keys: %s", got)
	}

	var out config.TLSConfig
	if err := DeserializeValue(raw, &out); err != nil {
		t.Fatalf("DeserializeValue: %v", err)
	}
	// Compare the scalar fields. (A whole-struct DeepEqual would trip on the
	// yaml→json→yaml round-trip normalising nil slices/maps in ACMEConfig to
	// empty, which is irrelevant here.)
	if out.Mode != in.Mode || out.CertSource != in.CertSource ||
		out.CertPath != in.CertPath || out.KeyPath != in.KeyPath ||
		out.CAPath != in.CAPath || out.MinVersion != in.MinVersion {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", out, in)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
