// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/acme"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	apptls "github.com/trickyearlobe-chef/chef-migration-metrics/internal/tls"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/webapi"
)

// fakeSecretStore is an in-memory acme.SecretStore for the cmd-level ACME tests
// (no DB, no network).
type fakeSecretStore struct {
	data map[string]json.RawMessage
}

func newFakeSecretStore() *fakeSecretStore {
	return &fakeSecretStore{data: map[string]json.RawMessage{}}
}

func (f *fakeSecretStore) Get(_ context.Context, key string) (json.RawMessage, error) {
	v, ok := f.data[key]
	if !ok {
		return nil, configstore.ErrNotFound
	}
	return v, nil
}

func (f *fakeSecretStore) GetSecret(ctx context.Context, key string) (json.RawMessage, error) {
	return f.Get(ctx, key)
}

func (f *fakeSecretStore) Set(_ context.Context, key string, value json.RawMessage, _ bool, _ string) error {
	f.data[key] = value
	return nil
}

func (f *fakeSecretStore) Delete(_ context.Context, key string) error {
	delete(f.data, key)
	return nil
}

// fakeObtainer is a CertObtainer that returns scripted material without any
// network, standing in for the ACME Manager.
type fakeObtainer struct {
	cert, key []byte
	err       error
	calls     int
}

func (f *fakeObtainer) Obtain(_ context.Context) (certPEM, keyPEM []byte, err error) {
	f.calls++
	return f.cert, f.key, f.err
}

// newACMEApp builds a serverApp wired with the holders setupACME needs, plus an
// http-01 ACME config pointing at free loopback ports.
func newACMEApp(t *testing.T) *serverApp {
	t.Helper()
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.tlsReload.SetOnReload(app.tlsStatus.SetHealthy)

	app.cfg.Server.TLS.Mode = "acme"
	app.cfg.Server.TLS.HTTPRedirectPort = freePort(t)
	return app
}

// promotingIssuer promotes a freshly obtained certificate into the running
// self-signed listener, clearing the degraded state.
func TestPromotingIssuer_PromotesInPlaceClearsDegraded(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.tlsReload.SetOnReload(app.tlsStatus.SetHealthy)

	res := app.degradeToSelfSigned(http.NewServeMux(), nil, errors.New("acme bootstrap"))
	defer shutdown(t, res)
	if !app.tlsStatus.IsDegraded() {
		t.Fatal("expected degraded before promotion")
	}

	realCert, realKey, err := apptls.GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("generate promotion cert: %v", err)
	}
	issuer := &promotingIssuer{
		inner:  &fakeObtainer{cert: realCert, key: realKey},
		reload: app.tlsReload,
		log:    app.tlsLog,
	}
	if _, _, err := issuer.Obtain(context.Background()); err != nil {
		t.Fatalf("Obtain: %v", err)
	}
	if app.tlsStatus.IsDegraded() {
		t.Error("expected degraded cleared after in-place promotion")
	}
	if !app.hstsEnabledFn()() {
		t.Error("HSTS gate must reopen once a real cert is promoted")
	}
}

// A failed Obtain (e.g. ToS not accepted) must propagate the error and leave the
// listener degraded — no promotion happens.
func TestPromotingIssuer_ObtainErrorStaysDegraded(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.tlsReload.SetOnReload(app.tlsStatus.SetHealthy)

	res := app.degradeToSelfSigned(http.NewServeMux(), nil, errors.New("acme bootstrap"))
	defer shutdown(t, res)

	issuer := &promotingIssuer{
		inner:  &fakeObtainer{err: acme.ErrTOSNotAccepted},
		reload: app.tlsReload,
		log:    app.tlsLog,
	}
	if _, _, err := issuer.Obtain(context.Background()); !errors.Is(err, acme.ErrTOSNotAccepted) {
		t.Fatalf("want ErrTOSNotAccepted, got %v", err)
	}
	if !app.tlsStatus.IsDegraded() {
		t.Error("expected listener to stay degraded when issuance fails")
	}
}

