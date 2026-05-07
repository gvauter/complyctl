## Context

The `complyctl scan` command currently supports four output formats via `--format`: `oscal`, `pretty`, `sarif`, and `otel`. The first three produce local files in their respective formats. The `otel` value is architecturally different — it triggers a post-scan export phase that ships evidence to a remote Beacon collector via provider gRPC `Export` RPCs.

The export logic lives in `cmd/complyctl/cli/scan.go` and is gated by `maybeExport()` checking `o.format == complytime.OutputFormatOTEL`. The underlying export orchestration (OIDC token resolution, provider routing, summary rendering) is mature and stays unchanged.

The collector endpoint and auth credentials are already configured via environment variables expanded in `complytime.yaml`. Adding an env var trigger aligns with that pattern.

**Supersedes**: This change supersedes the trigger mechanism defined in `specs/003-complybeacon-export/spec.md` — specifically FR-001 (`--format otel` as trigger), FR-011 (error without collector when `--format otel` used), and FR-012 (no behavior change when `--format otel` not used). The Export RPC, provider routing, and all other 003 requirements remain unchanged.

## Goals / Non-Goals

**Goals:**
- Decouple export triggering from the `--format` flag so users can export while also producing local format reports (e.g., `--format sarif` + export)
- Replace `--format otel` with `COMPLYTIME_EXPORT_ENABLED=true` environment variable
- Keep all existing export orchestration, OIDC auth, provider routing, and summary rendering unchanged
- Update tests to use the new trigger mechanism

**Non-Goals:**
- Changing the export protocol, provider interface, or gRPC contract
- Adding a `--export` CLI flag (env var only, consistent with collector credential configuration)
- Modifying the `Export` RPC, `ExportRequest`, `ExportResponse`, or `CollectorConfig` types
- Modifying any provider-side export logic (ProofWatch, GemaraEvidence, etc.)

## Decisions

### D1: Environment variable name — `COMPLYTIME_EXPORT_ENABLED`

Use `COMPLYTIME_EXPORT_ENABLED` as the env var name.

Rationale: Follows the `COMPLYTIME_` namespace prefix convention. The `_ENABLED` suffix is a common boolean toggle pattern. It is descriptive without being tied to the OTEL implementation detail.

Alternative considered: `COMPLYTIME_OTEL_EXPORT_ENABLED` — rejected because the env var controls whether complyctl triggers the export phase, not which protocol is used. The OTEL detail belongs to the provider/ProofWatch layer.

### D2: Trigger check location — in `maybeExport()` only

The `maybeExport()` function in `scan.go` currently checks `o.format != complytime.OutputFormatOTEL`. Replace this with a `strconv.ParseBool(os.Getenv("COMPLYTIME_EXPORT_ENABLED"))` call. When the env var is unset or empty, `ParseBool("")` returns `false, err` — treat this as "not enabled" silently (no warning for the common unset case). When `ParseBool` returns `true`, proceed with export. When `ParseBool` returns an error on a non-empty value (e.g., `"yes"`, `"on"`), log an error to stderr with the unrecognized value and the accepted values, then skip export. This uses Go's stdlib `strconv.ParseBool` (Constitution V: Do Not Reinvent the Wheel) which accepts `1`, `t`, `T`, `TRUE`, `true`, `True`, `0`, `f`, `F`, `FALSE`, `false`, `False`.

The collector config validation currently in `run()` (the `OutputFormatOTEL` guard block) moves into `runExport()` so it only runs when export is actually triggered. The nil check on `cfg.Collector` must occur before any dereference of collector fields.

Rationale: Single check point, minimal diff, no structural changes to the scan flow. `strconv.ParseBool` is stdlib and handles common boolean representations that operators use in CI/CD, systemd, and Kubernetes environments.

### D3: Remove `OutputFormatOTEL` constant entirely

Delete the `OutputFormatOTEL = "otel"` constant from `consts.go` and remove all references. The `--format` flag accepts only `oscal`, `pretty`, `sarif`. The format validation switch in `validate()` drops the `otel` case.

Rationale: Clean removal avoids dead code. If `otel` is accepted but silently ignored, users will be confused.

### D4: Evaluation log still written when export is enabled

When export is triggered, the scan still writes the evaluation log YAML (same as all other formats). If a `--format` is also specified, that format report is also written. Export runs after all local reports. When export is enabled but collector config is missing, the scan phase completes and writes the evaluation log before the export-phase error — the user gets partial output.

Rationale: Export and local reporting are now independent. Users can get both. The scan phase should not be blocked by export configuration issues.

### D5: Environment variable is env-var-only, not settable in `complytime.yaml`

`COMPLYTIME_EXPORT_ENABLED` is intentionally a process-level environment variable, not a field in `complytime.yaml`.

Rationale: This is a per-invocation toggle ("should this particular scan export?") rather than a workspace-level setting. The `complytime.yaml` collector section configures *where* to export; the env var controls *whether* to export for a given run. This separation is consistent with how CI/CD pipelines typically control feature gates (env vars) vs. configuration (config files).

### D6: No deprecation period — clean removal

The `--format otel` value is removed entirely. It is rejected with the standard invalid format error, the same as any unknown format value. No special migration error or deprecation warning.

Rationale: The feature was never deployed. There are no existing users to migrate. A migration-specific error message would be dead code.

## Risks / Trade-offs

- **[Breaking change]** Users and CI scripts using `--format otel` will break. → Mitigation: `otel` is rejected with the standard invalid format error. Migration path documented in release notes and README.
- **[Discoverability]** Env vars are less discoverable than CLI flags. → Mitigation: `complyctl scan --help` mentions the env var. `doctor` command references it when collector config is present but export is not enabled.
- **[Format + export interaction]** Users might set the env var and forget `--format`, getting no local report. → Acceptable: this matches existing behavior where no `--format` produces only the evaluation log YAML.
