// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package serverctl

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"
)

// httpBuild returns a BuildFunc that binds a real listener at the requested
// target and serves a body equal to its own bound address, so a client can tell
// which instance answered. Bound addresses are recorded for assertions.
func httpBuild(t *testing.T) (BuildFunc, *sync.Map) {
	t.Helper()
	served := &sync.Map{} // addr -> *http.Server (for cleanup)
	build := func(addr string, port int) (*Instance, error) {
		ln, err := net.Listen("tcp", net.JoinHostPort(addr, strconv.Itoa(port)))
		if err != nil {
			return nil, err
		}
		body := ln.Addr().String()
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, body)
		})}
		served.Store(body, srv)
		go func() { _ = srv.Serve(ln) }()
		return &Instance{Addr: body, Shutdown: srv.Shutdown}, nil
	}
	return build, served
}

// get fetches the body served at addr, or an error if the connection fails.
func get(addr string) (string, error) {
	c := &http.Client{Timeout: time.Second}
	resp, err := c.Get("http://" + addr + "/")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// Rebind binds the new target and serves it, then drains the old listener so a
// follow-up request hits the new instance and the old stops accepting.
func TestRebind_BindsNewServesNew_DrainsOld(t *testing.T) {
	build, _ := httpBuild(t)
	ctrl := New(build, func() time.Duration { return 2 * time.Second }, nil)

	portA := freePort(t)
	bootInst, err := build("127.0.0.1", portA)
	if err != nil {
		t.Fatalf("boot build: %v", err)
	}
	ctrl.Adopt(bootInst, "127.0.0.1", portA)

	// Old serving before rebind.
	if got, err := get(bootInst.Addr); err != nil || got != bootInst.Addr {
		t.Fatalf("pre-rebind get(%s) = %q, %v", bootInst.Addr, got, err)
	}

	portB := freePort(t)
	changed, err := ctrl.Rebind("127.0.0.1", portB)
	if err != nil || !changed {
		t.Fatalf("Rebind = changed %v, err %v; want true, nil", changed, err)
	}

	newAddr := fmt.Sprintf("127.0.0.1:%d", portB)
	if ctrl.CurrentAddr() != newAddr {
		t.Errorf("CurrentAddr = %q, want %q", ctrl.CurrentAddr(), newAddr)
	}
	if got, err := get(newAddr); err != nil || got != newAddr {
		t.Fatalf("post-rebind get(%s) = %q, %v; want new instance", newAddr, got, err)
	}

	// The old listener drains in the background; it must stop accepting.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := get(bootInst.Addr); err != nil {
			break // old closed
		}
		if time.Now().After(deadline) {
			t.Fatalf("old listener %s still accepting after rebind", bootInst.Addr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A failed bind of the new target is a no-op: the old instance keeps serving and
// the bind error is returned.
func TestRebind_BindFailureKeepsOld(t *testing.T) {
	build, _ := httpBuild(t)
	ctrl := New(build, nil, nil)

	portA := freePort(t)
	bootInst, err := build("127.0.0.1", portA)
	if err != nil {
		t.Fatalf("boot build: %v", err)
	}
	ctrl.Adopt(bootInst, "127.0.0.1", portA)

	// Occupy a port so the rebind target cannot be bound.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy: %v", err)
	}
	defer occupied.Close()
	badPort := occupied.Addr().(*net.TCPAddr).Port

	changed, err := ctrl.Rebind("127.0.0.1", badPort)
	if err == nil {
		t.Fatalf("Rebind to occupied port: want error, got nil")
	}
	if changed {
		t.Errorf("Rebind changed = true on bind failure; want false")
	}
	if ctrl.CurrentAddr() != bootInst.Addr {
		t.Errorf("CurrentAddr = %q after failed rebind; want unchanged %q", ctrl.CurrentAddr(), bootInst.Addr)
	}
	// Old still serving.
	if got, err := get(bootInst.Addr); err != nil || got != bootInst.Addr {
		t.Fatalf("old listener not serving after failed rebind: %q, %v", got, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = bootInst.Shutdown(ctx)
}

// Rebinding to the address/port already serving is a no-op (the BuildFunc is not
// called and nothing is torn down).
func TestRebind_UnchangedTargetIsNoop(t *testing.T) {
	var builds int
	build := func(addr string, port int) (*Instance, error) {
		builds++
		return &Instance{Addr: fmt.Sprintf("%s:%d", addr, port), Shutdown: func(context.Context) error { return nil }}, nil
	}
	ctrl := New(build, nil, nil)
	boot, _ := build("0.0.0.0", 8080)
	builds = 0 // discount the boot build
	ctrl.Adopt(boot, "0.0.0.0", 8080)

	// "" normalises to 0.0.0.0, so this is the same target.
	changed, err := ctrl.Rebind("", 8080)
	if err != nil {
		t.Fatalf("Rebind unchanged: %v", err)
	}
	if changed {
		t.Errorf("Rebind to same target reported changed; want no-op")
	}
	if builds != 0 {
		t.Errorf("BuildFunc called %d times on no-op rebind; want 0", builds)
	}
}

// Rebind before any Adopt is an error (there is nothing to retire).
func TestRebind_NoCurrentIsError(t *testing.T) {
	build, _ := httpBuild(t)
	ctrl := New(build, nil, nil)
	if _, err := ctrl.Rebind("127.0.0.1", freePort(t)); err == nil {
		t.Fatal("Rebind with no current instance: want error, got nil")
	}
}
