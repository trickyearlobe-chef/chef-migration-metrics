// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// RedirectPorts builds one redirect listener per port, folding in
// HTTPRedirectPort, de-duplicating, and skipping any port equal to the HTTPS
// port (which would redirect to itself).
func TestNewListener_MultipleRedirectPorts(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "multi-redirect",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour), "localhost")

	l, err := NewListener(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		ListenerConfig{
			ListenAddress:    "127.0.0.1",
			Port:             443,
			CertPath:         tc.CertPath,
			KeyPath:          tc.KeyPath,
			MinVersion:       "1.2",
			HTTPRedirectPort: 80,
			RedirectPorts:    []int{8080, 80, 443}, // 80 dup, 443 == HTTPS port
		}, nil)
	if err != nil {
		t.Fatalf("NewListener = %v", err)
	}

	got := l.RedirectAddrs()
	want := []string{"127.0.0.1:80", "127.0.0.1:8080"}
	if len(got) != len(want) {
		t.Fatalf("RedirectAddrs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("RedirectAddrs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Pre-bound redirect listeners supplied via SetRedirectListeners are served on
// directly (the live-rebind seam): Serve does not bind the address itself, and the
// redirect 301s to the HTTPS port.
func TestListener_ServesOnPreBoundRedirectListener(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "prebound-redirect",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour), "localhost", "127.0.0.1")

	httpsLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pre-bind https: %v", err)
	}
	httpsPort := httpsLn.Addr().(*net.TCPAddr).Port

	redirectLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pre-bind redirect: %v", err)
	}
	redirectPort := redirectLn.Addr().(*net.TCPAddr).Port

	l, err := NewListener(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "OK") }),
		ListenerConfig{
			ListenAddress:    "127.0.0.1",
			Port:             httpsPort,
			CertPath:         tc.CertPath,
			KeyPath:          tc.KeyPath,
			MinVersion:       "1.2",
			HTTPRedirectPort: redirectPort,
		}, nil)
	if err != nil {
		t.Fatalf("NewListener = %v", err)
	}
	l.SetHTTPSListener(httpsLn)
	l.SetRedirectListeners([]net.Listener{redirectLn})

	errCh := l.Serve()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = l.Shutdown(ctx)
		select {
		case <-errCh:
		default:
		}
	}()
	time.Sleep(200 * time.Millisecond)

	rc := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}
	rresp, err := rc.Get(fmt.Sprintf("http://127.0.0.1:%d/dash", redirectPort))
	if err != nil {
		t.Fatalf("redirect GET: %v", err)
	}
	defer func() { _ = rresp.Body.Close() }()
	if rresp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("redirect status = %d, want 301", rresp.StatusCode)
	}
	wantLoc := fmt.Sprintf("https://127.0.0.1:%d/dash", httpsPort)
	if loc := rresp.Header.Get("Location"); loc != wantLoc {
		t.Errorf("Location = %q, want %q", loc, wantLoc)
	}
}

// A pre-bound HTTPS listener supplied via SetHTTPSListener is served on directly
// (the 443 fallback seam, § 1.5), and a redirect listener points at the
// configured HTTPS port.
func TestListener_ServesOnPreBoundListenerWithRedirect(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "prebound",
		time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour), "localhost", "127.0.0.1")

	// Pre-bind the HTTPS listener ourselves, as the 443 path does.
	httpsLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pre-bind https: %v", err)
	}
	httpsPort := httpsLn.Addr().(*net.TCPAddr).Port
	redirectPort := freePort(t)

	l, err := NewListener(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "OK") }),
		ListenerConfig{
			ListenAddress: "127.0.0.1",
			Port:          httpsPort, // redirect target
			CertPath:      tc.CertPath,
			KeyPath:       tc.KeyPath,
			MinVersion:    "1.2",
			RedirectPorts: []int{redirectPort},
		}, nil)
	if err != nil {
		t.Fatalf("NewListener = %v", err)
	}
	l.SetHTTPSListener(httpsLn)

	errCh := l.Serve()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = l.Shutdown(ctx)
		select {
		case <-errCh:
		default:
		}
	}()
	time.Sleep(200 * time.Millisecond)

	// HTTPS served on the pre-bound listener.
	pool := x509.NewCertPool()
	pool.AddCert(tc.Leaf)
	client := &http.Client{
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}},
		Timeout:   5 * time.Second,
	}
	resp, err := client.Get(fmt.Sprintf("https://localhost:%d/", httpsPort))
	if err != nil {
		t.Fatalf("HTTPS GET: %v", err)
	}
	_ = resp.Body.Close()

	// Redirect listener points at the HTTPS port.
	rc := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}
	rresp, err := rc.Get(fmt.Sprintf("http://127.0.0.1:%d/dash", redirectPort))
	if err != nil {
		t.Fatalf("redirect GET: %v", err)
	}
	defer func() { _ = rresp.Body.Close() }()
	if rresp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("redirect status = %d, want 301", rresp.StatusCode)
	}
	wantLoc := fmt.Sprintf("https://127.0.0.1:%d/dash", httpsPort)
	if loc := rresp.Header.Get("Location"); loc != wantLoc {
		t.Errorf("Location = %q, want %q", loc, wantLoc)
	}
	if !strings.HasPrefix(rresp.Header.Get("Location"), "https://") {
		t.Errorf("redirect not to https: %q", rresp.Header.Get("Location"))
	}
}
