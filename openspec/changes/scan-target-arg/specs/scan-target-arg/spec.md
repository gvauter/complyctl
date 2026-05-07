## ADDED Requirements

### Requirement: Scan command accepts optional target positional argument
The `complyctl scan` command SHALL accept an optional positional argument specifying a target ID from `complytime.yaml`. When provided, the scan SHALL be scoped to only that target.

#### Scenario: Scan a specific target with --policy-id
- **GIVEN** a `complytime.yaml` with targets `prod` and `staging`, both referencing policy `nist`
- **WHEN** the user runs `complyctl scan prod --policy-id nist`
- **THEN** only target `prod` is scanned for policy `nist`

#### Scenario: Scan a specific target without --policy-id (single policy)
- **GIVEN** a `complytime.yaml` with target `prod` referencing exactly one policy `nist`
- **WHEN** the user runs `complyctl scan prod`
- **THEN** the policy `nist` is inferred and target `prod` is scanned for it

#### Scenario: Scan a specific target without --policy-id (multiple policies)
- **GIVEN** a `complytime.yaml` with target `prod` referencing policies `nist` and `cis`
- **WHEN** the user runs `complyctl scan prod`
- **THEN** the command SHALL exit with an error listing the available policies for target `prod` and instructing the user to specify `--policy-id`

#### Scenario: Target not found in config
- **GIVEN** a `complytime.yaml` with targets `prod` and `staging`
- **WHEN** the user runs `complyctl scan nonexistent`
- **THEN** the command SHALL exit with an error indicating target `nonexistent` was not found and listing available target IDs

### Requirement: Target and policy mismatch produces an error
When both a target positional argument and `--policy-id` are specified, the command SHALL validate that the target references the given policy.

#### Scenario: Target does not reference the given policy
- **GIVEN** a `complytime.yaml` with target `prod` referencing policy `nist` only
- **WHEN** the user runs `complyctl scan prod --policy-id cis`
- **THEN** the command SHALL exit with an error indicating target `prod` does not reference policy `cis` and listing the target's available policies

### Requirement: At least one of target or --policy-id is required
The `complyctl scan` command SHALL require at least one of a target positional argument or the `--policy-id` flag.

#### Scenario: Neither target nor --policy-id provided
- **WHEN** the user runs `complyctl scan` with no arguments and no `--policy-id`
- **THEN** the command SHALL exit with an error explaining that at least a target or `--policy-id` is required

### Requirement: Backward compatibility with --policy-id only
When no positional argument is provided and `--policy-id` is specified, the scan command SHALL behave identically to the current implementation — scanning all targets that reference the given policy.

#### Scenario: Policy-only scan (existing behavior)
- **GIVEN** a `complytime.yaml` with targets `prod` and `staging`, both referencing policy `nist`
- **WHEN** the user runs `complyctl scan --policy-id nist`
- **THEN** both `prod` and `staging` are scanned for policy `nist` (unchanged behavior)

### Requirement: Shell completion for target argument
The scan command SHALL provide shell completion for the target positional argument using target IDs from `complytime.yaml`.

#### Scenario: Tab completion suggests target IDs
- **WHEN** the user invokes shell completion for the first positional argument of `complyctl scan`
- **THEN** the completion SHALL offer the target IDs defined in `complytime.yaml`

## MODIFIED Requirements

None.

## REMOVED Requirements

None.
