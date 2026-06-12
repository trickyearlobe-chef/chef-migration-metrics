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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/collector"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/serverctl"
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

// adoptPlainBoot boots a plain-HTTP listener on app's configured target and
// adopts it into the unified listener controller, returning the boot address.
func adoptPlainBoot(t *testing.T, app *serverApp, handler http.Handler) string {
	t.Helper()
	res := app.servePlainHTTP(handler)
	if res.plainSrv == nil {
		t.Fatalf("servePlainHTTP produced no server")
	}
	app.adoptListenerController(handler,
		&serverctl.Instance{Addr: res.plainSrv.Addr, Shutdown: res.plainSrv.Shutdown},
		app.cfg.Server)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.listenerController.Shutdown(ctx)
	})
	return res.plainSrv.Addr
}

// writeSelfSignedFiles writes a fresh self-signed cert/key pair to temp files and
// returns their paths, for exercising the cert_source: file static-TLS path.
func writeSelfSignedFiles(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	certPEM, keyPEM, err := apptls.GenerateSelfSigned([]string{"localhost"})
	if err != nil {
		t.Fatalf("self-signed: %v", err)
	}
	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}

// offServerCfg / staticServerCfg build the desired server configs the rebinder
// applies in tests.
func offServerCfg(addr string, port int) config.ServerConfig {
	var cfg config.ServerConfig
	cfg.ListenAddress = addr
	cfg.Port = port
	return cfg
}

func staticServerCfg(addr string, port int, certPath, keyPath string) config.ServerConfig {
	cfg := offServerCfg(addr, port)
	cfg.TLS.Mode = "static"
	cfg.TLS.CertSource = "file"
	cfg.TLS.CertPath = certPath
	cfg.TLS.KeyPath = keyPath
	return cfg
}

// A plain-HTTP server adopted into the listener controller rebinds to a new port
// in place: the new address serves and the old one drains, with no restart.
func TestAdoptListenerController_PlainRebindsAcrossPorts(t *testing.T) {
	app := newTestApp(t)
	app.listenerRebind = webapi.NewListenerRebindHolder()

	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)

	oldAddr := adoptPlainBoot(t, app, http.NewServeMux())
	client := &http.Client{Timeout: time.Second}
	getWithRetry(t, client, "http://"+oldAddr+"/").Body.Close()

	newPort := freePort(t)
	gran, err := app.listenerRebind.Apply(offServerCfg("127.0.0.1", newPort))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	newAddr := fmt.Sprintf("127.0.0.1:%d", newPort)
	getWithRetry(t, client, "http://"+newAddr+"/").Body.Close()
	waitUnreachable(t, oldAddr)
}

// An apply to a port held by another process fails: the bind error is returned
// and the old listener keeps serving (bind-new-first is a no-op on failure).
func TestAdoptListenerController_PlainBindFailureKeepsOld(t *testing.T) {
	app := newTestApp(t)
	app.listenerRebind = webapi.NewListenerRebindHolder()

	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)

	oldAddr := adoptPlainBoot(t, app, http.NewServeMux())
	client := &http.Client{Timeout: time.Second}
	getWithRetry(t, client, "http://"+oldAddr+"/").Body.Close()

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy: %v", err)
	}
	defer occupied.Close()
	badPort := occupied.Addr().(*net.TCPAddr).Port

	if _, err := app.listenerRebind.Apply(offServerCfg("127.0.0.1", badPort)); err == nil {
		t.Fatal("Apply to occupied port: want error, got nil")
	}
	// Old listener must still be serving.
	if !reachable(oldAddr) {
		t.Errorf("old listener %s stopped serving after a failed apply", oldAddr)
	}
}

