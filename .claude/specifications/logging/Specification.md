# Logging - Component Specification

> Component specification for the Chef Migration Metrics logging subsystem.
> See the [top-level specification](../Specification.md) for project context.

---

## TL;DR

All components emit structured JSON log entries persisted to the PostgreSQL datastore and viewable in the web UI log viewer. Each entry has: timestamp, severity (`DEBUG`/`INFO`/`WARN`/`ERROR`), scope, and contextual metadata (org, cookbook, commit SHA, etc.). Log scopes: `collection`, `git_operation`, `test_kitchen`, `cookstyle_scan`, `readiness_evaluation`, `notification_dispatch`, `export_job`. External process stdout/stderr (TK, CookStyle, git) is captured and associated with the relevant scope. Configurable minimum severity level and retention period with automated purge.

---

## Overview

All components of Chef Migration Metrics must emit structured logs that are persisted to the datastore and viewable from the web UI log viewer. This enables operators to diagnose failures in the batch collection job, git operations, Test Kitchen runs, and CookStyle scans without requiring access to the underlying host or log files.

---

## Log Scopes

Logs are organised into scopes that correspond to the unit of work being performed. Each log entry belongs to exactly one scope. The defined scopes are:

| Scope | Description |
|-------|-------------|
| `collection_run` | A periodic node data collection run for a single Chef server organisation |
| `git_operation` | A clone or pull operation for a single cookbook git repository |
| `test_kitchen_run` | A Test Kitchen execution for a single cookbook against a single target Chef Client version |
| `cookstyle_scan` | A CookStyle scan for a single cookbook version sourced from the Chef server |
| `notification_dispatch` | A notification delivery attempt to a configured channel (webhook or email) |
| `export_job` | A data export operation (ready nodes, blocked nodes, or cookbook remediation report) |
| `tls` | TLS certificate lifecycle events — mode selection, certificate loading, reload, ACME issuance, renewal, expiry warnings, and errors |

---

## Log Entry Structure

Every log entry must include the following fields:

| Field | Type | Description |
|-------|------|-------------|
| `id` | UUID | Unique identifier for the log entry |
| `timestamp` | ISO-8601 UTC | Time at which the event occurred |
| `severity` | Enum | One of `DEBUG`, `INFO`, `WARN`, `ERROR` |
| `scope` | Enum | One of the scopes defined above |
| `message` | String | Human-readable description of the event |
| `organisation` | String | Chef server organisation name, where applicable |
| `cookbook_name` | String | Cookbook name, where applicable |
| `cookbook_version` | String | Cookbook version, where applicable (Chef server cookbooks) |
| `commit_sha` | String | Git commit SHA, where applicable (git-sourced cookbooks) |
| `chef_client_version` | String | Target Chef Client version, where applicable (Test Kitchen runs) |
| `process_output` | String | Captured stdout/stderr from an external process, where applicable |
| `notification_channel` | String | Name of the notification channel, where applicable (notification_dispatch scope) |
| `export_job_id` | String | Export job ID, where applicable (export_job scope) |
| `tls_domain` | String | Domain name associated with a TLS certificate event, where applicable (tls scope) |

Fields that are not applicable to a given scope may be omitted or set to null.

---

## Severity Levels

| Level | Usage |
|-------|-------|
| `DEBUG` | Detailed diagnostic information, typically only useful during development |
| `INFO` | Normal operational events (job started, job completed, cookbook skipped, etc.) |
| `WARN` | Unexpected but recoverable conditions (e.g. a cookbook repository has no default branch) |
| `ERROR` | Failures that require attention (e.g. Chef API authentication failure, Test Kitchen convergence failure) |

---

## External Process Output

When the application invokes an external process (Test Kitchen, CookStyle, git), the combined stdout and stderr of that process must be captured in full and stored in the `process_output` field of the associated log entry. This ensures that the full output of a failed Test Kitchen run or CookStyle scan is available in the web UI without requiring access to the host.

---

## Retention

Log entries must be retained for a configurable period (see [Configuration specification](../configuration/Specification.md)). Entries older than the configured retention period must be automatically purged from the datastore during each collection run.

---

## Configuration

The following logging-related settings are exposed via the application configuration:

| Setting | Description | Default |
|---------|-------------|---------|
| `log_level` | Minimum severity level to persist. Entries below this level are discarded. | `INFO` |
| `log_retention_days` | Number of days to retain log entries in the datastore before purging. | `90` |

---

## Notification Logging

Notification delivery attempts must be logged with the `notification_dispatch` scope:

