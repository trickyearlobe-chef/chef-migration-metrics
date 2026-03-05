# Logging Infrastructure — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

- [ ] Implement structured logging with consistent severity levels (`DEBUG`, `INFO`, `WARN`, `ERROR`)
- [ ] Include contextual metadata in each log entry (timestamp, severity, organisation, cookbook name, commit SHA as applicable)
- [ ] Persist log entries to the datastore
- [ ] Capture stdout/stderr from external processes and associate with the relevant log scope
- [ ] Implement log retention period configuration and automated purge of expired logs
- [ ] Implement log level configuration to control minimum persisted severity
- [ ] Implement `notification_dispatch` log scope for notification delivery logging
- [ ] Implement `export_job` log scope for export operation logging