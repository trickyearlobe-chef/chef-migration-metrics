// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package acme

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// route53API is the subset of the AWS SDK Route 53 client the DNS-01 solver
// drives. *route53.Client satisfies it structurally; tests supply a fake so the
// UPSERT/poll/DELETE flow is exercised without any AWS access (tls-acme.md § 3.4).
type route53API interface {
	ChangeResourceRecordSets(ctx context.Context, in *route53.ChangeResourceRecordSetsInput, optFns ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error)
	GetChange(ctx context.Context, in *route53.GetChangeInput, optFns ...func(*route53.Options)) (*route53.GetChangeOutput, error)
}

// Route53Solver publishes DNS-01 challenge proofs as TXT records in an Amazon
// Route 53 hosted zone. Present UPSERTs the `_acme-challenge.<domain>` TXT record
// and blocks until the change set reaches INSYNC before the CA validates;
// CleanUp removes it. It satisfies the Solver seam and holds no key material —
// the engine computes the TXT value from the account key (tls-acme.md § 3.3/3.4).
type Route53Solver struct {
	api          route53API
	hostedZoneID string
	ttl          int64
	log          LogFunc
	pollInterval time.Duration
	pollTimeout  time.Duration
}

const (
	// route53DefaultTTL is the TTL for challenge TXT records. They are short-lived
	// (created, validated, deleted), so a low value avoids stale caching.
	route53DefaultTTL = 60
	// route53PollInterval / route53PollTimeout bound the GetChange polling loop
	// that waits for a change set to propagate to INSYNC.
	route53PollInterval = 5 * time.Second
	route53PollTimeout  = 3 * time.Minute
)

// newRoute53Solver builds a solver over an explicit route53API (the seam tests
// inject). NewRoute53Solver is the production entry point that constructs the
// real client from resolved AWS settings.
func newRoute53Solver(api route53API, hostedZoneID string, log LogFunc) *Route53Solver {
	return &Route53Solver{
		api:          api,
		hostedZoneID: hostedZoneID,
		ttl:          route53DefaultTTL,
		log:          log,
		pollInterval: route53PollInterval,
		pollTimeout:  route53PollTimeout,
	}
}

// route53Settings is the resolved AWS configuration for the solver: the region
// and hosted zone, plus optional static credentials sourced from the encrypted
// config store. Empty credentials mean "let the AWS default chain (env vars,
// IAM instance role) resolve them" (tls-acme.md § 3.4).
type route53Settings struct {
	region          string
	hostedZoneID    string
	accessKeyID     string
	secretAccessKey string
}

// NewRoute53Solver resolves AWS settings (region, hosted zone, credentials) from
// dns_provider_config and the encrypted config store, builds a Route 53 client,
// and returns a solver. Credential resolution order (tls-acme.md § 3.4):
// config-store secrets → AWS_* env vars → IAM instance role. Region/zone come
// from dns_provider_config, falling back to the config-store. It performs no
// network I/O — the AWS default credential chain resolves lazily on first call.
func NewRoute53Solver(ctx context.Context, store SecretStore, dnsCfg map[string]string, log LogFunc) (*Route53Solver, error) {
	st, err := resolveRoute53Settings(ctx, store, dnsCfg)
	if err != nil {
		return nil, err
	}
	if st.hostedZoneID == "" {
		return nil, errors.New("acme route53: hosted_zone_id is required (set dns_provider_config.hosted_zone_id or the server.tls.acme.route53.hosted_zone_id config-store value)")
	}

	var opts []func(*awsconfig.LoadOptions) error
	if st.region != "" {
		opts = append(opts, awsconfig.WithRegion(st.region))
	}
	if st.accessKeyID != "" && st.secretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(st.accessKeyID, st.secretAccessKey, "")))
		logf(log, "DEBUG", "ACME route53: using AWS credentials from the encrypted config store")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("acme route53: load AWS config: %w", err)
	}
	return newRoute53Solver(route53.NewFromConfig(awsCfg), st.hostedZoneID, log), nil
}

// resolveRoute53Settings gathers region, hosted zone, and optional static
// credentials. dns_provider_config takes precedence over the config-store for
// region/zone; credentials come only from the config store (env/role are handled
// by the AWS default chain when these are empty). Static credentials are used
// only when BOTH halves are present — a half-configured pair is ignored.
func resolveRoute53Settings(ctx context.Context, store SecretStore, dnsCfg map[string]string) (route53Settings, error) {
	st := route53Settings{
		region:       firstNonEmpty(dnsCfg["region"], storeString(ctx, store, configstore.KeyServerTLSACMERoute53Region, false)),
		hostedZoneID: firstNonEmpty(dnsCfg["hosted_zone_id"], storeString(ctx, store, configstore.KeyServerTLSACMERoute53HostedZoneID, false)),
	}
	id := storeString(ctx, store, configstore.KeyServerTLSACMERoute53AccessKeyID, true)
	secret := storeString(ctx, store, configstore.KeyServerTLSACMERoute53SecretAccessKey, true)
	if id != "" && secret != "" {
		st.accessKeyID = id
		st.secretAccessKey = secret
	}
	return st, nil
}

