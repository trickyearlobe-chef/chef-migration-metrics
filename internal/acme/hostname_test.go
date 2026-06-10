// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// fastRegistrar builds a HostnameRegistrar over a fake route53API with a fast
// poll loop and stubbed IP-resolution seams (auto-detect returns a fixed IPv4,
// the interface seam errors). Tests override the seams as needed.
func fastRegistrar(api route53API, zone string) *HostnameRegistrar {
	return &HostnameRegistrar{
		api:          api,
		hostedZoneID: zone,
		ttl:          60,
		pollInterval: time.Millisecond,
		pollTimeout:  2 * time.Second,
		ifaceIPv4:    func(string) (string, error) { return "", errors.New("no interface in test") },
		autoIPv4:     func() (string, error) { return "203.0.113.10", nil },
	}
}

func TestResolveIPLiteralIPv4(t *testing.T) {
	r := fastRegistrar(&fakeRoute53{}, "Z")
	r.literalIP = "10.0.0.5"
	got, err := r.resolveIP()
	if err != nil {
		t.Fatalf("resolveIP: %v", err)
	}
	if got != "10.0.0.5" {
		t.Errorf("resolveIP = %q, want 10.0.0.5", got)
	}
}

func TestResolveIPLiteralInvalidErrors(t *testing.T) {
	r := fastRegistrar(&fakeRoute53{}, "Z")
	r.literalIP = "not-an-ip"
	if _, err := r.resolveIP(); err == nil {
		t.Fatal("resolveIP: expected error for a malformed literal IP")
	}
}

func TestResolveIPLiteralIPv6Errors(t *testing.T) {
	r := fastRegistrar(&fakeRoute53{}, "Z")
	r.literalIP = "2001:db8::1"
	if _, err := r.resolveIP(); err == nil {
		t.Fatal("resolveIP: expected error — only IPv4 A records are supported")
	}
}

func TestResolveIPInterfaceUsesSeam(t *testing.T) {
	r := fastRegistrar(&fakeRoute53{}, "Z")
	r.ifaceName = "eth0"
	r.ifaceIPv4 = func(name string) (string, error) {
		if name != "eth0" {
			t.Errorf("ifaceIPv4 name = %q, want eth0", name)
		}
		return "192.0.2.20", nil
	}
	got, err := r.resolveIP()
	if err != nil {
		t.Fatalf("resolveIP: %v", err)
	}
	if got != "192.0.2.20" {
		t.Errorf("resolveIP = %q, want 192.0.2.20", got)
	}
}

// An explicit-but-unusable interface returns an error with no fall-through to
// auto-detect (tls-acme.md § 3.13).
func TestResolveIPInterfaceUnusableNoFallthrough(t *testing.T) {
	r := fastRegistrar(&fakeRoute53{}, "Z")
	r.ifaceName = "eth9"
	r.ifaceIPv4 = func(string) (string, error) { return "", errors.New("no global-unicast IPv4") }
	r.autoIPv4 = func() (string, error) {
		t.Fatal("auto-detect must not be used when an interface is explicitly set")
		return "", nil
	}
	if _, err := r.resolveIP(); err == nil {
		t.Fatal("resolveIP: expected error for an unusable explicit interface")
	}
}

func TestResolveIPAutoDetect(t *testing.T) {
	r := fastRegistrar(&fakeRoute53{}, "Z")
	got, err := r.resolveIP()
	if err != nil {
		t.Fatalf("resolveIP: %v", err)
	}
	if got != "203.0.113.10" {
		t.Errorf("resolveIP = %q, want the auto-detected 203.0.113.10", got)
	}
}

