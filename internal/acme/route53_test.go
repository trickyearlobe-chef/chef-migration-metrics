// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// fakeRoute53 is an in-memory route53API for the solver tests — no real AWS.
// It records every ChangeResourceRecordSets call and serves a scripted sequence
// of change statuses to GetChange (repeating the last once exhausted).
type fakeRoute53 struct {
	changes    []*route53.ChangeResourceRecordSetsInput
	changeInfo *types.ChangeInfo
	changeErr  error

	getSequence []types.ChangeStatus
	getCalls    int
	getErr      error
}

func (f *fakeRoute53) ChangeResourceRecordSets(_ context.Context, in *route53.ChangeResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	f.changes = append(f.changes, in)
	if f.changeErr != nil {
		return nil, f.changeErr
	}
	info := f.changeInfo
	if info == nil {
		info = &types.ChangeInfo{Id: aws.String("/change/C0"), Status: types.ChangeStatusPending}
	}
	return &route53.ChangeResourceRecordSetsOutput{ChangeInfo: info}, nil
}

func (f *fakeRoute53) GetChange(_ context.Context, in *route53.GetChangeInput, _ ...func(*route53.Options)) (*route53.GetChangeOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	status := types.ChangeStatusInsync
	if n := len(f.getSequence); n > 0 {
		if f.getCalls < n {
			status = f.getSequence[f.getCalls]
		} else {
			status = f.getSequence[n-1]
		}
	}
	f.getCalls++
	return &route53.GetChangeOutput{ChangeInfo: &types.ChangeInfo{Id: in.Id, Status: status}}, nil
}

func fastSolver(api route53API, zone string) *Route53Solver {
	s := newRoute53Solver(api, zone, nil)
	s.pollInterval = time.Millisecond
	s.pollTimeout = 2 * time.Second
	return s
}

func TestRoute53SolverPresentUpsertsTXTAndWaitsInSync(t *testing.T) {
	f := &fakeRoute53{
		changeInfo:  &types.ChangeInfo{Id: aws.String("/change/C1"), Status: types.ChangeStatusPending},
		getSequence: []types.ChangeStatus{types.ChangeStatusPending, types.ChangeStatusInsync},
	}
	s := fastSolver(f, "Z123")
	ch := Challenge{Type: "dns-01", Domain: "metrics.example.com", Token: "tok", DNSValue: "abc123value"}

	if err := s.Present(context.Background(), ch); err != nil {
		t.Fatalf("Present: %v", err)
	}

	if len(f.changes) != 1 {
		t.Fatalf("ChangeResourceRecordSets calls = %d, want 1", len(f.changes))
	}
	in := f.changes[0]
	if got := aws.ToString(in.HostedZoneId); got != "Z123" {
		t.Errorf("HostedZoneId = %q, want Z123", got)
	}
	if n := len(in.ChangeBatch.Changes); n != 1 {
		t.Fatalf("changes in batch = %d, want 1", n)
	}
	c := in.ChangeBatch.Changes[0]
	if c.Action != types.ChangeActionUpsert {
		t.Errorf("Action = %q, want UPSERT", c.Action)
	}
	rr := c.ResourceRecordSet
	if got := aws.ToString(rr.Name); got != "_acme-challenge.metrics.example.com." {
		t.Errorf("record Name = %q, want _acme-challenge.metrics.example.com.", got)
	}
	if rr.Type != types.RRTypeTxt {
		t.Errorf("record Type = %q, want TXT", rr.Type)
	}
	if len(rr.ResourceRecords) != 1 {
		t.Fatalf("resource records = %d, want 1", len(rr.ResourceRecords))
	}
	if got := aws.ToString(rr.ResourceRecords[0].Value); got != `"abc123value"` {
		t.Errorf("TXT value = %s, want it double-quoted", got)
	}
	if f.getCalls < 2 {
		t.Errorf("GetChange calls = %d, want polling until INSYNC", f.getCalls)
	}
}

func TestRoute53SolverPresentImmediateInSyncSkipsPolling(t *testing.T) {
	f := &fakeRoute53{changeInfo: &types.ChangeInfo{Id: aws.String("/change/C2"), Status: types.ChangeStatusInsync}}
	s := fastSolver(f, "Z")
	if err := s.Present(context.Background(), Challenge{Domain: "a.example.com", DNSValue: "v"}); err != nil {
		t.Fatalf("Present: %v", err)
	}
	if f.getCalls != 0 {
		t.Errorf("GetChange calls = %d, want 0 when the change is already INSYNC", f.getCalls)
	}
}

