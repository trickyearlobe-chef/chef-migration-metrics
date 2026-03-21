# ChefSpec Scanner Integration — Specification

| Field          | Value                              |
|----------------|------------------------------------|
| **Status**     | Draft                              |
| **Authors**    | Chef Migration Metrics Team        |
| **Created**    | 2025-07-17                         |
| **Updated**    | 2025-07-17                         |

---

## 1. Overview

This specification describes the integration of a ChefSpec scanner into the
chef-migration-metrics application. ChefSpec is a unit-testing framework for
Chef cookbooks that runs recipes in-memory using ChefZero and validates resource
convergence behaviour via RSpec.

ChefSpec occupies a distinct position between static analysis and full
integration testing:

| Tool         | Analysis Type       | Infrastructure   | Confidence | Speed  |
|--------------|---------------------|-------------------|------------|--------|
| CookStyle    | Static analysis     | None              | Low–Medium | Fast   |
| ChefSpec     | Unit testing        | Docker (ChefZero) | Medium     | Medium |
| Test Kitchen | Integration testing | Docker/VM         | High       | Slow   |

Adding ChefSpec scanning provides an intermediate confidence signal that catches
runtime convergence errors, conditional logic bugs, and resource interaction
problems without requiring a full VM or multi-phase kitchen converge/verify
cycle.

### 1.1 Goals

- Run ChefSpec tests against git-sourced cookbooks that contain spec files.
- Execute all ChefSpec runs inside Docker containers to isolate side effects.
- Parse structured output and persist results per cookbook per target version.
- Feed pass/fail outcomes into the complexity scoring formula.
- Display results in the git repo detail page and dashboard.
- Follow the architectural patterns established by the existing CookStyle and
  Test Kitchen scanners.

### 1.2 Non-Goals

- Running ChefSpec against server-sourced cookbook snapshots. They lack the full
  repository context (Gemfile, spec_helper, fixtures) that ChefSpec requires.
- Modifying or generating ChefSpec tests. The scanner runs them as authored.
- Supporting non-RSpec test frameworks such as Minitest.
- Running ChefSpec directly on the host. All execution must be containerised.

---

## 2. Why Docker

ChefSpec tests run recipes through ChefZero, which performs in-memory
convergence of Chef resources. Although ChefZero does not actually execute
provider actions against the real operating system, cookbook code frequently
makes direct Ruby calls outside of Chef resources — file operations, shell
commands, library helpers, and `ruby_block` resources that execute arbitrary
code. These side effects can modify the host filesystem, install packages, or
alter system state in ways that are difficult to predict or clean up.

Running ChefSpec inside a disposable Docker container provides:

- **Isolation** — any side effects from Ruby calls, native gem compilation, or
  resource stubs that leak through are contained and discarded when the
  container exits.
- **Reproducibility** — the container image pins the Ruby version, Chef gem
  version, and OS libraries, eliminating "works on my machine" variance between
  collection hosts.
- **Consistency with Test Kitchen** — Test Kitchen already requires Docker. By
  sharing the same Docker dependency, ChefSpec adds no new infrastructure
  requirements.
- **Target version testing** — different container images can bundle different
  Chef Infra Client versions, allowing ChefSpec to run against the configured
  target Chef versions without contaminating the host's gem environment.

The scanner should build or reference a Docker image that contains Ruby, Bundler,
and the target Chef Infra Client gem. The cookbook directory is bind-mounted into
the container, and `bundle exec rspec` runs inside it. The container is destroyed
after each run.

---

## 3. Detection

### 3.1 New detection flag

The existing `has_test_suite` flag on the git repo record is too coarse. It is
set whenever any of `.kitchen.yml`, `test/`, or `spec/` exists. A cookbook may
have a `spec/` directory containing only a `spec_helper.rb` with no actual spec
files, or may have only InSpec integration tests under `test/`.

A new boolean column `has_chefspec` should be added to the `git_repos` table and
a corresponding field to the `GitRepo` struct. This flag is set during the
clone-or-pull phase of collection, alongside the existing `has_test_suite`
detection.

### 3.2 Detection logic

