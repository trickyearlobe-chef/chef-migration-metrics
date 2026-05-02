# Hypervisor Test Connection

## Goal

Add a "Test Connection" button to the Test Kitchen admin page that verifies connectivity to the configured hypervisor and returns the discovered templates.

## Specs

- Existing: `internal/hypervisor/hypervisor.go` interface, `handle_hypervisor.go` handlers

## Steps

1. Add `POST /api/v1/admin/hypervisor/test-connection` endpoint that calls `ListTemplates()`, returning success status, template count, hypervisor type, and the template list — or the error message on failure.
2. Register route in router.
3. Write test for the handler.
4. Add "Test Connection" button to the Driver Settings section in `AdminTestKitchenPage.tsx`.
5. Run tests, commit.

## Acceptance Criteria

- Button shows success (green) with template count on success.
- Button shows failure (red) with error message on failure.
- When no hypervisor is configured, shows an appropriate message.
- Templates returned can be used by the platform mapping UI.
