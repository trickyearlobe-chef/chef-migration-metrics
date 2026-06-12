// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	apptls "github.com/trickyearlobe-chef/chef-migration-metrics/internal/tls"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/webapi"
)

// freePort returns a currently-free TCP port on the loopback interface.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// newTestApp builds a serverApp with just enough wiring (bootstrap logger,
// config, TLS status holder) to exercise the plain-HTTP fallback helpers.
func newTestApp(t *testing.T) *serverApp {
	t.Helper()
	app := &serverApp{
		cfg: &config.Config{},
	}
	// Bind to a free loopback port so concurrent tests never collide.
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	app.setupBootstrapLogger()
	app.tlsStatus = webapi.NewTLSStatusHolder()
	// Default the automatic-443 bind (tls.md § 1.5) to "unavailable" so tests run
	// deterministically without attempting a privileged port bind; tests that
	// exercise the 443 path override this with a free-port listener.
	app.auto443Listen = func(string) (net.Listener, error) {
		return nil, errors.New("443 unavailable in test")
	}
	return app
}

func shutdown(t *testing.T, res serverResult) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if res.plainSrv != nil {
		_ = res.plainSrv.Shutdown(ctx)
	}
	if res.tlsListener != nil {
		_ = res.tlsListener.Shutdown(ctx)
	}
}

// An admin-requested restart (signalled on restartCh) must drain gracefully
// and return the non-zero restart exit code so the supervisor restarts the
// process. A plain SIGTERM/clean stop returns 0 (covered by the default path).
func TestAwaitShutdown_RestartRequestReturnsRestartCode(t *testing.T) {
	app := newTestApp(t)
	app.sched = &collector.Scheduler{} // unstarted — Stop() is a safe no-op
	app.restartCh = make(chan struct{}, 1)
	app.cfg.Server.GracefulShutdownSeconds = 1

	// Pre-load a restart request so awaitShutdown returns immediately.
	app.restartCh <- struct{}{}

	code := app.awaitShutdown(serverResult{})
	if code != exitCodeRestart {
		t.Fatalf("expected restart exit code %d, got %d", exitCodeRestart, code)
	}
}

// graceful_shutdown_seconds must be resolved live from the config holder at
// shutdown time, so a saved change applies without a restart (config live-reload
// H1). The boot config is only a fallback when no holder is wired.
func TestResolveShutdownTimeout_LiveFromHolder(t *testing.T) {
	app := newTestApp(t)

	// No holder wired: the boot config value is used.
	app.cfg.Server.GracefulShutdownSeconds = 30
	if got := app.resolveShutdownTimeout(); got != 30*time.Second {
		t.Errorf("boot fallback: got %v, want 30s", got)
	}

	// Holder wired with a different value: the live value wins over boot.
	live := &config.Config{}
	live.Server.GracefulShutdownSeconds = 5
	app.configHolder = configstore.NewConfigHolder(live, nil)
	if got := app.resolveShutdownTimeout(); got != 5*time.Second {
		t.Errorf("live holder: got %v, want 5s", got)
	}

	// A non-positive live value falls back to the 15s default.
	app.configHolder.Set(&config.Config{})
	if got := app.resolveShutdownTimeout(); got != 15*time.Second {
		t.Errorf("default: got %v, want 15s", got)
	}
}

// A static-TLS setup failure must flip the degraded flag and still return a
// running plain HTTP server — never an error that would exit the process.
func TestDegradeToPlainHTTP_SetsDegradedAndServes(t *testing.T) {
	app := newTestApp(t)

	res := app.degradeToPlainHTTP(http.NewServeMux(), errors.New("cert load failed: no such file"))
	defer shutdown(t, res)

	if res.plainSrv == nil {
		t.Fatal("expected a plain HTTP server, got nil")
	}
	if res.errCh == nil {
		t.Fatal("expected an error channel")
	}

	status := app.tlsStatus.Status()
	if !status.Degraded {
		t.Fatal("expected degraded TLS status after fallback")
	}
	if !strings.Contains(status.Reason, "cert load failed") {
		t.Errorf("reason = %q, want it to include the cause", status.Reason)
	}
	if !strings.HasPrefix(status.Reason, "TLS listener setup failed:") {
		t.Errorf("reason = %q, want the operator-facing prefix", status.Reason)
	}
}