func TestRoute53SolverCleanUpDeletesTXT(t *testing.T) {
	f := &fakeRoute53{}
	s := fastSolver(f, "Z")
	ch := Challenge{Domain: "a.example.com", DNSValue: "v"}
	if err := s.CleanUp(context.Background(), ch); err != nil {
		t.Fatalf("CleanUp: %v", err)
	}
	if len(f.changes) != 1 {
		t.Fatalf("ChangeResourceRecordSets calls = %d, want 1", len(f.changes))
	}
	c := f.changes[0].ChangeBatch.Changes[0]
	if c.Action != types.ChangeActionDelete {
		t.Errorf("Action = %q, want DELETE", c.Action)
	}
	if got := aws.ToString(c.ResourceRecordSet.Name); got != "_acme-challenge.a.example.com." {
		t.Errorf("record Name = %q, want _acme-challenge.a.example.com.", got)
	}
	// CleanUp is best-effort removal — it must not block on INSYNC.
	if f.getCalls != 0 {
		t.Errorf("GetChange calls = %d, want 0 (CleanUp does not poll)", f.getCalls)
	}
}

func TestRoute53SolverPresentReturnsChangeError(t *testing.T) {
	f := &fakeRoute53{changeErr: errors.New("access denied")}
	s := fastSolver(f, "Z")
	if err := s.Present(context.Background(), Challenge{Domain: "a.example.com", DNSValue: "v"}); err == nil {
		t.Fatal("Present: expected error when ChangeResourceRecordSets fails")
	}
}

func TestRoute53SolverWaitTimesOut(t *testing.T) {
	f := &fakeRoute53{
		changeInfo:  &types.ChangeInfo{Id: aws.String("/change/C3"), Status: types.ChangeStatusPending},
		getSequence: []types.ChangeStatus{types.ChangeStatusPending},
	}
	s := newRoute53Solver(f, "Z", nil)
	s.pollInterval = time.Millisecond
	s.pollTimeout = 20 * time.Millisecond
	if err := s.Present(context.Background(), Challenge{Domain: "a.example.com", DNSValue: "v"}); err == nil {
		t.Fatal("Present: expected timeout error when the change never reaches INSYNC")
	}
}

func TestResolveRoute53SettingsFromDNSConfigAndStoreSecrets(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	mustSetString(t, store, configstore.KeyServerTLSACMERoute53AccessKeyID, "AKIAEXAMPLE", true)
	mustSetString(t, store, configstore.KeyServerTLSACMERoute53SecretAccessKey, "secretvalue", true)

	st, err := resolveRoute53Settings(ctx, store, map[string]string{"region": "us-east-1", "hosted_zone_id": "Z9"})
	if err != nil {
		t.Fatalf("resolveRoute53Settings: %v", err)
	}
	if st.region != "us-east-1" {
		t.Errorf("region = %q, want us-east-1 (from dns_provider_config)", st.region)
	}
	if st.hostedZoneID != "Z9" {
		t.Errorf("hostedZoneID = %q, want Z9", st.hostedZoneID)
	}
	if st.accessKeyID != "AKIAEXAMPLE" || st.secretAccessKey != "secretvalue" {
		t.Errorf("creds = (%q,%q), want them from the config-store secrets", st.accessKeyID, st.secretAccessKey)
	}
}

func TestResolveRoute53SettingsFallsBackToStoreRegionAndZone(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	mustSetString(t, store, configstore.KeyServerTLSACMERoute53Region, "eu-west-1", false)
	mustSetString(t, store, configstore.KeyServerTLSACMERoute53HostedZoneID, "Zfromstore", false)

	st, err := resolveRoute53Settings(ctx, store, nil)
	if err != nil {
		t.Fatalf("resolveRoute53Settings: %v", err)
	}
	if st.region != "eu-west-1" {
		t.Errorf("region = %q, want eu-west-1 (from store fallback)", st.region)
	}
	if st.hostedZoneID != "Zfromstore" {
		t.Errorf("hostedZoneID = %q, want Zfromstore", st.hostedZoneID)
	}
	if st.accessKeyID != "" || st.secretAccessKey != "" {
		t.Errorf("creds = (%q,%q), want empty (none configured — env/role take over)", st.accessKeyID, st.secretAccessKey)
	}
}

func TestResolveRoute53SettingsIgnoresPartialStoreCreds(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	// Only the access key ID is present — without the secret it is unusable, so
	// resolution must fall through to env/role (empty static creds) rather than
	// half-configure the client.
	mustSetString(t, store, configstore.KeyServerTLSACMERoute53AccessKeyID, "AKIAEXAMPLE", true)

	st, err := resolveRoute53Settings(ctx, store, map[string]string{"hosted_zone_id": "Z9"})
	if err != nil {
		t.Fatalf("resolveRoute53Settings: %v", err)
	}
	if st.accessKeyID != "" || st.secretAccessKey != "" {
		t.Errorf("creds = (%q,%q), want empty when only one half is configured", st.accessKeyID, st.secretAccessKey)
	}
}

func TestNewRoute53SolverRequiresHostedZone(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	if _, err := NewRoute53Solver(ctx, store, map[string]string{"region": "us-east-1"}, nil); err == nil {
		t.Fatal("NewRoute53Solver: expected error when no hosted_zone_id is resolvable")
	}
}

func mustSetString(t *testing.T, store SecretStore, key, value string, secret bool) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", key, err)
	}
	if err := store.Set(context.Background(), key, raw, secret, "test"); err != nil {
		t.Fatalf("set %s: %v", key, err)
	}
}
