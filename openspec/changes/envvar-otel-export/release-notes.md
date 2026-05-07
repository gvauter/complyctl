## Breaking Changes

- **`--format otel` removed from `complyctl scan`**: The `otel` value for the `--format` flag has been removed. Export to a Beacon collector is now triggered via the `COMPLYTIME_EXPORT_ENABLED` environment variable.

  **Migration**: Replace `complyctl scan --format otel` with:
  ```bash
  COMPLYTIME_EXPORT_ENABLED=true complyctl scan [--format <format>]
  ```

  Export now works alongside any `--format` flag (e.g., `--format sarif`), allowing users to produce local reports and export evidence in a single scan invocation.

## New Features

- **`COMPLYTIME_EXPORT_ENABLED` environment variable**: Set to `true` (or any value accepted by Go's `strconv.ParseBool`: `1`, `t`, `T`, `TRUE`, `True`) to enable Beacon collector export after scan. Requires a `collector` section in `complytime.yaml`.

- **Doctor proactive diagnostic**: When a `collector` section is configured but `COMPLYTIME_EXPORT_ENABLED` is not set, `complyctl doctor` now warns that export will not trigger.