The detection function should use `git ls-tree` (the same approach used by
`detectTestSuite`) to inspect committed files under `spec/` without depending on
the working tree. It should look for at least one file matching the `_spec.rb`
suffix under the `spec/` directory tree. A repository that has `spec/` containing
only `spec_helper.rb` or other non-spec files should result in `has_chefspec`
being false.

### 3.3 Database migration

Add a `has_chefspec` boolean column to the `git_repos` table, defaulting to
false. Update the upsert query to include the new column in both the insert and
on-conflict-update clauses.

---

## 4. Configuration

### 4.1 Config structure

Add a `ChefSpecConfig` struct nested under the existing `analysis_tools` YAML
key, following the same pattern as `TestKitchenConfig`. The struct should
contain:

- **enabled** — boolean pointer defaulting to true when omitted. Controls
  whether ChefSpec scanning is active.
- **docker_image** — string specifying the Docker image to use for ChefSpec
  runs. When empty, the scanner should use a default image name that includes
  the target Chef version (e.g. `chef-migration-metrics/chefspec:<target>`).
- **extra_args** — string slice of additional CLI arguments passed to every
  rspec invocation. Useful for excluding tagged examples (e.g. `--tag ~slow`).

Add a `chefspec_timeout_minutes` integer field to `AnalysisToolsConfig` for the
per-run timeout, and a `chefspec_run` integer field to `ConcurrencyConfig` for
the worker pool size.

### 4.2 YAML layout

The configuration should nest under `analysis_tools.chefspec` with keys
`enabled`, `docker_image`, and `extra_args`. The timeout sits at
`analysis_tools.chefspec_timeout_minutes` and the concurrency at
`concurrency.chefspec_run`.

### 4.3 Tool detection

Add a Docker availability check to the embedded tool resolver if one does not
already exist. Since Test Kitchen already requires Docker, the resolver's
existing `DockerEnabled` flag can be reused. The ChefSpec scanner requires
Docker to be available. No additional host-level tools (rspec, bundler) are
needed because everything runs inside the container.

The three-way startup gate follows the existing pattern: if Docker is not
available, the scanner is not created. If Docker is available but the config
sets `enabled: false`, the scanner is not created and a log message is emitted.
Otherwise the scanner is created and injected into the collector.

---

## 5. Docker Execution Model

### 5.1 Image strategy

The scanner should support two image modes:

**Pre-built image** — when `docker_image` is configured, that image is used
directly. The operator is responsible for ensuring the image contains Ruby,
Bundler, and a compatible Chef Infra Client gem. This is the recommended
approach for air-gapped environments or when custom gems are needed.

**Default image** — when `docker_image` is empty, the scanner should attempt to
use a convention-based image name that incorporates the target Chef version. The
first run for a given target version may need to pull or build the image. The
image should be based on a minimal Ruby base (e.g. `ruby:3.1-slim`) with the
target Chef gem pre-installed.

### 5.2 Container lifecycle

For each ChefSpec run (one cookbook, one target version), the scanner should:

1. Start a new container from the configured or default image.
2. Bind-mount the cookbook checkout directory as a read-only volume.
3. Copy or mount a temporary working directory for Bundler caches and any
   generated files, so the read-only source is not modified.
4. Run `bundle install` followed by `bundle exec rspec --format json spec/`
   inside the container.
5. Capture stdout (JSON output) and stderr separately.
6. Remove the container after the run completes (equivalent to `docker run
   --rm`).

The container should run with no network access (`--network none`) unless the
cookbook's Gemfile requires fetching gems, in which case network access is
permitted during `bundle install` but should ideally be disabled for the actual
rspec execution.

### 5.3 Timeout handling

The scanner should enforce a per-run timeout using Docker's `--stop-timeout`
flag and a context deadline. If the container exceeds the timeout, it is killed
and the result is recorded with `timed_out` set to true.

### 5.4 Target Chef version

The target Chef version should be passed to the container as an environment
variable. If using the default image strategy, different target versions may
use different images. If using a pre-built image with multiple Chef versions
available, the environment variable controls which version ChefSpec loads.

