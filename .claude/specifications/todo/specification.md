# ToDo — Specification

Status key: [ ] Not started | [~] In progress | [x] Done

- [x] Write top-level project specification
- [x] Write component specification: Data Collection (node collection, cookbook fetching)
- [x] Write component specification: Analysis (cookbook usage, compatibility testing, readiness)
- [x] Write component specification: Data Visualisation (dashboard, trending)
- [x] Write component specification: Configuration
- [x] Write component specification: Authentication and Authorisation (SAML, LDAP, local accounts)
- [x] Write component specification: Logging
- [x] Write component specification: Datastore schema
- [x] Write component specification: Web API (HTTP layer between backend and frontend)
- [x] Write component specification: Packaging and Deployment (RPM, DEB, container, Docker Compose, Helm)
- [x] Write component specification: Elasticsearch Export (NDJSON export, Logstash pipeline, ELK testing stack)
- [x] Write database migration files for initial schema — `migrations/0001_initial_schema.up.sql` and `migrations/0001_initial_schema.down.sql` exist
- [x] Document background job scheduling and recovery behaviour
- [x] Populate specifications/chef-api/Specification.md with relevant API endpoint references
- [x] Flesh out analysis specification with design decisions (Test Kitchen invocation, CookStyle parsing, disk space evaluation)
- [x] Add Policyfile support to data collection, analysis, visualisation, web API, datastore, and configuration specifications
- [x] Add stale node detection to data collection, analysis, visualisation, web API, datastore, and configuration specifications
- [x] Add stale cookbook detection to data collection, visualisation, datastore, and configuration specifications
- [x] Add remediation guidance (auto-correct preview, migration docs, complexity scoring) to analysis, visualisation, web API, and datastore specifications
- [x] Add role dependency graph to data collection, visualisation, web API, and datastore specifications
- [x] Add data export capability to visualisation, web API, datastore, and configuration specifications
- [x] Add notification system (webhook, email) to visualisation, web API, datastore, logging, and configuration specifications
- [x] Add confidence indicators to visualisation and web API specifications
- [x] Add CookStyle version profiles to analysis specification
- [x] Add cookbook upload date / first-seen tracking to data collection and datastore specifications
- [x] Update Chef API specification with policy_name, policy_group, and ohai_time partial search attributes
- [x] Add embedded Ruby environment (CookStyle, Test Kitchen, kitchen-dokken) to packaging, analysis, and configuration specifications
- [x] Add `analysis_tools` configuration section (embedded_bin_dir, timeouts) to configuration specification
- [x] Add Elasticsearch export specification (document types, NDJSON format, Logstash pipeline, ELK stack)
- [x] Update analysis specification for embedded tool resolution and startup validation
- [x] Remove Dockerfile.analysis from packaging specification — single image now includes embedded tools
- [x] Write component specification: Secrets Storage (credential encryption, storage methods, resolution, rotation, Kubernetes integration)