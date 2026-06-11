// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

// AutoHTTPSPlan is the resolved listening layout for the automatic HTTPS-on-443
// behaviour (tls.md § 1.5): which port the HTTPS listener binds, and which ports
// run 301-redirects to it.
type AutoHTTPSPlan struct {
	// HTTPSPort is the port the HTTPS listener binds.
	HTTPSPort int
	// RedirectPorts are the ports that 301-redirect to HTTPSPort (may be empty).
	RedirectPorts []int
	// BoundTo443 reports whether the plan binds the standard 443 port.
	BoundTo443 bool
}

// ResolveAutoHTTPS decides the HTTPS + redirect port layout for a *healthy* TLS
// listener at startup (tls.md § 1.5). It is pure; the caller supplies the
// 443-bind decision via can443 so the policy can be unit-tested and the real
// bind kept at the edge.
//
//   - serverPort: the configured server.port (0 ⇒ 8080 default).
//   - httpRedirectPort: server.tls.http_redirect_port (0 ⇒ unset).
//   - can443: probe reporting whether 443 can be bound right now. It is consulted
//     at most once, and only when it matters (serverPort != 443).
//
// Rules:
//   - serverPort == 443: serve HTTPS on 443 directly, no auto old-port redirect.
//   - serverPort != 443 and 443 bindable: HTTPS on 443; redirect serverPort →
//     443; an explicit httpRedirectPort also redirects to 443.
//   - serverPort != 443 and 443 NOT bindable: fall back to HTTPS on serverPort,
//     no 443 and no auto old-port redirect; an explicit httpRedirectPort still
//     redirects (now to serverPort, where HTTPS actually serves).
func ResolveAutoHTTPS(serverPort, httpRedirectPort int, can443 func() bool) AutoHTTPSPlan {
	if serverPort == 0 {
		serverPort = 8080
	}

	plan := AutoHTTPSPlan{}
	addRedirect := func(p int) {
		if p > 0 && p != plan.HTTPSPort {
			plan.RedirectPorts = append(plan.RedirectPorts, p)
		}
	}

	switch {
	case serverPort == 443:
		// Already on the standard port — nothing to auto-redirect.
		plan.HTTPSPort = 443
		plan.BoundTo443 = true
		addRedirect(httpRedirectPort)
	case can443 != nil && can443():
		plan.HTTPSPort = 443
		plan.BoundTo443 = true
		addRedirect(serverPort)       // old URL keeps working via redirect
		addRedirect(httpRedirectPort) // explicit redirect also targets 443
	default:
		// 443 unavailable (§ 1.4): serve HTTPS on the configured port, no 443.
		plan.HTTPSPort = serverPort
		plan.BoundTo443 = false
		addRedirect(httpRedirectPort) // keep the explicit redirect working
	}
	return plan
}
