// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package hypervisor

// clientOptions holds optional configuration shared by hypervisor backends.
type clientOptions struct {
	insecureSkipTLSVerify bool
}

// ClientOption configures a hypervisor client.
type ClientOption func(*clientOptions)

// WithInsecureSkipTLSVerify controls whether the client verifies the
// hypervisor's TLS certificate. Verification is ON by default; pass true only
// for lab hypervisors using self-signed certificates (sourced from
// driver_settings.<type>_insecure).
func WithInsecureSkipTLSVerify(insecure bool) ClientOption {
	return func(o *clientOptions) { o.insecureSkipTLSVerify = insecure }
}

func applyClientOptions(opts []ClientOption) clientOptions {
	var o clientOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}
