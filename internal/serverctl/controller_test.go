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
	"testing"
	"time"
)

// httpBuilder returns a build closure that binds a real listener at addr:port
// and serves a body equal to its own bound address, so a client can tell which
// instance answered. Each returned closure builds one instance.
func httpBuilder(t *testing.T, addr string, port int) func() (*Instance, error) {
	t.Helper()
	return func() (*Instance, error) {
		ln, err := net.Listen("tcp", net.JoinHostPort(addr, strconv.Itoa(port)))
		if err != nil {
			return nil, err
		}
		body := ln.Addr().String()
		srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, body)
		})}
		go func() { _ = srv.Serve(ln) }()
		return &Instance{Addr: body, Shutdown: srv.Shutdown}, nil
	}
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
	ctrl := New(func() time.Duration { return 2 * time.Second }, nil)

	portA := freePort(t)
	bootInst, err := httpBuilder(t, "127.0.0.1", portA)()
	if err != nil {
		t.Fatalf("boot build: %v", err)
	}
	ctrl.Adopt(bootInst, fmt.Sprintf("plain|127.0.0.1:%d", portA))

	// Old serving before rebind.
	if got, err := get(bootInst.Addr); err != nil || got != bootInst.Addr {
		t.Fatalf("pre-rebind get(%s) = %q, %v", bootInst.Addr, got, err)
	}

	portB := freePort(t)
	newAddr := fmt.Sprintf("127.0.0.1:%d", portB)
	changed, err := ctrl.Rebind(fmt.Sprintf("plain|%s", newAddr), httpBuilder(t, "127.0.0.1", portB))
	if err != nil || !changed {
		t.Fatalf("Rebind = changed %v, err %v; want true, nil", changed, err)
	}

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

// A failed build (e.g. the new target cannot bind) is a no-op: the old instance
// keeps serving and the error is returned.
func TestRebind_BuildFailureKeepsOld(t *testing.T) {
	ctrl := New(nil, nil)

	portA := freePort(t)
	bootInst, err := httpBuilder(t, "127.0.0.1", portA)()
	if err != nil {
		t.Fatalf("boot build: %v", err)
	}
	ctrl.Adopt(bootInst, fmt.Sprintf("plain|127.0.0.1:%d", portA))

	// Occupy a port so the rebind target cannot be bound.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy: %v", err)
	}
	defer occupied.Close()
	badPort := occupied.Addr().(*net.TCPAddr).Port

	changed, err := ctrl.Rebind(fmt.Sprintf("plain|127.0.0.1:%d", badPort), httpBuilder(t, "127.0.0.1", badPort))
	if err == nil {
		t.Fatalf("Rebind to occupied port: want error, got nil")
	}
	if changed {
		t.Errorf("Rebind changed = true on build failure; want false")
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

// Rebinding to the key already serving is a no-op (build is not called and
// nothing is torn down), even when a build closure is supplied.
func TestRebind_UnchangedKeyIsNoop(t *testing.T) {
	var builds int
	build := func() (*Instance, error) {
		builds++
		return &Instance{Addr: "0.0.0.0:8080", Shutdown: func(context.Context) error { return nil }}, nil
	}
	ctrl := New(nil, nil)
	boot, _ := build()
	builds = 0 // discount the boot build
	ctrl.Adopt(boot, "plain|0.0.0.0:8080")

	changed, err := ctrl.Rebind("plain|0.0.0.0:8080", build)
	if err != nil {
		t.Fatalf("Rebind unchanged: %v", err)
	}
	if changed {
		t.Errorf("Rebind to same key reported changed; want no-op")
	}
	if builds != 0 {
		t.Errorf("build called %d times on no-op rebind; want 0", builds)
	}
}

// A key whose address/port are unchanged but whose variant differs (e.g. an
// off↔static mode toggle on the same port) is NOT a no-op: build is invoked and
// the instance swapped. This proves the opaque key — not the bound address —
// drives the comparison. The build returns a fake instance (no real bind) because
// the point under test is that the controller does not short-circuit.
func TestRebind_SameAddrDifferentVariantRebuilds(t *testing.T) {
	ctrl := New(func() time.Duration { return 2 * time.Second }, nil)

	drained := make(chan struct{})
	boot := &Instance{Addr: "127.0.0.1:8443", Shutdown: func(context.Context) error { close(drained); return nil }}
	ctrl.Adopt(boot, "plain|127.0.0.1:8443")

	var built int
	next := &Instance{Addr: "127.0.0.1:8443", Shutdown: func(context.Context) error { return nil }}
	build := func() (*Instance, error) {
		built++
		return next, nil
	}
	// Same addr:port, different variant.
	changed, err := ctrl.Rebind("tls|127.0.0.1:8443", build)
	if err != nil || !changed {
		t.Fatalf("Rebind variant change = changed %v, err %v; want true, nil", changed, err)
	}
	if built != 1 {
		t.Errorf("build called %d times; want 1", built)
	}
	if ctrl.CurrentKey() != "tls|127.0.0.1:8443" {
		t.Errorf("CurrentKey = %q, want tls|127.0.0.1:8443", ctrl.CurrentKey())
	}
	// The old instance drains in the background.
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Error("old instance was not drained after a variant rebind")
	}
}

