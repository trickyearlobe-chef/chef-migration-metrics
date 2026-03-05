# ToDo — Project Setup

Status key: [ ] Not started | [~] In progress | [x] Done

- [ ] Initialise Go repository structure (`go mod init`)
- [x] Add LICENSE file (Apache 2.0)
- [x] Add README.md with project overview and getting started guide
- [x] Document technology stack (Go backend, React frontend)
- [x] Create `.gitignore` with patterns for build output, Go, Node.js, IDE, OS, secrets, runtime data, and test artifacts
- [x] Create `.dockerignore` to keep Docker build context small and exclude secrets from the daemon
- [x] Create `.helmignore` at `deploy/helm/chef-migration-metrics/.helmignore` for Helm chart packaging
- [x] Add ignore file maintenance rule to `Claude.md`
- [ ] Set up Go dependency management (`go.mod`, `go.sum`)
- [x] Set up CI pipeline
- [ ] Set up database migration tooling (`golang-migrate/migrate` or equivalent)
- [ ] Create `migrations/` directory and establish migration file naming convention
- [ ] Implement automatic migration execution on application startup
- [ ] Verify pending migrations cause startup failure with a descriptive error
- [x] Create `Makefile` with build, test, lint, package, version bump, and functional test targets