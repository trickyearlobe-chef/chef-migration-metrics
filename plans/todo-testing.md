# Testing — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

- [ ] Unit tests for stale cookbook detection logic
- [ ] Unit tests for Elasticsearch NDJSON export (document format, doc_id generation, .tmp suffix handling)
- [ ] Unit tests for Elasticsearch high-water-mark tracking (incremental export, first-run full export)
- [ ] Integration tests for Elasticsearch export pipeline (write NDJSON → Logstash → Elasticsearch → Kibana query)
- [ ] End-to-end test covering collection → analysis → remediation → dashboard display
- [ ] Verify RPM installs, starts, and runs on a fresh RHEL/Rocky/Alma system (with embedded tools)
- [ ] Verify DEB installs, starts, and runs on a fresh Debian/Ubuntu system (with embedded tools)
- [ ] Verify Docker Compose stack starts and passes health checks
- [ ] Verify ELK testing stack starts and Logstash indexes test data into Elasticsearch

