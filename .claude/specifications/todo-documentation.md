# Documentation — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

- [ ] Document installation and deployment
- [ ] Document installation via RPM package
- [ ] Document installation via DEB package
- [ ] Document installation via container image (Docker)
- [ ] Document local development with Docker Compose
- [ ] Document Kubernetes deployment with Helm chart
- [ ] Document configuration reference
- [ ] Document Chef server API credentials setup
- [ ] Document git repository URL configuration
- [ ] Document authentication provider setup (SAML, LDAP, local)
- [ ] Document Policyfile support (what is collected, how to filter by policy name/group)
- [ ] Document stale node and stale cookbook detection (thresholds, dashboard indicators)
- [ ] Document remediation guidance features (auto-correct preview, migration docs, complexity scoring)
- [ ] Document dependency graph view (how to read the graph, filtering, table alternative)
- [ ] Document data export functionality (formats, Chef search query usage with knife ssh)
- [ ] Document notification configuration (webhook setup for Slack/Teams/PagerDuty, email/SMTP setup, trigger configuration)
- [ ] Document confidence indicators (high vs. medium vs. untested)
- [ ] Document cookbook complexity scoring model (weights, labels, blast radius)
- [ ] Document embedded analysis tools (CookStyle, Test Kitchen embedded in all packages, no Chef Workstation required, Docker is only external dep for TK)
- [ ] Document `analysis_tools` configuration section (embedded_bin_dir, timeouts, PATH fallback for dev environments)
- [ ] Document Elasticsearch export setup (enable in config, configure output directory, start ELK stack)
- [ ] Document ELK testing stack usage (`deploy/elk/` Docker Compose, Kibana dashboard creation, suggested visualisations)
- [ ] Document Logstash pipeline configuration and how to keep it in sync with document types
- [ ] Document building the embedded Ruby environment from source (`make build-embedded`)
- [ ] Document contributing guidelines