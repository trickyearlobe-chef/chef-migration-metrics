// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package jit implements Just-In-Time user provisioning for SAML logins.
package jit

import (
	"context"
	"fmt"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/auth/samlsp"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
)

// UserStore defines the datastore methods required by the JIT provisioner.
type UserStore interface {
	UpsertSAMLUser(ctx context.Context, p datastore.InsertUserParams) (datastore.User, bool, error)
	RecordLoginSuccess(ctx context.Context, username string) error
}

// Logger is a function that logs events.
type Logger func(level, msg string)

// Provisioner handles JIT user creation/update on SAML login.
type Provisioner struct {
	store  UserStore
	logger Logger
}

// New creates a JIT Provisioner.
func New(store UserStore, logger Logger) *Provisioner {
	if logger == nil {
		logger = func(string, string) {}
	}
	return &Provisioner{store: store, logger: logger}
}

// Provision creates or updates a user from SAML assertion data.
// Returns the provisioned user and whether it was newly created.
func (p *Provisioner) Provision(ctx context.Context, info *samlsp.UserInfo) (datastore.User, bool, error) {
	if info == nil {
		return datastore.User{}, false, fmt.Errorf("jit: nil user info")
	}
	if info.SAMLSubject == "" {
		return datastore.User{}, false, fmt.Errorf("jit: empty SAML subject")
	}

	// Derive username: use the extracted username, falling back to a
	// sanitised form of the SAML subject.
	//
	// The fallback exists because users.username is NOT NULL — with no name in
	// the assertion there is nothing else to store, so one is invented rather
	// than the sign-in failing. That is the wrong trade: a person coined from an
	// opaque token has their work hanging off a string nobody chose. A sign-in
	// carrying no name should be refused, naming the missing claim.
	username := info.Username
	if username == "" {
		username = sanitiseUsername(info.SAMLSubject)
	}

	role := info.Role
	if role == "" {
		role = "viewer"
	}

	params := datastore.InsertUserParams{
		Username:     username,
		DisplayName:  info.DisplayName,
		Email:        info.Email,
		Role:         role,
		AuthProvider: "saml",
		SAMLSubject:  info.SAMLSubject,
	}

	user, isNew, err := p.store.UpsertSAMLUser(ctx, params)
	if err != nil {
		return datastore.User{}, false, fmt.Errorf("jit: provisioning user: %w", err)
	}

	if isNew {
		p.logger("INFO", fmt.Sprintf("JIT provisioned new SAML user: username=%s subject=%s role=%s",
			user.Username, info.SAMLSubject, user.Role))
	} else {
		p.logger("INFO", fmt.Sprintf("JIT updated existing SAML user: username=%s subject=%s role=%s",
			user.Username, info.SAMLSubject, user.Role))
	}

	// Record login success (updates last_login_at, resets failed attempts).
	if loginErr := p.store.RecordLoginSuccess(ctx, user.Username); loginErr != nil {
		p.logger("WARN", fmt.Sprintf("JIT: failed to record login success for %s: %v", user.Username, loginErr))
	}

	return user, isNew, nil
}

// sanitiseUsername creates a safe username from a SAML subject by replacing
// problematic characters with underscores and truncating.
func sanitiseUsername(subject string) string {
	// Take the part after the last colon (the NameID portion).
	if idx := strings.LastIndex(subject, ":"); idx >= 0 && idx < len(subject)-1 {
		subject = subject[idx+1:]
	}

	// Replace characters that are problematic for usernames.
	replacer := strings.NewReplacer(
		"@", "_at_",
		" ", "_",
		"/", "_",
		"\\", "_",
	)
	result := replacer.Replace(subject)

	// Truncate to 64 characters.
	if len(result) > 64 {
		result = result[:64]
	}

	return strings.ToLower(result)
}
