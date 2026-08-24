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
	"sync"
	"time"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/acme"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/serverctl"
	apptls "github.com/trickyearlobe-chef/chef-migration-metrics/internal/tls"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/webapi"
)

// acmeRuntime bundles the three long-lived resources of a running acme topology
// — the HTTPS listener, the port-80 http-01 challenge/redirect server, and the
// background renewer — so they can be torn down as a unit. It is wrapped in a
// single serverctl.Instance (H4c-1) whose Shutdown is rt.shutdown, letting the
// controller own and (H4c-2) swap the whole topology like any other listener.
type acmeRuntime struct {
	listener      interface{ Shutdown(context.Context) error }
	challengeSrv  *http.Server
	renewerCancel context.CancelFunc
}

// shutdown cancels the renewer first (so no new issuance starts while draining),
// then stops the challenge/redirect server, then drains the HTTPS listener. Each
// resource is nil-guarded (dns-01 has no challenge server; a degraded bootstrap
// still has a renewer). A challenge shutdown error is non-fatal; the listener
// drain error is returned.
func (rt *acmeRuntime) shutdown(ctx context.Context) error {
	if rt.renewerCancel != nil {
		rt.renewerCancel()
	}
	if rt.challengeSrv != nil {
		_ = rt.challengeSrv.Shutdown(ctx)
	}
	if rt.listener != nil {
		return rt.listener.Shutdown(ctx)
	}
	return nil
}

