## Why

The `complyctl scan` command currently requires `--policy-id` and scans every target in `complytime.yaml` that references that policy. In workspaces with multiple targets (e.g., `production-cluster`, `staging`, `dev-workstation`), there is no way to scan a single target without editing the config file. Users need to scope scans to specific targets for faster iteration, CI pipeline per-environment runs, and debugging individual target configurations.

## What Changes

- Add an optional positional argument `<target>` to the `scan` command: `complyctl scan <target> [--policy-id <ID>]`
- When `<target>` is specified, the scan is scoped to only that target (matched by its `id` field in `complytime.yaml`)
- Make `--policy-id` optional when a target is specified and that target references exactly one policy (inferred). When the target references multiple policies, `--policy-id` is required to disambiguate.
- When neither `<target>` nor `--policy-id` is provided, the command returns an error requiring at least one.
- When both are specified, validate that the target actually references the given policy — hard error if it doesn't.
- Add shell completion for the `<target>` positional argument based on target IDs from `complytime.yaml`.

## Capabilities

### New Capabilities
- `scan-target-arg`: Positional target argument for the scan command with policy inference and validation.

### Modified Capabilities

## Impact

- **CLI surface**: `complyctl scan` gains an optional positional argument. `--policy-id` changes from required to conditionally required. Fully backward-compatible — existing `complyctl scan --policy-id X` invocations continue to work unchanged.
- **Code**: `cmd/complyctl/cli/scan.go` (command definition, validation, target resolution), `cmd/complyctl/cli/generate.go` (may benefit from same target scoping).
- **Tests**: Unit tests for new validation logic, E2E tests for target-scoped scans, shell completion tests.
- **No dependency changes**.
