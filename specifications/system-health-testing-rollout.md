# System Health — Testing, Rollout Considerations

## Testing

### Backend

- `internal/syshealth/syshealth_test.go`:
  - `TestSnapshot_ReturnsValidStats` — verifies all fields are populated
    with sane values (disk total > 0, CPU count > 0, etc.)
  - `TestSnapshot_DiskAlerts` — injects low thresholds and verifies
    warning/critical alerts are generated
  - `TestSnapshot_CPUAlerts` — same for CPU load
  - `TestSnapshot_MemoryAlerts` — same for memory
  - `TestSnapshot_NoAlerts` — high thresholds, verifies empty alerts
  - `TestShouldPauseCollection_Critical` — returns true when critical
    alerts present
  - `TestShouldPauseCollection_WarningOnly` — returns false
  - `TestShouldPauseCollection_NoAlerts` — returns false

- `internal/webapi/handle_dashboard_test.go` or dedicated test file:
  - Method-not-allowed tests for the admin endpoint
  - Happy path returning valid JSON with expected fields

- `internal/collector/scheduler_test.go`:
  - Test that collection is skipped when `ShouldPauseCollection` returns
    true (inject a mock or use test thresholds)

### Frontend

No dedicated frontend tests are required at this stage. The page follows
the same pattern as other admin pages and is verified manually.

## Rollout Considerations

- No database migration required — all metrics are read at request time.
- The `system_health` config section is entirely optional. If omitted,
  defaults apply and the feature works out of the box.
- The admin page is only visible to admin users. Non-admin users cannot
  access the endpoint or the page.
- On unsupported platforms (e.g. Windows), load average and OS memory
  metrics return zero with no alerts generated for those metrics.
