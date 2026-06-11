// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/acme"
	apptls "github.com/trickyearlobe-chef/chef-migration-metrics/internal/tls"
)

func TestTLSHealthy_DBPair(t *testing.T) {
	app := newTestApp(t)
	cert, key, err := apptls.GenerateSelfSigned([]string{"x.example.com"})
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	if !app.tlsHealthy(apptls.ListenerConfig{CertSource: "db", CertPEM: cert, KeyPEM: key}) {
		t.Error("valid db pair = unhealthy, want healthy")
	}
	if app.tlsHealthy(apptls.ListenerConfig{CertSource: "db"}) {
		t.Error("missing db pair = healthy, want unhealthy")
	}
	if app.tlsHealthy(apptls.ListenerConfig{CertSource: "db", CertPEM: cert, KeyPEM: []byte("nope")}) {
		t.Error("bad db key = healthy, want unhealthy")
	}
}

func TestTLSHealthy_FileMissing(t *testing.T) {
	app := newTestApp(t)
	if app.tlsHealthy(apptls.ListenerConfig{CertSource: "file", CertPath: "/no/such/c", KeyPath: "/no/such/k"}) {
		t.Error("missing files = healthy, want unhealthy")
	}
}

func TestPlanAutoHTTPS_443Available(t *testing.T) {
	app := newTestApp(t)
	serverPort := app.cfg.Server.Port

	ln, err := net.Listen("tcp", "127.0.0.1:0") // stands in for the 443 bind
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	app.auto443Listen = func(string) (net.Listener, error) { return ln, nil }

	port, redirects, got := app.planAutoHTTPS(0)
	if port != 443 {
		t.Errorf("HTTPS port = %d, want 443", port)
	}
	if got != ln {
		t.Error("planAutoHTTPS did not return the pre-bound 443 listener")
	}
	if len(redirects) != 1 || redirects[0] != serverPort {
		t.Errorf("redirects = %v, want [%d]", redirects, serverPort)
	}
}

func TestPlanAutoHTTPS_443AvailableWithHTTPRedirectPort(t *testing.T) {
	app := newTestApp(t)
	serverPort := app.cfg.Server.Port
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer func() { _ = ln.Close() }()
	app.auto443Listen = func(string) (net.Listener, error) { return ln, nil }

	_, redirects, _ := app.planAutoHTTPS(80)
	want := []int{serverPort, 80}
	if len(redirects) != 2 || redirects[0] != want[0] || redirects[1] != want[1] {
		t.Errorf("redirects = %v, want %v", redirects, want)
	}
}

func TestPlanAutoHTTPS_443Unavailable_FallsBack(t *testing.T) {
	app := newTestApp(t) // default seam: 443 unavailable
	serverPort := app.cfg.Server.Port

	port, redirects, ln := app.planAutoHTTPS(0)
	if port != serverPort {
		t.Errorf("HTTPS port = %d, want server.port %d", port, serverPort)
	}
	if ln != nil {
		t.Error("expected no pre-bound listener on 443 failure")
	}
	if len(redirects) != 0 {
		t.Errorf("redirects = %v, want none on fallback", redirects)
	}
}

// A healthy ACME setup with 443 available binds the HTTPS lifeboat on 443 and
// adds a server.port → 443 redirect listener (tls.md § 1.5).
func TestSetupACME_Healthy_BindsAuto443Redirect(t *testing.T) {
	app := newACMEApp(t)
	app.cfg.Server.TLS.ACME.Domains = []string{"metrics.example.com"}
	app.cfg.Server.TLS.ACME.Challenge = "http-01"
	app.cfg.Server.TLS.ACME.AgreeToTOS = false
	serverPort := app.cfg.Server.Port

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	app.auto443Listen = func(string) (net.Listener, error) { return ln, nil }

	store := newFakeSecretStore()
	storage := acme.NewStorage(store)
	cert, key, err := apptls.GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("seed cert: %v", err)
	}
	if err := storage.SetCertificate(context.Background(), cert, key); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := app.setupACME(http.NewServeMux(), store, 5*time.Second)
	if err != nil {
		t.Fatalf("setupACME: %v", err)
	}
	defer shutdownACME(t, res)

	if app.tlsStatus.IsDegraded() {
		t.Fatal("expected healthy with a stored cert")
	}
	wantRedirect := fmt.Sprintf("%s:%d", app.cfg.Server.ListenAddress, serverPort)
	found := false
	for _, a := range res.tlsListener.RedirectAddrs() {
		if a == wantRedirect {
			found = true
		}
	}
	if !found {
		t.Errorf("RedirectAddrs = %v, want one of them %q (server.port → 443)",
			res.tlsListener.RedirectAddrs(), wantRedirect)
	}
}

// A healthy ACME setup with 443 unavailable serves HTTPS on server.port with no
// auto redirect (the § 1.4 fallback).
func TestSetupACME_Healthy_443Unavailable_NoAutoRedirect(t *testing.T) {
	// http-01 healthy: the port-80 challenge server is separate from the HTTPS
	// Listener's redirect set (RedirectAddrs), so an empty RedirectAddrs proves
	// no auto server.port → 443 redirect was added on the fallback.
	app := newACMEApp(t) // default seam: 443 unavailable
	app.cfg.Server.TLS.ACME.Domains = []string{"metrics.example.com"}
	app.cfg.Server.TLS.ACME.Challenge = "http-01"
	app.cfg.Server.TLS.ACME.AgreeToTOS = false

	store := newFakeSecretStore()
	storage := acme.NewStorage(store)
	cert, key, err := apptls.GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("seed cert: %v", err)
	}
	if err := storage.SetCertificate(context.Background(), cert, key); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := app.setupACME(http.NewServeMux(), store, 5*time.Second)
	if err != nil {
		t.Fatalf("setupACME: %v", err)
	}
	defer shutdownACME(t, res)

	if len(res.tlsListener.RedirectAddrs()) != 0 {
		t.Errorf("RedirectAddrs = %v, want none on 443 fallback", res.tlsListener.RedirectAddrs())
	}
}