func TestRegisterUpsertsARecordPerDomain(t *testing.T) {
	f := &fakeRoute53{
		changeInfo:  &types.ChangeInfo{Id: aws.String("/change/A1"), Status: types.ChangeStatusPending},
		getSequence: []types.ChangeStatus{types.ChangeStatusInsync},
	}
	r := fastRegistrar(f, "Zone1")
	r.literalIP = "198.51.100.7"

	if err := r.Register(context.Background(), []string{"a.example.com", "b.example.com"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(f.changes) != 2 {
		t.Fatalf("ChangeResourceRecordSets calls = %d, want 2 (one A record per domain)", len(f.changes))
	}
	in := f.changes[0]
	if got := aws.ToString(in.HostedZoneId); got != "Zone1" {
		t.Errorf("HostedZoneId = %q, want Zone1", got)
	}
	c := in.ChangeBatch.Changes[0]
	if c.Action != types.ChangeActionUpsert {
		t.Errorf("Action = %q, want UPSERT", c.Action)
	}
	rr := c.ResourceRecordSet
	if got := aws.ToString(rr.Name); got != "a.example.com." {
		t.Errorf("record Name = %q, want a.example.com.", got)
	}
	if rr.Type != types.RRTypeA {
		t.Errorf("record Type = %q, want A", rr.Type)
	}
	if aws.ToInt64(rr.TTL) != 60 {
		t.Errorf("TTL = %d, want 60", aws.ToInt64(rr.TTL))
	}
	if got := aws.ToString(rr.ResourceRecords[0].Value); got != "198.51.100.7" {
		t.Errorf("A value = %q, want 198.51.100.7 (unquoted)", got)
	}
}

func TestRegisterSkipsWildcardWithWarn(t *testing.T) {
	f := &fakeRoute53{getSequence: []types.ChangeStatus{types.ChangeStatusInsync}}
	r := fastRegistrar(f, "Z")
	r.literalIP = "198.51.100.7"

	if err := r.Register(context.Background(), []string{"*.example.com", "ok.example.com"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(f.changes) != 1 {
		t.Fatalf("ChangeResourceRecordSets calls = %d, want 1 (wildcard skipped)", len(f.changes))
	}
	if got := aws.ToString(f.changes[0].ChangeBatch.Changes[0].ResourceRecordSet.Name); got != "ok.example.com." {
		t.Errorf("record Name = %q, want ok.example.com. (wildcard not published)", got)
	}
}

// A Route 53 error on one domain is logged and returned but does not stop the
// other domains from being attempted (fail-soft, tls-acme.md § 3.13).
func TestRegisterFailSoftContinuesOnError(t *testing.T) {
	f := &fakeRoute53{changeErr: errors.New("access denied")}
	r := fastRegistrar(f, "Z")
	r.literalIP = "198.51.100.7"

	err := r.Register(context.Background(), []string{"a.example.com", "b.example.com"})
	if err == nil {
		t.Fatal("Register: expected an error to be returned when UPSERT fails")
	}
	if len(f.changes) != 2 {
		t.Errorf("ChangeResourceRecordSets calls = %d, want 2 (kept going after the first failure)", len(f.changes))
	}
}

// When the IP cannot be resolved, Register returns an error and never calls
// Route 53.
func TestRegisterResolutionFailureNoCalls(t *testing.T) {
	f := &fakeRoute53{}
	r := fastRegistrar(f, "Z")
	r.literalIP = "garbage"

	if err := r.Register(context.Background(), []string{"a.example.com"}); err == nil {
		t.Fatal("Register: expected error when the IP cannot be resolved")
	}
	if len(f.changes) != 0 {
		t.Errorf("ChangeResourceRecordSets calls = %d, want 0 when resolution fails", len(f.changes))
	}
}

// Re-asserting after the host IP changes (DHCP) UPSERTs the new value.
func TestRegisterReassertsChangedIP(t *testing.T) {
	f := &fakeRoute53{getSequence: []types.ChangeStatus{types.ChangeStatusInsync}}
	r := fastRegistrar(f, "Z")
	ip := "203.0.113.1"
	r.autoIPv4 = func() (string, error) { return ip, nil }

	if err := r.Register(context.Background(), []string{"a.example.com"}); err != nil {
		t.Fatalf("Register #1: %v", err)
	}
	ip = "203.0.113.99" // DHCP lease changed
	if err := r.Register(context.Background(), []string{"a.example.com"}); err != nil {
		t.Fatalf("Register #2: %v", err)
	}
	if len(f.changes) != 2 {
		t.Fatalf("ChangeResourceRecordSets calls = %d, want 2", len(f.changes))
	}
	if got := aws.ToString(f.changes[1].ChangeBatch.Changes[0].ResourceRecordSet.ResourceRecords[0].Value); got != "203.0.113.99" {
		t.Errorf("second UPSERT A value = %q, want the new IP 203.0.113.99", got)
	}
}

// NewHostnameRegistrar built off a solver shares the solver's client and zone.
func TestSolverNewHostnameRegistrarSharesClient(t *testing.T) {
	f := &fakeRoute53{getSequence: []types.ChangeStatus{types.ChangeStatusInsync}}
	s := newRoute53Solver(f, "ZShared", nil)
	s.pollInterval = time.Millisecond
	reg := s.NewHostnameRegistrar(30, "198.51.100.5", "", nil)
	if reg.ttl != 30 {
		t.Errorf("ttl = %d, want 30", reg.ttl)
	}
	if err := reg.Register(context.Background(), []string{"a.example.com"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(f.changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(f.changes))
	}
	if got := aws.ToString(f.changes[0].HostedZoneId); got != "ZShared" {
		t.Errorf("HostedZoneId = %q, want ZShared (shared from solver)", got)
	}
}

// A non-positive TTL from config falls back to the default.
func TestNewHostnameRegistrarDefaultsTTL(t *testing.T) {
	s := newRoute53Solver(&fakeRoute53{}, "Z", nil)
	reg := s.NewHostnameRegistrar(0, "", "", nil)
	if reg.ttl != route53DefaultTTL {
		t.Errorf("ttl = %d, want default %d", reg.ttl, route53DefaultTTL)
	}
}
