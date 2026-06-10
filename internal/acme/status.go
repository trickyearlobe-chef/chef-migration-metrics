// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// Status is the operator-facing ACME health surface (tls-acme.md § 3.14),
// persisted non-secret at server.tls.acme.status and read by the admin config
// GET to populate the Server & TLS status panel. All times are RFC 3339 strings
// so the value serialises cleanly to the frontend and an unset field is empty.
type Status struct {
	// LastRenewal is the time of the last successful issue/renewal; empty if
	// none has succeeded yet.
	LastRenewal string `json:"last_renewal,omitempty"`
	// LastError is the most recent issuance/renewal error; empty after a success.
	LastError string `json:"last_error,omitempty"`
	// HostnameError is the most recent self-registration error (§ 3.13); empty
	// after a success or when register_hostname is off.
	HostnameError string `json:"hostname_error,omitempty"`
}

// Status returns the persisted operator status. A never-written entry is not an
// error: it yields the zero Status so callers (the admin GET) need not special
// case "no status yet".
func (s *Storage) Status(ctx context.Context) (Status, error) {
	raw, err := s.store.Get(ctx, configstore.KeyServerTLSACMEStatus)
	if err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			return Status{}, nil
		}
		return Status{}, err
	}
	var st Status
	if err := json.Unmarshal(raw, &st); err != nil {
		return Status{}, fmt.Errorf("acme: decode status: %w", err)
	}
	return st, nil
}

// UpdateStatus applies mutate to the current status and persists the result. It
// is read-modify-write so the renewal-outcome writer and the
// hostname-registration writer never clobber each other's fields. The status is
// always stored non-secret.
func (s *Storage) UpdateStatus(ctx context.Context, mutate func(*Status)) error {
	cur, err := s.Status(ctx)
	if err != nil {
		return err
	}
	mutate(&cur)
	value, err := json.Marshal(cur)
	if err != nil {
		return fmt.Errorf("acme: encode status: %w", err)
	}
	if err := s.store.Set(ctx, configstore.KeyServerTLSACMEStatus, value, false, updatedBy); err != nil {
		return fmt.Errorf("acme: store status: %w", err)
	}
	return nil
}