---

## 6. Data Storage

### 6.1 Table design

Create a `git_repo_chefspec_results` table following the same conventions as
the existing `git_repo_test_kitchen_results` table. The table should store:

- **Identity** — auto-generated UUID primary key, foreign key to `git_repos`
  with cascade delete.
- **Scope** — target Chef version and commit SHA at the time of the run.
- **Outcome** — boolean `passed` flag, integer counts for examples, failures,
  pending, and errors. A boolean `timed_out` flag.
- **Duration** — integer `duration_seconds`.
- **Failure details** — JSONB column containing an array of failure objects,
  each with the example description, file path, line number, and error message.
  This provides drill-down capability without storing the entire rspec output.
- **Raw output** — text columns for process stdout and stderr, retained for
  debugging.
- **Timestamps** — `started_at`, `completed_at`, and `created_at`.

The table should have a unique constraint on `(git_repo_id, target_chef_version)`
to support upsert semantics. Indexes should cover `git_repo_id` and `passed`.

### 6.2 Datastore struct and CRUD methods

Create a `GitRepoChefSpecResult` struct with JSON tags matching the column names.
Create an upsert params struct for insert operations.

The following CRUD methods should be implemented on the datastore, following the
established patterns for the test kitchen results:

- **Upsert** — insert or update on the unique constraint, returning the full
  result row.
- **Get latest** — retrieve the most recent result for a given repo ID and
  target version, used by the skip-if-unchanged check.
- **List by repo ID** — return all results for a repo, used by the detail API.
- **List by name** — join with `git_repos` to look up by cookbook name.
- **Delete by repo ID** — bulk delete for forced re-test (rescan action).

---

## 7. Scanner Implementation

### 7.1 Location and structure

The scanner should live in `internal/analysis/chefspec.go` with tests in
`internal/analysis/chefspec_test.go`. It should follow the same structural
patterns as the CookStyle and Test Kitchen scanners: an executor interface for
testability, a scanner struct holding configuration and dependencies, functional
options for dependency injection in tests, and separate batch/single entry
points.

### 7.2 Executor interface

Define a `ChefSpecExecutor` interface with a single `Run` method that accepts a
context, working directory, and variadic string arguments, returning stdout,
stderr, exit code, and error. Follow the existing convention where a non-zero
exit code is not treated as an error if the process ran to completion — this is
important because rspec exits with code 1 when tests fail, which is a valid
(non-error) outcome.

The default executor implementation should invoke `docker run` with the
appropriate flags: `--rm` for cleanup, bind-mount for the cookbook directory,
environment variables for the target Chef version, and the rspec command as the
container entrypoint arguments.

### 7.3 Batch entry point

The `TestGitRepos` method should accept a slice of git repo records, a list of
target Chef versions, and a directory resolver function. It should:

1. Filter to repos where `has_chefspec` is true.
2. Skip repos with an empty head commit SHA or no local clone directory.
3. Build a cross-product of repos and target versions as work items.
4. Execute work items through a bounded worker pool (semaphore channel sized to
   the configured concurrency).
5. Return an aggregate batch result with counts for total, tested, skipped,
   passed, failed, and errors.

### 7.4 Per-repo flow

Each work item should follow a pipeline:

1. **Skip check** — query the database for an existing result at the same repo
   ID and target version. If the stored commit SHA matches the repo's current
   head commit SHA, skip the run. This avoids re-running tests when the
   cookbook code has not changed.

2. **Container setup** — prepare the Docker run command with the cookbook
   directory bind-mounted, the target Chef version set as an environment
   variable, and the timeout configured.

3. **Bundle install** — run `bundle install` inside the container. If the
   cookbook has no Gemfile, fall back to running rspec directly with the
   system-installed gems in the image. Log a warning if bundle install fails
   and record the result as an error.

4. **Execute rspec** — run `bundle exec rspec --format json spec/` (plus any
   configured extra arguments) inside the container. Capture stdout and stderr.

5. **Handle failures** — differentiate between timeouts (container killed),
   process crashes (no valid output), and test failures (non-zero exit with
   valid JSON output).

