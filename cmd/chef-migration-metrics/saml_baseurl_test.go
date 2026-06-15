// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

func TestResolveSPBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		spBaseURL  string
		spEntityID string
		scheme     string
		httpsPort  int
		want       string
	}{
		{
			name:      "explicit sp_base_url wins and trailing slash trimmed",
			spBaseURL: "https://cmm.example.com/",
			// entity ID and fallback inputs are ignored when sp_base_url is set.
			spEntityID: "https://other.example.com/saml",
			scheme:     "https",
			httpsPort:  443,
			want:       "https://cmm.example.com",
		},
		{
			name:       "falls back to http(s) sp_entity_id host",
			spEntityID: "https://app.example.com/saml",
			scheme:     "https",
			httpsPort:  443,
			want:       "https://app.example.com",
		},
		{
			name:       "fallback omits standard https port 443",
			spEntityID: "urn:not-a-url",
			scheme:     "https",
			httpsPort:  443,
			want:       "https://localhost",
		},
		{
			name:       "fallback keeps non-standard https port",
			spEntityID: "urn:not-a-url",
			scheme:     "https",
			httpsPort:  8443,
			want:       "https://localhost:8443",
		},
		{
			name:      "fallback plain http keeps the listen port",
			scheme:    "http",
			httpsPort: 8080,
			want:      "http://localhost:8080",
		},
		{
			name:      "fallback omits standard http port 80",
			scheme:    "http",
			httpsPort: 80,
			want:      "http://localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSPBaseURL(tt.spBaseURL, tt.spEntityID, tt.scheme, tt.httpsPort)
			if got != tt.want {
				t.Errorf("resolveSPBaseURL(%q, %q, %q, %d) = %q, want %q",
					tt.spBaseURL, tt.spEntityID, tt.scheme, tt.httpsPort, got, tt.want)
			}
		})
	}
}