// http-01 with no stored cert comes up self-signed (degraded) over HTTPS, starts
// the challenge/redirect server and the renewer, and never returns a fatal
// error. agree_to_tos is false so the renewer's first Obtain short-circuits with
// no network call.
func TestSetupACME_HTTP01ComesUpSelfSignedDegraded(t *testing.T) {
	app := newACMEApp(t)
	app.cfg.Server.TLS.ACME.Domains = []string{"metrics.example.com"}
	app.cfg.Server.TLS.ACME.Challenge = "http-01"
	app.cfg.Server.TLS.ACME.CAURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
	app.cfg.Server.TLS.ACME.AgreeToTOS = false // keeps the renewer offline

	res, err := app.setupACME(http.NewServeMux(), newFakeSecretStore(), 5*time.Second)
	if err != nil {
		t.Fatalf("setupACME returned a fatal error (must fail open): %v", err)
	}
	defer shutdownACME(t, res)

	if res.tlsListener == nil {
		t.Fatal("expected an HTTPS listener in acme mode")
	}
	if res.challengeSrv == nil {
		t.Error("expected an http-01 challenge/redirect server")
	}
	if res.renewerCancel == nil {
		t.Error("expected a renewer cancel func")
	}
	if !app.tlsStatus.IsDegraded() {
		t.Error("expected degraded (self-signed) when no cert is stored yet")
	}
	if app.tlsStatus.Status().Kind != webapi.DegradedKindSelfSigned {
		t.Errorf("kind = %q, want self-signed", app.tlsStatus.Status().Kind)
	}

	// Really serving HTTPS with a self-signed cert.
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed by design
	}}
	resp := getWithRetry(t, client, "https://"+res.tlsListener.Addr()+"/")
	resp.Body.Close()
}

// A stored issued certificate is served immediately (healthy, not degraded).
func TestSetupACME_StoredCertServedHealthy(t *testing.T) {
	app := newACMEApp(t)
	app.cfg.Server.TLS.ACME.Domains = []string{"metrics.example.com"}
	app.cfg.Server.TLS.ACME.Challenge = "http-01"
	app.cfg.Server.TLS.ACME.AgreeToTOS = false

	store := newFakeSecretStore()
	// Seed a valid issued cert/key (a self-signed pair is structurally valid).
	storage := acme.NewStorage(store)
	cert, key, err := apptls.GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("seed cert: %v", err)
	}
	if err := storage.SetCertificate(context.Background(), cert, key); err != nil {
		t.Fatalf("seed SetCertificate: %v", err)
	}

	res, err := app.setupACME(http.NewServeMux(), store, 5*time.Second)
	if err != nil {
		t.Fatalf("setupACME: %v", err)
	}
	defer shutdownACME(t, res)

	if app.tlsStatus.IsDegraded() {
		t.Error("expected healthy (not degraded) when a stored cert is present")
	}
}

// http-01 without http_redirect_port cannot work: it degrades to self-signed and
// does not start a challenge server or renewer.
func TestSetupACME_HTTP01NoRedirectPortDegrades(t *testing.T) {
	app := newACMEApp(t)
	app.cfg.Server.TLS.HTTPRedirectPort = 0
	app.cfg.Server.TLS.ACME.Domains = []string{"metrics.example.com"}
	app.cfg.Server.TLS.ACME.Challenge = "http-01"

	res, err := app.setupACME(http.NewServeMux(), newFakeSecretStore(), 5*time.Second)
	if err != nil {
		t.Fatalf("setupACME must fail open: %v", err)
	}
	defer shutdownACME(t, res)

	if res.challengeSrv != nil {
		t.Error("no challenge server should start without a redirect port")
	}
	if res.renewerCancel != nil {
		t.Error("no renewer should start when http-01 cannot work")
	}
	if !app.tlsStatus.IsDegraded() {
		t.Error("expected degraded self-signed when http-01 has no redirect port")
	}
}

// dns-01 is not yet implemented: it degrades to self-signed rather than exiting.
func TestSetupACME_DNS01Degrades(t *testing.T) {
	app := newACMEApp(t)
	app.cfg.Server.TLS.ACME.Domains = []string{"metrics.example.com"}
	app.cfg.Server.TLS.ACME.Challenge = "dns-01"

	res, err := app.setupACME(http.NewServeMux(), newFakeSecretStore(), 5*time.Second)
	if err != nil {
		t.Fatalf("setupACME must fail open for dns-01: %v", err)
	}
	defer shutdownACME(t, res)

	if res.challengeSrv != nil {
		t.Error("dns-01 must not start an http challenge server")
	}
	if !app.tlsStatus.IsDegraded() {
		t.Error("expected degraded self-signed for unimplemented dns-01")
	}
}

func TestIsACMEStaging(t *testing.T) {
	cases := map[string]bool{
		"https://acme-staging-v02.api.letsencrypt.org/directory": true,
		"https://acme-v02.api.letsencrypt.org/directory":         false,
		"": false,
	}
	for url, want := range cases {
		if got := isACMEStaging(url); got != want {
			t.Errorf("isACMEStaging(%q) = %v, want %v", url, got, want)
		}
	}
}

