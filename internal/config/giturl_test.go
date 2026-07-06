// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package config

import "testing"

func TestValidateGitBaseURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		ok   bool
	}{
		// Valid scheme URLs.
		{"ssh with numeric port", "ssh://git@git.example.com:7999/is_chef_ckbks", true},
		{"ssh default port slash path", "ssh://git@git.example.com/group/chef/cookbooks", true},
		{"https", "https://git.example.com/group/cookbooks", true},
		{"https no path (base)", "https://git.example.com", true},
		{"http", "http://git.example.com/group", true},
		{"git scheme", "git://git.example.com/group", true},
		// Valid scp-style.
		{"scp-style with user", "git@git.example.com:group/chef/cookbooks", true},
		{"scp-style no user", "git.example.com:group/cookbooks", true},

		// The footgun: ssh:// scheme + non-numeric segment after host colon.
		{"ssh hybrid non-numeric port", "ssh://git@git.example.com:group/chef/cookbooks", false},
		// Other invalids.
		{"empty", "", false},
		{"whitespace only", "   ", false},
		{"unsupported scheme", "ftp://git.example.com/group", false},
		{"scp-style missing path", "git@git.example.com:", false},
		{"no colon, no scheme", "git.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateGitBaseURL(tc.url)
			if tc.ok && err != nil {
				t.Errorf("ValidateGitBaseURL(%q) = %v, want ok", tc.url, err)
			}
			if !tc.ok && err == nil {
				t.Errorf("ValidateGitBaseURL(%q) = nil, want error", tc.url)
			}
		})
	}
}

// The ssh hybrid error must be actionable — name the URL and suggest the fix.
func TestValidateGitBaseURL_HybridMessage(t *testing.T) {
	err := ValidateGitBaseURL("ssh://git@git.example.com:group/chef/cookbooks")
	if err == nil {
		t.Fatal("expected an error for the ssh:// hybrid")
	}
	msg := err.Error()
	for _, want := range []string{"git@host:path", "ssh://git@host/path"} {
		if !contains(msg, want) {
			t.Errorf("error message %q should suggest %q", msg, want)
		}
	}
}

func TestValidateGitBaseURLs_ReportsIndex(t *testing.T) {
	urls := []string{
		"ssh://git@git.example.com:7999/is_chef_ckbks",  // ok
		"git@git.example.com:group/cookbooks",           // ok
		"ssh://git@git.example.com:group/chef/cookbooks", // bad (index 2)
	}
	err := ValidateGitBaseURLs(urls)
	if err == nil {
		t.Fatal("expected an error for the bad URL at index 2")
	}
	if !contains(err.Error(), "git_base_urls[2]") {
		t.Errorf("error %q should name the offending index", err.Error())
	}
}

func TestValidateGitBaseURLs_EmptyAllowed(t *testing.T) {
	if err := ValidateGitBaseURLs(nil); err != nil {
		t.Errorf("empty list should be allowed, got %v", err)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
