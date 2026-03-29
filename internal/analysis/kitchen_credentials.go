// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"context"
	"fmt"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// KitchenCredentials holds resolved credential values for a Test Kitchen
// run. The Cleanup function MUST be called after the Test Kitchen process
// exits to zero all plaintext from memory.
type KitchenCredentials struct {
	// EnvVars maps environment variable names to their plaintext values.
	// These must be injected into the Test Kitchen child process.
	EnvVars map[string]string

	// plaintexts holds references to all resolved plaintext byte slices
	// so they can be zeroed on cleanup.
	plaintexts [][]byte
}

// Cleanup zeros all credential plaintext from memory and clears the env
// var map. This MUST be called after the Test Kitchen process exits.
func (kc *KitchenCredentials) Cleanup() {
	for _, p := range kc.plaintexts {
		secrets.ZeroBytes(p)
	}
	kc.plaintexts = nil
	for k := range kc.EnvVars {
		kc.EnvVars[k] = ""
	}
	kc.EnvVars = nil
}

// ResolveKitchenCredentials resolves all driver secrets and transport
// secrets for a Test Kitchen run. Returns a KitchenCredentials containing
// the env vars to inject, and a Cleanup function that MUST be called
// after the TK process exits.
//
// If resolver is nil, returns empty credentials (no secrets to resolve).
// Individual credential resolution failures are collected; the function
// returns all errors joined rather than failing on the first.
func ResolveKitchenCredentials(
	ctx context.Context,
	resolver *secrets.CredentialResolver,
	tkConfig config.TestKitchenConfig,
) (*KitchenCredentials, error) {
	if resolver == nil {
		return &KitchenCredentials{EnvVars: map[string]string{}}, nil
	}

	kc := &KitchenCredentials{EnvVars: make(map[string]string)}
	var errs []string

	// 1. Driver secrets.
	for key, credName := range tkConfig.DriverSecrets {
		resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
			CredentialName: credName,
		})
		if err != nil {
			errs = append(errs, fmt.Sprintf("driver secret %q (credential %q): %v", key, credName, err))
			continue
		}
		envName := driverSecretEnvVar(key)
		kc.EnvVars[envName] = string(resolved.Plaintext)
		kc.plaintexts = append(kc.plaintexts, resolved.Plaintext)
	}

	// 2. Transport secrets (password and SSH key per platform).
	for _, entry := range tkConfig.PlatformMap {
		if entry.Transport == nil {
			continue
		}
		if entry.Transport.PasswordCredential != "" {
			resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
				CredentialName: entry.Transport.PasswordCredential,
			})
			if err != nil {
				errs = append(errs, fmt.Sprintf("transport password for %q (credential %q): %v",
					entry.KitchenName, entry.Transport.PasswordCredential, err))
			} else {
				envName := transportPasswordEnvVar(entry.KitchenName)
				kc.EnvVars[envName] = string(resolved.Plaintext)
				kc.plaintexts = append(kc.plaintexts, resolved.Plaintext)
			}
		}
		if entry.Transport.SSHKeyCredential != "" {
			resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
				CredentialName: entry.Transport.SSHKeyCredential,
			})
			if err != nil {
				errs = append(errs, fmt.Sprintf("transport SSH key for %q (credential %q): %v",
					entry.KitchenName, entry.Transport.SSHKeyCredential, err))
			} else {
				envName := transportKeyEnvVar(entry.KitchenName)
				kc.EnvVars[envName] = string(resolved.Plaintext)
				kc.plaintexts = append(kc.plaintexts, resolved.Plaintext)
			}
		}
	}

	if len(errs) > 0 {
		return kc, fmt.Errorf("credential resolution errors: %s", strings.Join(errs, "; "))
	}
	return kc, nil
}

// InjectCredentialEnvVars returns a copy of the base environment with the
// credential env vars appended. This is used to build the child process
// environment. It also strips any pre-existing CMM_TK_* variables from
// the base environment to avoid leaking stale credentials.
func InjectCredentialEnvVars(baseEnv []string, creds *KitchenCredentials) []string {
	if creds == nil || len(creds.EnvVars) == 0 {
		return baseEnv
	}

	// Filter out any existing CMM_TK_* vars from base.
	out := make([]string, 0, len(baseEnv)+len(creds.EnvVars))
	for _, kv := range baseEnv {
		if idx := strings.IndexByte(kv, '='); idx > 0 {
			key := kv[:idx]
			if strings.HasPrefix(strings.ToUpper(key), "CMM_TK_") {
				continue
			}
		}
		out = append(out, kv)
	}

	// Append credential env vars.
	for k, v := range creds.EnvVars {
		out = append(out, k+"="+v)
	}
	return out
}

// ValidateDriverCredentials checks that all referenced driver_secrets
// and transport credentials exist and can be decrypted. Returns a list
// of validation errors. A nil resolver returns no errors (credential
// validation is skipped when no store is available).
func ValidateDriverCredentials(
	ctx context.Context,
	resolver *secrets.CredentialResolver,
	tkConfig config.TestKitchenConfig,
) []string {
	if resolver == nil {
		return nil
	}

	var errs []string

	// Check driver secrets.
	for key, credName := range tkConfig.DriverSecrets {
		_, err := resolver.Resolve(ctx, secrets.CredentialSource{
			CredentialName: credName,
		})
		if err != nil {
			errs = append(errs, fmt.Sprintf("driver_secrets[%q] → credential %q: %v", key, credName, err))
		}
	}

	// Check transport secrets.
	for _, entry := range tkConfig.PlatformMap {
		if entry.Transport == nil {
			continue
		}
		if entry.Transport.PasswordCredential != "" {
			_, err := resolver.Resolve(ctx, secrets.CredentialSource{
				CredentialName: entry.Transport.PasswordCredential,
			})
			if err != nil {
				errs = append(errs, fmt.Sprintf("platform %q transport password → credential %q: %v",
					entry.KitchenName, entry.Transport.PasswordCredential, err))
			}
		}
		if entry.Transport.SSHKeyCredential != "" {
			_, err := resolver.Resolve(ctx, secrets.CredentialSource{
				CredentialName: entry.Transport.SSHKeyCredential,
			})
			if err != nil {
				errs = append(errs, fmt.Sprintf("platform %q transport SSH key → credential %q: %v",
					entry.KitchenName, entry.Transport.SSHKeyCredential, err))
			}
		}
	}

	return errs
}
