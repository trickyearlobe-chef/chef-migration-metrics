// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"net"
	"strconv"
)

// TestBind verifies that the given listen address and port can be bound right
// now by opening and immediately closing a TCP listener. It is used as a
// save-time preflight so an operator cannot persist a listen_address/port the
// server cannot bind — which would otherwise force the bind-failure fallback
// and degraded mode on the next restart (encrypted-config-store.md).
//
// An empty address is treated as 0.0.0.0 and a zero port as 8080, matching the
// listener defaults.
func TestBind(listenAddr string, port int) error {
	if listenAddr == "" {
		listenAddr = "0.0.0.0"
	}
	if port == 0 {
		port = 8080
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(listenAddr, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return ln.Close()
}
