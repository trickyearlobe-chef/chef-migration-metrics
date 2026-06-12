// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package serverctl owns the process's live HTTP(S) listener and rebinds it in
// place when server.listen_address/port changes, without restarting the
// process (configuration-live-reload.md, listener-rebind).
//
// The rebind protocol is bind-new-first / keep-old / rollback: the new listener
// is bound and serving before the old one is retired, so a failed bind (e.g. the
// port is already held by another process) is a no-op that leaves the running
// service untouched and surfaces the error to the operator — strictly safer than
// a process re-exec, which would tear down in-flight work and could leave the
// service down on an unsupervised host.
//
// The package is deliberately free of application/TLS knowledge: how to bind and
// serve a target is injected as a BuildFunc, so the same protocol drives plain
// HTTP and static TLS listeners and is unit-testable in isolation.
package serverctl

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LogFunc receives operational log lines (level is INFO/ERROR).
type LogFunc func(level, msg string)

// Instance is a bound, serving HTTP(S) endpoint owned by a Controller.
type Instance struct {
	// Addr is the actual bound address (host:port), e.g. from ln.Addr().
	Addr string
	// Shutdown gracefully drains in-flight requests and closes the listener.
	// It is called with a timeout-bearing context when the instance is retired.
	Shutdown func(ctx context.Context) error
}

// BuildFunc binds and starts serving a new Instance at the requested target. It
// MUST bind before returning (bind-new-first): a non-nil error means nothing was
// bound and the caller keeps the previous Instance serving. On success the
// Instance is already accepting connections.
type BuildFunc func(addr string, port int) (*Instance, error)

// Controller owns the live listener and rebinds it in place.
type Controller struct {
	build BuildFunc
	drain func() time.Duration
	log   LogFunc

	mu      sync.Mutex
	current *Instance
	addr    string // normalised configured target address
	port    int    // configured target port
}

// New builds a Controller. drain returns the graceful drain budget for retiring
// the old listener (read live so a saved graceful_shutdown_seconds applies); nil
// defaults to 15s. log nil is a no-op.
func New(build BuildFunc, drain func() time.Duration, log LogFunc) *Controller {
	if log == nil {
		log = func(string, string) {}
	}
	if drain == nil {
		drain = func() time.Duration { return 15 * time.Second }
	}
	return &Controller{build: build, drain: drain, log: log}
}

// Adopt records the boot-time Instance already serving at addr:port, so the
// first Rebind knows what to retire and can detect a no-op.
func (c *Controller) Adopt(inst *Instance, addr string, port int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = inst
	c.addr = normAddr(addr)
	c.port = port
}

// Rebind binds a new Instance at addr:port and, once it is serving, drains and
// closes the old one in the background. It returns:
//
//   - (false, nil) when the target is unchanged — a no-op; the BuildFunc is not
//     called and nothing is torn down.
//   - (true, nil) when the rebind succeeded.
//   - (false, err) when the new bind failed; the old Instance keeps serving and
//     nothing was torn down.
func (c *Controller) Rebind(addr string, port int) (changed bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.current == nil {
		return false, fmt.Errorf("serverctl: no current listener to rebind")
	}
	if normAddr(addr) == c.addr && port == c.port {
		return false, nil
	}

	// Bind-new-first: build and start the new instance before retiring the old.
	// A bind failure here leaves the old instance untouched.
	next, err := c.build(addr, port)
	if err != nil {
		return false, err
	}

	old := c.current
	c.current = next
	c.addr = normAddr(addr)
	c.port = port

	// Drain the old listener in the background: in-flight requests may run for
	// the full graceful budget and the save response must not block on them. The
	// old listener stops accepting new connections immediately (Shutdown closes
	// it first), so traffic moves to the new instance at once.
	go c.drainOld(old)

	return true, nil
}

func (c *Controller) drainOld(old *Instance) {
	ctx, cancel := context.WithTimeout(context.Background(), c.drain())
	defer cancel()
	if err := old.Shutdown(ctx); err != nil {
		c.log("ERROR", fmt.Sprintf("serverctl: draining previous listener %s: %v", old.Addr, err))
		return
	}
	c.log("INFO", fmt.Sprintf("serverctl: previous listener %s drained and closed", old.Addr))
}

// Shutdown gracefully drains and closes the current instance. It is used at
// process shutdown so the live listener drains — which, after a rebind, is one
// the boot serverResult no longer references. No-op when nothing was adopted.
func (c *Controller) Shutdown(ctx context.Context) error {
	c.mu.Lock()
	inst := c.current
	c.mu.Unlock()
	if inst == nil {
		return nil
	}
	return inst.Shutdown(ctx)
}

// CurrentAddr returns the actual bound address of the live instance, or "" when
// none has been adopted.
func (c *Controller) CurrentAddr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current == nil {
		return ""
	}
	return c.current.Addr
}

// normAddr maps an empty listen address to 0.0.0.0 so "" and "0.0.0.0" compare
// equal (both bind all interfaces), matching the listener default elsewhere.
func normAddr(a string) string {
	if a == "" {
		return "0.0.0.0"
	}
	return a
}
