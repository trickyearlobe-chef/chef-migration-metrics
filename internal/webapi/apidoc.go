// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package webapi

// What each address is for, in one line, keyed by "METHOD path".
//
// This is the one hand-written part of the API description, because nothing can
// derive purpose from a route table. It is safe to hand-write only because the
// *set* is not: the generator emits every recorded route whether or not there
// is a line here, and the journey suite names every operation that says nothing
// about itself. So a missing line is visible, and a line for an address that no
// longer exists is visible too.
//
// Write for somebody who has never seen this system and is choosing between a
// long list of capabilities — an assistant does exactly that, and picks wrong
// when two entries read alike.
var apiDocs = map[string]string{
	"DELETE /api/v1/admin/backups/{id}":                                  "Delete a backup.",
	"DELETE /api/v1/admin/config/test-kitchen":                           "Go back to the shipped test machine settings.",
	"DELETE /api/v1/admin/credentials/{name}":                            "Remove a stored secret.",
	"DELETE /api/v1/admin/users/{username}":                              "Remove a local account.",
	"DELETE /api/v1/cookstyle/custom-cops/{name}":                        "Remove a static check written here.",
	"DELETE /api/v1/cookstyle/scan-scope":                                "Go back to scanning everything.",
	"DELETE /api/v1/git-repos/{name}/exclude":                            "Put a previously excluded repository back into scanning.",
	"DELETE /api/v1/hypervisor/vms/{id}/destroy":                         "Destroy one test machine.",
	"DELETE /api/v1/kitchen/batches/{id}":                                "Delete a batch.",
	"DELETE /api/v1/kitchen/git/exclusions/{id}":                         "Put a repository back into test runs.",
	"DELETE /api/v1/kitchen/node-runs/{id}":                              "Delete a recorded run.",
	"DELETE /api/v1/owners/{name}":                                       "Remove an owner.",
	"DELETE /api/v1/owners/{name}/assignments/{id}":                      "Take one thing off an owner.",
	"DELETE /api/v1/ownership/aliases":                                   "Remove a recorded name.",
	"DELETE /api/v1/ownership/aliases/{id}":                              "Remove one recorded name.",
	"DELETE /api/v1/ownership/import/mappings/{id}":                      "Delete a saved import.",
	"DELETE /api/v1/saved-filters/{id}":                                  "Throw away a kept selection.",
	"GET /api/v1/admin/backups":                                          "Backups taken, and how big each is.",
	"GET /api/v1/admin/backups/status":                                   "Whether backups are running as configured, and when the last one succeeded.",
	"GET /api/v1/admin/backups/{id}":                                     "One backup.",
	"GET /api/v1/admin/config/analysis-tools":                            "Where the static check and its supporting tools are, and which versions are in use.",
	"GET /api/v1/admin/config/auth":                                      "How people sign in, and how long sessions last.",
	"GET /api/v1/admin/config/backup":                                    "Whether backups run, and when.",
	"GET /api/v1/admin/config/collection":                                "How often collection runs and what it gathers.",
	"GET /api/v1/admin/config/concurrency":                               "How much work the service will do at once.",
	"GET /api/v1/admin/config/exports":                                   "What may be extracted, and how much at once.",
	"GET /api/v1/admin/config/git-urls":                                  "How cookbook names are turned into repository addresses.",
	"GET /api/v1/admin/config/ingest":                                    "Whether pushed Chef telemetry is accepted, and what is done with it.",
	"GET /api/v1/admin/config/logging":                                   "How much the service writes to its log.",
	"GET /api/v1/admin/config/organisations":                             "Which Chef servers and organisations are collected from.",
	"GET /api/v1/admin/config/readiness":                                 "What counts as ready to move, and what blocks it.",
	"GET /api/v1/admin/config/server":                                    "How the service listens \u2014 addresses, ports and certificates.",
	"GET /api/v1/admin/config/target-versions":                           "The Chef version everything is being judged against. There is exactly one.",
	"GET /api/v1/admin/config/test-kitchen":                              "How test machines are built and how many may run at once.",
	"GET /api/v1/admin/credentials":                                      "The stored secrets this service uses to reach other systems. Never returns the secret itself.",
	"GET /api/v1/admin/diagnostic-bundle":                                "Everything support would need to diagnose this deployment, in one download, with secrets removed.",
	"GET /api/v1/admin/performance/explain/catalog":                      "The queries whose plans can be inspected.",
	"GET /api/v1/admin/platform-display-names":                           "What each platform is called on screen.",
	"GET /api/v1/admin/platform-mapping/status":                          "Whether reported operating systems are being recognised, and which are not.",
	"GET /api/v1/admin/saml/endpoints":                                   "The addresses an identity provider needs to be told about.",
	"GET /api/v1/admin/saml/sp-certificate":                              "The certificate this service signs SAML with.",
	"GET /api/v1/admin/status":                                           "What the service is doing right now: collection, scanning, test runs.",
	"GET /api/v1/admin/system-health":                                    "Whether the parts this depends on are healthy \u2014 database, disk, the servers it collects from.",
	"GET /api/v1/admin/users":                                            "The local accounts that can sign in.",
	"GET /api/v1/auth/info":                                              "Which ways of signing in this deployment offers, before anybody tries.",
	"GET /api/v1/auth/me":                                                "Who the caller is and what they are allowed to do.",
	"GET /api/v1/auth/me/tokens":                                         "The API credentials the caller has made for their own tools, and roughly when each was last used. Never the secrets — those are shown once, when created.",
	"POST /api/v1/auth/me/tokens":                                        "Create an API credential for a tool, in the caller's own name and at their own level of access. The secret comes back once and is never stored. Ask for write access only if the tool needs to record findings; without it the credential can only read.",
	"DELETE /api/v1/auth/me/tokens/{id}":                                 "Destroy one of the caller's API credentials. It stops working immediately.",
	"GET /api/v1/auth/saml/acs":                                          "Where the identity provider sends somebody back to after they sign in.",
	"GET /api/v1/auth/saml/login":                                        "Start signing in through the identity provider.",
	"GET /api/v1/auth/saml/metadata":                                     "This service's SAML details, to hand to an identity provider.",
	"GET /api/v1/auth/saml/slo":                                          "Where the identity provider sends somebody after they sign out.",
	"GET /api/v1/cookbooks":                                              "Every cookbook in the estate with its verdict for the target version. Filterable and paged.",
	"GET /api/v1/cookbooks/{name}":                                       "One cookbook: its versions, what the static check found, and what happened when it was last run on a real machine.",
	"GET /api/v1/cookbooks/{name}/committers":                            "Who has been committing to this cookbook \u2014 the candidates for who owns it.",
	"GET /api/v1/cookbooks/{name}/platform-coverage":                     "Which platforms this cookbook has been proven on, and which are untested.",
	"GET /api/v1/cookbooks/{name}/{version}/remediation":                 "What would have to change in one version of a cookbook to make it work on the target version.",
	"GET /api/v1/cookstyle/cop-drift":                                    "Static checks that have appeared or changed since the tool was last upgraded, and so have no judgement recorded yet.",
	"GET /api/v1/cookstyle/cops":                                         "Every static check, grouped: which findings we judge to block the upgrade and which are tidying. Ask this before reading findings one at a time.",
	"GET /api/v1/cookstyle/cops/{cop_name}/classification":               "Our judgement about one static check \u2014 blocking or tidying \u2014 and why.",
	"GET /api/v1/cookstyle/cops/{cop_name}/cookbooks":                    "Every cookbook one static check fires on.",
	"GET /api/v1/cookstyle/custom-cops":                                  "Static checks written here rather than shipped by the tool.",
	"GET /api/v1/cookstyle/custom-cops/{name}":                           "One static check written here.",
	"GET /api/v1/cookstyle/scan-scope":                                   "Which files in a repository the scan looks at \u2014 a converge never runs CI and rake files, so findings in them are noise.",
	"GET /api/v1/dashboard/complexity/trend":                             "Whether the work remaining is getting simpler or harder over time.",
	"GET /api/v1/dashboard/cookbook-compatibility":                       "How many cookbooks will survive the target version, and how many will not.",
	"GET /api/v1/dashboard/cookbook-download-status":                     "Whether cookbook source is actually being fetched, or collection is quietly failing.",
	"GET /api/v1/dashboard/cookstyle/recompute-trend":                    "How verdicts have moved as our judgement of the static findings changed.",
	"GET /api/v1/dashboard/deployment/status":                            "How far the rollout of the target version has actually got.",
	"GET /api/v1/dashboard/deployment/trend":                             "How the rollout of the target version has progressed over time.",
	"GET /api/v1/dashboard/git-repo-compatibility":                       "The same verdict counted by repository rather than by cookbook.",
	"GET /api/v1/dashboard/platform-distribution":                        "Which operating systems the estate runs on.",
	"GET /api/v1/dashboard/readiness":                                    "How much of the estate can move to the target version, and what is holding the rest back.",
	"GET /api/v1/dashboard/readiness/trend":                              "Whether readiness is improving, over time.",
	"GET /api/v1/dashboard/stale/trend":                                  "How much of what is held is going out of date \u2014 machines that have stopped reporting.",
	"GET /api/v1/dashboard/test-kitchen-compatibility":                   "How many cookbooks have been proven on a real machine, as against read clean.",
	"GET /api/v1/dashboard/version-distribution":                         "Which Chef versions the estate is actually running.",
	"GET /api/v1/dashboard/version-distribution/trend":                   "How the spread of Chef versions has moved over time.",
	"GET /api/v1/exports/{job_id}":                                       "How a queued export is getting on.",
	"GET /api/v1/exports/{job_id}/download":                              "Fetch what a queued export produced.",
	"GET /api/v1/failure-register":                                       "Findings a person recorded after seeing something run \u2014 the human verdict, which outranks both the static check and the test run.",
	"GET /api/v1/failure-register/subject/{subject_type}/{subject_name}": "Everything ever recorded about one cookbook or repository, in order.",
	"GET /api/v1/failure-register/{id}":                                  "One recorded finding.",
	"GET /api/v1/features":                                               "Which optional parts of the service this deployment has switched on.",
	"GET /api/v1/filters/complexity-labels":                              "The complexity bands in use, for narrowing a list.",
	"GET /api/v1/filters/environments":                                   "The Chef environments present, for narrowing a list.",
	"GET /api/v1/filters/platforms":                                      "The operating systems present, for narrowing a list.",
	"GET /api/v1/filters/policy-groups":                                  "The policy groups present, for narrowing a list.",
	"GET /api/v1/filters/policy-names":                                   "The policy names present, for narrowing a list.",
	"GET /api/v1/filters/roles":                                          "The roles present, for narrowing a list.",
	"GET /api/v1/filters/run-chef-versions":                              "The Chef versions seen in reported runs, for narrowing the run history.",
	"GET /api/v1/filters/run-organisations":                              "The organisations that have reported Chef runs, for narrowing the run history.",
	"GET /api/v1/filters/tags":                                           "The tags present, for narrowing a list.",
	"GET /api/v1/filters/target-chef-versions":                           "The Chef versions machines are currently on, for narrowing a list.",
	"GET /api/v1/git-repos":                                              "Every source repository this service holds, with its verdict for the target version.",
	"GET /api/v1/git-repos/excluded":                                     "Repositories deliberately left out of scanning, and why.",
	"GET /api/v1/git-repos/{name}":                                       "One repository: its cookbooks, its findings, and who commits to it.",
	"GET /api/v1/git-repos/{name}/committers":                            "Who has been committing to this repository.",
	"GET /api/v1/git-repos/{name}/files":                                 "What is in this repository, as a directory listing.",
	"GET /api/v1/git-repos/{name}/files/content":                         "The contents of one file in this repository \u2014 the source behind a finding.",
	"GET /api/v1/git-repos/{name}/{version}/remediation":                 "What would have to change in this repository to make one version work on the target version.",
	"GET /api/v1/health":                                                 "Whether the service is up. Answers without a session, for a load balancer or a monitor.",
	"GET /api/v1/hypervisor/templates":                                   "The machine images available to build test machines from.",
	"GET /api/v1/hypervisor/vms":                                         "Test machines currently in existence.",
	"GET /api/v1/kitchen/analysis/cookbooks":                             "Which cookbooks are worth running on a real machine, and what each would need.",
	"GET /api/v1/kitchen/analysis/cookbooks/{name}":                      "What running one cookbook on a real machine would take.",
	"GET /api/v1/kitchen/analysis/platforms":                             "Which platforms would have to be available to test what is here.",
	"GET /api/v1/kitchen/analysis/summary":                               "How much of the estate could be proven by running it on a real machine, rather than read statically.",
	"GET /api/v1/kitchen/batches":                                        "Groups of test runs put together to be run as one job.",
	"GET /api/v1/kitchen/batches/{id}":                                   "One batch and what is in it.",
	"GET /api/v1/kitchen/batches/{id}/instances":                         "The test machines one batch is using.",
	"GET /api/v1/kitchen/batches/{id}/progress":                          "How far a running batch has got.",
	"GET /api/v1/kitchen/git/exclusions":                                 "Repositories deliberately left out of test runs, and why.",
	"GET /api/v1/kitchen/git/instances":                                  "Test machines built from repositories, and what state each is in.",
	"GET /api/v1/kitchen/git/results":                                    "What happened when repositories were last run on a real machine.",
	"GET /api/v1/kitchen/node-runs":                                      "Test runs that rebuilt a real machine's configuration.",
	"GET /api/v1/kitchen/node-runs/{id}":                                 "One such run: what it did, and where it failed.",
	"GET /api/v1/kitchen/queue":                                          "Test runs waiting to start, and what is holding each up.",
	"GET /api/v1/kitchen/queue/stats":                                    "The shape of the queue \u2014 how much is waiting, running and done.",
	"GET /api/v1/kitchen/queue/{id}":                                     "One queued test run.",
	"GET /api/v1/logs":                                                   "What the service has been doing, as log entries. Filterable.",
	"GET /api/v1/logs/collection-runs":                                   "Each collection run: when it ran, how long it took, and whether it finished.",
	"GET /api/v1/logs/{id}":                                              "One log entry in full.",
	"GET /api/v1/nodes":                                                  "Every managed machine, with what is blocking each one from the target version. Filterable and paged.",
	"GET /api/v1/nodes/by-cookbook/{cookbook_name}":                      "Every machine that uses one cookbook \u2014 how many upgrades one fix would unblock.",
	"GET /api/v1/nodes/by-version/{chef_version}":                        "Every machine running one particular Chef version.",
	"GET /api/v1/nodes/disks/{organisation}/{name}":                      "What one machine's disks look like, for judging whether it can take an upgrade.",
	"GET /api/v1/nodes/runs/{organisation}/{name}":                       "What happened the last times Chef ran on one machine.",
	"GET /api/v1/nodes/{organisation}/{name}":                            "One machine: its Chef version, run list, platform, and the cookbooks blocking its upgrade.",
	"GET /api/v1/nodes/{organisation}/{name}/dependency-graph":           "What one machine depends on, as a graph \u2014 its roles and cookbooks and how they reach each other.",
	"GET /api/v1/openapi.json":                                           "This description: every address the service serves, what it is for, and the access it needs.",
	"GET /api/v1/organisations":                                          "Every Chef organisation this service collects from.",
	"GET /api/v1/organisations/{name}":                                   "One Chef organisation: what it holds and when it was last collected.",
	"GET /api/v1/owners":                                                 "The people and teams who own things here.",
	"GET /api/v1/owners/{name}":                                          "One owner: who they are, and how else they are written down.",
	"GET /api/v1/owners/{name}/assignments":                              "What one owner is responsible for.",
	"GET /api/v1/ownership/aliases":                                      "The other names one person is written down under \u2014 SAML email, username, git address.",
	"GET /api/v1/ownership/aliases/suggest":                              "Names that look like they belong to somebody already here.",
	"GET /api/v1/ownership/aliases/{id}":                                 "One recorded name.",
	"GET /api/v1/ownership/audit-log":                                    "Every change made to ownership, and by whom.",
	"GET /api/v1/ownership/duplicates":                                   "People who appear to have been written down twice.",
	"GET /api/v1/ownership/duplicates/dismissed":                         "Pairs already judged to be different people.",
	"GET /api/v1/ownership/import/clear":                                 "What removing everything a previous import brought in would take away.",
	"GET /api/v1/ownership/import/mappings":                              "Saved imports \u2014 a source, a query and a column mapping, optionally on a schedule.",
	"GET /api/v1/ownership/import/mappings/{id}":                         "One saved import.",
	"GET /api/v1/ownership/import/rejections":                            "The rows the last import could not use \u2014 the most direct statement of a source system's data quality.",
	"GET /api/v1/ownership/lookup":                                       "Who owns a given cookbook, repository or machine.",
	"GET /api/v1/remediation/priority":                                   "What to fix first: everything broken, ranked by what it costs against what it unblocks.",
	"GET /api/v1/remediation/summary":                                    "The shape of the remaining work, grouped, before reading any of it in detail.",
	"GET /api/v1/roles":                                                  "Every Chef role, with how much of the estate each one reaches.",
	"GET /api/v1/roles/{name}":                                           "One role: what it pulls in, and what that blocks.",
	"GET /api/v1/roles/{name}/dependency-graph":                          "What one role depends on, as a graph \u2014 where a single change would reach furthest.",
	"GET /api/v1/run-events/nodes":                                       "Machines that have reported a Chef run, whether or not this service collects from their server.",
	"GET /api/v1/run-events/nodes/{organisation}/{name}":                 "One machine's reported runs, including machines nothing else here knows about.",
	"GET /api/v1/run-events/runs":                                        "Chef runs as they were reported, with the error and backtrace where one failed.",
	"GET /api/v1/saved-filters":                                          "Selections somebody named and kept, so a slice of the estate built yesterday can be got back.",
	"GET /api/v1/server/tls-status":                                      "Whether the server is serving the certificate it was configured with, or fell back because it could not.",
	"GET /api/v1/version":                                                "Which build is running, and which database schema it expects.",
	"GET /api/v1/ws":                                                     "A live stream of events as collection, scanning and test runs progress.",
	"PATCH /api/v1/failure-register/{id}":                                "Revise a recorded finding. Nothing is overwritten; the earlier wording stays readable.",
	"PATCH /api/v1/saved-filters/{id}":                                   "Rename or adjust a kept selection.",
	"POST /api/v1/admin/backups":                                         "Take a backup now.",
	"POST /api/v1/admin/backups/{id}/restore":                            "Restore from a backup. Replaces what is held now.",
	"POST /api/v1/admin/config/server/generate-csr":                      "Produce a certificate signing request, to get a certificate issued.",
	"POST /api/v1/admin/credentials":                                     "Store a secret for reaching another system.",
	"POST /api/v1/admin/credentials/{name}/test":                         "Check a stored secret still works, without revealing it.",
	"POST /api/v1/admin/hypervisor/test-connection":                      "Check the service can reach the hypervisor, before relying on it.",
	"POST /api/v1/admin/performance/explain":                             "Show how the database would answer one query, for diagnosing a slow screen.",
	"POST /api/v1/admin/performance/vacuum":                              "Reclaim database space. Locks the tables it works on, so it is not for a working day.",
	"POST /api/v1/admin/platform-display-names/reset":                    "Go back to the shipped platform names.",
	"POST /api/v1/admin/rescan-all-cookstyle":                            "Re-run the static check over everything. Needed after our judgement about which findings block the upgrade changes.",
	"POST /api/v1/admin/restart":                                         "Restart the service.",
	"POST /api/v1/admin/saml/generate-keypair":                           "Generate a fresh SAML signing key. The identity provider must be told about the new certificate.",
	"POST /api/v1/admin/users":                                           "Create a local account.",
	"POST /api/v1/auth/login":                                            "Sign in with a username and password.",
	"POST /api/v1/auth/logout":                                           "Sign out and invalidate the session.",
	"POST /api/v1/cookbooks/{name}/committers/assign":                    "Record one of this cookbook's committers as its owner.",
	"POST /api/v1/cookbooks/{name}/rescan":                               "Run the static check over this cookbook again now, rather than waiting for the next collection.",
	"POST /api/v1/cookbooks/{name}/reset-git":                            "Forget which repository this cookbook was matched to, so it can be matched again.",
	"POST /api/v1/cookstyle/custom-cops":                                 "Add a static check of our own.",
	"POST /api/v1/exports":                                               "Pull the whole of something out \u2014 every machine, cookbook, role or repository \u2014 as CSV. Streams; not paged.",
	"POST /api/v1/failure-register":                                      "Record what you saw when you ran it, against a cookbook or repository.",
	"POST /api/v1/failure-register/{id}/resolve":                         "Mark a recorded finding as dealt with.",
	"POST /api/v1/git-repos/{name}/committers/assign":                    "Record one of this repository's committers as its owner.",
	"POST /api/v1/git-repos/{name}/exclude":                              "Leave this repository out of scanning, with a reason.",
	"POST /api/v1/git-repos/{name}/rescan":                               "Fetch and re-check this repository now.",
	"POST /api/v1/git-repos/{name}/reset":                                "Clear what is held about this repository so it is collected again from scratch.",
	"POST /api/v1/hypervisor/cleanup":                                    "Destroy test machines nothing is using any more.",
	"POST /api/v1/hypervisor/vms/{id}/destroy":                           "Destroy one test machine.",
	"POST /api/v1/ingest":                                                "Where Chef run telemetry is delivered. Unauthenticated by design; the sender is a Chef Automate data feed, not a person.",
	"POST /api/v1/mcp":                                                   "The assistant surface: a short, chosen set of tools an AI assistant in an editor can use to read this estate, spoken as Model Context Protocol over Streamable HTTP.",
	"POST /api/v1/kitchen/analysis/trigger":                              "Work out again what could be tested, now.",
	"POST /api/v1/kitchen/batches":                                       "Group a selection of things to test into one job.",
	"POST /api/v1/kitchen/batches/{id}/cancel":                           "Stop a batch. Machines already being built finish being built; cancelling cannot reach into a clone already in flight.",
	"POST /api/v1/kitchen/batches/{id}/run":                              "Start a batch running.",
	"POST /api/v1/kitchen/git/exclusions":                                "Leave a repository out of test runs.",
	"POST /api/v1/kitchen/git/run":                                       "Run one repository on a real machine.",
	"POST /api/v1/kitchen/git/run-all":                                   "Run every eligible repository on a real machine. Bounded by the configured concurrency.",
	"POST /api/v1/kitchen/node-run":                                      "Rebuild one real machine's configuration on a test machine and see whether it converges.",
	"POST /api/v1/kitchen/orphan-sweep":                                  "Find and remove test machines left behind by runs that did not finish cleanly.",
	"POST /api/v1/kitchen/queue/{id}/cancel":                             "Take one run out of the queue.",
	"POST /api/v1/kitchen/queue/{id}/retry":                              "Put a failed run back on the queue.",
	"POST /api/v1/owners":                                                "Add a person or team.",
	"POST /api/v1/owners/{name}/assignments":                             "Make an owner responsible for something.",
	"POST /api/v1/ownership/aliases":                                     "Record another name for a person.",
	"POST /api/v1/ownership/aliases/import":                              "Load alternative names in bulk.",
	"POST /api/v1/ownership/aliases/{id}":                                "Change one recorded name.",
	"POST /api/v1/ownership/duplicates/dismiss":                          "Say two people who look alike are genuinely different.",
	"POST /api/v1/ownership/duplicates/rescan":                           "Look for duplicated people again. Walks the whole catalogue, so it runs in the background.",
	"POST /api/v1/ownership/duplicates/restore":                          "Undo that judgement, so a pair is offered again.",
	"POST /api/v1/ownership/import/clear":                                "Remove what previous imports brought in. Hand-made owners and assignments survive.",
	"POST /api/v1/ownership/import/commit":                               "Run an import for real.",
	"POST /api/v1/ownership/import/mappings":                             "Save an import so it can be run again or scheduled.",
	"POST /api/v1/ownership/import/mappings/{id}/run":                    "Run a saved import now and wait for the result, rather than waiting for its schedule.",
	"POST /api/v1/ownership/import/preview":                              "What an import would do, without doing it.",
	"POST /api/v1/ownership/import/profile":                              "What a source's columns actually contain, before deciding how to map them.",
	"POST /api/v1/ownership/import/tables":                               "What tables and views a database connection can see, for somebody who cannot inspect it directly.",
	"POST /api/v1/ownership/merge":                                       "Merge two records that turned out to be the same person.",
	"POST /api/v1/ownership/reassign":                                    "Move something from one owner to another.",
	"POST /api/v1/saved-filters":                                         "Keep the current selection under a name.",
	"PUT /api/v1/admin/config/analysis-tools":                            "Change which analysis tools are used.",
	"PUT /api/v1/admin/config/auth":                                      "Change how people sign in.",
	"PUT /api/v1/admin/config/backup":                                    "Change whether backups run and when. Takes effect immediately.",
	"PUT /api/v1/admin/config/collection":                                "Change the collection schedule. Takes effect immediately.",
	"PUT /api/v1/admin/config/concurrency":                               "Change how much work runs at once. Takes effect immediately.",
	"PUT /api/v1/admin/config/exports":                                   "Change what may be extracted.",
	"PUT /api/v1/admin/config/git-urls":                                  "Change how cookbook names are turned into repository addresses.",
	"PUT /api/v1/admin/config/ingest":                                    "Change whether pushed telemetry is accepted.",
	"PUT /api/v1/admin/config/logging":                                   "Change how much is logged. Takes effect immediately, without a restart.",
	"PUT /api/v1/admin/config/organisations":                             "Change which Chef servers and organisations are collected from. Takes effect immediately.",
	"PUT /api/v1/admin/config/readiness":                                 "Change what counts as ready to move. Every machine's verdict is recomputed.",
	"PUT /api/v1/admin/config/server":                                    "Change how the service listens. A setting that would lock everybody out falls back rather than applying.",
	"PUT /api/v1/admin/config/target-versions":                           "Change the version everything is judged against. Every verdict is recomputed against it.",
	"PUT /api/v1/admin/config/test-kitchen":                              "Change how test machines are built.",
	"PUT /api/v1/admin/credentials/{name}":                               "Replace a stored secret.",
	"PUT /api/v1/admin/platform-display-names":                           "Change what a platform is called on screen.",
	"PUT /api/v1/admin/users/{username}":                                 "Change a local account.",
	"PUT /api/v1/admin/users/{username}/password":                        "Set a local account's password.",
	"PUT /api/v1/cookstyle/cops/{cop_name}/classification":               "Change our judgement about whether one static check blocks the upgrade.",
	"PUT /api/v1/cookstyle/custom-cops/{name}":                           "Change a static check written here.",
	"PUT /api/v1/cookstyle/scan-scope":                                   "Change which files the scan looks at.",
	"PUT /api/v1/kitchen/batches/{id}":                                   "Change what is in a batch.",
	"PUT /api/v1/owners/{name}":                                          "Correct an owner's details.",
	"PUT /api/v1/ownership/import/mappings/{id}":                         "Change a saved import.",
}