6. **Parse output** — unmarshal the RSpec JSON output to extract example count,
   failure count, pending count, duration, and individual failure details
   (description, file path, line number, exception message).

7. **Determine outcome** — set `passed` to true when the failure count is zero.

8. **Log** — emit structured log messages under the `chefspec_run` scope.

9. **Persist** — upsert the result to the database. Persistence errors should
   be logged but not halt execution.

### 7.5 RSpec JSON output

RSpec's `--format json` produces a JSON object with an `examples` array and a
`summary` object. Each example has fields for description, status (passed,
failed, pending), file path, line number, run time, and an optional exception
object with class, message, and backtrace. The summary contains duration,
example count, failure count, pending count, and errors-outside-of-examples
count. The scanner should parse this structure and extract the relevant fields
for storage.

---

## 8. Collector Integration

### 8.1 Collector option

Add a functional option `WithChefSpecScanner` that sets the scanner on the
collector struct, following the same pattern as `WithCookstyleScanner` and
`WithKitchenScanner`. The scanner field should be a pointer that is nil when
ChefSpec scanning is not configured.

### 8.2 Pipeline position

ChefSpec should run as Step 11b in the collection pipeline, after CookStyle
scanning (Step 11) and before Test Kitchen testing (Step 12):

| Step | Description                        | Scanner           |
|------|------------------------------------|-------------------|
| 11   | CookStyle scanning (git repos)     | cookstyleScanner  |
| 11b  | ChefSpec testing (git repos)       | chefspecScanner   |
| 12   | Test Kitchen testing (git repos)   | kitchenScanner    |
| 13   | Autocorrect previews + complexity  | autocorrectGen    |

This ordering is intentional. ChefSpec is faster than Test Kitchen and shares
the Docker dependency, so running it first provides quicker feedback. There is
no data dependency between Steps 11b and 12, so a future optimisation could run
them in parallel.

### 8.3 Guard clause

The guard clause should follow the existing three-way pattern: skip if the
scanner is nil, if the git repo directory resolver is nil, or if no target Chef
versions are configured. Log aggregate batch results after completion.

### 8.4 Startup wiring

In `main.go`, the scanner should be created when Docker is available and the
ChefSpec config is enabled (the two-gate pattern). The scanner constructor
receives the database, logger, concurrency, timeout, and ChefSpec config. It
is injected into the collector via the functional option.

---

## 9. Complexity Scoring Integration

### 9.1 New weight constant

Add a `WeightChefSpecFail` constant with a value of 8. This positions ChefSpec
failures between CookStyle per-offense weights (1–5) and Test Kitchen failures
(10–20) in the scoring hierarchy:

| Signal                    | Weight | Rationale                                     |
|---------------------------|--------|-----------------------------------------------|
| CookStyle modernize       | 1      | Style suggestion, not a real problem           |
| CookStyle deprecation     | 3      | Will break eventually                          |
| CookStyle correctness     | 3      | Likely bug                                     |
| CookStyle manual fix      | 4      | Requires human intervention                    |
| CookStyle error/fatal     | 5      | Definite problem                               |
| **ChefSpec failure**      | **8**  | **Unit test failure — real convergence problem in-memory** |
| Test Kitchen test fail    | 10     | Integration test failure — high confidence     |
| Test Kitchen converge fail| 20     | Will not even converge — critical              |

ChefSpec failures have higher confidence than static analysis (they exercise
actual convergence logic) but lower than Test Kitchen (which runs against a real
OS with actual package managers, services, and filesystems).

### 9.2 New input struct

Add a `ChefSpecSummary` struct to the complexity input, containing boolean
fields for `HasResult` and `Passed`, and an integer `FailureCount`. This
follows the same pattern as the existing `TestKitchenSummary`.

### 9.3 Updated score formula

The `ComputeComplexityScore` function should be extended to add
`WeightChefSpecFail` when a ChefSpec result exists and is not passed. This
check should sit between the CookStyle weights and the Test Kitchen weights in
the function body.

### 9.4 Scorer integration

