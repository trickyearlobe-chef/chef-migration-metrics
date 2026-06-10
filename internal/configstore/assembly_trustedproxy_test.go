// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package configstore

import (
	"slices"
	"testing"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
)

// server.trusted_proxy round-trips through ConfigToSections → AssembleConfigRaw
// so a CLI- or UI-set value is read back at startup/reload (behind-proxy
// plain-HTTP deployment, tls-static.md). Before this wiring the value lived only
// in YAML and was lost on migration to the DB.
func TestServerTrustedProxy_RoundTrip(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.TrustedProxy = true

	sections, err := ConfigToSections(cfg)
	if err != nil {
		t.Fatalf("ConfigToSections: %v", err)
	}

	raw, ok := sections[KeyServerTrustedProxy]
	if !ok {
		t.Fatalf("ConfigToSections did not produce a %q section", KeyServerTrustedProxy)
	}
	if string(raw) != "true" {
		t.Errorf("stored value = %s, want true", raw)
	}

	assembled, err := AssembleConfigRaw(sections)
	if err != nil {
		t.Fatalf("AssembleConfigRaw: %v", err)
	}
	if !assembled.Server.TrustedProxy {
		t.Error("TrustedProxy = false, want true after round-trip")
	}
}

// KeyServerTrustedProxy is a recognised config key so the YAML migration's
// unknown-key detection does not flag it.
func TestServerTrustedProxy_IsKnownKey(t *testing.T) {
	if !slices.Contains(AllConfigKeys(), KeyServerTrustedProxy) {
		t.Errorf("%q not in AllConfigKeys()", KeyServerTrustedProxy)
	}
}