// What a handler requires beyond the wrapper its route was registered with.
//
// Some handlers check the caller's role themselves, per method, because the
// same address serves reads that everybody may make and writes that they may
// not. Those requirements are invisible in the route table, so they are
// declared here and folded into the description by effectiveRole.
//
// This is a second place where a fact about access is written down, which is
// exactly the shape of thing that rots — so it is not trusted. Every entry, and
// every absence, is checked against the running service by
// TestOpenAPI_DescribedRoleIsTheRoleEnforced, which probes each operation and
// fails with the line to add or remove.
var apiRoles = map[string]RouteRole{
	"DELETE /api/v1/cookstyle/custom-cops/{name}":          RoleAdmin,
	"DELETE /api/v1/cookstyle/scan-scope":                  RoleAdmin,
	"DELETE /api/v1/git-repos/{name}/exclude":              RoleAdmin,
	"DELETE /api/v1/kitchen/batches/{id}":                  RoleOperator,
	"DELETE /api/v1/kitchen/git/exclusions/{id}":           RoleAdmin,
	"DELETE /api/v1/kitchen/node-runs/{id}":                RoleOperator,
	"DELETE /api/v1/owners/{name}":                         RoleAdmin,
	"DELETE /api/v1/owners/{name}/assignments/{id}":        RoleOperator,
	"DELETE /api/v1/ownership/aliases":                     RoleOperator,
	"DELETE /api/v1/ownership/aliases/{id}":                RoleOperator,
	"DELETE /api/v1/ownership/import/mappings/{id}":        RoleAdmin,
	"GET /api/v1/cookstyle/cops/{cop_name}/classification": RoleAdmin,
	"GET /api/v1/ownership/duplicates":                     RoleAdmin,
	"GET /api/v1/ownership/duplicates/dismissed":           RoleAdmin,
	"GET /api/v1/ownership/import/clear":                   RoleAdmin,
	"GET /api/v1/ownership/import/mappings":                RoleAdmin,
	"GET /api/v1/ownership/import/mappings/{id}":           RoleAdmin,
	"GET /api/v1/ownership/import/rejections":              RoleAdmin,
	"PATCH /api/v1/failure-register/{id}":                  RoleOperator,
	"POST /api/v1/cookbooks/{name}/committers/assign":      RoleOperator,
	"POST /api/v1/cookbooks/{name}/reset-git":              RoleOperator,
	"POST /api/v1/cookstyle/custom-cops":                   RoleAdmin,
	"POST /api/v1/failure-register":                        RoleOperator,
	"POST /api/v1/failure-register/{id}/resolve":           RoleOperator,
	"POST /api/v1/git-repos/{name}/committers/assign":      RoleOperator,
	"POST /api/v1/git-repos/{name}/exclude":                RoleAdmin,
	"POST /api/v1/git-repos/{name}/reset":                  RoleOperator,
	"POST /api/v1/kitchen/batches/{id}/cancel":             RoleOperator,
	"POST /api/v1/kitchen/batches/{id}/run":                RoleOperator,
	"POST /api/v1/kitchen/git/exclusions":                  RoleAdmin,
	"POST /api/v1/owners":                                  RoleOperator,
	"POST /api/v1/owners/{name}/assignments":               RoleOperator,
	"POST /api/v1/ownership/aliases":                       RoleOperator,
	"POST /api/v1/ownership/aliases/import":                RoleOperator,
	"POST /api/v1/ownership/aliases/{id}":                  RoleOperator,
	"POST /api/v1/ownership/duplicates/dismiss":            RoleAdmin,
	"POST /api/v1/ownership/duplicates/rescan":             RoleAdmin,
	"POST /api/v1/ownership/duplicates/restore":            RoleAdmin,
	"POST /api/v1/ownership/import/clear":                  RoleAdmin,
	"POST /api/v1/ownership/import/commit":                 RoleAdmin,
	"POST /api/v1/ownership/import/mappings":               RoleAdmin,
	"POST /api/v1/ownership/import/mappings/{id}/run":      RoleAdmin,
	"POST /api/v1/ownership/import/preview":                RoleAdmin,
	"POST /api/v1/ownership/import/profile":                RoleAdmin,
	"POST /api/v1/ownership/import/tables":                 RoleAdmin,
	"POST /api/v1/ownership/merge":                         RoleAdmin,
	"POST /api/v1/ownership/reassign":                      RoleOperator,
	"PUT /api/v1/cookstyle/cops/{cop_name}/classification": RoleAdmin,
	"PUT /api/v1/cookstyle/custom-cops/{name}":             RoleAdmin,
	"PUT /api/v1/cookstyle/scan-scope":                     RoleAdmin,
	"PUT /api/v1/kitchen/batches/{id}":                     RoleOperator,
	"PUT /api/v1/owners/{name}":                            RoleOperator,
	"PUT /api/v1/ownership/import/mappings/{id}":           RoleAdmin,
}

