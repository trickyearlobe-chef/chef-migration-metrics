// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Tests: HSTSMiddleware
// ---------------------------------------------------------------------------

func TestHSTSMiddleware_AddsHeaderOnTLS(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := HSTSMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Simulate TLS by setting the TLS field.
	req.TLS = &tls.ConnectionState{}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	hsts := rr.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Fatal("expected Strict-Transport-Security header, got empty")
	}
	if !strings.Contains(hsts, "max-age=31536000") {
		t.Errorf("HSTS header missing max-age=31536000: %q", hsts)
	}
	if !strings.Contains(hsts, "includeSubDomains") {
		t.Errorf("HSTS header missing includeSubDomains: %q", hsts)
	}
}

func TestHSTSMiddleware_AddsHeaderOnXForwardedProto(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := HSTSMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	hsts := rr.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Fatal("expected HSTS header with X-Forwarded-Proto: https")
	}
}

func TestHSTSMiddleware_NoHeaderOnPlainHTTP(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := HSTSMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// No TLS, no X-Forwarded-Proto.

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	hsts := rr.Header().Get("Strict-Transport-Security")
	if hsts != "" {
		t.Errorf("expected no HSTS header on plain HTTP, got %q", hsts)
	}
}

func TestHSTSMiddleware_PassesThroughToHandler(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "hello")
		w.WriteHeader(http.StatusTeapot)
		fmt.Fprint(w, "body content")
	})

	handler := HSTSMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.TLS = &tls.ConnectionState{}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusTeapot)
	}
	if rr.Header().Get("X-Custom") != "hello" {
		t.Error("inner handler's custom header not passed through")
	}
	if rr.Body.String() != "body content" {
		t.Errorf("body = %q, want %q", rr.Body.String(), "body content")
	}
}

func TestHSTSMiddleware_XForwardedProtoCaseInsensitive(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := HSTSMiddleware(inner)

	for _, proto := range []string{"HTTPS", "Https", "https", "hTtPs"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("X-Forwarded-Proto", proto)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		hsts := rr.Header().Get("Strict-Transport-Security")
		if hsts == "" {
			t.Errorf("expected HSTS header for X-Forwarded-Proto=%q, got empty", proto)
		}
	}
}

func TestHSTSMiddleware_NoHeaderOnHTTPXForwardedProto(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := HSTSMiddleware(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "http")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	hsts := rr.Header().Get("Strict-Transport-Security")
	if hsts != "" {
		t.Errorf("expected no HSTS header for X-Forwarded-Proto=http, got %q", hsts)
	}
}

// ---------------------------------------------------------------------------
// Tests: redirectHandler
// ---------------------------------------------------------------------------

func TestRedirectHandler_StandardPort(t *testing.T) {
	l := &Listener{
		cfg: ListenerConfig{
			Port: 443,
		},
	}

	handler := l.redirectHandler()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/health", nil)
	req.Host = "example.com"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusMovedPermanently)
	}

	loc := rr.Header().Get("Location")
	want := "https://example.com/api/v1/health"
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

func TestRedirectHandler_NonStandardPort(t *testing.T) {
	l := &Listener{
		cfg: ListenerConfig{
			Port: 8443,
		},
	}

	handler := l.redirectHandler()
	req := httptest.NewRequest(http.MethodGet, "http://example.com:8080/dashboard?org=prod", nil)
	req.Host = "example.com:8080"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusMovedPermanently)
	}

	loc := rr.Header().Get("Location")
	want := "https://example.com:8443/dashboard?org=prod"
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

