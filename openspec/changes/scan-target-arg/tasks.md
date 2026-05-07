## 1. Update scan command definition

- [x] 1.1 Change `cobra.NoArgs` to `cobra.MaximumNArgs(1)` in `scanCmd()`
- [x] 1.2 Remove `cmd.MarkFlagRequired("policy-id")` — validation moves to custom logic
- [x] 1.3 Add a `target` field to `scanOptions` struct, populated from `args[0]` when present
- [x] 1.4 Update scan command `Use` to `scan [target] [flags]`, update `Short`, `Long`, and `Example` with target usage
- [x] 1.5 Add `ValidArgsFunction` for the positional arg that reads `complytime.yaml` and returns target IDs for shell completion

## 2. Implement target resolution and policy inference

- [x] 2.1 Add `resolveTarget(cfg, targetID)` helper that finds a target by ID in `cfg.Targets`, returns error with available IDs if not found
- [x] 2.2 Add `resolvePolicy(cfg, target, policyID)` helper that handles the three cases: (a) both target and policy-id given — validate target references the policy, (b) target given without policy-id — infer if single policy, error with list if multiple, (c) no target — require policy-id
- [x] 2.3 Modify `run()` to call resolution logic: resolve target first (if provided), then resolve policy. Pass the resolved target ID through to `scanPolicy()`.
- [x] 2.5 In `scanPolicy()`, narrow `policyTargets` AFTER `ensureGenerated()` but BEFORE `executeScanPhase()`. Generation must run for all targets referencing the policy (per D7), only the scan execution is target-scoped.
- [x] 2.4 Update the error message for missing policy-id to reflect the new options: "specify a target or --policy-id (see complyctl scan --help)"

## 3. Update tests

- [x] 3.1 [P] Add unit test: `complyctl scan prod --policy-id nist` scopes to single target
- [x] 3.2 [P] Add unit test: `complyctl scan prod` with single-policy target infers policy
- [x] 3.3 [P] Add unit test: `complyctl scan prod` with multi-policy target produces error listing policies
- [x] 3.4 [P] Add unit test: `complyctl scan nonexistent` produces error with available target IDs
- [x] 3.5 [P] Add unit test: `complyctl scan prod --policy-id cis` when prod doesn't have cis produces mismatch error
- [x] 3.6 [P] Add unit test: `complyctl scan` with no args and no --policy-id produces error
- [x] 3.7 [P] Add unit test: `complyctl scan --policy-id nist` without target scans all targets (backward compat)
- [x] 3.8 Update existing `TestScanOptions_Validate_*` tests if validation logic changes
- [x] 3.9 Add E2E test for target-scoped scan

## 4. Cleanup and verify

- [x] 4.1 Update `README.md` scan section with target argument documentation
- [x] 4.2 Run `go build ./...` and `go test ./...` to verify
