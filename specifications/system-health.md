# System Health Monitoring

## Overview

The CMM application monitors the health of the host machine it runs on,
exposing CPU load, memory usage, and disk space metrics via a dedicated
admin API endpoint. The frontend renders these on an admin-only "System
Stats" page with visual alerts when thresholds are breached. The
collection scheduler uses the same checks as a circuit breaker — pausing
data collection when the host is under resource pressure.

## Goals

- Give administrators early warning when disk space or CPU load is
  dangerously high on the CMM host.
- Automatically pause collection runs when resources are critically low
  to prevent the application from filling the disk or starving other
  processes.
- Avoid external dependencies — use Go stdlib (`os`, `runtime`,
  `syscall`) for all metrics. No `gopsutil` or cgo.

## System Metrics Collected

Moved to [system-health-metrics.md](system-health-metrics.md).

## Package Layout

Moved to [system-health-package-layout.md](system-health-package-layout.md).

## Configuration

Moved to [system-health-configuration.md](system-health-configuration.md).

## API Endpoint

Moved to [system-health-api-endpoint.md](system-health-api-endpoint.md).

## Collection Circuit Breaker

Moved to [system-health-circuit-breaker.md](system-health-circuit-breaker.md).

## Frontend — Admin System Stats Page

Moved to [system-health-frontend.md](system-health-frontend.md).

## Testing

Moved to [system-health-testing-rollout.md](system-health-testing-rollout.md).

## Rollout Considerations

Moved to [system-health-testing-rollout.md](system-health-testing-rollout.md).