func TestRedirectHandler_PreservesQueryString(t *testing.T) {
	l := &Listener{
		cfg: ListenerConfig{
			Port: 443,
		},
	}

	handler := l.redirectHandler()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/search?q=test&page=2", nil)
	req.Host = "example.com"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	want := "https://example.com/search?q=test&page=2"
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

func TestRedirectHandler_PreservesPath(t *testing.T) {
	l := &Listener{
		cfg: ListenerConfig{
			Port: 443,
		},
	}

	handler := l.redirectHandler()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/nodes/org1/node1", nil)
	req.Host = "example.com"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	want := "https://example.com/api/v1/nodes/org1/node1"
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

func TestRedirectHandler_RootPath(t *testing.T) {
	l := &Listener{
		cfg: ListenerConfig{
			Port: 443,
		},
	}

	handler := l.redirectHandler()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Host = "example.com"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	want := "https://example.com/"
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

func TestRedirectHandler_HostWithExistingPort(t *testing.T) {
	l := &Listener{
		cfg: ListenerConfig{
			Port: 8443,
		},
	}

	handler := l.redirectHandler()
	req := httptest.NewRequest(http.MethodGet, "http://myhost.local:80/path", nil)
	req.Host = "myhost.local:80"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	loc := rr.Header().Get("Location")
	want := "https://myhost.local:8443/path"
	if loc != want {
		t.Errorf("Location = %q, want %q", loc, want)
	}
}

func TestRedirectHandler_PostMethod(t *testing.T) {
	l := &Listener{
		cfg: ListenerConfig{
			Port: 443,
		},
	}

	handler := l.redirectHandler()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/v1/data", nil)
	req.Host = "example.com"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Even POST requests get redirected — per spec, the redirect listener
	// serves ONLY redirects.
	if rr.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusMovedPermanently)
	}
}

// ---------------------------------------------------------------------------
// Tests: ListenerConfig.setDefaults
// ---------------------------------------------------------------------------

func TestListenerConfig_SetDefaults_ZeroValues(t *testing.T) {
	cfg := ListenerConfig{}
	cfg.setDefaults()

	if cfg.ListenAddress != "0.0.0.0" {
		t.Errorf("ListenAddress = %q, want %q", cfg.ListenAddress, "0.0.0.0")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want %d", cfg.Port, 8080)
	}
	if cfg.MinVersion != "1.2" {
		t.Errorf("MinVersion = %q, want %q", cfg.MinVersion, "1.2")
	}
	if cfg.GracefulShutdownTimeout != 15*time.Second {
		t.Errorf("GracefulShutdownTimeout = %v, want %v", cfg.GracefulShutdownTimeout, 15*time.Second)
	}
	if cfg.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 30*time.Second)
	}
	if cfg.WriteTimeout != 60*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 60*time.Second)
	}
	if cfg.IdleTimeout != 120*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", cfg.IdleTimeout, 120*time.Second)
	}
}

func TestListenerConfig_SetDefaults_PreservesExplicitValues(t *testing.T) {
	cfg := ListenerConfig{
		ListenAddress:           "127.0.0.1",
		Port:                    443,
		MinVersion:              "1.3",
		GracefulShutdownTimeout: 5 * time.Second,
		ReadTimeout:             10 * time.Second,
		WriteTimeout:            20 * time.Second,
		IdleTimeout:             30 * time.Second,
	}
	cfg.setDefaults()

	if cfg.ListenAddress != "127.0.0.1" {
		t.Errorf("ListenAddress = %q, want %q", cfg.ListenAddress, "127.0.0.1")
	}
	if cfg.Port != 443 {
		t.Errorf("Port = %d, want %d", cfg.Port, 443)
	}
	if cfg.MinVersion != "1.3" {
		t.Errorf("MinVersion = %q, want %q", cfg.MinVersion, "1.3")
	}
	if cfg.GracefulShutdownTimeout != 5*time.Second {
		t.Errorf("GracefulShutdownTimeout = %v, want %v", cfg.GracefulShutdownTimeout, 5*time.Second)
	}
	if cfg.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 10*time.Second)
	}
	if cfg.WriteTimeout != 20*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 20*time.Second)
	}
	if cfg.IdleTimeout != 30*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", cfg.IdleTimeout, 30*time.Second)
	}
}

// ---------------------------------------------------------------------------
// Tests: NewListener
// ---------------------------------------------------------------------------

