# Configuration — Live Reload Requirement

**All configuration changes made via the admin API (config store) MUST take effect immediately without requiring an application restart.** This applies to all config-store-backed settings including:

- Backup schedule (cron expression, enabled/disabled)
- Target Chef version
- Collection schedule
- Git base URLs
- Kitchen settings
- Any other admin-configurable value

Components that consume config-store values must either:
1. Read the current value from the config store on each use (pull), OR
2. Subscribe to config-change notifications and update their state (push)

Approach (1) is preferred for simplicity unless the component has long-lived state (e.g. a cron scheduler that needs to reschedule its next tick).

Requiring an application restart to pick up config changes is a **bug**.
