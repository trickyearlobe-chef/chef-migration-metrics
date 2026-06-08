// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package tls

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestTestBindSucceedsOnFreeEphemeralPort(t *testing.T) {
	// Bind an ephemeral port to discover a free one, close it, then assert
	// TestBind can re-bind it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen failed: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	if err := TestBind("127.0.0.1", port); err != nil {
		t.Fatalf("TestBind on free port %d: %v", port, err)
	}
}

func TestTestBindFailsWhenPortInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen failed: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	if err := TestBind("127.0.0.1", port); err == nil {
		t.Fatalf("TestBind expected error on in-use port %d, got nil", port)
	}
}

func TestBindPermissionRemediationOnEACCES(t *testing.T) {
	// A real net.Listen permission error wraps syscall.EACCES through
	// *net.OpError -> *os.SyscallError; simulate that shape.
	err := &net.OpError{
		Op:  "listen",
		Net: "tcp",
		Err: &net.OpError{Err: syscall.EACCES},
	}
	const port = 443
	msg := BindPermissionRemediation("0.0.0.0", port, err)
	if msg == "" {
		t.Fatal("expected remediation message for EACCES, got empty string")
	}
	for _, want := range []string{"CAP_NET_BIND_SERVICE", "semanage", strconv.Itoa(port)} {
		if !strings.Contains(msg, want) {
			t.Errorf("remediation message missing %q: %s", want, msg)
		}
	}
}

func TestBindPermissionRemediationWrappedEACCES(t *testing.T) {
	// errors.Is must see through arbitrary wrapping.
	err := errors.Join(errors.New("listen tcp :443"), syscall.EACCES)
	if got := BindPermissionRemediation("0.0.0.0", 443, err); got == "" {
		t.Fatal("expected remediation for wrapped EACCES, got empty string")
	}
}

func TestBindPermissionRemediationEmptyForOtherErrors(t *testing.T) {
	cases := []error{
		nil,
		syscall.EADDRINUSE,
		errors.New("some other failure"),
	}
	for _, err := range cases {
		if got := BindPermissionRemediation("0.0.0.0", 8080, err); got != "" {
			t.Errorf("expected empty remediation for %v, got %q", err, got)
		}
	}
}