func TestNewListener_MissingCertPath(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	_, err := NewListener(handler, ListenerConfig{
		CertPath: "",
		KeyPath:  "/some/key.pem",
	}, nil)
	if err == nil {
		t.Fatal("expected error for missing cert_path, got nil")
	}
}

func TestNewListener_MissingKeyPath(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	_, err := NewListener(handler, ListenerConfig{
		CertPath: "/some/cert.pem",
		KeyPath:  "",
	}, nil)
	if err == nil {
		t.Fatal("expected error for missing key_path, got nil")
	}
}

func TestNewListener_InvalidCertFiles(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	_, err := NewListener(handler, ListenerConfig{
		CertPath: "/nonexistent/cert.pem",
		KeyPath:  "/nonexistent/key.pem",
	}, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent cert files, got nil")
	}
}

func TestNewListener_ValidCert(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "listener-test",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost",
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	l, err := NewListener(handler, ListenerConfig{
		CertPath:   tc.CertPath,
		KeyPath:    tc.KeyPath,
		MinVersion: "1.2",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if l.CertManager() == nil {
		t.Error("CertManager() returned nil")
	}
	if l.Addr() == "" {
		t.Error("Addr() returned empty string")
	}
}

func TestNewListener_WithRedirectPort(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "redirect-test",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost",
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	l, err := NewListener(handler, ListenerConfig{
		CertPath:         tc.CertPath,
		KeyPath:          tc.KeyPath,
		HTTPRedirectPort: 8080,
		Port:             8443,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if l.RedirectAddr() == "" {
		t.Error("RedirectAddr() returned empty string when redirect port is set")
	}
	if !strings.Contains(l.RedirectAddr(), "8080") {
		t.Errorf("RedirectAddr() = %q, expected it to contain 8080", l.RedirectAddr())
	}
}

func TestNewListener_NoRedirectPort(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "no-redirect",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost",
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	l, err := NewListener(handler, ListenerConfig{
		CertPath:         tc.CertPath,
		KeyPath:          tc.KeyPath,
		HTTPRedirectPort: 0,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if l.RedirectAddr() != "" {
		t.Errorf("RedirectAddr() = %q, want empty when redirect port is 0", l.RedirectAddr())
	}
}

func TestNewListener_WithCAPath_mTLS(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "mtls-listener",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost",
	)
	caPath, _, _ := generateTestCA(t, dir, "mtls-ca")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	l, err := NewListener(handler, ListenerConfig{
		CertPath: tc.CertPath,
		KeyPath:  tc.KeyPath,
		CAPath:   caPath,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !l.IsMTLSEnabled() {
		t.Error("IsMTLSEnabled() = false, want true")
	}
}

func TestNewListener_WithoutCAPath_NoMTLS(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "no-mtls-listener",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost",
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	l, err := NewListener(handler, ListenerConfig{
		CertPath: tc.CertPath,
		KeyPath:  tc.KeyPath,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if l.IsMTLSEnabled() {
		t.Error("IsMTLSEnabled() = true, want false when no CA path set")
	}
}

func TestNewListener_NilLogFunc(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "nil-log",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost",
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// Should not panic with nil log func.
	l, err := NewListener(handler, ListenerConfig{
		CertPath: tc.CertPath,
		KeyPath:  tc.KeyPath,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = l
}

// ---------------------------------------------------------------------------
// Tests: CertSummary
// ---------------------------------------------------------------------------

func TestCertSummary_IncludesSubjectAndIssuer(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "summary-test",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"a.example.com", "b.example.com",
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	l, err := NewListener(handler, ListenerConfig{
		CertPath: tc.CertPath,
		KeyPath:  tc.KeyPath,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summary := l.CertSummary()
	if !strings.Contains(summary, "summary-test") {
		t.Errorf("CertSummary() = %q, expected to contain subject CN", summary)
	}
	if !strings.Contains(summary, "not_before=") {
		t.Errorf("CertSummary() = %q, expected to contain not_before", summary)
	}
	if !strings.Contains(summary, "not_after=") {
		t.Errorf("CertSummary() = %q, expected to contain not_after", summary)
	}
	if !strings.Contains(summary, "a.example.com") {
		t.Errorf("CertSummary() = %q, expected to contain DNS names", summary)
	}
}

func TestCertSummary_NoDNSNames(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "no-dns-summary",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	l, err := NewListener(handler, ListenerConfig{
		CertPath: tc.CertPath,
		KeyPath:  tc.KeyPath,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	summary := l.CertSummary()
	if strings.Contains(summary, "dns_names") {
		t.Errorf("CertSummary() = %q, should not contain dns_names when there are none", summary)
	}
}

// ---------------------------------------------------------------------------
// Tests: MinTLSVersionString
// ---------------------------------------------------------------------------

func TestMinTLSVersionString_TLS12(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "ver12",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	l, err := NewListener(handler, ListenerConfig{
		CertPath:   tc.CertPath,
		KeyPath:    tc.KeyPath,
		MinVersion: "1.2",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v := l.MinTLSVersionString(); v != "TLS 1.2" {
		t.Errorf("MinTLSVersionString() = %q, want %q", v, "TLS 1.2")
	}
}

func TestMinTLSVersionString_TLS13(t *testing.T) {
	dir := t.TempDir()
	tc := generateTestCert(t, dir, "ver13",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	l, err := NewListener(handler, ListenerConfig{
		CertPath:   tc.CertPath,
		KeyPath:    tc.KeyPath,
		MinVersion: "1.3",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if v := l.MinTLSVersionString(); v != "TLS 1.3" {
		t.Errorf("MinTLSVersionString() = %q, want %q", v, "TLS 1.3")
	}
}

// ---------------------------------------------------------------------------
// Tests: NewPlainListener
// ---------------------------------------------------------------------------

func TestNewPlainListener_Defaults(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	srv := NewPlainListener(handler, "", 0)
	if srv.Addr != "0.0.0.0:8080" {
		t.Errorf("Addr = %q, want %q", srv.Addr, "0.0.0.0:8080")
	}
	if srv.TLSConfig != nil {
		t.Error("expected nil TLSConfig for plain listener")
	}
}

func TestNewPlainListener_ExplicitValues(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	srv := NewPlainListener(handler, "127.0.0.1", 9090,
		WithPlainReadTimeout(5*time.Second),
		WithPlainWriteTimeout(10*time.Second),
		WithPlainIdleTimeout(15*time.Second),
	)
	if srv.Addr != "127.0.0.1:9090" {
		t.Errorf("Addr = %q, want %q", srv.Addr, "127.0.0.1:9090")
	}
	if srv.ReadTimeout != 5*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", srv.ReadTimeout, 5*time.Second)
	}
	if srv.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", srv.WriteTimeout, 10*time.Second)
	}
	if srv.IdleTimeout != 15*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", srv.IdleTimeout, 15*time.Second)
	}
}

func TestNewPlainListener_HandlerIsWired(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	srv := NewPlainListener(handler, "", 0)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)

	if !called {
		t.Error("handler was not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

// ---------------------------------------------------------------------------
// Tests: joinMax helper
// ---------------------------------------------------------------------------

func TestJoinMax_UnderLimit(t *testing.T) {
	result := joinMax([]string{"a", "b", "c"}, 5)
	if result != "a, b, c" {
		t.Errorf("joinMax = %q, want %q", result, "a, b, c")
	}
}

func TestJoinMax_AtLimit(t *testing.T) {
	result := joinMax([]string{"a", "b", "c"}, 3)
	if result != "a, b, c" {
		t.Errorf("joinMax = %q, want %q", result, "a, b, c")
	}
}

func TestJoinMax_OverLimit(t *testing.T) {
	result := joinMax([]string{"a", "b", "c", "d", "e"}, 3)
	if result != "a, b, c, ..." {
		t.Errorf("joinMax = %q, want %q", result, "a, b, c, ...")
	}
}

func TestJoinMax_Empty(t *testing.T) {
	result := joinMax([]string{}, 5)
	if result != "" {
		t.Errorf("joinMax = %q, want empty string", result)
	}
}

func TestJoinMax_Single(t *testing.T) {
	result := joinMax([]string{"only"}, 5)
	if result != "only" {
		t.Errorf("joinMax = %q, want %q", result, "only")
	}
}

// ---------------------------------------------------------------------------
// Tests: Serve and Shutdown (integration — actually binds to ports)
// ---------------------------------------------------------------------------

func TestListener_ServeAndShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	tc := generateTestCert(t, dir, "serve-test",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost", "127.0.0.1",
	)

	// Find free ports.
	httpsPort := freePort(t)
	redirectPort := freePort(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	logFn, logs := collectLogs()

	l, err := NewListener(handler, ListenerConfig{
		ListenAddress:    "127.0.0.1",
		Port:             httpsPort,
		CertPath:         tc.CertPath,
		KeyPath:          tc.KeyPath,
		MinVersion:       "1.2",
		HTTPRedirectPort: redirectPort,
	}, logFn)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}

	errCh := l.Serve()

	// Give the servers a moment to start.
	time.Sleep(200 * time.Millisecond)

	// Load the self-signed cert into a CA pool for the client.
	certPool := x509.NewCertPool()
	certPool.AddCert(tc.Leaf)

	// Test HTTPS endpoint.
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certPool,
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("https://localhost:%d/", httpsPort))
	if err != nil {
		t.Fatalf("HTTPS GET error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK" {
		t.Errorf("HTTPS body = %q, want %q", string(body), "OK")
	}

	// Verify HSTS header is present.
	hsts := resp.Header.Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("expected HSTS header on HTTPS response")
	}

	// Test HTTP redirect endpoint.
	redirectClient := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Don't follow redirects — we want to inspect the 301.
			return http.ErrUseLastResponse
		},
		Timeout: 5 * time.Second,
	}

	redirectResp, err := redirectClient.Get(fmt.Sprintf("http://127.0.0.1:%d/api/v1/health", redirectPort))
	if err != nil {
		t.Fatalf("HTTP redirect GET error: %v", err)
	}
	defer redirectResp.Body.Close()

	if redirectResp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("redirect status = %d, want %d", redirectResp.StatusCode, http.StatusMovedPermanently)
	}

	loc := redirectResp.Header.Get("Location")
	expectedLoc := fmt.Sprintf("https://127.0.0.1:%d/api/v1/health", httpsPort)
	if loc != expectedLoc {
		t.Errorf("Location = %q, want %q", loc, expectedLoc)
	}

	// Graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := l.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown error: %v", err)
	}

	// Check for any server errors.
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("server error: %v", err)
		}
	default:
	}

	if !logs.Contains("HTTPS server listening") {
		t.Errorf("expected 'HTTPS server listening' log, got: %v", logs.Snapshot())
	}
	if !logs.Contains("redirect listener") {
		t.Errorf("expected redirect listener log, got: %v", logs.Snapshot())
	}
}

func TestListener_ServeWithoutRedirect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	tc := generateTestCert(t, dir, "no-redirect-serve",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost",
	)

	httpsPort := freePort(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "secure")
	})

	l, err := NewListener(handler, ListenerConfig{
		ListenAddress: "127.0.0.1",
		Port:          httpsPort,
		CertPath:      tc.CertPath,
		KeyPath:       tc.KeyPath,
		MinVersion:    "1.2",
	}, nil)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}

	_ = l.Serve()
	time.Sleep(200 * time.Millisecond)

	certPool := x509.NewCertPool()
	certPool.AddCert(tc.Leaf)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: certPool,
			},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(fmt.Sprintf("https://localhost:%d/test", httpsPort))
	if err != nil {
		t.Fatalf("HTTPS GET error: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "secure" {
		t.Errorf("body = %q, want %q", string(body), "secure")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l.Shutdown(ctx)
}

func TestListener_ShutdownTimesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	tc := generateTestCert(t, dir, "shutdown-timeout",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost",
	)

	httpsPort := freePort(t)

	// Handler that blocks forever.
	started := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		// Block forever to simulate a slow request.
		select {}
	})

	l, err := NewListener(handler, ListenerConfig{
		ListenAddress: "127.0.0.1",
		Port:          httpsPort,
		CertPath:      tc.CertPath,
		KeyPath:       tc.KeyPath,
		MinVersion:    "1.2",
	}, nil)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}

	_ = l.Serve()
	time.Sleep(200 * time.Millisecond)

	certPool := x509.NewCertPool()
	certPool.AddCert(tc.Leaf)

	// Start a request that will block.
	go func() {
		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: certPool,
				},
			},
			Timeout: 30 * time.Second,
		}
		// We don't care about the response — the request will be killed.
		client.Get(fmt.Sprintf("https://localhost:%d/block", httpsPort))
	}()

	// Wait for the handler to start processing.
	<-started

	// Shutdown with a very short timeout — should return an error.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	err = l.Shutdown(ctx)
	if err == nil {
		t.Error("expected timeout error from Shutdown, got nil")
	}
}

// ---------------------------------------------------------------------------
// Tests: Serve picks up cert reload (integration)
// ---------------------------------------------------------------------------

func TestListener_ServesReloadedCert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	tc := generateTestCert(t, dir, "reload-serve-v1",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(24*time.Hour),
		"localhost",
	)

	httpsPort := freePort(t)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	l, err := NewListener(handler, ListenerConfig{
		ListenAddress: "127.0.0.1",
		Port:          httpsPort,
		CertPath:      tc.CertPath,
		KeyPath:       tc.KeyPath,
		MinVersion:    "1.2",
	}, nil)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}

	_ = l.Serve()
	time.Sleep(200 * time.Millisecond)

	// Build a client that trusts the first cert.
	certPool := x509.NewCertPool()
	certPool.AddCert(tc.Leaf)

	makeClient := func(pool *x509.CertPool) *http.Client {
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					RootCAs: pool,
				},
			},
			Timeout: 5 * time.Second,
		}
	}

	// Verify initial cert works.
	resp, err := makeClient(certPool).Get(fmt.Sprintf("https://localhost:%d/", httpsPort))
	if err != nil {
		t.Fatalf("initial HTTPS GET error: %v", err)
	}
	resp.Body.Close()

	// Generate a new cert and overwrite the files.
	tc2 := generateTestCert(t, dir, "reload-serve-v2",
		time.Now().Add(-1*time.Hour),
		time.Now().Add(48*time.Hour),
		"localhost",
	)
	copyFile(t, tc2.CertPath, tc.CertPath)
	copyFile(t, tc2.KeyPath, tc.KeyPath)

	// Trigger reload.
	if err := l.CertManager().Reload(); err != nil {
		t.Fatalf("Reload() error: %v", err)
	}

	// Build a client that trusts the second cert.
	certPool2 := x509.NewCertPool()
	certPool2.AddCert(tc2.Leaf)

	// New connections should see the new cert.
	resp2, err := makeClient(certPool2).Get(fmt.Sprintf("https://localhost:%d/", httpsPort))
	if err != nil {
		t.Fatalf("HTTPS GET after reload error: %v", err)
	}
	resp2.Body.Close()

	if resp2.TLS == nil {
		t.Fatal("TLS connection state is nil after reload")
	}
	if len(resp2.TLS.PeerCertificates) == 0 {
		t.Fatal("no peer certificates after reload")
	}
	servedCN := resp2.TLS.PeerCertificates[0].Subject.CommonName
	if servedCN != "reload-serve-v2" {
		t.Errorf("served cert CN = %q, want %q", servedCN, "reload-serve-v2")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l.Shutdown(ctx)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// freePort returns a port that is currently available for binding.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("could not find free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}