// shutdownACME drains an acme-mode serverResult (renewer + challenge server +
// HTTPS listener).
func shutdownACME(t *testing.T, res serverResult) {
	t.Helper()
	if res.renewerCancel != nil {
		res.renewerCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if res.challengeSrv != nil {
		_ = res.challengeSrv.Shutdown(ctx)
	}
	if res.tlsListener != nil {
		_ = res.tlsListener.Shutdown(ctx)
	}
}

func TestACMETriggerHolder_ForwardsAndNoOpBeforeBind(t *testing.T) {
	h := &acmeTriggerHolder{}
	// Before binding, Trigger must be a safe no-op.
	h.Trigger()

	calls := 0
	h.Set(func() { calls++ })
	h.Trigger()
	h.Trigger()
	if calls != 2 {
		t.Errorf("bound trigger called %d times, want 2", calls)
	}
}

// stubShutdowner records that Shutdown was called, standing in for the HTTPS
// listener in the acmeRuntime composition test.
type stubShutdowner struct{ called bool }

func (s *stubShutdowner) Shutdown(context.Context) error { s.called = true; return nil }

// acmeRuntime.shutdown must tear down all three resources it owns — cancel the
// renewer, stop the port-80 challenge/redirect server, and drain the HTTPS
// listener — so a single Instance.Shutdown (H4c-1) cleans up the whole ACME
// topology on a controller drain or a future rebind swap.
func TestACMERuntimeShutdown_TearsDownRenewerChallengeListener(t *testing.T) {
	cancelled := false
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind challenge: %v", err)
	}
	challenge := &http.Server{Handler: http.NewServeMux()}
	go func() { _ = challenge.Serve(ln) }()
	caddr := ln.Addr().String()

	listener := &stubShutdowner{}
	rt := &acmeRuntime{
		listener:      listener,
		challengeSrv:  challenge,
		renewerCancel: func() { cancelled = true },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rt.shutdown(ctx); err != nil {
		t.Fatalf("rt.shutdown: %v", err)
	}

	if !cancelled {
		t.Error("renewer was not cancelled")
	}
	if !listener.called {
		t.Error("HTTPS listener was not drained")
	}
	if c, derr := net.DialTimeout("tcp", caddr, 200*time.Millisecond); derr == nil {
		c.Close()
		t.Error("challenge/redirect server still listening after shutdown")
	}
}

// In acme mode, when a listener rebinder is wired, setupACME adopts the live
// topology into the serverctl.Controller as a single composite Instance: the
// challenge server + renewer move into the Instance's Shutdown (so the boot
// serverResult no longer carries them), draining via the controller stops
// everything, and app.acmeActive records that we are serving acme. This is the
// H4c-1 seam — transitions are still refused (H4c-2).
func TestSetupACME_AdoptsControllerForRebind(t *testing.T) {
	app := newACMEApp(t)
	app.listenerRebind = webapi.NewListenerRebindHolder()
	app.cfg.Server.TLS.ACME.Domains = []string{"metrics.example.com"}
	app.cfg.Server.TLS.ACME.Challenge = "http-01"
	app.cfg.Server.TLS.ACME.AgreeToTOS = false // keeps the renewer offline

	res, err := app.setupACME(http.NewServeMux(), newFakeSecretStore(), 5*time.Second)
	if err != nil {
		t.Fatalf("setupACME: %v", err)
	}

	if !app.acmeActive {
		t.Error("acmeActive must be set when serving the acme topology")
	}
	if app.listenerController == nil {
		t.Fatal("controller must be adopted in acme mode when a rebinder is wired")
	}
	if res.challengeSrv != nil || res.renewerCancel != nil {
		t.Error("adopted acme: challenge + renewer teardown must move into the controller Instance")
	}
	if res.tlsListener == nil {
		t.Fatal("expected an HTTPS listener")
	}
	addr := res.tlsListener.Addr()

	client := insecureTLSClient()
	getWithRetry(t, client, "https://"+addr+"/").Body.Close()

	// The controller is now the sole owner — draining it stops the HTTPS listener
	// (and, via the composite Instance, the renewer + challenge server).
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := app.listenerController.Shutdown(ctx); err != nil {
		t.Fatalf("controller shutdown: %v", err)
	}
	if resp, gerr := client.Get("https://" + addr + "/"); gerr == nil {
		resp.Body.Close()
		t.Error("HTTPS listener still answering after controller shutdown")
	}
}

// Leaving acme (acme→off, acme→static) is deferred to H4c-2: while acmeActive the
// applier refuses every target so the save is reported restart_required.
// bootACMEForRebind boots a degraded (no stored cert) http-01 acme topology with
// the listener rebinder wired and adopted into the controller, returning the live
// acme HTTPS address. The renewer stays offline (agree_to_tos false). The caller
// drains via app.listenerController on cleanup.
func bootACMEForRebind(t *testing.T, app *serverApp) string {
	t.Helper()
	app.listenerRebind = webapi.NewListenerRebindHolder()
	app.cfg.Server.TLS.ACME.Domains = []string{"metrics.example.com"}
	app.cfg.Server.TLS.ACME.Challenge = "http-01"
	app.cfg.Server.TLS.ACME.AgreeToTOS = false

	res, err := app.setupACME(http.NewServeMux(), newFakeSecretStore(), 5*time.Second)
	if err != nil {
		t.Fatalf("setupACME: %v", err)
	}
	if app.listenerController == nil {
		t.Fatal("acme boot did not adopt the controller")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.listenerController.Shutdown(ctx)
	})
	return res.tlsListener.Addr()
}

