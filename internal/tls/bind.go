// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
)

// TestBind verifies that the given listen address and port can be bound right
// now by opening and immediately closing a TCP listener. It is used as a
// save-time preflight so an operator cannot persist a listen_address/port the
// server cannot bind — which would otherwise force the bind-failure fallback
// and degraded mode on the next restart.
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

// BindPermissionRemediation returns operator-facing remediation guidance when a
// listener bind was denied by the OS (an EACCES permission error), and an empty
// string otherwise — so callers append it only when it is relevant. A
// permission denial on a non-root service almost always means one of two OS
// layers blocked the bind: a privileged port (<1024) without
// CAP_NET_BIND_SERVICE, or an SELinux port label that does not permit it. The
// message names both layers and the affected port so the operator can act
// without guessing which one fired. See journeys/service-access.md.
func BindPermissionRemediation(listenAddr string, port int, err error) string {
	if err == nil || !errors.Is(err, syscall.EACCES) {
		return ""
	}
	if listenAddr == "" {
		listenAddr = "0.0.0.0"
	}
	if port == 0 {
		port = 8080
	}
	return fmt.Sprintf(
		"binding %s:%d was denied by the OS. If this is a privileged port (<1024), "+
			"a non-root service needs the CAP_NET_BIND_SERVICE capability — the packaged "+
			"systemd unit grants it via AmbientCapabilities; for a manual/dev run: "+
			"sudo setcap cap_net_bind_service=+ep <binary>. On an enforcing SELinux host "+
			"the port must also be labelled: sudo semanage port -a -t http_port_t -p tcp %d",
		listenAddr, port, port)
}
