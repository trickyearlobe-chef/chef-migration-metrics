# System Health — Collection Circuit Breaker

## Collection Circuit Breaker

The scheduler loop in `internal/collector/scheduler.go` checks system
health before each run:

```
timer fires →
  if collector.IsRunning() → skip (existing behaviour)
  else if config.PauseCollectionOnCritical && ShouldPauseCollection(snapshot) →
      log WARN "collection paused: <alert messages>"
      skip
  else → run collection
```

The check is lightweight (one `Statfs` call per unique disk path + one
`/proc/loadavg` read) and adds negligible overhead to the scheduling
loop.

The circuit breaker does **not** interrupt a run that is already in
progress. It only prevents new runs from starting.