The `scoreOneGitRepo` function should load the latest ChefSpec result for the
repo and target version (using the same get-latest pattern as Test Kitchen) and
populate the `ChefSpecSummary` on the complexity input. Errors loading ChefSpec
results should be logged as warnings but not prevent scoring.

---

## 10. Web API

### 10.1 DataStore interface

Add three methods to the `DataStore` interface:

- List ChefSpec results by git repo ID (for the detail response).
- Get the latest ChefSpec result for a repo and target version (for the
  complexity scorer).
- Delete ChefSpec results by repo ID (for the rescan action).

### 10.2 Git repo detail response

Update the git repo detail entry struct to include a `chefspec` field containing
a slice of ChefSpec results, positioned between `cookstyle` and `test_kitchen`
in the JSON output. Populate it by calling the list method for each repo.

### 10.3 Cookbook detail response

Update the cookbook detail's git repo sub-struct to also include the `chefspec`
field, so ChefSpec results are visible when viewing a cookbook that has a git
source.

### 10.4 Rescan endpoint

Update the existing git repo rescan handler to also delete ChefSpec results
alongside the existing CookStyle, complexity, and autocorrect preview cleanup.
This ensures a full rescan re-runs ChefSpec.

### 10.5 Reset endpoint

No additional work is needed. The existing git repo reset deletes the `git_repos`
row, and the cascade delete on the foreign key automatically removes associated
ChefSpec results.

---

## 11. Frontend

### 11.1 TypeScript types

Add a `ChefSpecResult` interface with fields for id, git repo ID, target Chef
version, commit SHA, passed, example count, failure count, pending count, error
count, timed out, duration seconds, and timestamps.

Update the `GitRepoDetail` interface to include an optional `chefspec` array of
`ChefSpecResult`.

Update the `GitRepo` interface to include the `has_chefspec` boolean detection
flag.

### 11.2 Git repo detail page

Add a ChefSpec Results section between the existing CookStyle and Test Kitchen
sections. When results exist, render a card per target version showing: target
version, passed/failed badge, timed-out indicator (if applicable), example,
failure, and pending counts, and duration with completion timestamp.

When no results exist, show a contextual empty state:

- If `has_chefspec` is true: display a "Not Yet Run" badge with a message
  explaining that ChefSpec tests were detected but have not run yet, and will
  appear after the next collection run.
- If `has_chefspec` is false: display a "No Specs" badge with a message
  explaining that the repository does not contain ChefSpec tests, and suggest
  adding `spec/*_spec.rb` files to enable unit testing.

### 11.3 Status badge tooltip

Add a tooltip entry for a `chefspec_only` compatibility variant: "Unit tests
(ChefSpec) passed — medium confidence, no integration test."

### 11.4 Dashboard integration

Add a ChefSpec summary card to the dashboard showing aggregate counts of passed,
failed, and untested cookbooks. This follows the same pattern as the existing
CookStyle and Test Kitchen compatibility cards. The backend should expose the
aggregated data through the existing dashboard endpoint or a new sub-endpoint.

### 11.5 Readiness evaluation impact

The readiness evaluator should incorporate ChefSpec results as an additional
confidence tier. The compatibility verdict hierarchy becomes:

| CookStyle | ChefSpec | Test Kitchen | Verdict                     |
|-----------|----------|--------------|-----------------------------|
| Passed    | Passed   | Passed       | Compatible (high confidence)|
| Passed    | Passed   | Not run      | Compatible (medium confidence, ChefSpec only) |
| Passed    | Not run  | Not run      | Compatible (low–medium confidence, CookStyle only) |
| Passed    | Failed   | Any          | Incompatible                |
| Failed    | Any      | Any          | Incompatible                |

This introduces a new compatibility tier between the existing Test Kitchen–backed
"compatible" and the CookStyle-only "compatible_cookstyle_only" verdicts.

---

## 12. Logging

Add a new logging scope `chefspec_run` following the existing `test_kitchen_run`
and `cookstyle_scan` scope conventions.

Log messages should cover: batch start with work item count, batch completion
with aggregate counts, individual pass with example count and duration,
individual failure with failure-to-example ratio and duration, timeout events,
skip events with commit SHA, and process-level errors.

