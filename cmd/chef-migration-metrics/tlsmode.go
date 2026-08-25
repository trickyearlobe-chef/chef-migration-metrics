// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/trickyearlobe-chef/chef-migration-metrics/internal/configstore"
)

// `tls mode <off|static|acme>` is the deliberate-deployment counterpart to the
// recovery-framed `tls reset` . It sets
// server.tls.mode in the DB; `mode off` plus `--trusted-proxy` puts the app in
// plain-HTTP-behind-a-TLS-terminating-proxy mode by also setting
// server.trusted_proxy so HSTS / scheme detection honour X-Forwarded-Proto.
// Same env-only store, section-preserving read-modify-write, and idempotency as
// the Chunk 3a repair CLI; no break-glass override (recovery boundary = host
// access).

const trustedProxyFlag = "--trusted-proxy"

// validTLSMode reports whether m is an accepted server.tls.mode value.
func validTLSMode(m string) bool {
	switch m {
	case "off", "static", "acme":
		return true
	default:
		return false
	}
}

// parseTLSModeArgs extracts the mode token and the optional --trusted-proxy flag
// from the `tls mode` arguments (which may appear in any order). A bare
// --trusted-proxy means true; --trusted-proxy=<bool> parses the value. The
// returned *bool is nil when the flag is absent (trusted_proxy left untouched).
func parseTLSModeArgs(args []string) (mode string, tp *bool, err error) {
	for _, arg := range args {
		switch {
		case arg == trustedProxyFlag:
			t := true
			tp = &t
		case strings.HasPrefix(arg, trustedProxyFlag+"="):
			v, perr := strconv.ParseBool(strings.TrimPrefix(arg, trustedProxyFlag+"="))
			if perr != nil {
				return "", nil, fmt.Errorf("invalid value for %s: %q (want true or false)", trustedProxyFlag, strings.TrimPrefix(arg, trustedProxyFlag+"="))
			}
			tp = &v
		case strings.HasPrefix(arg, "-"):
			return "", nil, fmt.Errorf("unknown flag %q", arg)
		default:
			if mode != "" {
				return "", nil, fmt.Errorf("unexpected extra argument %q", arg)
			}
			mode = arg
		}
	}
	if mode == "" {
		return "", nil, errors.New("missing mode (want off, static, or acme)")
	}
	if !validTLSMode(mode) {
		return "", nil, fmt.Errorf("invalid mode %q (want off, static, or acme)", mode)
	}
	return mode, tp, nil
}

// tlsSetMode sets server.tls.mode to mode, preserving all other TLS fields. It
// never creates a section where none exists (that would shadow a YAML-managed
// config). It is the generalised form of tlsResetMode.
func tlsSetMode(ctx context.Context, store *configstore.Store, mode, updatedBy string) (tlsRepairResult, error) {
	tls, ok, err := loadTLSSection(ctx, store)
	if err != nil {
		return repairNoChange, err
	}
	if !ok {
		return repairNoSection, nil
	}
	if tls.Mode == mode {
		return repairNoChange, nil
	}
	tls.Mode = mode
	if err := saveTLSSection(ctx, store, tls, updatedBy); err != nil {
		return repairNoChange, err
	}
	return repairChanged, nil
}

// tlsSetTrustedProxy sets server.trusted_proxy to val. The key defaults to false
// when absent, so setting false on a store without the key is a no-op (no orphan
// key written).
func tlsSetTrustedProxy(ctx context.Context, store *configstore.Store, val bool, updatedBy string) (tlsRepairResult, error) {
	cur, err := loadTrustedProxy(ctx, store)
	if err != nil {
		return repairNoChange, err
	}
	if cur == val {
		return repairNoChange, nil
	}
	raw, err := configstore.SerializeValue(val)
	if err != nil {
		return repairNoChange, fmt.Errorf("serialising server.trusted_proxy: %w", err)
	}
	if err := store.Set(ctx, configstore.KeyServerTrustedProxy, raw, false, updatedBy); err != nil {
		return repairNoChange, fmt.Errorf("writing server.trusted_proxy: %w", err)
	}
	return repairChanged, nil
}

// loadTrustedProxy reads the server.trusted_proxy scalar, defaulting to false
// when the key is absent.
func loadTrustedProxy(ctx context.Context, store *configstore.Store) (bool, error) {
	raw, err := store.Get(ctx, configstore.KeyServerTrustedProxy)
	if errors.Is(err, configstore.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading server.trusted_proxy: %w", err)
	}
	var v bool
	if err := configstore.DeserializeValue(raw, &v); err != nil {
		return false, fmt.Errorf("decoding server.trusted_proxy: %w", err)
	}
	return v, nil
}

// runTLSMode handles `tls mode <off|static|acme> [--trusted-proxy[=bool]]`.
// args is os.Args after the `tls mode` tokens. It returns a process exit code.
func runTLSMode(args []string) int {
	mode, tp, err := parseTLSModeArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n\n", err)
		printTLSUsage()
		return 2
	}

	store, cleanup, err := openRepairStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer cleanup()

	ctx := context.Background()

	modeRes, err := tlsSetMode(ctx, store, mode, repairUpdatedBy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if modeRes == repairNoSection {
		fmt.Println("no server.tls configuration found in the database — TLS is not DB-managed; nothing to change")
		return 0
	}

	switch modeRes {
	case repairChanged:
		fmt.Printf("server.tls.mode set to %q\n", mode)
	case repairNoChange:
		fmt.Printf("server.tls.mode is already %q — no change\n", mode)
	}

	if tp != nil {
		tpRes, err := tlsSetTrustedProxy(ctx, store, *tp, repairUpdatedBy)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		switch tpRes {
		case repairChanged:
			fmt.Printf("server.trusted_proxy set to %v\n", *tp)
		case repairNoChange:
			fmt.Printf("server.trusted_proxy is already %v — no change\n", *tp)
		}
	}

	fmt.Println("restart the server to apply.")
	return 0
}
