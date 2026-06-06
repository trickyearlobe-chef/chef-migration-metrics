# Analysis — Scheduling and Trigger

## Scheduling and Trigger

The analysis component runs **after** the data collection component completes a collection cycle. The trigger sequence is:

1. Data collection completes (node data collected, cookbooks fetched, git repos pulled)
2. Cookbook usage analysis runs (Phase 1–3 as described above)
3. CookStyle scans run for any new/unscanned Chef server cookbook versions
4. Test Kitchen runs execute for any cookbooks where HEAD has changed
5. Node upgrade readiness evaluation runs
5a. Platform coverage analysis runs (compares kitchen platforms to production node platforms per cookbook)
6. Metric snapshots are written for historical trending

Steps 3 and 4 may run concurrently since they operate on independent cookbook sets (Chef server-sourced vs. git-sourced).