// acme→static exit on a NEW port (H4c-2a): the applier leaves acme by building the
// static listener via the normal closure and release-first swapping the whole acme
// Instance, so the renewer + port-80 challenge + HTTPS all tear down and the new
// static HTTPS serves. acmeActive is cleared so subsequent saves use static logic.
func TestSetupACME_ExitToStaticNewPort(t *testing.T) {
	app := newACMEApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	acmeAddr := bootACMEForRebind(t, app)

	client := insecureTLSClient()
	getWithRetry(t, client, "https://"+acmeAddr+"/").Body.Close()

	certPath, keyPath := writeSelfSignedFiles(t)
	newPort := freePort(t)
	gran, err := app.listenerRebind.Apply(staticServerCfg("127.0.0.1", newPort, certPath, keyPath))
	if err != nil {
		t.Fatalf("acme→static Apply: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}
	if app.acmeActive {
		t.Error("acmeActive must be cleared after leaving acme")
	}

	getWithRetry(t, client, fmt.Sprintf("https://127.0.0.1:%d/", newPort)).Body.Close()
	waitUnreachable(t, acmeAddr) // old acme HTTPS (+ challenge + renewer) drained
}

// acme→static exit on the SAME port the acme HTTPS held: release-first frees the
// port and the static build reclaims it via the bind retry.
func TestSetupACME_ExitToStaticSamePort(t *testing.T) {
	app := newACMEApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	samePort := app.cfg.Server.Port // acme HTTPS binds the configured port when degraded
	acmeAddr := bootACMEForRebind(t, app)

	client := insecureTLSClient()
	getWithRetry(t, client, "https://"+acmeAddr+"/").Body.Close()

	certPath, keyPath := writeSelfSignedFiles(t)
	gran, err := app.listenerRebind.Apply(staticServerCfg("127.0.0.1", samePort, certPath, keyPath))
	if err != nil {
		t.Fatalf("acme→static same-port Apply: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}
	getWithRetry(t, client, fmt.Sprintf("https://127.0.0.1:%d/", samePort)).Body.Close()
}

// acme→off exit (H4c-2a): leaves acme for plain HTTP, tearing down the acme
// topology; the new plain listener serves and acmeActive is cleared.
func TestSetupACME_ExitToOff(t *testing.T) {
	app := newACMEApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	acmeAddr := bootACMEForRebind(t, app)

	tlsClient := insecureTLSClient()
	getWithRetry(t, tlsClient, "https://"+acmeAddr+"/").Body.Close()

	newPort := freePort(t)
	gran, err := app.listenerRebind.Apply(offServerCfg("127.0.0.1", newPort))
	if err != nil {
		t.Fatalf("acme→off Apply: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}
	if app.acmeActive {
		t.Error("acmeActive must be cleared after leaving acme")
	}
	getWithRetry(t, &http.Client{Timeout: time.Second}, fmt.Sprintf("http://127.0.0.1:%d/", newPort)).Body.Close()
	waitUnreachable(t, acmeAddr)
}

// Re-planning the acme topology in place (entry, or an acme-internal change such
// as a different challenge port) is deferred to H4c-2b: the applier still refuses
// any acme target so the save is reported restart_required.
func TestSetupACME_RefusesACMEInternalChange(t *testing.T) {
	app := newACMEApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	_ = bootACMEForRebind(t, app)

	internal := app.cfg.Server // still mode acme
	internal.TLS.HTTPRedirectPort = freePort(t)
	if _, err := app.listenerRebind.Apply(internal); !errors.Is(err, webapi.ErrNoListenerRebinder) {
		t.Errorf("acme-internal change: err = %v, want ErrNoListenerRebinder", err)
	}
}