// Writes that genuinely read nothing from the request body, keyed by
// "METHOD path".
//
// Running a scan, cancelling a run, restarting the service: what to do is in
// the address, so there is nothing to send. Saying so is not the same as saying
// nothing — an undescribed body and a body that does not exist look identical
// to a caller, and the first sends them hunting through our source while the
// second is the answer.
//
// This is a list about absence, so it cannot be derived — but it is held from
// both sides. An entry for something nothing serves goes red, an address that
// reads a body and is listed here goes red, and a write that is neither listed
// nor described goes red. See openapi_bodies_test.go.
var bodylessWrites = map[string]bool{
	"POST /api/v1/admin/backups":                      true,
	"POST /api/v1/admin/credentials/{name}/test":      true,
	"POST /api/v1/admin/hypervisor/test-connection":   true,
	"POST /api/v1/admin/performance/vacuum":           true,
	"POST /api/v1/admin/platform-display-names/reset": true,
	"POST /api/v1/admin/rescan-all-cookstyle":         true,
	"POST /api/v1/admin/restart":                      true,
	"POST /api/v1/admin/saml/generate-keypair":        true,
	"POST /api/v1/auth/logout":                        true,
	"POST /api/v1/cookbooks/{name}/rescan":            true,
	"POST /api/v1/cookbooks/{name}/reset-git":         true,
	"POST /api/v1/git-repos/{name}/rescan":            true,
	"POST /api/v1/git-repos/{name}/reset":             true,
	"POST /api/v1/hypervisor/cleanup":                 true,
	"POST /api/v1/hypervisor/vms/{id}/destroy":        true,
	"POST /api/v1/kitchen/analysis/trigger":           true,
	"POST /api/v1/kitchen/batches/{id}/cancel":        true,
	"POST /api/v1/kitchen/batches/{id}/run":           true,
	"POST /api/v1/kitchen/orphan-sweep":               true,
	"POST /api/v1/kitchen/queue/{id}/cancel":          true,
	"POST /api/v1/kitchen/queue/{id}/retry":           true,
	"POST /api/v1/ownership/duplicates/rescan":        true,
	"POST /api/v1/ownership/import/clear":             true,
	"POST /api/v1/ownership/import/mappings/{id}/run": true,

	// What to extract and in what format are asked for in the address, not
	// sent as a document.
	"POST /api/v1/exports": true,
}