// setupACME wires TLS mode: acme. It always brings the server up on HTTPS — the
// stored issued certificate if one exists in the DB, otherwise an ephemeral
// self-signed certificate (degraded) — and runs a background renewer that
// obtains/renews a real certificate and promotes it in place with no restart.
// It never returns a fatal error for an unobtainable
// certificate (ToS not accepted, order failure, unreachable CA): such states
// fail open so the operator can always reach the UI to fix the configuration.
//
// store is the encrypted config store backing ACME state (account key, issued
// cert/key); *configstore.Store satisfies acme.SecretStore.
func (app *serverApp) setupACME(handler http.Handler, store acme.SecretStore, shutdownTimeout time.Duration) (serverResult, error) {
	app.startup.Info("TLS mode: acme (automatic certificate management)")
	acfg := app.cfg.Server.TLS.ACME
	ctx := context.Background()

	if store == nil {
		// Defensive: setupSecrets makes the store mandatory, so this should not
		// happen. Fail open rather than panic.
		return app.degradeToSelfSigned(handler, acfg.Domains,
			errors.New("acme mode requires the encrypted config store (set CMM_CREDENTIAL_ENCRYPTION_KEY)")), nil
	}

	if isACMEStaging(acfg.CAURL) {
		app.startup.Warn(fmt.Sprintf(
			"ACME ca_url is a staging endpoint (%s) — issued certificates will NOT be trusted by browsers",
			acfg.CAURL))
	}

	engineCfg := acme.Config{
		Domains:         acfg.Domains,
		Email:           acfg.Email,
		CAURL:           acfg.CAURL,
		Challenge:       acfg.Challenge,
		RenewBeforeDays: acfg.RenewBeforeDays,
		AgreeToTOS:      acfg.AgreeToTOS,
	}

	// Build the challenge solver and, for http-01, the port-80 challenge/redirect
	// server. http-01 publishes the proof over HTTP; dns-01 publishes a TXT record
	// via the Route 53 solver (no port-80 listener). An unsupported or unwireable
	// challenge fails open to self-signed.
	var solver acme.Solver
	var challengeSrv *http.Server
	var challengeHandler http.Handler
	var renewerOpts []acme.RenewerOption
	switch acfg.Challenge {
	case "http-01":
		if app.cfg.Server.TLS.HTTPRedirectPort == 0 {
			app.startup.Error("ACME challenge http-01 requires http_redirect_port (set http_redirect_port: 80) — no certificate can be obtained")
			return app.degradeToSelfSigned(handler, acfg.Domains,
				errors.New("acme http-01 requires http_redirect_port")), nil
		}
		httpSolver := acme.NewHTTP01Solver(app.tlsLog)
		solver = httpSolver
		// The challenge/redirect server is built once the effective HTTPS port is
		// known (after the auto-443 decision below), so its redirect targets 443
		// when automatic HTTPS on 443 is in effect.
		challengeHandler = httpSolver.Handler()
	case "dns-01":
		if acfg.DNSProvider != "route53" {
			app.startup.Error(fmt.Sprintf(
				"ACME challenge dns-01 supports only dns_provider: route53 (got %q) — no certificate can be obtained", acfg.DNSProvider))
			return app.degradeToSelfSigned(handler, acfg.Domains,
				fmt.Errorf("unsupported acme dns_provider %q", acfg.DNSProvider)), nil
		}
		r53, derr := acme.NewRoute53Solver(ctx, store, acfg.DNSProviderConfig, app.tlsLog)
		if derr != nil {
			app.startup.Error(fmt.Sprintf(
				"ACME dns-01 Route 53 solver unavailable: %v — no certificate can be obtained", derr))
			return app.degradeToSelfSigned(handler, acfg.Domains, derr), nil
		}
		solver = r53

		// Optional hostname self-registration: publish an A record per domain so
		// the FQDN resolves to this host, reusing the Route 53 client/zone/creds.
		// It runs at the start of every renewal cycle (and
		// so once at startup) and is fail-soft — orthogonal to certificate
		// issuance, never blocks renewal or fail-open.
		if acfg.RegisterHostname {
			reg := r53.NewHostnameRegistrar(acfg.HostnameTTL, acfg.HostnameIP, acfg.HostnameInterface, app.tlsLog)
			domains := acfg.Domains
			renewerOpts = append(renewerOpts, acme.WithHostnameRegistrar(func(ctx context.Context) error {
				// The registrar logs its own ERROR on the tls scope; the returned
				// error is recorded in operator status (§ 3.14) by the renewer but
				// never gates renewal.
				return reg.Register(ctx, domains)
			}))
			app.startup.Info(fmt.Sprintf(
				"ACME hostname self-registration enabled (A record per domain, ttl %ds)", acfg.HostnameTTL))
		}
	default:
		app.startup.Error(fmt.Sprintf("ACME challenge %q is not supported", acfg.Challenge))
		return app.degradeToSelfSigned(handler, acfg.Domains,
			fmt.Errorf("unsupported acme challenge %q", acfg.Challenge)), nil
	}

	storage := acme.NewStorage(store)

	// Always come up on HTTPS: serve the stored issued certificate if present,
	// otherwise an ephemeral self-signed certificate (degraded) while the renewer
	// obtains a real one.
	certPEM, keyPEM, certErr := storage.Certificate(ctx)
	degraded := certErr != nil
	if degraded {
		var genErr error
		certPEM, keyPEM, genErr = apptls.GenerateSelfSigned(acfg.Domains)
		if genErr != nil {
			// Even self-signed generation failed — last-resort plain HTTP.
			return app.degradeToPlainHTTP(handler, fmt.Errorf("acme self-signed bootstrap: %w", genErr)), nil
		}
	}

	// Automatic HTTPS on 443 applies only when a real certificate
	// is loaded (healthy). The degraded self-signed bootstrap holds server.port,
	// and is not moved to 443 at runtime if issuance later succeeds in place. In
	// http-01 the port-80 challenge server is the redirect listener; pass 0 so
	// the HTTPS listener does not also bind that port — only the server.port → 443
	// redirect (when healthy and 443 is bound) is added here.
	effHTTPSPort := app.cfg.Server.Port
	if effHTTPSPort == 0 {
		effHTTPSPort = 8080
	}
	var redirectPorts []int
	var https443Ln net.Listener
	if !degraded {
		effHTTPSPort, redirectPorts, https443Ln = app.planAutoHTTPS(0)
	}

	// Build the http-01 challenge/redirect server now the effective HTTPS port is
	// known: ordinary traffic 301s to that port (443 when auto-443 is in effect,
	// else server.port); the challenge path keeps priority.
	if challengeHandler != nil {
		challengeSrv = apptls.NewChallengeRedirectServer(
			app.cfg.Server.ListenAddress,
			app.cfg.Server.TLS.HTTPRedirectPort,
			effHTTPSPort,
			challengeHandler,
		)
	}

	lcfg := apptls.ListenerConfig{
		ListenAddress: app.cfg.Server.ListenAddress,
		Port:          effHTTPSPort,
		CertSource:    "db",
		CertPEM:       certPEM,
		KeyPEM:        keyPEM,
		MinVersion:    app.cfg.Server.TLS.MinVersion,
		// Port 80 is owned by the challenge/redirect server for the whole process
		// lifetime, so the HTTPS listener must not also bind it; the only redirect
		// it runs is the auto server.port → 443 one (§ 1.5).
		HTTPRedirectPort:        0,
		RedirectPorts:           redirectPorts,
		GracefulShutdownTimeout: shutdownTimeout,
		TrustedProxy:            app.cfg.Server.TrustedProxy,
		HSTSEnabled:             app.hstsEnabledFn(),
	}
	listener, lerr := apptls.NewListener(handler, lcfg, app.tlsLog)
	if lerr != nil {
		if https443Ln != nil {
			_ = https443Ln.Close()
		}
		return app.degradeToSelfSigned(handler, acfg.Domains, lerr), nil
	}
	if https443Ln != nil {
		listener.SetHTTPSListener(https443Ln)
	}

	if degraded {
		reason := fmt.Sprintf(
			"ACME: no certificate yet for %v — serving a self-signed certificate over HTTPS (degraded) while issuance is attempted",
			acfg.Domains)
		app.startup.Error(reason)
		if app.tlsStatus != nil {
			app.tlsStatus.SetDegradedKind(webapi.DegradedKindSelfSigned, reason)
		}
	} else {
		app.startup.Info(fmt.Sprintf("ACME: serving stored certificate: %s", listener.CertSummary()))
	}

	// Register the listener's CertManager so the renewer can promote a freshly
	// issued certificate in place — SetOnReload (wired in setupAndServeHTTP)
	// clears the degraded state and resumes HSTS without a restart.
	if app.tlsReload != nil {
		app.tlsReload.Set(listener.CertManager())
	}

	// The renewer obtains/renews in the background. promotingIssuer wraps the
	// manager so a successful Obtain swaps the live certificate in place.
	manager := acme.NewManager(storage, solver, engineCfg, acme.WithLogger(app.tlsLog))
	issuer := &promotingIssuer{inner: manager, reload: app.tlsReload, log: app.tlsLog}
	renewer := acme.NewRenewer(storage, issuer, engineCfg, app.tlsLog, renewerOpts...)
	// Bind the admin re-register trigger to this renewer so an ACME config save
	// re-asserts hostname registration / issuance immediately.
	if app.acmeTrigger != nil {
		app.acmeTrigger.Set(renewer.Trigger)
	}
	renewCtx, renewCancel := context.WithCancel(context.Background())
	go renewer.Run(renewCtx)
	app.startup.Info(fmt.Sprintf(
		"ACME renewal loop started (challenge: %s, renew_before_days: %d)",
		acfg.Challenge, acfg.RenewBeforeDays))

	// Start the challenge/redirect server on the redirect port (port 80). A bind
	// failure (port 80 typically needs elevated privileges) is logged but NOT
	// fatal: the HTTPS UI stays reachable and the renewer keeps the server in the
	// degraded self-signed state until issuance can succeed. dns-01 validates via
	// a TXT record and has no port-80 listener (challengeSrv is nil).
	if challengeSrv != nil {
		go func() {
			app.startup.Info(fmt.Sprintf("ACME http-01 challenge/redirect server listening on %s", challengeSrv.Addr))
			if err := challengeSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				app.startup.Error(fmt.Sprintf(
					"ACME http-01 challenge/redirect server failed on %s: %v — certificate issuance cannot complete until this is resolved",
					challengeSrv.Addr, err))
			}
		}()
	}

	// We are serving the full acme topology. Fold the three live resources into one
	// composite Instance (H4c-1) so the controller owns and drains them as a unit.
	app.acmeActive = true
	rt := &acmeRuntime{listener: listener, challengeSrv: challengeSrv, renewerCancel: renewCancel}
	res := serverResult{errCh: listener.Serve(), tlsListener: listener}

	if app.listenerRebind != nil {
		// Adopt the topology into the controller; its Instance.Shutdown (rt.shutdown)
		// now owns renewer + challenge + listener teardown, so awaitShutdown drains
		// everything via the controller and the boot serverResult must not also carry
		// them (single owner — avoids a double teardown and gives H4c-2 the swap seam).
		app.adoptListenerController(handler,
			&serverctl.Instance{Addr: listener.Addr(), Shutdown: rt.shutdown},
			app.cfg.Server)
	} else {
		// No rebinder wired (unsupervised host / tests): the controller is not adopted,
		// so the boot serverResult keeps the challenge + renewer for awaitShutdown's
		// fallback teardown path.
		res.challengeSrv = challengeSrv
		res.renewerCancel = renewCancel
	}

	return res, nil
}

