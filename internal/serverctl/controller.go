// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package serverctl owns the process's live HTTP(S) listener and rebinds it in
// place when the server configuration changes (server.listen_address/port, and
// the off↔static TLS mode transition), without restarting the process.
//
// The rebind protocol is bind-new-first / keep-old / rollback: the new listener
// is bound and serving before the old one is retired, so a failed build (e.g. the
// port is already held by another process) is a no-op that leaves the running
// service untouched and surfaces the error to the operator — strictly safer than
// a process re-exec, which would tear down in-flight work and could leave the
// service down on an unsupervised host.
//
// The package is deliberately free of application/TLS knowledge: the target is an
// opaque key string (so off↔static on the same port still differs) and how to
// bind and serve it is injected as a per-call build closure. The same protocol
// therefore drives plain HTTP and static-TLS listeners and is unit-testable in
// isolation.
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

// BuildFunc binds and starts serving a new Instance. It MUST bind before
// returning (bind-new-first): a non-nil error means nothing was bound and the
// caller keeps the previous Instance serving. On success the Instance is already
// accepting connections.
type BuildFunc func() (*Instance, error)

// Controller owns the live listener and rebinds it in place.
type Controller struct {
	drain func() time.Duration
	log   LogFunc

	mu      sync.Mutex
	current *Instance
	key     string // opaque target fingerprint (e.g. "tls|0.0.0.0:8443")
}

// New builds a Controller. drain returns the graceful drain budget for retiring
// the old listener (read live so a saved graceful_shutdown_seconds applies); nil
// defaults to 15s. log nil is a no-op.
func New(drain func() time.Duration, log LogFunc) *Controller {
	if log == nil {
		log = func(string, string) {}
	}
	if drain == nil {
		drain = func() time.Duration { return 15 * time.Second }
	}
	return &Controller{drain: drain, log: log}
}

// Adopt records the boot-time Instance already serving under key, so the first
// Rebind knows what to retire and can detect a no-op.
func (c *Controller) Adopt(inst *Instance, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = inst
	c.key = key
}

// Rebind builds a new Instance via build and, once it is serving, drains and
// closes the old one in the background (bind-new-first). The opaque key — not the
// bound address — drives no-op detection. Use this when the new target differs
// from the old (a free port), so the new listener can bind before the old is
// retired. It returns:
//
//   - (false, nil) when key is unchanged — a no-op; build is not called and
//     nothing is torn down.
//   - (true, nil) when the rebind succeeded.
//   - (false, err) when build failed; the old Instance keeps serving and nothing
//     was torn down.
func (c *Controller) Rebind(key string, build BuildFunc) (changed bool, err error) {
	return c.swap(key, build, false)
}

// RebindInPlace retires the current Instance FIRST (releasing its bound port),
// then builds the replacement. It is for a same-address:port variant change (e.g.
// off↔static on one port), where bind-new-first is impossible because the old
// listener still holds the port. The caller MUST have already validated that the
// replacement is constructible (e.g. its certificate is usable) so the only
// remaining failure is the bind itself, which build should retry briefly to
// absorb the OS port-release lag. On build failure the old Instance is already
// being retired — nothing is left serving the target — so the error is returned
// and the persisted config recovers on the next restart.
func (c *Controller) RebindInPlace(key string, build BuildFunc) (changed bool, err error) {
	return c.swap(key, build, true)
}

// swap is the shared rebind body. With releaseOldFirst false it binds the new
// instance before retiring the old (bind-new-first); with true it retires the old
// first, freeing the port for a same-target rebuild.
func (c *Controller) swap(key string, build BuildFunc, releaseOldFirst bool) (changed bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.current == nil {
		return false, fmt.Errorf("serverctl: no current listener to rebind")
	}
	if key == c.key {
		return false, nil
	}

	old := c.current

	if releaseOldFirst {
		// Release the port: the old listener stops accepting and drains in-flight
		// in the background (Shutdown closes the listener first), so build can bind
		// the freed port. A build failure leaves nothing serving the target.
		go c.drainOld(old)
		next, err := build()
		if err != nil {
			c.current = nil
			c.key = ""
			return false, err
		}
		c.current = next
		c.key = key
		return true, nil
	}

	// Bind-new-first: build and start the new instance before retiring the old.
	// A build failure here leaves the old instance untouched.
	next, err := build()
	if err != nil {
		return false, err
	}

	c.current = next
	c.key = key

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

// CurrentKey returns the opaque target key of the live instance, or "" when none
// has been adopted. The caller owns key construction, so it can compare a desired
// key against the current one — e.g. to detect a same-address:port variant change
// that cannot bind-new-first.
func (c *Controller) CurrentKey() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.key
}
