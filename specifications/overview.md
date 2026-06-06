# Specifications Overview

> Index of every component specification for Chef Migration Metrics. This is the routing layer for agents: find the relevant spec here, then open it. Large specs are a thin index plus `<spec>-<section>.md` part files — open only the section you need. `CLAUDE.md` (repo root) holds the working rules; `project-conventions.md` holds Go/DB/frontend conventions.

| Specification | Summary |
|---------------|---------|
| [analysis](analysis.md) | Cookbook usage analysis, compatibility testing (Test Kitchen + CookStyle), remediation guidance, and per-node upgrade readiness. |
| [auth](auth.md) | Authentication: local bcrypt accounts and SAML 2.0 SSO, with viewer/operator/admin RBAC. |
| [backup-restore](backup-restore.md) | Backup and restore of the datastore and application configuration. |
| [batch-estimate](batch-estimate.md) | Estimation and sizing of migration batches. |
| [blast-radius](blast-radius.md) | Impact analysis — how many nodes, roles, and policies a given cookbook affects. |
| [bulk-kitchen-scanning](bulk-kitchen-scanning.md) | Bulk scanning of cookbooks through Test Kitchen. |
| [chef-api](chef-api.md) | Chef Infra Server API client and reference used by data collection. |
| [configuration](configuration.md) | The application's full configuration surface, live-reloadable without restart. |
| [data-collection](data-collection.md) | Collecting node, cookbook, and git-repo data from Chef servers and Git. |
| [data-export](data-export.md) | Exporting metrics and data (synchronous and async job-based). |
| [datastore](datastore.md) | PostgreSQL schema and data-access layer. DDL lives in `migrations/`; see the stub for the table map. |
| [dependency-graph](dependency-graph.md) | Cookbook and role dependency graph construction and queries. |
| [deployment-dashboard](deployment-dashboard.md) | Per-version deployment progress dashboard. |
| [diagnostic-bundle](diagnostic-bundle.md) | Support/diagnostic bundle for offline troubleshooting (VDI/file-transfer constraints). |
| [dual-compatibility-signals](dual-compatibility-signals.md) | Combining Test Kitchen and CookStyle signals into a compatibility verdict. |
| [encrypted-config-store](encrypted-config-store.md) | Encrypted storage backend for configuration values. |
| [enriched-metric-snapshots](enriched-metric-snapshots.md) | Periodic enriched metric snapshots for historical trending. |
| [filter-ux-overhaul](filter-ux-overhaul.md) | Dashboard filtering UX across org, environment, role, policy, platform, and owner. |
| [git-repo-file-browser](git-repo-file-browser.md) | Browsing files within cloned git cookbook repositories. |
| [kitchen-analyser](kitchen-analyser.md) | Analysis of Test Kitchen configurations and results. |
| [kitchen-instance-exclusions](kitchen-instance-exclusions.md) | Excluding specific kitchen instances from test runs. |
| [kitchen-refactor](kitchen-refactor.md) | Refactor of the Test Kitchen subsystem. |
| [kitchen-run-queue](kitchen-run-queue.md) | Queueing and scheduling of Test Kitchen runs. |
| [logging](logging.md) | Structured JSON logging persisted to PostgreSQL and viewable in the web log viewer. |
| [node-list-enhancements](node-list-enhancements.md) | Enhancements to the node list view (columns, filters). |
| [ownership](ownership.md) | Ownership model: owners, assignments, bulk reassignment, audit log, and committer-to-owner workflows. |
| [packaging](packaging.md) | RPM/DEB/Docker packaging, including the embedded Ruby/Test Kitchen environment. |
| [parallel-deployment-tracking](parallel-deployment-tracking.md) | Tracking parallel, per-version deployment progress across the fleet. |
| [performance-diagnostics](performance-diagnostics.md) | Performance diagnostics and profiling. |
| [platform-display-grouping](platform-display-grouping.md) | Grouping platforms for display in the UI. |
| [platform-display-names](platform-display-names.md) | Friendly display names for platforms. |
| [platform-mapping-ui](platform-mapping-ui.md) | UI for mapping platforms to Test Kitchen images. |
| [roles](roles.md) | Chef roles and their cookbook references. |
| [secrets-storage](secrets-storage.md) | Credential storage, encryption, master key management, and rotation. |
| [semantic-contracts](semantic-contracts.md) | Semantic API/data contracts between components. |
| [server-side-pagination](server-side-pagination.md) | Server-side pagination for list endpoints. |
| [staleness-tiers](staleness-tiers.md) | Node staleness tiers and thresholds. |
| [system-health](system-health.md) | Host/system metrics collection and the admin system-stats page. |
| [test-kitchen-config-ui](test-kitchen-config-ui.md) | UI for Test Kitchen configuration. |
| [test-kitchen-drivers](test-kitchen-drivers.md) | Multi-driver Test Kitchen abstraction (dokken, vcenter, ec2, …): overlays, credentials, platform mapping. |
| [tls](tls.md) | TLS listening modes: off (plain HTTP), static cert/key, and ACME (Let's Encrypt). |
| [ui-polish-phase7](ui-polish-phase7.md) | UI polish work, phase 7. |
| [version-battery-bars](version-battery-bars.md) | "Battery bar" visualisation of per-version upgrade progress. |
| [visualisation](visualisation.md) | Dashboard and log-viewer visualisations. |
| [web-api](web-api.md) | RESTful JSON API (Go) between the backend and the React frontend. |
| [websocket-log-streaming](websocket-log-streaming.md) | WebSocket streaming of live logs to the UI. |