- **Successful deliveries** are logged at `INFO` severity with the channel name, event type, and a summary of the notification content.
- **Failed deliveries** are logged at `ERROR` severity with the channel name, event type, error message, and retry count.
- **Retries** are logged at `WARN` severity with the retry attempt number and delay before the next attempt.
- The notification payload is not stored in the log entry `process_output` field (it is stored in the `notification_history` table instead — see [Datastore Specification](../datastore/Specification.md)).

---

## Export Job Logging

Export operations are logged with the `export_job` scope:

- **Export requests** are logged at `INFO` severity with the export type, format, filters, and requesting username.
- **Export completions** are logged at `INFO` severity with the row count, file size, and duration.
- **Export failures** are logged at `ERROR` severity with the error message and any partial results.

---

## TLS Logging

TLS certificate lifecycle events are logged with the `tls` scope. See the [TLS and Certificate Management specification](../tls/Specification.md) for full details on the TLS subsystem.

### Startup Events

| Event | Severity | Message |
|-------|----------|---------|
| TLS mode selected | `INFO` | "TLS mode: off" or "TLS mode: static (cert: /path/to/cert.pem)" or "TLS mode: acme (domains: [example.com])" |
| Certificate loaded | `INFO` | "TLS certificate loaded: subject=CN=example.com, issuer=..., expires=2025-01-01T00:00:00Z" |
| mTLS enabled | `INFO` | "Mutual TLS enabled: client certificates validated against /path/to/ca.pem" |
| HTTP redirect listener started | `INFO` | "HTTP-to-HTTPS redirect listener started on :80" |
| ACME staging CA detected | `WARN` | "ACME CA URL is a staging endpoint — certificates will not be trusted by clients" |
| ACME ToS not accepted | `ERROR` | "ACME Terms of Service not accepted. Set server.tls.acme.agree_to_tos: true. ToS URL: \<url\>" |
| TLS disabled warning | `WARN` | "TLS is disabled — authentication traffic will not be encrypted. Use a TLS-terminating reverse proxy in production." |
| Certificate expired at startup | `WARN` | "TLS certificate for [example.com] is expired (expired 2025-01-01T00:00:00Z). Serving with expired certificate." |
| Key file permissions too open | `WARN` | "TLS private key file /path/to/key.pem has permissions 0644 — should be 0600 or more restrictive" |

### Certificate Reload Events (Static Mode)

| Event | Severity | Message |
|-------|----------|---------|
| Certificate reloaded | `INFO` | "TLS certificate reloaded from disk" |
| Certificate reload failed | `ERROR` | "TLS certificate reload failed: \<reason\>. Continuing with previous certificate." |
| SIGHUP received | `INFO` | "Received SIGHUP — reloading TLS certificate" |
| Certificate file change detected | `INFO` | "TLS certificate file change detected — reloading" |

### ACME Events

| Event | Severity | Message |
|-------|----------|---------|
| ACME certificate obtained | `INFO` | "ACME certificate obtained for [example.com], expires 2025-04-01T00:00:00Z" |
| ACME certificate renewed | `INFO` | "ACME certificate renewed for [example.com], new expiry 2025-07-01T00:00:00Z" |
| ACME renewal failed | `ERROR` | "ACME certificate renewal failed for [example.com]: \<error\>. Current certificate expires 2025-04-01T00:00:00Z" |
| Certificate expiring soon | `WARN` | "TLS certificate for [example.com] expires in 5 days. Renewal has not succeeded." |
| ACME challenge started | `DEBUG` | "ACME HTTP-01 challenge started for example.com" |
| ACME challenge completed | `DEBUG` | "ACME HTTP-01 challenge completed for example.com" |
| ACME challenge failed | `ERROR` | "ACME HTTP-01 challenge failed for example.com: \<error\>" |
| ACME DNS record created | `DEBUG` | "ACME DNS-01: TXT record created at _acme-challenge.example.com via cloudflare" |
| ACME DNS record cleaned up | `DEBUG` | "ACME DNS-01: TXT record removed from _acme-challenge.example.com" |

The `tls_domain` field is populated for all `tls`-scoped log entries where a specific domain is relevant (certificate load, ACME issuance, renewal, challenges). For general TLS events (mode selection, redirect listener startup), the field is omitted or null.

---

## Web UI Log Viewer

The log viewer is documented in the [Visualisation specification](../visualisation/Specification.md). This component specification covers only the production and storage of log entries; the viewer specification covers display, filtering, and navigation.

---

## Related Specifications

- [Top-level Specification](../Specification.md)
- [Visualisation Specification](../visualisation/Specification.md) — log viewer UI
- [Configuration Specification](../configuration/Specification.md) — `logging.level` and `logging.retention_days`
- [TLS and Certificate Management Specification](../tls/Specification.md) — defines the TLS events logged under the `tls` scope