---

## 13. Admin Actions

Add a "Rescan All ChefSpec" button to the admin actions page. It should delete
all rows from the ChefSpec results table and any associated complexity records,
then respond with the count of deleted rows. ChefSpec tests will re-run on the
next collection cycle.

The existing per-repo git reset already handles cleanup via cascade delete, so
no additional per-repo admin action is needed.

---

## 14. Testing Strategy

### 14.1 Scanner unit tests

Tests should cover: repos with `has_chefspec` set to false are skipped, repos
with an unchanged commit SHA are skipped, valid RSpec JSON output is parsed
correctly, zero failures results in passed being true, non-zero failures results
in passed being false with failure details extracted, container timeout results
in timed-out being true, garbled or empty stdout is recorded as an error rather
than a test failure, bundle install failure is recorded as an error with a
warning logged, and the worker pool respects the concurrency limit.

All scanner tests should use the executor interface with a test fake rather than
invoking Docker.

### 14.2 Detection tests

Tests should cover: a repository with `spec/unit/default_spec.rb` is detected
as having ChefSpec, a repository with only `spec/spec_helper.rb` is not, a
repository with no `spec/` directory is not, and a repository with an empty
`spec/` directory is not.

### 14.3 Complexity scoring tests

Tests should cover: a ChefSpec failure adds the expected weight to the score, a
ChefSpec pass contributes zero, no ChefSpec result contributes zero, and a
combined scenario with CookStyle offenses plus ChefSpec failure plus Test Kitchen
failure produces the correct total.

### 14.4 Datastore and API tests

Follow the existing patterns: datastore tests exercise CRUD operations against
a test database, and API handler tests use mock store implementations added to
the test mock struct.

---

## 15. Implementation Order

The implementation should be delivered in the following sequence:

1. Database migration — add `has_chefspec` column to `git_repos` and create the
   `git_repo_chefspec_results` table.
2. Configuration — add the ChefSpec config struct, concurrency setting, and
   timeout setting.
3. Detection — implement the ChefSpec detection function, update the
   clone-or-pull flow and the git repo upsert query.
4. Datastore CRUD — implement the results file with upsert, get, list, and
   delete methods.
5. Scanner — implement the ChefSpec scanner with Docker-based execution.
6. Collector integration — add the collector option and wire into the collection
   pipeline at Step 11b.
7. Startup wiring — create the scanner in main with the two-gate pattern.
8. Complexity scoring — add the weight constant, summary struct, update the
   score formula and the git repo scorer.
9. Web API — update the DataStore interface, detail response structs, and rescan
   handler.
10. Frontend types — update TypeScript interfaces.
11. Frontend UI — add the ChefSpec section to the git repo detail page with
    empty-state handling.
12. Dashboard — add the ChefSpec summary card.
13. Admin actions — add the rescan-all button.
14. Logging — add the chefspec_run scope.
15. Tests — unit tests across all layers.
16. Documentation — update the README and configuration reference.

---

## 16. Future Considerations

### 16.1 Parallel ChefSpec and Test Kitchen

Steps 11b and 12 have no data dependency and could run concurrently. This would
require coordinating the worker pools to stay within overall Docker resource
limits. An errgroup or similar pattern could manage this.

### 16.2 Coverage metrics

RSpec can produce coverage reports via SimpleCov. A future enhancement could
extract line and branch coverage percentages and store them alongside test
results, providing a coverage metric in the dashboard.

### 16.3 Failure trend tracking

The current upsert-on-unique-constraint approach overwrites previous results. A
future enhancement could track pass/fail trends over time to detect regressions.

### 16.4 Selective re-runs

When a cookbook has many spec files but only a subset changed between commits,
rspec could be invoked with specific file paths derived from `git diff`. This
optimisation should only be considered after the baseline implementation is
stable.

### 16.5 Shared Docker image with Test Kitchen

If both ChefSpec and Test Kitchen use the same base images, image pulls and
builds could be shared. This is particularly valuable in bandwidth-constrained
environments where pulling large images is expensive.