// A static-TLS listener adopted into a controller rebinds the HTTPS listener to
// a new port in place: HTTPS serves on the new address and the old drains.
// adoptStaticBoot boots a static-TLS (file cert) listener on app's configured
// target and adopts it into the unified listener controller, returning the boot
// HTTPS address.
func adoptStaticBoot(t *testing.T, app *serverApp, handler http.Handler, certPath, keyPath string) string {
	t.Helper()
	lcfg := apptls.ListenerConfig{
		ListenAddress: app.cfg.Server.ListenAddress,
		Port:          app.cfg.Server.Port,
		CertSource:    "file",
		CertPath:      certPath,
		KeyPath:       keyPath,
		HSTSEnabled:   app.hstsEnabledFn(),
	}
	boot, err := apptls.NewListener(handler, lcfg, app.tlsLog)
	if err != nil {
		t.Fatalf("boot listener: %v", err)
	}
	boot.Serve()
	app.cfg.Server.TLS.Mode = "static"
	app.cfg.Server.TLS.CertSource = "file"
	app.cfg.Server.TLS.CertPath = certPath
	app.cfg.Server.TLS.KeyPath = keyPath
	app.adoptListenerController(handler,
		&serverctl.Instance{Addr: boot.Addr(), Shutdown: boot.Shutdown}, app.cfg.Server)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.listenerController.Shutdown(ctx)
	})
	return boot.Addr()
}

// adoptStaticBootWithRedirect boots a static-TLS (file cert) HTTPS listener plus
// an http_redirect_port redirect listener on app's configured target and adopts it
// into the unified listener controller, returning the boot HTTPS address. It mirrors
// the boot path where a single HTTPS listener owns the configured port and an
// explicit redirect listener serves http_redirect_port (no auto-443).
func adoptStaticBootWithRedirect(t *testing.T, app *serverApp, handler http.Handler, certPath, keyPath string, redirectPort int) string {
	t.Helper()
	lcfg := apptls.ListenerConfig{
		ListenAddress:    app.cfg.Server.ListenAddress,
		Port:             app.cfg.Server.Port,
		CertSource:       "file",
		CertPath:         certPath,
		KeyPath:          keyPath,
		HTTPRedirectPort: redirectPort,
		HSTSEnabled:      app.hstsEnabledFn(),
	}
	boot, err := apptls.NewListener(handler, lcfg, app.tlsLog)
	if err != nil {
		t.Fatalf("boot listener: %v", err)
	}
	boot.Serve()
	app.cfg.Server.TLS.Mode = "static"
	app.cfg.Server.TLS.CertSource = "file"
	app.cfg.Server.TLS.CertPath = certPath
	app.cfg.Server.TLS.KeyPath = keyPath
	app.cfg.Server.TLS.HTTPRedirectPort = redirectPort
	app.adoptListenerController(handler,
		&serverctl.Instance{Addr: boot.Addr(), Shutdown: boot.Shutdown}, app.cfg.Server)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.listenerController.Shutdown(ctx)
	})
	return boot.Addr()
}

// waitRedirects polls until the HTTP listener at addr 301-redirects to https, or
// fails after the deadline. It proves a redirect listener is bound and serving.
func waitRedirects(t *testing.T, addr string) {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       time.Second,
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://" + addr + "/")
		if err == nil {
			loc := resp.Header.Get("Location")
			code := resp.StatusCode
			_ = resp.Body.Close()
			if code == http.StatusMovedPermanently && strings.HasPrefix(loc, "https://") {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("redirect listener %s did not 301 to https within the deadline", addr)
}

// insecureTLSClient is an HTTP client that accepts the self-signed certs the
// rebind tests serve.
func insecureTLSClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed by design
	}}
}

// waitTLSUnreachable fails if the HTTPS listener at addr is still accepting after
// the drain window.
func waitTLSUnreachable(t *testing.T, client *http.Client, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := client.Get("https://" + addr + "/"); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("old HTTPS listener %s still accepting after rebind", addr)
}

// A static-TLS listener adopted into the controller rebinds the HTTPS listener to
// a new port in place: HTTPS serves on the new address and the old drains.
func TestAdoptListenerController_TLSRebindsAcrossPorts(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)

	oldAddr := adoptStaticBoot(t, app, http.NewServeMux(), certPath, keyPath)
	tlsClient := insecureTLSClient()
	getWithRetry(t, tlsClient, "https://"+oldAddr+"/").Body.Close()

	newPort := freePort(t)
	gran, err := app.listenerRebind.Apply(staticServerCfg("127.0.0.1", newPort, certPath, keyPath))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	newAddr := fmt.Sprintf("127.0.0.1:%d", newPort)
	getWithRetry(t, tlsClient, "https://"+newAddr+"/").Body.Close()
	waitTLSUnreachable(t, tlsClient, oldAddr)
}

