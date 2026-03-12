# ToDo — Project Setup

Status key: [ ] Not started | [~] In progress | [x] Done

- [ ] Fix 50+ `errcheck` linter violations and re-enable the linter — `errcheck` is temporarily disabled in `.golangci.yml`. Violations span `frontend/embed_test.go`, `internal/analysis/`, `internal/chefapi/`, `internal/collector/`, `internal/config/`, `internal/datastore/`, `internal/frontend/`, `internal/remediation/`, `internal/secrets/`, and `internal/tls/`. Common patterns: unchecked `defer f.Close()`, `defer rows.Close()`, `defer stmt.Close()`, `defer resp.Body.Close()`, logging calls (`log.Debug`, `log.Warn`, `log.Info`, `log.Error`), and test helper calls (`os.WriteFile`, `os.Remove`, `os.Mkdir`, `w.Write`).