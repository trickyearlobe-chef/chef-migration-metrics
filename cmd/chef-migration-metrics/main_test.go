// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
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
	if res.plainSrv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = res.plainSrv.Shutdown(ctx)
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