// Writes that take a file or form fields rather than a JSON document, keyed by
// "METHOD path".
//
// Importing owners means uploading the spreadsheet somebody exported, and
// asking a system of record what tables it holds means sending the connection
// details as form fields. Both are real input, so neither belongs in
// bodylessWrites — a caller told "this takes nothing" would send nothing and
// get a refusal with no idea why.
//
// The fields themselves are declared at the registration site with takesForm.
// They are read as string keys rather than decoded into a type, so nothing can
// reflect them and the names have to be written down — which is why two tests
// in openapi_forms_test.go hold them against the handlers from both sides, and
// why this list is what says an address here must name its fields at all.
var uploadWrites = map[string]bool{
	"POST /api/v1/ownership/aliases/import": true,
	"POST /api/v1/ownership/import/commit":  true,
	"POST /api/v1/ownership/import/preview": true,
	"POST /api/v1/ownership/import/profile": true,
	"POST /api/v1/ownership/import/tables":  true,
}

// Bodies this service reads but deliberately does not describe, keyed by
// "METHOD path", with the reason — which is served, so a caller reads why
// rather than assuming we forgot.
//
// The reason is always the same one: this service does not decide the shape.
// Naming a type for data that arrives from somewhere else turns a change at
// the far end into a decode failure here, and the whole point of the ingest
// path is that it keeps working when the sender adds a field.
var undescribedBodies = map[string]string{
	"POST /api/v1/ingest": "Node telemetry, as the sender emits it. Deliberately not " +
		"described: this service does not decide this shape, and pinning it would turn a " +
		"change at the sending end into a rejected delivery.",
}

