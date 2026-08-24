// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
)

// HostnameRegistrar publishes and maintains a DNS A record per ACME domain so
// the server's FQDN resolves to the host, reusing the Route 53 client, hosted
// zone, and ChangeResourceRecordSets permission of the DNS-01 solver.
// It is opt-in (server.tls.acme.register_hostname) and
// fail-soft: a resolution or Route 53 error is logged on the tls scope but never
// blocks issuance, renewal, or the fail-open path.
type HostnameRegistrar struct {
	api          route53API
	hostedZoneID string
	ttl          int64
	log          LogFunc
	pollInterval time.Duration
	pollTimeout  time.Duration

	// IP-resolution inputs (server.tls.acme.hostname_ip / hostname_interface).
	literalIP string
	ifaceName string

	// Resolution seams — production wiring resolves real interfaces / the
	// default route; tests inject fakes so no host networking is required.
	ifaceIPv4 func(name string) (string, error)
	autoIPv4  func() (string, error)
}

// NewHostnameRegistrar builds a registrar that reuses this solver's Route 53
// client, hosted zone, and poll settings. ttl is the A-record TTL in seconds
// (server.tls.acme.hostname_ttl); a non-positive value falls back to the
// default. literalIP / ifaceName select the IP-resolution strategy (§ 3.13).
func (s *Route53Solver) NewHostnameRegistrar(ttl int, literalIP, ifaceName string, log LogFunc) *HostnameRegistrar {
	t := int64(ttl)
	if t <= 0 {
		t = route53DefaultTTL
	}
	return &HostnameRegistrar{
		api:          s.api,
		hostedZoneID: s.hostedZoneID,
		ttl:          t,
		log:          log,
		pollInterval: s.pollInterval,
		pollTimeout:  s.pollTimeout,
		literalIP:    literalIP,
		ifaceName:    ifaceName,
		ifaceIPv4:    ifaceGlobalUnicastIPv4,
		autoIPv4:     defaultRouteIPv4,
	}
}

// Register UPSERTs an A record for each domain pointing at the resolved host
// IPv4, waiting for each change to reach INSYNC. Wildcard domains are skipped
// with a WARN (an A record cannot be published for a wildcard). It is fail-soft:
// a resolution failure or a per-domain Route 53 error is
// logged on the tls scope and returned, but the caller must never block
// issuance/renewal on the result. On a resolution failure no Route 53 call is
// made. The returned error is the first failure encountered (or nil).
func (r *HostnameRegistrar) Register(ctx context.Context, domains []string) error {
	ip, err := r.resolveIP()
	if err != nil {
		logf(r.log, "ERROR", "ACME hostname self-registration skipped: %v", err)
		return err
	}

	var firstErr error
	for _, d := range domains {
		d = strings.TrimSuffix(d, ".")
		if d == "" {
			continue
		}
		if strings.HasPrefix(d, "*.") {
			logf(r.log, "WARN", "ACME hostname self-registration: skipping wildcard domain %q (no A record possible)", d)
			continue
		}
		if cerr := r.upsertA(ctx, d, ip); cerr != nil {
			logf(r.log, "ERROR", "ACME hostname self-registration for %s failed: %v", d, cerr)
			if firstErr == nil {
				firstErr = cerr
			}
			continue
		}
		logf(r.log, "INFO", "ACME hostname A record for %s -> %s is INSYNC", d, ip)
	}
	return firstErr
}

// resolveIP picks the A-record target (first wins):
// literal hostname_ip, then the named hostname_interface's global-unicast IPv4,
// then auto-detect (default-route IPv4). An explicit-but-unusable literal or
// interface is an error with no fall-through to auto-detect.
func (r *HostnameRegistrar) resolveIP() (string, error) {
	if r.literalIP != "" {
		ip := net.ParseIP(r.literalIP)
		if ip == nil || ip.To4() == nil {
			return "", fmt.Errorf("hostname_ip %q is not a valid IPv4 address", r.literalIP)
		}
		return ip.To4().String(), nil
	}
	if r.ifaceName != "" {
		return r.ifaceIPv4(r.ifaceName)
	}
	return r.autoIPv4()
}

// upsertA UPSERTs the A record for one domain and waits for INSYNC.
func (r *HostnameRegistrar) upsertA(ctx context.Context, domain, ip string) error {
	out, err := r.api.ChangeResourceRecordSets(ctx, r.aRecordChange(domain, ip))
	if err != nil {
		return fmt.Errorf("acme route53: upsert A for %s: %w", domain, err)
	}
	return pollChangeInSync(ctx, r.api, out.ChangeInfo, r.pollInterval, r.pollTimeout)
}

// aRecordChange builds a single-change UPSERT batch for the host A record. The
// name is the fully qualified domain and the value is the resolved IPv4.
func (r *HostnameRegistrar) aRecordChange(domain, ip string) *route53.ChangeResourceRecordSetsInput {
	return &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(r.hostedZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{{
				Action: types.ChangeActionUpsert,
				ResourceRecordSet: &types.ResourceRecordSet{
					Name: aws.String(domain + "."),
					Type: types.RRTypeA,
					TTL:  aws.Int64(r.ttl),
					ResourceRecords: []types.ResourceRecord{{
						Value: aws.String(ip),
					}},
				},
			}},
		},
	}
}

// ifaceGlobalUnicastIPv4 returns the first global-unicast IPv4 address bound to
// the named interface, or an error if the interface is unknown or has none.
func ifaceGlobalUnicastIPv4(name string) (string, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", fmt.Errorf("hostname_interface %q: %w", name, err)
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "", fmt.Errorf("hostname_interface %q: %w", name, err)
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}
		if ip4 := ip.To4(); ip4 != nil && ip.IsGlobalUnicast() {
			return ip4.String(), nil
		}
	}
	return "", fmt.Errorf("hostname_interface %q has no global-unicast IPv4 address", name)
}

// defaultRouteIPv4 returns the IPv4 address the OS would source off-link traffic
// from — the interface carrying the default route. A UDP "connection" only makes
// the kernel select a route and source address; no packets are sent.
func defaultRouteIPv4() (string, error) {
	// 192.0.2.1 is TEST-NET-1 (RFC 5737) — reserved, never routed, used here
	// only to make the kernel choose a source address.
	conn, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return "", fmt.Errorf("auto-detect default-route IPv4: %w", err)
	}
	defer func() { _ = conn.Close() }()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP.To4() == nil {
		return "", errors.New("auto-detect default-route IPv4: no IPv4 source address")
	}
	return addr.IP.To4().String(), nil
}