// RebindInPlace retires the old instance first (releasing its port) and then
// builds the replacement, swapping it in. Used for a same-port variant change.
func TestRebindInPlace_DrainsOldThenBuilds(t *testing.T) {
	ctrl := New(func() time.Duration { return 2 * time.Second }, nil)

	drained := make(chan struct{})
	boot := &Instance{Addr: "127.0.0.1:8443", Shutdown: func(context.Context) error { close(drained); return nil }}
	ctrl.Adopt(boot, "plain|127.0.0.1:8443")

	var built int
	next := &Instance{Addr: "127.0.0.1:8443", Shutdown: func(context.Context) error { return nil }}
	build := func() (*Instance, error) {
		built++
		// The old instance must already be draining when build runs.
		select {
		case <-drained:
		case <-time.After(time.Second):
			t.Error("old instance not drained before build (release-old-first violated)")
		}
		return next, nil
	}
	changed, err := ctrl.RebindInPlace("tls|127.0.0.1:8443", build)
	if err != nil || !changed {
		t.Fatalf("RebindInPlace = changed %v, err %v; want true, nil", changed, err)
	}
	if built != 1 {
		t.Errorf("build called %d times; want 1", built)
	}
	if ctrl.CurrentKey() != "tls|127.0.0.1:8443" {
		t.Errorf("CurrentKey = %q, want tls|127.0.0.1:8443", ctrl.CurrentKey())
	}
}

// When the replacement build fails after the old instance was released, nothing
// is left serving: the error is returned and CurrentAddr is empty.
func TestRebindInPlace_BuildFailureLeavesNothing(t *testing.T) {
	ctrl := New(func() time.Duration { return 2 * time.Second }, nil)

	drained := make(chan struct{})
	boot := &Instance{Addr: "127.0.0.1:8443", Shutdown: func(context.Context) error { close(drained); return nil }}
	ctrl.Adopt(boot, "plain|127.0.0.1:8443")

	build := func() (*Instance, error) {
		return nil, fmt.Errorf("bind failed")
	}
	changed, err := ctrl.RebindInPlace("tls|127.0.0.1:8443", build)
	if err == nil || changed {
		t.Fatalf("RebindInPlace = changed %v, err %v; want false, error", changed, err)
	}
	if ctrl.CurrentAddr() != "" {
		t.Errorf("CurrentAddr = %q after failed in-place rebind; want empty", ctrl.CurrentAddr())
	}
	// The old instance was still released.
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Error("old instance was not drained on a failed in-place rebind")
	}
}

// Rebind before any Adopt is an error (there is nothing to retire).
func TestRebind_NoCurrentIsError(t *testing.T) {
	ctrl := New(nil, nil)
	if _, err := ctrl.Rebind("plain|127.0.0.1:0", httpBuilder(t, "127.0.0.1", freePort(t))); err == nil {
		t.Fatal("Rebind with no current instance: want error, got nil")
	}
}