// Routes whose handler can reach ParsePagination but which do not paginate.
//
// Every one of these is a handler registered at several addresses, or a subtree
// serving many, where some other address under the same function is the one
// that pages. Reachability cannot tell them apart, so each was measured against
// a running instance with tools/api-probe/probe.py and recorded here rather
// than guessed. Re-run it rather than reasoning about this list.
//
// This is a record of measurements, not a design decision, and it is held from
// both sides: an entry for something nothing serves goes red, and a route that
// pages without either declaring it or appearing here goes red. See
// openapi_query_test.go.
var unpaginatedDespiteReaching = map[string]bool{
	// Dispatches on the path across a dozen addresses; the ones that page
	// declare it themselves.
	"/api/v1/ownership/import/mappings/":  true,
	"/api/v1/ownership/import/rejections": true,
	"/api/v1/ownership/import/tables":     true,
	"/api/v1/ownership/import/profile":    true,
	"/api/v1/ownership/import/preview":    true,
	"/api/v1/ownership/import/commit":     true,
	"/api/v1/ownership/import/clear":      true,
	// handleOwnershipEndpoints, likewise.
	"/api/v1/ownership/lookup":   true,
	"/api/v1/ownership/reassign": true,
	// Subtrees whose paging address is a sibling.
	"/api/v1/cookbooks/":         true,
	"/api/v1/admin/credentials/": true,
	"/api/v1/admin/users/":       true,
	"/api/v1/owners/":            true,
	"/api/v1/failure-register/":  true,
	"/api/v1/nodes/":             true,
}
