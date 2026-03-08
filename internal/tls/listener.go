// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// ListenerConfig holds the parameters needed to set up TLS and redirect
// listeners. It maps directly to the relevant fields in config.ServerConfig
// and config.TLSConfig, avoiding a direct import of the config package.
type ListenerConfig struct {
	// ListenAddress is the address to bind (e.g. "0.0.0.0").
	ListenAddress string

	// Port is the primary HTTPS port.
	Port int

	// CertPath is the path to the PEM-encoded certificate file (chain).
	CertPath string

	// KeyPath is the path to the PEM-encoded private key file.
	KeyPath string

	// CAPath is the optional path to a PEM-encoded CA bundle for mTLS.
	CAPath string

	// MinVersion is the minimum TLS protocol version ("1.2" or "1.3").
	MinVersion string

	// HTTPRedirectPort, when non-zero, starts a secondary HTTP listener
	// that redirects all requests to HTTPS. Set to 0 to disable.
	HTTPRedirectPort int

	// GracefulShutdownTimeout is the maximum time to wait for in-flight
	// requests to complete during shutdown.
	GracefulShutdownTimeout time.Duration

	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration

	// WriteTimeout is the maximum duration before timing out writes of
	// the response.
	WriteTimeout time.Duration

	// IdleTimeout is the maximum amount of time to wait for the next
	// request when keep-alives are enabled.
	IdleTimeout time.Duration
}

// setDefaults fills in zero-valued fields with sensible defaults.
func (lc *ListenerConfig) setDefaults() {
	if lc.ListenAddress == "" {
		lc.ListenAddress = "0.0.0.0"
	}
	if lc.Port == 0 {
		lc.Port = 8080
	}
	if lc.MinVersion == "" {
		lc.MinVersion = "1.2"
	}
	if lc.GracefulShutdownTimeout <= 0 {
		lc.GracefulShutdownTimeout = 15 * time.Second
	}
	if lc.ReadTimeout <= 0 {
		lc.ReadTimeout = 30 * time.Second
	}
	if lc.WriteTimeout <= 0 {
		lc.WriteTimeout = 60 * time.Second
	}
	if lc.IdleTimeout <= 0 {
		lc.IdleTimeout = 120 * time.Second
	}
}

// Listener manages the HTTPS server and the optional HTTP-to-HTTPS redirect
// server. It owns the CertManager and exposes methods to start and
// gracefully stop both servers.
type Listener struct {
	cfg         ListenerConfig
	certManager *CertManager
	httpsSrv    *http.Server
	redirectSrv *http.Server
	log         LogFunc
}

// NewListener creates a Listener for TLS static mode. The handler is the
// application's primary HTTP handler (e.g. the webapi.Router). The
// CertManager is constructed internally from the ListenerConfig paths.
//
// The caller is responsible for calling Serve() to start listening and
// Shutdown() to stop.
func NewListener(handler http.Handler, cfg ListenerConfig, log LogFunc) (*Listener, error) {
	cfg.setDefaults()

	if log == nil {
		log = func(string, string) {}
	}

	// Build CertManager options.
	var cmOpts []CertManagerOption
	cmOpts = append(cmOpts, WithLogger(log))
	if cfg.CAPath != "" {
		cmOpts = append(cmOpts, WithCAPath(cfg.CAPath))
	}

	cm, err := NewCertManager(cfg.CertPath, cfg.KeyPath, cmOpts...)
	if err != nil {
		return nil, err
	}

	tlsCfg := cm.TLSConfig(cfg.MinVersion)

	addr := fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.Port)

	httpsSrv := &http.Server{
		Addr:         addr,
		Handler:      HSTSMiddleware(handler),
		TLSConfig:    tlsCfg,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}

	l := &Listener{
		cfg:         cfg,
		certManager: cm,
		httpsSrv:    httpsSrv,
		log:         log,
	}

	// Set up the optional HTTP-to-HTTPS redirect listener.
	if cfg.HTTPRedirectPort > 0 {
		redirectAddr := fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.HTTPRedirectPort)
		l.redirectSrv = &http.Server{
			Addr:         redirectAddr,
			Handler:      l.redirectHandler(),
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
			IdleTimeout:  30 * time.Second,
		}
	}

	return l, nil
}

