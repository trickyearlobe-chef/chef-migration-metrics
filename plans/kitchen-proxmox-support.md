# Plan: Add kitchen-proxmox Support

## Goal

Add `proxmox` as a built-in Test Kitchen driver profile so operators can provision Proxmox VMs for cookbook testing without using the `custom` profile.

## Specs to Read

- `.claude/specifications/test-kitchen-drivers.md` (§ Built-in Driver Profiles, § Configuration Schema, § Overlay Generation)
- `.claude/specifications/packaging.md` (§4.5 Kitchen Drivers)
- `.claude/specifications/project-conventions.md`

## Context

`kitchen-proxmox` (https://github.com/trickyearlobe-chef/kitchen-proxmox) is a Test Kitchen driver that clones Proxmox VM templates via the Proxmox API. Key details from the driver source:

- Gem name: `kitchen-proxmox`
- Image field: `template` (the Proxmox VM template name to clone)
- Typical connection settings: `proxmox_url`, `proxmox_username`, `proxmox_node`
- Typical secrets: `proxmox_password` (or token via `proxmox_token`)
- Transport: SSH (Linux VMs)

## Steps

1. **Update spec** — Add `proxmox` row to the Built-in Driver Profiles table in `test-kitchen-drivers.md`. Ask user before modifying.
2. **Add built-in profile** — Add `"proxmox"` entry to `builtinProfiles` map in `internal/analysis/driver_profiles.go` with `ImageFieldName: "template"` and `TypicalSecrets: []string{"proxmox_password"}`.
3. **Write tests first** — Add `TestLookupProfile_Proxmox` to `driver_profiles_test.go`. Add `"proxmox"` to the `BuiltinIgnoresOverride` map. Add `"proxmox"` to `TestIsBuiltinDriver_Known` list.
4. **Update config validation** — Add `"proxmox": true` to the `knownDrivers` map in `internal/config/config.go` `validateAnalysisTools`.
5. **Update config comment** — Add `proxmox` to the Driver field doc comment listing built-in profiles in `config.go`.
6. **Add config test** — Add `TestValidation_ProxmoxDriverConfig` to `config_test.go` verifying proxmox driver settings parse correctly.
7. **Update packaging spec** — Add `kitchen-proxmox` to the gem table in `packaging.md` §4.5 and to the Dockerfile gem install list in §4.2. Ask user before modifying.
8. **Update README** — Add `proxmox` to the built-in profiles list in the driver configuration table. Add a Proxmox config example alongside the vCenter one.
9. **Run tests** — `go test ./internal/analysis/... ./internal/config/...` to verify.
10. **Commit** — One commit for tests + implementation.

## Acceptance Criteria

- `LookupProfile("proxmox", "")` returns `ImageFieldName: "template"`.
- `IsBuiltinDriver("proxmox")` returns `true`.
- `proxmox` is recognised by config validation (no warning).
- `BuiltinDriverNames()` includes `"proxmox"` in sorted output.
- All existing tests still pass.
- Spec, packaging spec, and README updated to list proxmox.