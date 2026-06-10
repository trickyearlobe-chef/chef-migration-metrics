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