// off→static with a port change rebinds the plain listener to an HTTPS listener
// in place (H4a): HTTPS serves on the new port and the old plain listener drains.
func TestAdoptListenerController_OffToStaticNewPort(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)

	oldAddr := adoptPlainBoot(t, app, http.NewServeMux())
	getWithRetry(t, &http.Client{Timeout: time.Second}, "http://"+oldAddr+"/").Body.Close()

	certPath, keyPath := writeSelfSignedFiles(t)
	newPort := freePort(t)
	gran, err := app.listenerRebind.Apply(staticServerCfg("127.0.0.1", newPort, certPath, keyPath))
	if err != nil {
		t.Fatalf("Apply off→static: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	newAddr := fmt.Sprintf("127.0.0.1:%d", newPort)
	tlsClient := insecureTLSClient()
	getWithRetry(t, tlsClient, "https://"+newAddr+"/").Body.Close()
	waitUnreachable(t, oldAddr) // old plain listener drained
}

// static→off with a port change rebinds the HTTPS listener to a plain listener in
// place (H4a): plain HTTP serves on the new port and the old HTTPS drains.
func TestAdoptListenerController_StaticToOffNewPort(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)

	oldAddr := adoptStaticBoot(t, app, http.NewServeMux(), certPath, keyPath)
	tlsClient := insecureTLSClient()
	getWithRetry(t, tlsClient, "https://"+oldAddr+"/").Body.Close()

	newPort := freePort(t)
	gran, err := app.listenerRebind.Apply(offServerCfg("127.0.0.1", newPort))
	if err != nil {
		t.Fatalf("Apply static→off: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	newAddr := fmt.Sprintf("127.0.0.1:%d", newPort)
	getWithRetry(t, &http.Client{Timeout: time.Second}, "http://"+newAddr+"/").Body.Close()
	waitTLSUnreachable(t, tlsClient, oldAddr) // old HTTPS listener drained
}

// A mode toggle that keeps the SAME address:port is applied live: bind-new-first
// is impossible (the old listener holds the port), so the controller releases the
// old plain listener and binds the new HTTPS listener on the freed port (with a
// bind retry to absorb the release lag). The new HTTPS serves on the same port.
func TestAdoptListenerController_SamePortModeToggleAppliesLive(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	samePort := app.cfg.Server.Port

	oldAddr := adoptPlainBoot(t, app, http.NewServeMux())
	getWithRetry(t, &http.Client{Timeout: time.Second}, "http://"+oldAddr+"/").Body.Close()

	certPath, keyPath := writeSelfSignedFiles(t)
	gran, err := app.listenerRebind.Apply(staticServerCfg("127.0.0.1", samePort, certPath, keyPath))
	if err != nil {
		t.Fatalf("Apply same-port off→static: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}
	// HTTPS now serves on the same port (the plain listener was replaced).
	getWithRetry(t, insecureTLSClient(), fmt.Sprintf("https://127.0.0.1:%d/", samePort)).Body.Close()
}

// A same-port mode toggle with an UNUSABLE certificate is rejected before the old
// listener is touched: the construct-without-bind validation fails, the error is
// returned, and the old plain listener keeps serving (nothing released).
func TestAdoptListenerController_SamePortToggleBadCertKeepsOld(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	samePort := app.cfg.Server.Port

	oldAddr := adoptPlainBoot(t, app, http.NewServeMux())
	getWithRetry(t, &http.Client{Timeout: time.Second}, "http://"+oldAddr+"/").Body.Close()

	// Point at a non-existent cert file so NewListener (construct) fails.
	bad := staticServerCfg("127.0.0.1", samePort, filepath.Join(t.TempDir(), "missing.pem"), filepath.Join(t.TempDir(), "missing.key"))
	if _, err := app.listenerRebind.Apply(bad); err == nil {
		t.Fatal("Apply same-port toggle with bad cert: want error, got nil")
	}
	// Old plain listener untouched — validation happened before any release.
	if !reachable(oldAddr) {
		t.Errorf("old listener %s stopped serving after a rejected bad-cert toggle", oldAddr)
	}
}

// An ACME target is not rebindable in place yet (H4c): the applier refuses it so
// the save is reported restart_required. (A static http_redirect_port target IS
// rebindable as of H4b-2 — see the redirect tests below.)
func TestAdoptListenerController_RefusesUnsupportedTargets(t *testing.T) {
	app := newTestApp(t)
	app.listenerRebind = webapi.NewListenerRebindHolder()
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	adoptPlainBoot(t, app, http.NewServeMux())

	acme := offServerCfg("127.0.0.1", freePort(t))
	acme.TLS.Mode = "acme"
	if _, err := app.listenerRebind.Apply(acme); !errors.Is(err, webapi.ErrNoListenerRebinder) {
		t.Errorf("Apply acme target: err = %v, want ErrNoListenerRebinder", err)
	}
}

// A same-mode static change that only raises min_version is applied live in place
// (H4b-1): bind-new-first is impossible (the new listener wants the port the old
// one holds), so the controller validates the rebuilt listener then rebinds it on
// the freed port. The rebuilt HTTPS listener serves on the same port and now
// rejects a TLS 1.2 client — proving the new min_version actually took effect.
func TestAdoptListenerController_StaticMinVersionAppliesLive(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	samePort := app.cfg.Server.Port

	oldAddr := adoptStaticBoot(t, app, http.NewServeMux(), certPath, keyPath)
	getWithRetry(t, insecureTLSClient(), "https://"+oldAddr+"/").Body.Close()

	cfg := staticServerCfg("127.0.0.1", samePort, certPath, keyPath)
	cfg.TLS.MinVersion = "1.3"
	gran, err := app.listenerRebind.Apply(cfg)
	if err != nil {
		t.Fatalf("Apply min_version raise: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	// The rebuilt listener serves on the same port.
	url := fmt.Sprintf("https://127.0.0.1:%d/", samePort)
	getWithRetry(t, insecureTLSClient(), url).Body.Close()

	// A TLS 1.2-max client is now rejected (poll until the old listener has fully
	// drained and only the min-1.3 listener answers).
	tls12 := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12, //nolint:gosec // self-signed test cert
	}}}
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, getErr := tls12.Get(url)
		if getErr != nil {
			break // rejected — min 1.3 is enforced
		}
		resp.Body.Close()
		if time.Now().After(deadline) {
			t.Fatal("TLS 1.2 client still accepted after min_version raised to 1.3")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Re-saving an unchanged static config is a no-op: the controller key (including
// the TLS topology fingerprint) is unchanged, so nothing is rebound and the save
// reports applied, not listener. The listener keeps serving on the same port.
func TestAdoptListenerController_StaticUnchangedNoOp(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	samePort := app.cfg.Server.Port

	oldAddr := adoptStaticBoot(t, app, http.NewServeMux(), certPath, keyPath)
	getWithRetry(t, insecureTLSClient(), "https://"+oldAddr+"/").Body.Close()

	gran, err := app.listenerRebind.Apply(staticServerCfg("127.0.0.1", samePort, certPath, keyPath))
	if err != nil {
		t.Fatalf("Apply unchanged static: %v", err)
	}
	if gran != webapi.ReloadApplied {
		t.Errorf("granularity = %v, want applied (no-op)", gran)
	}
	// Nothing was torn down — still serving on the same port.
	getWithRetry(t, insecureTLSClient(), fmt.Sprintf("https://127.0.0.1:%d/", samePort)).Body.Close()
}

// Adding an http_redirect_port to a running static deployment is applied live
// (H4b-2): the HTTPS port is unchanged so the controller rebinds in place (releases
// the old, rebinds the same HTTPS port + the new redirect listener). HTTPS still
// serves on its port and the new redirect listener 301s to https.
func TestAdoptListenerController_StaticAddRedirectAppliesLive(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	samePort := app.cfg.Server.Port

	oldAddr := adoptStaticBoot(t, app, http.NewServeMux(), certPath, keyPath)
	getWithRetry(t, insecureTLSClient(), "https://"+oldAddr+"/").Body.Close()

	redirectPort := freePort(t)
	cfg := staticServerCfg("127.0.0.1", samePort, certPath, keyPath)
	cfg.TLS.HTTPRedirectPort = redirectPort
	gran, err := app.listenerRebind.Apply(cfg)
	if err != nil {
		t.Fatalf("Apply add redirect: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	// HTTPS still serves on the same port, and the new redirect listener 301s.
	getWithRetry(t, insecureTLSClient(), fmt.Sprintf("https://127.0.0.1:%d/", samePort)).Body.Close()
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", redirectPort))
}

// Changing an existing http_redirect_port is applied live (H4b-2): the old redirect
// listener drains and the new redirect port begins serving, with HTTPS unchanged on
// its port.
func TestAdoptListenerController_StaticChangeRedirectAppliesLive(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	samePort := app.cfg.Server.Port
	oldRedirect := freePort(t)

	adoptStaticBootWithRedirect(t, app, http.NewServeMux(), certPath, keyPath, oldRedirect)
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", oldRedirect))

	newRedirect := freePort(t)
	cfg := staticServerCfg("127.0.0.1", samePort, certPath, keyPath)
	cfg.TLS.HTTPRedirectPort = newRedirect
	gran, err := app.listenerRebind.Apply(cfg)
	if err != nil {
		t.Fatalf("Apply change redirect: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	getWithRetry(t, insecureTLSClient(), fmt.Sprintf("https://127.0.0.1:%d/", samePort)).Body.Close()
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", newRedirect))
	waitUnreachable(t, fmt.Sprintf("127.0.0.1:%d", oldRedirect)) // old redirect drained
}

// A static field change (min_version) while an http_redirect_port stays the same
// rebinds in place: RebindInPlace releases BOTH the HTTPS port and the redirect
// port, so the rebuild must reclaim both via the retrying bind (H4b-2). Both serve
// again after the rebind.
func TestAdoptListenerController_StaticRedirectKeptAcrossFieldChange(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	samePort := app.cfg.Server.Port
	redirectPort := freePort(t)

	adoptStaticBootWithRedirect(t, app, http.NewServeMux(), certPath, keyPath, redirectPort)
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", redirectPort))

	cfg := staticServerCfg("127.0.0.1", samePort, certPath, keyPath)
	cfg.TLS.HTTPRedirectPort = redirectPort
	cfg.TLS.MinVersion = "1.3"
	gran, err := app.listenerRebind.Apply(cfg)
	if err != nil {
		t.Fatalf("Apply min_version raise (redirect kept): %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	// Both the HTTPS port and the (reclaimed) redirect port serve after the rebind.
	getWithRetry(t, insecureTLSClient(), fmt.Sprintf("https://127.0.0.1:%d/", samePort)).Body.Close()
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", redirectPort))
}

// Removing an http_redirect_port is applied live (H4b-2): the redirect listener
// drains and only HTTPS keeps serving on its port.
func TestAdoptListenerController_StaticRemoveRedirectAppliesLive(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	samePort := app.cfg.Server.Port
	redirectPort := freePort(t)

	adoptStaticBootWithRedirect(t, app, http.NewServeMux(), certPath, keyPath, redirectPort)
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", redirectPort))

	gran, err := app.listenerRebind.Apply(staticServerCfg("127.0.0.1", samePort, certPath, keyPath))
	if err != nil {
		t.Fatalf("Apply remove redirect: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	getWithRetry(t, insecureTLSClient(), fmt.Sprintf("https://127.0.0.1:%d/", samePort)).Body.Close()
	waitUnreachable(t, fmt.Sprintf("127.0.0.1:%d", redirectPort)) // redirect drained
}

// adoptAuto443Boot boots the automatic-HTTPS-on-443 lifeboat topology (tls.md
// § 1.5): a static-TLS HTTPS listener on a free "443 stand-in" port with the
// configured server.port (and an optional http_redirect_port) redirecting to it,
// then adopts it into the listener controller with autoHTTPSActive set. It returns
// the lifeboat HTTPS port. Mirrors the setupAndServeHTTP auto-443 branch without a
// privileged 443 bind.
func adoptAuto443Boot(t *testing.T, app *serverApp, handler http.Handler, certPath, keyPath string, redirectPort int) int {
	t.Helper()
	ln443, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind 443 stand-in: %v", err)
	}
	httpsPort := ln443.Addr().(*net.TCPAddr).Port

	redirectPorts := []int{app.cfg.Server.Port}
	if redirectPort > 0 {
		redirectPorts = append(redirectPorts, redirectPort)
	}
	lcfg := apptls.ListenerConfig{
		ListenAddress: "127.0.0.1",
		Port:          httpsPort,
		CertSource:    "file",
		CertPath:      certPath,
		KeyPath:       keyPath,
		RedirectPorts: redirectPorts,
		HSTSEnabled:   app.hstsEnabledFn(),
	}
	boot, err := apptls.NewListener(handler, lcfg, app.tlsLog)
	if err != nil {
		_ = ln443.Close()
		t.Fatalf("boot auto-443 listener: %v", err)
	}
	boot.SetHTTPSListener(ln443)
	boot.Serve()

	app.cfg.Server.TLS.Mode = "static"
	app.cfg.Server.TLS.CertSource = "file"
	app.cfg.Server.TLS.CertPath = certPath
	app.cfg.Server.TLS.KeyPath = keyPath
	app.cfg.Server.TLS.HTTPRedirectPort = redirectPort
	app.autoHTTPSActive = true
	app.autoHTTPSPort = httpsPort
	app.adoptListenerController(handler,
		&serverctl.Instance{Addr: boot.Addr(), Shutdown: boot.Shutdown}, app.cfg.Server)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.listenerController.Shutdown(ctx)
	})
	return httpsPort
}

// A same-mode static field change (min_version) on an active auto-443 lifeboat is
// re-planned in place (H4b-3): RebindInPlace releases the lifeboat HTTPS port and
// the configured-port redirect, and the rebuild reclaims both. HTTPS still serves
// on the lifeboat port (now min 1.3 — a TLS 1.2 client is rejected) and the
// redirect keeps 301ing.
func TestAdoptListenerController_Auto443MinVersionAppliesLive(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	redirectFrom := app.cfg.Server.Port

	httpsPort := adoptAuto443Boot(t, app, http.NewServeMux(), certPath, keyPath, 0)
	httpsURL := fmt.Sprintf("https://127.0.0.1:%d/", httpsPort)
	getWithRetry(t, insecureTLSClient(), httpsURL).Body.Close()
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", redirectFrom))

	cfg := staticServerCfg("127.0.0.1", app.cfg.Server.Port, certPath, keyPath)
	cfg.TLS.MinVersion = "1.3"
	gran, err := app.listenerRebind.Apply(cfg)
	if err != nil {
		t.Fatalf("Apply auto-443 min_version raise: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	// HTTPS still serves on the lifeboat port and the redirect keeps working.
	getWithRetry(t, insecureTLSClient(), httpsURL).Body.Close()
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", redirectFrom))

	// A TLS 1.2-max client is now rejected (poll until only the min-1.3 listener answers).
	tls12 := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, MaxVersion: tls.VersionTLS12, //nolint:gosec // self-signed test cert
	}}}
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, getErr := tls12.Get(httpsURL)
		if getErr != nil {
			break
		}
		resp.Body.Close()
		if time.Now().After(deadline) {
			t.Fatal("TLS 1.2 client still accepted after auto-443 min_version raised to 1.3")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Changing server.port on an active auto-443 lifeboat re-plans the redirect in
// place (H4b-3): HTTPS stays on the lifeboat port, the new configured port begins
// redirecting, and the old redirect drains. The HTTPS target (the lifeboat port) is
// unchanged so it is a same-target RebindInPlace.
func TestAdoptListenerController_Auto443ServerPortChangeAppliesLive(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	oldRedirect := app.cfg.Server.Port

	httpsPort := adoptAuto443Boot(t, app, http.NewServeMux(), certPath, keyPath, 0)
	httpsURL := fmt.Sprintf("https://127.0.0.1:%d/", httpsPort)
	getWithRetry(t, insecureTLSClient(), httpsURL).Body.Close()
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", oldRedirect))

	newPort := freePort(t)
	cfg := staticServerCfg("127.0.0.1", newPort, certPath, keyPath)
	gran, err := app.listenerRebind.Apply(cfg)
	if err != nil {
		t.Fatalf("Apply auto-443 server.port change: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	getWithRetry(t, insecureTLSClient(), httpsURL).Body.Close()  // HTTPS unchanged
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", newPort))       // new redirect serves
	waitUnreachable(t, fmt.Sprintf("127.0.0.1:%d", oldRedirect)) // old redirect drained
}

// Adding an http_redirect_port to an active auto-443 lifeboat is applied live
// (H4b-3): HTTPS stays on the lifeboat port, the configured-port redirect keeps
// working, and the new explicit redirect listener begins 301ing.
func TestAdoptListenerController_Auto443AddRedirectAppliesLive(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	redirectFrom := app.cfg.Server.Port

	httpsPort := adoptAuto443Boot(t, app, http.NewServeMux(), certPath, keyPath, 0)
	httpsURL := fmt.Sprintf("https://127.0.0.1:%d/", httpsPort)
	getWithRetry(t, insecureTLSClient(), httpsURL).Body.Close()

	explicitRedirect := freePort(t)
	cfg := staticServerCfg("127.0.0.1", app.cfg.Server.Port, certPath, keyPath)
	cfg.TLS.HTTPRedirectPort = explicitRedirect
	gran, err := app.listenerRebind.Apply(cfg)
	if err != nil {
		t.Fatalf("Apply auto-443 add redirect: %v", err)
	}
	if gran != webapi.ReloadListener {
		t.Errorf("granularity = %v, want listener", gran)
	}

	getWithRetry(t, insecureTLSClient(), httpsURL).Body.Close()
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", redirectFrom))     // configured-port redirect kept
	waitRedirects(t, fmt.Sprintf("127.0.0.1:%d", explicitRedirect)) // new explicit redirect
}

// Re-saving an unchanged auto-443 config is a no-op: the controller key is
// unchanged so nothing is rebound and the save reports applied, not listener.
func TestAdoptListenerController_Auto443UnchangedNoOp(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)
	explicitRedirect := freePort(t)

	httpsPort := adoptAuto443Boot(t, app, http.NewServeMux(), certPath, keyPath, explicitRedirect)
	httpsURL := fmt.Sprintf("https://127.0.0.1:%d/", httpsPort)
	getWithRetry(t, insecureTLSClient(), httpsURL).Body.Close()

	cfg := staticServerCfg("127.0.0.1", app.cfg.Server.Port, certPath, keyPath)
	cfg.TLS.HTTPRedirectPort = explicitRedirect
	gran, err := app.listenerRebind.Apply(cfg)
	if err != nil {
		t.Fatalf("Apply unchanged auto-443: %v", err)
	}
	if gran != webapi.ReloadApplied {
		t.Errorf("granularity = %v, want applied (no-op)", gran)
	}
	getWithRetry(t, insecureTLSClient(), httpsURL).Body.Close()
}

// Leaving the auto-443 topology (mode → off) is refused in place (H4b-3 residual):
// the full topology teardown is restart-required, so Apply returns
// ErrNoListenerRebinder and the old lifeboat keeps serving.
func TestAdoptListenerController_Auto443RefusesModeChange(t *testing.T) {
	app := newTestApp(t)
	app.tlsReload = webapi.NewTLSReloadHolder()
	app.listenerRebind = webapi.NewListenerRebindHolder()

	certPath, keyPath := writeSelfSignedFiles(t)
	app.cfg.Server.ListenAddress = "127.0.0.1"
	app.cfg.Server.Port = freePort(t)

	httpsPort := adoptAuto443Boot(t, app, http.NewServeMux(), certPath, keyPath, 0)

	if _, err := app.listenerRebind.Apply(offServerCfg("127.0.0.1", app.cfg.Server.Port)); !errors.Is(err, webapi.ErrNoListenerRebinder) {
		t.Errorf("Apply auto-443 → off: err = %v, want ErrNoListenerRebinder", err)
	}
	// Old lifeboat untouched — still serving HTTPS.
	getWithRetry(t, insecureTLSClient(), fmt.Sprintf("https://127.0.0.1:%d/", httpsPort)).Body.Close()
}
