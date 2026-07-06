// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ValidateGitBaseURL checks that a git base URL is a well-formed git remote the
// collector can clone from. It accepts:
//
//   - scheme URLs: ssh://[user@]host[:PORT]/path (PORT numeric), https://,
//     http://, git://
//   - scp-style: [user@]host:path (no scheme; ':' separates host from path)
//
// It rejects the common footgun of mixing the two — an ssh:// URL with a
// non-numeric segment after the host colon (e.g.
// "ssh://git@host:group/repo"), where ssh interprets "group" as a port. Such a
// URL clones fine as scp-style ("git@host:group/repo") but fails under the
// ssh:// scheme. The returned error names the offending URL and the fix.
func ValidateGitBaseURL(raw string) error {
	s := strings.TrimSpace(raw)
	if s == "" {
		return errors.New("git base URL must not be empty")
	}

	if i := strings.Index(s, "://"); i >= 0 {
		scheme := strings.ToLower(s[:i])
		switch scheme {
		case "ssh", "http", "https", "git":
		default:
			return fmt.Errorf("%q: unsupported scheme %q — use ssh://, https://, http://, git://, or scp-style user@host:path", s, s[:i])
		}
		u, err := url.Parse(s)
		if err != nil {
			if scheme == "ssh" {
				return fmt.Errorf("%q: invalid ssh:// URL — in an ssh:// URL ':' introduces a numeric port and the path starts with '/'. For a default-port host use scp-style git@host:path, or ssh://git@host/path (%v)", s, err)
			}
			return fmt.Errorf("%q: invalid URL: %v", s, err)
		}
		if u.Host == "" {
			return fmt.Errorf("%q: missing host", s)
		}
		// url.Parse tolerates an empty/non-numeric port on some inputs; guard
		// the ssh:// hybrid explicitly so the numeric-port rule is enforced.
		if p := u.Port(); p != "" && !isAllDigits(p) {
			return fmt.Errorf("%q: %q is not a numeric port — for a scp-style path use git@host:path (no scheme), or ssh://git@host/path", s, p)
		}
		return nil
	}

	// scp-style: [user@]host:path
	colon := strings.Index(s, ":")
	if colon < 0 {
		return fmt.Errorf("%q: not a recognised git remote — use scp-style user@host:path or a scheme URL (ssh://, https://, http://, git://)", s)
	}
	hostPart := s[:colon]
	pathPart := s[colon+1:]
	host := hostPart
	if at := strings.LastIndex(hostPart, "@"); at >= 0 {
		host = hostPart[at+1:]
	}
	if host == "" {
		return fmt.Errorf("%q: missing host before ':'", s)
	}
	if pathPart == "" {
		return fmt.Errorf("%q: missing path after ':' — a base URL needs a path segment (e.g. git@host:group)", s)
	}
	return nil
}

// ValidateGitBaseURLs validates each URL in order, returning the first error
// (prefixed with its position) so the caller can surface exactly which entry is
// wrong. Empty input is allowed (no base URLs configured).
func ValidateGitBaseURLs(urls []string) error {
	for i, u := range urls {
		if err := ValidateGitBaseURL(u); err != nil {
			return fmt.Errorf("git_base_urls[%d]: %w", i, err)
		}
	}
	return nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
