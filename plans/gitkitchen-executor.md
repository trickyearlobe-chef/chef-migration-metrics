# Git Kitchen Executor

## Goal

Implement `internal/gitkitchen/` package that runs Test Kitchen for git-cloned cookbooks.

## Specs to Read

- `internal/nodekitchen/config_gen.go` — overlay generation pattern
- `internal/config/config.go` — TestKitchenConfig, PlatformMapEntry, ImageEntry
- `internal/config/platform_match.go` — MatchPlatform
- `internal/analysis/driver_profiles.go` — LookupProfile

## Steps

1. Write overlay tests (`overlay_test.go`)
2. Write overlay implementation (`overlay.go`)
3. Write executor tests (`executor_test.go`)
4. Write executor implementation (`executor.go`)
5. Run `go build` and `go test` to verify

## Acceptance Criteria

- All tests pass
- `go build ./internal/gitkitchen/` succeeds
- Overlay generates correct YAML with driver, platform, provisioner, transport blocks
- RunInstance creates isolated workspace, generates overlay, runs kitchen, returns result
