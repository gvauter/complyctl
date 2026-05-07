## 1. Remove otel format constant and validation

- [x] 1.1 Delete `OutputFormatOTEL` constant from `internal/complytime/consts.go`
- [x] 1.2 Remove `otel` from the format validation switch in `scan.go` `validate()` — falls through to generic invalid format error
- [x] 1.3 [P] Remove `otel` from shell completion list in `scanCmd()` and from the `--format` flag description
- [x] 1.4 [P] Remove `otel` from the scan command examples in `scanCmd()`

## 2. Add environment variable export trigger

- [x] 2.1 Add `ExportEnabledEnvVar` constant (`COMPLYTIME_EXPORT_ENABLED`) to `internal/complytime/consts.go`
- [x] 2.2 Modify `maybeExport()` in `scan.go` to use `strconv.ParseBool(os.Getenv(complytime.ExportEnabledEnvVar))` instead of checking `o.format == complytime.OutputFormatOTEL`. When unset/empty, skip silently. When ParseBool returns true, proceed with export. When ParseBool returns error on a non-empty value, log an error to stderr with the unrecognized value and accepted values, then skip export.
- [x] 2.3 Move the collector config validation (the `OutputFormatOTEL` guard block in `run()`) into `runExport()` so it only fires when export is actually triggered. Ensure the `cfg.Collector == nil` check occurs before any dereference of `cfg.Collector` fields.
- [x] 2.4 Add env var reference to the scan command `Long` or help text description

## 3. Update doctor command

- [x] 3.1 Update `internal/doctor/doctor.go` collector check messaging (the string `"no collector configured (optional — needed for --format otel)"`) to reference `COMPLYTIME_EXPORT_ENABLED` instead of `--format otel`
- [x] 3.2 Add proactive diagnostic to doctor: when a `collector` section exists in `complytime.yaml` but `COMPLYTIME_EXPORT_ENABLED` is not set or is falsy, emit a warning suggesting the user set `COMPLYTIME_EXPORT_ENABLED=true`. When the env var is truthy, suppress this warning.

## 4. Update tests

- [x] 4.1 Update format validation unit tests in `cmd/complyctl/cli/cli_test.go` — `otel` should now be rejected with migration guidance. Also update `TestScanOptions_Run_OtelWithoutCollector` to use `t.Setenv("COMPLYTIME_EXPORT_ENABLED", "true")` instead of referencing `OutputFormatOTEL`
- [x] 4.2 Rename and update E2E test `TestE2E_ScanFormatOtel` → `TestE2E_ScanExportEnabled` to use `COMPLYTIME_EXPORT_ENABLED=true` env var instead of `--format otel`
- [x] 4.3 Rename and update E2E test `TestE2E_ScanFormatOtelNoCollector` → `TestE2E_ScanExportEnabledNoCollector` to use env var trigger
- [x] 4.4 [P] Removed — `--format otel` now hits the standard invalid format error (covered by existing `TestScanOptions_Validate_InvalidFormat`)
- [x] 4.5 [P] Add test case verifying export works alongside `--format sarif` when env var is set. Assert both: (a) SARIF report file exists in output directory, AND (b) export summary table appears in stdout
- [x] 4.6 [P] Add table-driven unit test for env var parsing via `strconv.ParseBool`: verify export triggers for truthy values (`"true"`, `"TRUE"`, `"True"`, `"1"`, `"t"`, `"T"`), does NOT trigger for falsy values (`"false"`, `"FALSE"`, `"0"`, `"f"`), does NOT trigger and logs error for unrecognized values (`"yes"`, `"on"`, `"enabled"`), and does NOT trigger silently when unset/empty. Use `t.Setenv()` for isolation
- [x] 4.7 [P] Add test verifying `complyctl scan --help` output contains `COMPLYTIME_EXPORT_ENABLED`

## 5. Update proto comments

- [x] 5.1 Update the `Export` RPC comment in `api/plugin/plugin.proto` to reference `COMPLYTIME_EXPORT_ENABLED` instead of `--format otel`, and regenerate `plugin_grpc.pb.go`
- [x] 5.2 Update the `runExport()` function comment in `scan.go` to reference the env var trigger instead of `--format otel`

## 6. Update upstream spec and documentation

- [x] 6.1 Add supersession notice to `specs/003-complybeacon-export/spec.md` noting that the trigger mechanism (FR-001, FR-011, FR-012) is replaced by `COMPLYTIME_EXPORT_ENABLED` env var per this change
- [x] 6.2 Update `README.md` scan section with `COMPLYTIME_EXPORT_ENABLED` env var and usage example
- [x] 6.3 Update `AGENTS.md` "Recent Changes" section with breaking change entry
- [x] 6.4 Draft release notes / CHANGELOG entry covering the breaking change, migration path, and new capability

## 7. Final cleanup and verify

- [x] 7.1 Search codebase for remaining references to `OutputFormatOTEL`, `"otel"`, `--format otel`, and `format otel` in code and comments — remove/update all
- [x] 7.2 Run `go build ./...` and `go test ./...` to verify all changes compile and pass
