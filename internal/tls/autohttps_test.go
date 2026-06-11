// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"reflect"
	"testing"
)

func TestResolveAutoHTTPS_HealthyBinds443WithOldPortRedirect(t *testing.T) {
	plan := ResolveAutoHTTPS(8080, 0, func() bool { return true })
	if !plan.BoundTo443 || plan.HTTPSPort != 443 {
		t.Fatalf("HTTPSPort=%d BoundTo443=%v, want 443/true", plan.HTTPSPort, plan.BoundTo443)
	}
	if !reflect.DeepEqual(plan.RedirectPorts, []int{8080}) {
		t.Errorf("RedirectPorts=%v, want [8080]", plan.RedirectPorts)
	}
}

func TestResolveAutoHTTPS_HealthyWithHTTPRedirectPort(t *testing.T) {
	// Both the old-port redirect and the explicit http_redirect_port target 443.
	plan := ResolveAutoHTTPS(8080, 80, func() bool { return true })
	if plan.HTTPSPort != 443 {
		t.Fatalf("HTTPSPort=%d, want 443", plan.HTTPSPort)
	}
	if !reflect.DeepEqual(plan.RedirectPorts, []int{8080, 80}) {
		t.Errorf("RedirectPorts=%v, want [8080 80]", plan.RedirectPorts)
	}
}

func TestResolveAutoHTTPS_443BindFailureFallsBackNoRedirect(t *testing.T) {
	plan := ResolveAutoHTTPS(8080, 0, func() bool { return false })
	if plan.BoundTo443 || plan.HTTPSPort != 8080 {
		t.Fatalf("HTTPSPort=%d BoundTo443=%v, want 8080/false", plan.HTTPSPort, plan.BoundTo443)
	}
	if len(plan.RedirectPorts) != 0 {
		t.Errorf("RedirectPorts=%v, want none on fallback", plan.RedirectPorts)
	}
}

func TestResolveAutoHTTPS_443FailureStillHonoursHTTPRedirectPort(t *testing.T) {
	// The explicit http_redirect_port keeps working, now targeting server.port.
	plan := ResolveAutoHTTPS(8080, 80, func() bool { return false })
	if plan.HTTPSPort != 8080 || plan.BoundTo443 {
		t.Fatalf("HTTPSPort=%d BoundTo443=%v, want 8080/false", plan.HTTPSPort, plan.BoundTo443)
	}
	if !reflect.DeepEqual(plan.RedirectPorts, []int{80}) {
		t.Errorf("RedirectPorts=%v, want [80]", plan.RedirectPorts)
	}
}

func TestResolveAutoHTTPS_ServerPortAlready443_NoProbeNoRedirect(t *testing.T) {
	probed := false
	plan := ResolveAutoHTTPS(443, 0, func() bool { probed = true; return true })
	if probed {
		t.Error("can443 probe must not be called when server.port is already 443")
	}
	if !plan.BoundTo443 || plan.HTTPSPort != 443 {
		t.Fatalf("HTTPSPort=%d BoundTo443=%v, want 443/true", plan.HTTPSPort, plan.BoundTo443)
	}
	if len(plan.RedirectPorts) != 0 {
		t.Errorf("RedirectPorts=%v, want none (nothing to redirect)", plan.RedirectPorts)
	}
}

func TestResolveAutoHTTPS_ServerPort443WithHTTPRedirectPort(t *testing.T) {
	plan := ResolveAutoHTTPS(443, 80, func() bool { return true })
	if !reflect.DeepEqual(plan.RedirectPorts, []int{80}) {
		t.Errorf("RedirectPorts=%v, want [80]", plan.RedirectPorts)
	}
}

func TestResolveAutoHTTPS_ZeroServerPortDefaults8080(t *testing.T) {
	plan := ResolveAutoHTTPS(0, 0, func() bool { return true })
	if !reflect.DeepEqual(plan.RedirectPorts, []int{8080}) {
		t.Errorf("RedirectPorts=%v, want [8080] (default port)", plan.RedirectPorts)
	}
}

func TestResolveAutoHTTPS_DropsRedirectEqualToHTTPSPort(t *testing.T) {
	// A degenerate http_redirect_port == 443 with server.port already 443 must
	// not produce a self-targeting redirect.
	plan := ResolveAutoHTTPS(443, 443, func() bool { return true })
	if len(plan.RedirectPorts) != 0 {
		t.Errorf("RedirectPorts=%v, want none", plan.RedirectPorts)
	}
}
