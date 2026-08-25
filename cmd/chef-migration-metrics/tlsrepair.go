// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/config"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/datastore"
	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/secrets"
)

// Repair CLI — host-side TLS lockout recovery. Once TLS material
// lives in the DB (cert_source: db, ACME state, or ca_path) the old "move the
// file on the host" recovery no longer applies; these subcommands are the
// escape hatch. They need the same host-side access as before: DATABASE_URL and
// CMM_CREDENTIAL_ENCRYPTION_KEY. There is deliberately no break-glass env/flag
// override — the recovery boundary is host access, not a runtime lever.

// tlsRepairResult reports what a repair mutation did, so the caller can print
// clear operator output and keep the command idempotent.
type tlsRepairResult int

const (
	repairChanged   tlsRepairResult = iota // a change was written
	repairNoChange                         // already in the desired state
	repairNoSection                        // no server.tls section in the DB
)

const repairUpdatedBy = "repair-cli"

// runTLSCommand dispatches the `tls` repair subcommands. args is os.Args after
// the `tls` token. It returns a process exit code.
func runTLSCommand(args []string) int {
	if len(args) == 0 {
		printTLSUsage()
		return 2
	}

	switch args[0] {
	case "reset":
		return runTLSRepair(tlsResetMode,
			"server.tls.mode set to 'off' — restart the server to apply (it will start in plain HTTP)",
			"server.tls.mode is already 'off' — nothing to do",
			"no server.tls configuration found in the database — TLS is not DB-managed; nothing to reset")
	case "clear-ca":
		return runTLSRepair(tlsClearCA,
			"removed server.tls.ca_path — restart the server to apply (mTLS disabled; TLS stays on)",
			"no server.tls.ca_path is set — nothing to do",
			"no server.tls configuration found in the database — nothing to clear")
	case "mode":
		return runTLSMode(args[1:])
	case "help", "-h", "--help":
		printTLSUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown tls subcommand %q\n\n", args[0])
		printTLSUsage()
		return 2
	}
}

func printTLSUsage() {
	fmt.Fprint(os.Stderr, `Usage: chef-migration-metrics tls <subcommand>

Host-side TLS lockout recovery. Requires DATABASE_URL (or CMM_DATABASE_URL) and
CMM_CREDENTIAL_ENCRYPTION_KEY in the environment.

Subcommands:
  reset      Set server.tls.mode to 'off' so the server starts in plain HTTP.
             Recovers any mode (bad DB cert, mTLS lock, stuck ACME).
             Recovery-framed alias for 'mode off'.
  clear-ca   Remove server.tls.ca_path to recover an mTLS lockout while
             keeping TLS enabled.
  mode <off|static|acme> [--trusted-proxy[=true|false]]
             Set server.tls.mode for a deliberate deployment change. With
             'mode off --trusted-proxy', also sets server.trusted_proxy so the
             app runs plain HTTP behind a TLS-terminating proxy and honours
             X-Forwarded-Proto for HSTS / scheme detection.

Restart the server after running a repair command for it to take effect.
`)
}

// runTLSRepair wires the config store from the environment, runs a repair
// mutation, prints operator output, and returns an exit code. It is the shared
// body of the reset and clear-ca subcommands.
func runTLSRepair(
	mutate func(context.Context, *configstore.Store, string) (tlsRepairResult, error),
	changedMsg, noChangeMsg, noSectionMsg string,
) int {
	store, cleanup, err := openRepairStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer cleanup()

	res, err := mutate(context.Background(), store, repairUpdatedBy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	switch res {
	case repairChanged:
		fmt.Println(changedMsg)
	case repairNoChange:
		fmt.Println(noChangeMsg)
	case repairNoSection:
		fmt.Println(noSectionMsg)
	}
	return 0
}

// openRepairStore builds an encrypted config store from the host environment.
// The database URL and master encryption key are read from the environment only
// — the repair CLI does not load the YAML config file, so it works even when
// the on-disk config is broken or absent.
func openRepairStore() (store *configstore.Store, cleanup func(), err error) {
	dbURL := os.Getenv("CMM_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		return nil, nil, errors.New("no database URL configured (set CMM_DATABASE_URL or DATABASE_URL)")
	}

	masterKey := os.Getenv("CMM_CREDENTIAL_ENCRYPTION_KEY")
	if masterKey == "" {
		return nil, nil, errors.New("CMM_CREDENTIAL_ENCRYPTION_KEY is required but not set")
	}

	db, err := datastore.Open(dbURL)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to database: %w", err)
	}

	enc, err := secrets.NewEncryptor(masterKey)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("invalid encryption key: %w", err)
	}

	cleanup = func() {
		enc.Close()
		_ = db.Close()
	}
	return configstore.NewStore(db, enc), cleanup, nil
}

// loadTLSSection reads and decodes the server.tls section. It returns
// (nil, repairNoSection's caller-signal) handling via ok=false when no section
// exists so the mutations stay free of shadowing-write side effects.
func loadTLSSection(ctx context.Context, store *configstore.Store) (tls config.TLSConfig, ok bool, err error) {
	raw, getErr := store.Get(ctx, configstore.KeyServerTLS)
	if errors.Is(getErr, configstore.ErrNotFound) {
		return config.TLSConfig{}, false, nil
	}
	if getErr != nil {
		return config.TLSConfig{}, false, fmt.Errorf("reading server.tls from config store: %w", getErr)
	}
	if err := configstore.DeserializeValue(raw, &tls); err != nil {
		return config.TLSConfig{}, false, fmt.Errorf("decoding server.tls: %w", err)
	}
	return tls, true, nil
}

// saveTLSSection re-serialises and stores the server.tls section (non-secret).
func saveTLSSection(ctx context.Context, store *configstore.Store, tls config.TLSConfig, updatedBy string) error {
	raw, err := configstore.SerializeValue(tls)
	if err != nil {
		return fmt.Errorf("serialising server.tls: %w", err)
	}
	if err := store.Set(ctx, configstore.KeyServerTLS, raw, false, updatedBy); err != nil {
		return fmt.Errorf("writing server.tls: %w", err)
	}
	return nil
}

// tlsResetMode sets server.tls.mode to "off" so the server starts in plain HTTP
// on the next restart, preserving all other TLS fields. It is the recovery-framed
// alias for `tls mode off` (see tlsSetMode).
func tlsResetMode(ctx context.Context, store *configstore.Store, updatedBy string) (tlsRepairResult, error) {
	return tlsSetMode(ctx, store, "off", updatedBy)
}

// tlsClearCA removes server.tls.ca_path to recover an mTLS lockout while leaving
// TLS enabled. Other fields are preserved.
func tlsClearCA(ctx context.Context, store *configstore.Store, updatedBy string) (tlsRepairResult, error) {
	tls, ok, err := loadTLSSection(ctx, store)
	if err != nil {
		return repairNoChange, err
	}
	if !ok {
		return repairNoSection, nil
	}
	if tls.CAPath == "" {
		return repairNoChange, nil
	}
	tls.CAPath = ""
	if err := saveTLSSection(ctx, store, tls, updatedBy); err != nil {
		return repairNoChange, err
	}
	return repairChanged, nil
}