// Present UPSERTs the challenge TXT record and waits for the change to reach
// INSYNC, so the record is observable before the CA validates.
func (s *Route53Solver) Present(ctx context.Context, ch Challenge) error {
	out, err := s.api.ChangeResourceRecordSets(ctx, s.changeInput(types.ChangeActionUpsert, ch))
	if err != nil {
		return fmt.Errorf("acme route53: upsert TXT for %s: %w", ch.Domain, err)
	}
	logf(s.log, "DEBUG", "ACME dns-01 TXT record upserted for %s, waiting for propagation", ch.Domain)
	if err := s.waitInSync(ctx, out.ChangeInfo); err != nil {
		return err
	}
	logf(s.log, "DEBUG", "ACME dns-01 TXT record for %s is INSYNC", ch.Domain)
	return nil
}

// CleanUp DELETEs the challenge TXT record. It is best-effort — removal does not
// wait for INSYNC (the authorization has already settled by the time CleanUp
// runs), so a slow propagation never blocks the order flow.
func (s *Route53Solver) CleanUp(ctx context.Context, ch Challenge) error {
	if _, err := s.api.ChangeResourceRecordSets(ctx, s.changeInput(types.ChangeActionDelete, ch)); err != nil {
		return fmt.Errorf("acme route53: delete TXT for %s: %w", ch.Domain, err)
	}
	return nil
}

// changeInput builds a single-change batch for the challenge TXT record. The
// record name is `_acme-challenge.<domain>.` (fully qualified) and the value is
// the engine-computed DNS-01 digest enclosed in double quotes, as Route 53
// requires for TXT character-strings.
func (s *Route53Solver) changeInput(action types.ChangeAction, ch Challenge) *route53.ChangeResourceRecordSetsInput {
	name := "_acme-challenge." + strings.TrimSuffix(ch.Domain, ".") + "."
	return &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(s.hostedZoneID),
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{{
				Action: action,
				ResourceRecordSet: &types.ResourceRecordSet{
					Name: aws.String(name),
					Type: types.RRTypeTxt,
					TTL:  aws.Int64(s.ttl),
					ResourceRecords: []types.ResourceRecord{{
						Value: aws.String(strconv.Quote(ch.DNSValue)),
					}},
				},
			}},
		},
	}
}

// waitInSync polls GetChange until the change set is INSYNC or pollTimeout
// elapses. A nil change info (nothing to track) is treated as already settled.
func (s *Route53Solver) waitInSync(ctx context.Context, info *types.ChangeInfo) error {
	return pollChangeInSync(ctx, s.api, info, s.pollInterval, s.pollTimeout)
}

// pollChangeInSync polls api.GetChange until the change set reaches INSYNC or
// timeout elapses. A nil change info (nothing to track) is treated as already
// settled. Shared by the DNS-01 TXT solver and the hostname A-record registrar.
func pollChangeInSync(ctx context.Context, api route53API, info *types.ChangeInfo, interval, timeout time.Duration) error {
	if info == nil || info.Id == nil {
		return nil
	}
	id := info.Id
	status := info.Status

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		if status == types.ChangeStatusInsync {
			return nil
		}
		if err := sleepCtx(ctx, interval); err != nil {
			return fmt.Errorf("acme route53: change %s did not reach INSYNC: %w", aws.ToString(id), err)
		}
		out, err := api.GetChange(ctx, &route53.GetChangeInput{Id: id})
		if err != nil {
			return fmt.Errorf("acme route53: poll GetChange for %s: %w", aws.ToString(id), err)
		}
		if out.ChangeInfo != nil {
			status = out.ChangeInfo.Status
		}
	}
}

// sleepCtx waits for d or until ctx is cancelled, returning ctx.Err() on
// cancellation.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// storeString reads a JSON-encoded string value from the config store, returning
// "" for any miss (not found, not secret, decode error). secret selects the
// GetSecret accessor for encrypted values.
func storeString(ctx context.Context, store SecretStore, key string, secret bool) string {
	if store == nil {
		return ""
	}
	var (
		raw json.RawMessage
		err error
	)
	if secret {
		raw, err = store.GetSecret(ctx, key)
	} else {
		raw, err = store.Get(ctx, key)
	}
	if err != nil {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

// firstNonEmpty returns the first non-empty argument, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