// A static-TLS setup failure now falls open to a self-signed HTTPS listener
// (encrypted recovery UI), not cleartext: the degraded status carries the
// self-signed kind, the listener actually serves HTTPS, and HSTS is suppressed.
func TestDegradeToSelfSigned_ServesHTTPSDegraded(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()

	res := app.degradeToSelfSigned(http.NewServeMux(), nil, errors.New("cert load failed: no such file"))
	defer shutdown(t, res)

	if res.tlsListener == nil {
		t.Fatal("expected a self-signed TLS listener, got nil")
	}
	status := app.tlsStatus.Status()
	if !status.Degraded {
		t.Fatal("expected degraded TLS status after self-signed fallback")
	}
	if status.Kind != webapi.DegradedKindSelfSigned {
		t.Errorf("kind = %q, want %q", status.Kind, webapi.DegradedKindSelfSigned)
	}
	if !strings.Contains(status.Reason, "cert load failed") {
		t.Errorf("reason = %q, want it to include the cause", status.Reason)
	}
	// HSTS must be suppressed while degraded (self-signed cert).
	if app.hstsEnabledFn()() {
		t.Error("HSTS gate must be closed while serving a self-signed cert")
	}

	// The listener really serves HTTPS with a self-signed cert.
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed by design
	}}
	url := "https://" + res.tlsListener.Addr() + "/"
	resp := getWithRetry(t, client, url)
	defer resp.Body.Close()
	if resp.Header.Get("Strict-Transport-Security") != "" {
		t.Error("HSTS header must not be sent over the self-signed degraded listener")
	}
}

// Promoting a real certificate in place over the degraded self-signed listener
// clears the degraded state and re-opens the HSTS gate without a restart.
func TestDegradeToSelfSigned_PromotionClearsDegraded(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.tlsReload.SetOnReload(app.tlsStatus.SetHealthy)

	res := app.degradeToSelfSigned(http.NewServeMux(), nil, errors.New("bad cert"))
	defer shutdown(t, res)

	if !app.tlsStatus.IsDegraded() {
		t.Fatal("expected degraded before promotion")
	}

	// A valid pair saved/issued reloads in place and clears degraded.
	realCert, realKey, err := apptls.GenerateSelfSigned([]string{"metrics.example.com"})
	if err != nil {
		t.Fatalf("generate promotion cert: %v", err)
	}
	if err := app.tlsReload.Reload(realCert, realKey); err != nil {
		t.Fatalf("promotion reload: %v", err)
	}
	if app.tlsStatus.IsDegraded() {
		t.Error("expected degraded cleared after in-place promotion")
	}
	if !app.hstsEnabledFn()() {
		t.Error("HSTS gate must reopen once a real cert is promoted")
	}
}

