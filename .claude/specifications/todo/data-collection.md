# Data Collection — ToDo

Status key: [ ] Not started | [~] In progress | [x] Done

---

## Node Collection

- [ ] Implement Chef Infra Server API client in Go with native RSA signed request authentication (no external signing libraries)
- [ ] Implement partial search against node index (`POST /organizations/NAME/search/node`)
- [ ] Collect required node attributes:
  - [ ] `name`
  - [ ] `chef_environment`
  - [ ] `automatic.chef_packages.chef.version`
  - [ ] `automatic.platform` and `automatic.platform_version`
  - [ ] `automatic.platform_family`
  - [ ] `automatic.filesystem` (disk space)
  - [ ] `automatic.cookbooks` (resolved cookbook list)
  - [ ] `run_list`
  - [ ] `automatic.roles` (expanded)
  - [ ] `policy_name` (Policyfile policy name, top-level attribute)
  - [ ] `policy_group` (Policyfile policy group, top-level attribute)
  - [ ] `automatic.ohai_time` (Unix timestamp of last Chef client run)
- [ ] Support multiple Chef server organisations
- [ ] Collect from multiple organisations in parallel using goroutines (one per organisation)
- [ ] Bound organisation collection concurrency with the `concurrency.organisation_collection` worker pool setting
- [ ] Use `errgroup` or equivalent to coordinate goroutines and aggregate errors without cancelling successful collections
- [ ] Implement concurrent pagination within a single organisation — fetch pages in parallel once total node count is known, bounded by the `concurrency.node_page_fetching` worker pool setting
- [ ] Implement periodic background collection job
- [ ] Implement fault tolerance — failure in one organisation must not block others
- [ ] Implement checkpoint/resume so failed jobs can continue without starting over
- [ ] Persist collected node data to datastore with timestamps

## Policyfile Support

- [ ] Classify nodes as Policyfile nodes (both `policy_name` and `policy_group` non-null) or classic nodes
- [ ] Persist `policy_name` and `policy_group` in node snapshots
- [ ] Ensure cookbook usage analysis works identically for Policyfile and classic nodes (both use `automatic.cookbooks`)

## Stale Node Detection

- [ ] After collection, compare each node's `ohai_time` against the current time
- [ ] Flag nodes whose `ohai_time` is older than `collection.stale_node_threshold_days` (default: 7) as stale in the datastore
- [ ] Persist `is_stale` flag on `node_snapshots` rows
- [ ] Include stale flagging in the collection run sequence (step 5, after cookbook fetching)

## Cookbook Fetching

- [ ] Implement cookbook fetch from Chef server (`GET /organizations/NAME/cookbooks/NAME/VERSION`)
- [ ] Skip download of cookbook versions already present in the datastore (immutability optimisation)
- [ ] Key all Chef server cookbook data in the datastore by organisation + cookbook name + version
- [ ] Implement manual rescan option to force re-download and re-analysis of a specific cookbook version
- [ ] Implement cookbook clone from git repository
- [ ] Support multiple configured base git URLs
- [ ] Pull latest changes from git repositories on every collection run
- [ ] Run git pull operations across multiple repositories in parallel using goroutines, bounded by the `concurrency.git_pull` worker pool setting
- [ ] Detect default branch automatically (`main` or `master`)
- [ ] Record HEAD commit SHA for the default branch after each pull
- [ ] Detect whether a fetched cookbook includes a test suite
- [ ] Record `first_seen_at` timestamp for each cookbook version (proxy for upload date if Chef server does not expose one)
- [ ] Flag cookbooks as stale when most recent version's `first_seen_at` is older than `collection.stale_cookbook_threshold_days` (default: 365)

## Role Dependency Graph Collection

- [ ] Fetch full list of roles per organisation using `GET /organizations/ORG/roles`
- [ ] Fetch role detail per role using `GET /organizations/ORG/roles/ROLE_NAME`
- [ ] Parse each role's `run_list` to extract cookbook references (`recipe[cookbook::recipe]`) and nested role references (`role[other_role]`)
- [ ] Build directed graph of role → role and role → cookbook dependencies
- [ ] Persist dependency graph to the `role_dependencies` table in the datastore
- [ ] Refresh dependency graph on every collection run