// CertManager returns the underlying CertManager so callers can trigger
// manual reloads (e.g. from a SIGHUP handler) or inspect the current
// certificate.
func (l *Listener) CertManager() *CertManager {
	return l.certManager
}

// Serve starts the HTTPS listener (and the redirect listener if configured)
// in background goroutines. It returns immediately. Use the returned error
// channel to detect fatal listener errors. The channel is closed when
// both servers have stopped.
//
// Callers should also call CertManager().WatchForChanges() if automatic
// filesystem-based certificate reload is desired.
func (l *Listener) Serve() <-chan error {
	errCh := make(chan error, 2)

	go func() {
		l.log("INFO", fmt.Sprintf("HTTPS server listening on %s", l.httpsSrv.Addr))
		// ListenAndServeTLS with empty cert/key paths because the
		// tls.Config.GetCertificate callback handles certificate
		// selection — we don't need the server to load files itself.
		if err := l.httpsSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("HTTPS server: %w", err)
		}
	}()

	if l.redirectSrv != nil {
		go func() {
			l.log("INFO", fmt.Sprintf("HTTP-to-HTTPS redirect listener on %s", l.redirectSrv.Addr))
			if err := l.redirectSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("HTTP redirect server: %w", err)
			}
		}()
	}

	return errCh
}

// Shutdown gracefully shuts down both servers (redirect first, then HTTPS)
// and closes the CertManager. It blocks until both servers have drained
// or the context deadline is exceeded.
func (l *Listener) Shutdown(ctx context.Context) error {
	var errs []error

	// Stop the redirect server first — it's less important and stopping
	// it first avoids sending clients to an HTTPS port that's shutting down.
	if l.redirectSrv != nil {
		l.log("INFO", "shutting down HTTP redirect listener...")
		if err := l.redirectSrv.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("redirect server shutdown: %w", err))
		}
	}

	l.log("INFO", "shutting down HTTPS server...")
	if err := l.httpsSrv.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("HTTPS server shutdown: %w", err))
	}

	// Stop the certificate file watcher.
	if err := l.certManager.Close(); err != nil {
		errs = append(errs, fmt.Errorf("cert manager close: %w", err))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Addr returns the HTTPS server's listen address string (host:port).
func (l *Listener) Addr() string {
	return l.httpsSrv.Addr
}

// RedirectAddr returns the HTTP redirect server's listen address, or an
// empty string if the redirect listener is not configured.
func (l *Listener) RedirectAddr() string {
	if l.redirectSrv == nil {
		return ""
	}
	return l.redirectSrv.Addr
}

// ---------------------------------------------------------------------------
// HTTP-to-HTTPS redirect handler
// ---------------------------------------------------------------------------

// redirectHandler returns an http.Handler that responds to all requests with
// a 301 Moved Permanently redirect to the HTTPS equivalent URL.
//
// Per the specification, the redirect listener serves ONLY redirects — no
// API responses, no static assets, no health checks. This prevents
// accidental exposure of sensitive data over plain HTTP.
func (l *Listener) redirectHandler() http.Handler {
	httpsPort := l.cfg.Port

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Determine the target host for the redirect. Strip any existing
		// port from the Host header and replace it with the HTTPS port.
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		// Only append the port if it's non-standard for HTTPS.
		target := "https://"
		if httpsPort != 443 {
			target += net.JoinHostPort(host, fmt.Sprintf("%d", httpsPort))
		} else {
			target += host
		}
		target += r.URL.RequestURI()

		http.Redirect(w, r, target, http.StatusMovedPermanently)
	})
}

// ---------------------------------------------------------------------------
// HSTS middleware
// ---------------------------------------------------------------------------

