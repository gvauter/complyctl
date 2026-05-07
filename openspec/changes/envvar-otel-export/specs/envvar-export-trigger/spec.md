## ADDED Requirements

### Requirement: Export triggered by environment variable
The `complyctl scan` command SHALL trigger evidence export to the configured Beacon collector when the environment variable `COMPLYTIME_EXPORT_ENABLED` is set to a truthy value as defined by Go's `strconv.ParseBool` (`1`, `t`, `T`, `TRUE`, `true`, `True`). Export SHALL NOT be triggered by any `--format` flag value. When the env var is set to a value that `strconv.ParseBool` cannot parse, the command SHALL log an error and disable export.

#### Scenario: Export enabled via env var
- **GIVEN** a workspace with a valid `collector` section in `complytime.yaml` and providers that support export
- **WHEN** the user runs `complyctl scan` with `COMPLYTIME_EXPORT_ENABLED` set to a truthy value (e.g., `true`, `TRUE`, `1`)
- **THEN** the scan command executes the scan phase, writes any requested format report, and then runs the export phase to ship evidence to the collector

#### Scenario: Export enabled with format flag
- **GIVEN** a workspace with a valid `collector` section in `complytime.yaml`
- **WHEN** `COMPLYTIME_EXPORT_ENABLED=true` is set and `--format sarif` is also specified
- **THEN** the scan command produces the SARIF report locally AND runs the export phase

#### Scenario: Export disabled via falsy value
- **GIVEN** a workspace with or without a `collector` section
- **WHEN** `COMPLYTIME_EXPORT_ENABLED` is not set, set to an empty string, or set to a falsy value (`false`, `FALSE`, `0`, `f`, `F`)
- **THEN** the scan command SHALL NOT run the export phase regardless of other flags or configuration

#### Scenario: Export env var set to unrecognized value
- **GIVEN** a workspace with or without a `collector` section
- **WHEN** `COMPLYTIME_EXPORT_ENABLED` is set to a value that `strconv.ParseBool` cannot parse (e.g., `yes`, `on`, `enabled`)
- **THEN** the scan command SHALL log an error to stderr indicating the value is not recognized and listing accepted values, and SHALL NOT run the export phase

#### Scenario: Export enabled without collector config
- **GIVEN** no `collector` section exists in `complytime.yaml`
- **WHEN** the user runs `complyctl scan` with `COMPLYTIME_EXPORT_ENABLED=true` set
- **THEN** the scan phase completes and writes the evaluation log, then the command exits with an error indicating no collector is configured

### Requirement: Format flag no longer accepts otel
The `--format` flag on the `complyctl scan` command SHALL accept only `oscal`, `pretty`, and `sarif` as valid values. The value `otel` SHALL be rejected as an invalid format.

#### Scenario: User passes --format otel
- **WHEN** a user runs `complyctl scan --format otel`
- **THEN** the command SHALL exit with the standard invalid format error

#### Scenario: Shell completion excludes otel
- **WHEN** a user invokes shell completion for the `--format` flag
- **THEN** only `oscal`, `pretty`, and `sarif` SHALL be offered as completions

### Requirement: Scan help text documents export env var
The `complyctl scan` command help text SHALL mention the `COMPLYTIME_EXPORT_ENABLED` environment variable and its purpose.

#### Scenario: Help text includes env var reference
- **WHEN** a user runs `complyctl scan --help`
- **THEN** the output SHALL include a reference to `COMPLYTIME_EXPORT_ENABLED` for enabling Beacon collector export

### Requirement: Doctor warns when collector configured but export not enabled
The `complyctl doctor` command SHALL warn when a `collector` section is present in `complytime.yaml` but `COMPLYTIME_EXPORT_ENABLED` is not set or is falsy. This helps operators diagnose the common misconfiguration where the collector is configured but export never triggers.

#### Scenario: Collector configured but export not enabled
- **GIVEN** a `complytime.yaml` with a valid `collector` section
- **WHEN** the user runs `complyctl doctor` without `COMPLYTIME_EXPORT_ENABLED` set (or set to a falsy value)
- **THEN** the doctor output SHALL include a warning indicating that a collector is configured but export is not enabled, and suggest setting `COMPLYTIME_EXPORT_ENABLED=true`

#### Scenario: Collector configured and export enabled
- **GIVEN** a `complytime.yaml` with a valid `collector` section
- **WHEN** the user runs `complyctl doctor` with `COMPLYTIME_EXPORT_ENABLED=true` set
- **THEN** the doctor output SHALL NOT emit the "export not enabled" warning

## MODIFIED Requirements

None — all changes are additions or removals.

## REMOVED Requirements

### Requirement: Format flag accepts otel value
**Reason**: The `otel` format value conflated output format selection with export behavior. Export is now triggered independently via the `COMPLYTIME_EXPORT_ENABLED` environment variable.
**Migration**: Replace `--format otel` with `COMPLYTIME_EXPORT_ENABLED=true complyctl scan [--format <format>]`.
