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
	// EnvVars maps environment variable names to their plaintext values
	// as byte slices. Using []byte instead of string ensures that
	// Cleanup can zero every copy of the credential data — Go strings
	// are immutable so string copies would persist in heap until GC.
	// The values are converted to strings only at injection time in
	// InjectCredentialEnvVars.
	EnvVars map[string][]byte
}

// Cleanup zeros all credential plaintext from memory and clears the env
// var map. This MUST be called after the Test Kitchen process exits.
func (kc *KitchenCredentials) Cleanup() {
	for k, v := range kc.EnvVars {
		secrets.ZeroBytes(v)
		delete(kc.EnvVars, k)
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
		return &KitchenCredentials{EnvVars: map[string][]byte{}}, nil
	}

	kc := &KitchenCredentials{EnvVars: make(map[string][]byte)}
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
		kc.EnvVars[envName] = resolved.Plaintext
	}

	// 2. Chef license key.
	if tkConfig.ChefLicenseKeyCredential != "" {
		resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
			CredentialName: tkConfig.ChefLicenseKeyCredential,
		})
		if err != nil {
			errs = append(errs, fmt.Sprintf("chef_license_key_credential (credential %q): %v", tkConfig.ChefLicenseKeyCredential, err))
		} else {
			kc.EnvVars["CMM_TK_CHEF_LICENSE_KEY"] = resolved.Plaintext
		}
	}

	// 3. Transport secrets (password and SSH key per image).
	for _, img := range tkConfig.Images {
		if img.Transport == nil {
			continue
		}
		if img.Transport.PasswordCredential != "" {
			resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
				CredentialName: img.Transport.PasswordCredential,
			})
			if err != nil {
				errs = append(errs, fmt.Sprintf("transport password for image %q (credential %q): %v",
					img.Name, img.Transport.PasswordCredential, err))
			} else {
				envName := transportPasswordEnvVar(img.Name)
				kc.EnvVars[envName] = resolved.Plaintext
			}
		}
		if img.Transport.SSHKeyCredential != "" {
			resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
				CredentialName: img.Transport.SSHKeyCredential,
			})
			if err != nil {
				errs = append(errs, fmt.Sprintf("transport SSH key for image %q (credential %q): %v",
					img.Name, img.Transport.SSHKeyCredential, err))
			} else {
				envName := transportKeyEnvVar(img.Name)
				kc.EnvVars[envName] = resolved.Plaintext
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
// environment. The []byte credential values are converted to strings only
// here, at the last moment before injection. It also strips any
// pre-existing CMM_TK_* variables from the base environment to avoid
// leaking stale credentials.
func InjectCredentialEnvVars(baseEnv []string, creds *KitchenCredentials) []string {
	// Always filter out any existing CMM_TK_* vars from base to avoid
	// leaking stale credentials, even when creds is nil or empty.
	credCount := 0
	if creds != nil {
		credCount = len(creds.EnvVars)
	}
	out := make([]string, 0, len(baseEnv)+credCount)
	for _, kv := range baseEnv {
		if idx := strings.IndexByte(kv, '='); idx > 0 {
			key := kv[:idx]
			if strings.HasPrefix(strings.ToUpper(key), "CMM_TK_") {
				continue
			}
		}
		out = append(out, kv)
	}

	if creds == nil || len(creds.EnvVars) == 0 {
		return out
	}

	// Append credential env vars — string conversion happens here at
	// injection time so the []byte originals remain zeroable.
	for k, v := range creds.EnvVars {
		out = append(out, k+"="+string(v))
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
		resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
			CredentialName: credName,
		})
		if resolved != nil {
			secrets.ZeroBytes(resolved.Plaintext)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("driver_secrets[%q] → credential %q: %v", key, credName, err))
		}
	}

	// Check Chef license key credential.
	if tkConfig.ChefLicenseKeyCredential != "" {
		resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
			CredentialName: tkConfig.ChefLicenseKeyCredential,
		})
		if resolved != nil {
			secrets.ZeroBytes(resolved.Plaintext)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("chef_license_key_credential → credential %q: %v", tkConfig.ChefLicenseKeyCredential, err))
		}
	}

	// Check transport secrets (per image, not per platform).
	for _, img := range tkConfig.Images {
		if img.Transport == nil {
			continue
		}
		if img.Transport.PasswordCredential != "" {
			resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
				CredentialName: img.Transport.PasswordCredential,
			})
			if resolved != nil {
				secrets.ZeroBytes(resolved.Plaintext)
			}
			if err != nil {
				errs = append(errs, fmt.Sprintf("image %q transport password → credential %q: %v",
					img.Name, img.Transport.PasswordCredential, err))
			}
		}
		if img.Transport.SSHKeyCredential != "" {
			resolved, err := resolver.Resolve(ctx, secrets.CredentialSource{
				CredentialName: img.Transport.SSHKeyCredential,
			})
			if resolved != nil {
				secrets.ZeroBytes(resolved.Plaintext)
			}
			if err != nil {
				errs = append(errs, fmt.Sprintf("image %q transport SSH key → credential %q: %v",
					img.Name, img.Transport.SSHKeyCredential, err))
			}
		}
	}

	return errs
}