// getWithRetry dials a freshly-started listener, retrying briefly while the
// background Serve goroutine binds the port.
func getWithRetry(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	var lastErr error
	for range 50 {
		resp, err := client.Get(url)
		if err == nil {
			return resp
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("GET %s never succeeded: %v", url, lastErr)
	return nil
}

// The plain-HTTP path (mode: off) must not mark the server degraded.
func TestServePlainHTTP_NotDegraded(t *testing.T) {
	app := newTestApp(t)

	res := app.servePlainHTTP(http.NewServeMux())
	defer shutdown(t, res)

	if res.plainSrv == nil {
		t.Fatal("expected a plain HTTP server")
	}
	if app.tlsStatus.Status().Degraded {
		t.Error("plain HTTP mode must not be degraded")
	}
}

// A bad (unbindable) configured port must not lock out the UI: the server
// falls back to the bootstrap target, binds it, and flags degraded mode.
func TestServePlainHTTP_BadPortFallsBackDegraded(t *testing.T) {
	app := newTestApp(t)

	// Occupy a port so the configured target cannot be bound.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer occupied.Close()
	badPort := occupied.Addr().(*net.TCPAddr).Port

	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = badPort
	app.bootstrapListenAddr = "127.0.0.1"
	app.bootstrapPort = freePort(t)

	res := app.servePlainHTTP(http.NewServeMux())
	defer shutdown(t, res)

	if res.plainSrv == nil {
		t.Fatal("expected a fallback plain HTTP server, got nil")
	}
	status := app.tlsStatus.Status()
	if !status.Degraded {
		t.Fatal("expected degraded status after bind fallback")
	}
	if !strings.Contains(status.Reason, "fallback") {
		t.Errorf("reason = %q, want it to mention the fallback", status.Reason)
	}
	if strings.HasSuffix(res.plainSrv.Addr, fmt.Sprintf(":%d", badPort)) {
		t.Errorf("server bound the bad port %d, expected a fallback", badPort)
	}
}

// reachable reports whether a plain-HTTP GET to addr succeeds.
func reachable(addr string) bool {
	c := &http.Client{Timeout: time.Second}
	resp, err := c.Get("http://" + addr + "/")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

// waitUnreachable polls until addr stops accepting (the drained old listener).
func waitUnreachable(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !reachable(addr) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("listener %s still accepting after rebind", addr)
}

// A plain-HTTP server adopted into a controller rebinds to a new port in place:
// the new address serves and the old one drains, with no restart.
func TestAdoptPlainController_RebindsAcrossPorts(t *testing.T) {
	app := newTestApp(t)
	app.listenerRebind = webapi.NewListenerRebindHolder()

	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	app.bootstrapListenAddr = "127.0.0.1"
	app.bootstrapPort = app.cfg.Server.Port

	res := app.servePlainHTTP(http.NewServeMux())
	app.adoptPlainController(http.NewServeMux(), res)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.listenerController.Shutdown(ctx)
	})

	oldAddr := res.plainSrv.Addr
	if !reachable(oldAddr) {
		// Give the boot goroutine a moment to start accepting.
		client := &http.Client{Timeout: time.Second}
		getWithRetry(t, client, "http://"+oldAddr+"/").Body.Close()
	}

	newPort := freePort(t)
	gran, err := app.listenerRebind.Rebind("127.0.0.1", newPort)
	if err != nil {
		t.Fatalf("Rebind: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	newAddr := fmt.Sprintf("127.0.0.1:%d", newPort)
	client := &http.Client{Timeout: time.Second}
	getWithRetry(t, client, "http://"+newAddr+"/").Body.Close()
	waitUnreachable(t, oldAddr)
}

// A rebind to a port held by another process fails: the bind error is returned
// and the old listener keeps serving (bind-new-first is a no-op on failure).
func TestAdoptPlainController_RebindBindFailureKeepsOld(t *testing.T) {
	app := newTestApp(t)
	app.listenerRebind = webapi.NewListenerRebindHolder()

	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	app.bootstrapListenAddr = "127.0.0.1"
	app.bootstrapPort = app.cfg.Server.Port

	res := app.servePlainHTTP(http.NewServeMux())
	app.adoptPlainController(http.NewServeMux(), res)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.listenerController.Shutdown(ctx)
	})

	oldAddr := res.plainSrv.Addr
	client := &http.Client{Timeout: time.Second}
	getWithRetry(t, client, "http://"+oldAddr+"/").Body.Close()

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy: %v", err)
	}
	defer occupied.Close()
	badPort := occupied.Addr().(*net.TCPAddr).Port

	if _, err := app.listenerRebind.Rebind("127.0.0.1", badPort); err == nil {
		t.Fatal("Rebind to occupied port: want error, got nil")
	}
	// Old listener must still be serving.
	if !reachable(oldAddr) {
		t.Errorf("old listener %s stopped serving after a failed rebind", oldAddr)
	}
}

// A static-TLS listener adopted into a controller rebinds the HTTPS listener to
// a new port in place: HTTPS serves on the new address and the old drains.
func TestAdoptTLSController_RebindsAcrossPorts(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPEM, keyPEM, err := apptls.GenerateSelfSigned([]string{"localhost"})
	if err != nil {
		t.Fatalf("self-signed: %v", err)
	}
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)

	lcfg := apptls.ListenerConfig{
		ListenAddress: "127.0.0.1",
		Port:          app.cfg.Server.Port,
		CertSource:    "db",
		CertPEM:       certPEM,
		KeyPEM:        keyPEM,
		HSTSEnabled:   app.hstsEnabledFn(),
	}
	boot, err := apptls.NewListener(http.NewServeMux(), lcfg, app.tlsLog)
	if err != nil {
		t.Fatalf("boot listener: %v", err)
	}
	boot.Serve()
	app.adoptTLSController(http.NewServeMux(), lcfg, boot)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.listenerController.Shutdown(ctx)
	})

	tlsClient := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed by design
	}}
	oldAddr := boot.Addr()
	getWithRetry(t, tlsClient, "https://"+oldAddr+"/").Body.Close()

	newPort := freePort(t)
	gran, err := app.listenerRebind.Rebind("127.0.0.1", newPort)
	if err != nil {
		t.Fatalf("Rebind: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	newAddr := fmt.Sprintf("127.0.0.1:%d", newPort)
	getWithRetry(t, tlsClient, "https://"+newAddr+"/").Body.Close()

	// Old HTTPS listener drains.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := tlsClient.Get("https://" + oldAddr + "/"); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("old HTTPS listener %s still accepting after rebind", oldAddr)
}