// promotingIssuer wraps an acme.CertObtainer so that a successful issuance is
// promoted into the running HTTPS listener in place (no restart). The reload
// holder swaps the live certificate via ReloadFromPEM and, via its onReload
// callback, clears any degraded self-signed state and resumes HSTS.
// A promotion failure is logged but does not fail the
// issuance — the new certificate is persisted and a restart will pick it up.
type promotingIssuer struct {
	inner  acme.CertObtainer
	reload *webapi.TLSReloadHolder
	log    func(level, msg string)
}

func (p *promotingIssuer) Obtain(ctx context.Context) (certPEM, keyPEM []byte, err error) {
	certPEM, keyPEM, err = p.inner.Obtain(ctx)
	if err != nil {
		return certPEM, keyPEM, err
	}
	if p.reload != nil {
		if rerr := p.reload.Reload(certPEM, keyPEM); rerr != nil {
			p.log("ERROR", fmt.Sprintf(
				"ACME certificate obtained but in-place promotion failed: %v — a restart will apply it", rerr))
		} else {
			p.log("INFO", "ACME certificate promoted in place (degraded state cleared if set)")
		}
	}
	return certPEM, keyPEM, err
}

// isACMEStaging reports whether the CA directory URL looks like a staging
// endpoint (issued certificates are untrusted). Used to WARN at startup.
func isACMEStaging(caURL string) bool {
	return strings.Contains(strings.ToLower(caURL), "staging")
}

// acmeTriggerHolder forwards an admin "re-assert now" request (POST/PUT of ACME
// config) to the running Renewer's Trigger. It is wired into the router before
// setupACME builds the renewer — the same chicken-and-egg the tlsReload holder
// solves — and bound once the renewer exists. Calls before binding, or in
// non-ACME modes, are no-ops. Safe for concurrent use.
type acmeTriggerHolder struct {
	mu sync.Mutex
	fn func()
}

// Set binds the holder to the renewer's Trigger (or any non-blocking func).
func (h *acmeTriggerHolder) Set(fn func()) {
	h.mu.Lock()
	h.fn = fn
	h.mu.Unlock()
}

// Trigger forwards to the bound function, or is a no-op if none is bound yet.
func (h *acmeTriggerHolder) Trigger() {
	h.mu.Lock()
	fn := h.fn
	h.mu.Unlock()
	if fn != nil {
		fn()
	}
}