// HSTSMiddleware wraps an http.Handler and adds the Strict-Transport-Security
// header to every response when the request was served over TLS. The HSTS
// max-age is set to 1 year (31536000 seconds) with includeSubDomains, which
// is the recommended configuration for production deployments.
//
// The header is NOT added to plain HTTP responses (e.g. the redirect
// listener) because browsers would ignore it on non-secure connections and
// it could cause confusion during debugging.
func HSTSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The TLS field is non-nil when the request was served over TLS.
		// In a reverse-proxy setup, the proxy should set X-Forwarded-Proto
		// but we only set HSTS when we know for certain it's TLS.
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
// Helpers for creating a plain HTTP (mode: off) server — provided here so
// that main.go can use a consistent API for both modes.
// ---------------------------------------------------------------------------

// NewPlainListener creates a standard *http.Server for plain HTTP mode
// (tls.mode: off). This is a convenience constructor so that main.go can
// use a parallel code path for both TLS and non-TLS modes.
func NewPlainListener(handler http.Handler, listenAddr string, port int, opts ...PlainOption) *http.Server {
	if listenAddr == "" {
		listenAddr = "0.0.0.0"
	}
	if port == 0 {
		port = 8080
	}

	cfg := &plainConfig{
		readTimeout:  30 * time.Second,
		writeTimeout: 60 * time.Second,
		idleTimeout:  120 * time.Second,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", listenAddr, port),
		Handler:      handler,
		ReadTimeout:  cfg.readTimeout,
		WriteTimeout: cfg.writeTimeout,
		IdleTimeout:  cfg.idleTimeout,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}
}

// PlainOption configures a plain HTTP server.
type PlainOption func(*plainConfig)

type plainConfig struct {
	readTimeout  time.Duration
	writeTimeout time.Duration
	idleTimeout  time.Duration
}

// WithPlainReadTimeout sets the read timeout for the plain HTTP server.
func WithPlainReadTimeout(d time.Duration) PlainOption {
	return func(c *plainConfig) { c.readTimeout = d }
}

// WithPlainWriteTimeout sets the write timeout for the plain HTTP server.
func WithPlainWriteTimeout(d time.Duration) PlainOption {
	return func(c *plainConfig) { c.writeTimeout = d }
}

// WithPlainIdleTimeout sets the idle timeout for the plain HTTP server.
func WithPlainIdleTimeout(d time.Duration) PlainOption {
	return func(c *plainConfig) { c.idleTimeout = d }
}

// ---------------------------------------------------------------------------
// TLS info for startup logging
// ---------------------------------------------------------------------------

// CertSummary returns a human-readable summary of the currently loaded
// certificate, suitable for startup log messages.
func (l *Listener) CertSummary() string {
	leaf := l.certManager.LeafCert()
	if leaf == nil {
		return "no certificate loaded"
	}

	dnsNames := ""
	if len(leaf.DNSNames) > 0 {
		dnsNames = fmt.Sprintf(", dns_names=[%s]", joinMax(leaf.DNSNames, 5))
	}

	return fmt.Sprintf("subject=%q, issuer=%q, not_before=%s, not_after=%s%s",
		leaf.Subject.CommonName,
		leaf.Issuer.CommonName,
		leaf.NotBefore.Format(time.RFC3339),
		leaf.NotAfter.Format(time.RFC3339),
		dnsNames,
	)
}

// joinMax joins up to max strings with ", " and appends "..." if truncated.
func joinMax(ss []string, max int) string {
	if len(ss) <= max {
		return strings.Join(ss, ", ")
	}
	return strings.Join(ss[:max], ", ") + ", ..."
}

// MinTLSVersionString returns a human-readable string for the configured
// minimum TLS version.
func (l *Listener) MinTLSVersionString() string {
	switch l.httpsSrv.TLSConfig.MinVersion {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	default:
		return fmt.Sprintf("unknown (0x%04x)", l.httpsSrv.TLSConfig.MinVersion)
	}
}

// IsMTLSEnabled returns true if mutual TLS (client certificate verification)
// is enabled.
func (l *Listener) IsMTLSEnabled() bool {
	return l.httpsSrv.TLSConfig != nil && l.httpsSrv.TLSConfig.ClientAuth == tls.RequireAndVerifyClientCert
}
