# Architecture Review

Created: 2026-06-14

## Scope

This review checks the current PI-Pro CLI architecture documents for implementation readiness, ambiguity, consistency, and YAGNI alignment.

Reviewed documents:

```text
auth-and-config-flow.md
asset-io-sqlite-flow.md
cli-command-design.md
install-and-init-flow.md
output-and-error-contract.md
schema-contract.md
schema-registry-and-resolution-flow.md
server-endpoint-contract.md
server-responsibilities.md
task-polling-and-server-client-flow.md
validation-normalization-pipeline.md
```

## Overall Assessment

The architecture is mostly ready for Go project scaffolding.

The main CLI lifecycle is covered:

```text
install.sh
  ↓
pi-pro init
  ↓
pi-pro auth login
  ↓
types inspect/list
  ↓
generateImage/generateVoice/generateVideo
  ↓
task polling
  ↓
asset download and SQLite mapping
```

The most important boundaries are clear:

```text
CLI validates and normalizes agent input
Server performs provider-specific payload mapping
CLI polls tasks; server owns callbacks and task state
Schemas are data, not executable code
install.sh installs binary only
pi-pro init downloads mutable runtime files
```

## Ready Decisions

These areas are sufficiently defined for implementation:

```text
top-level command surface
provider/model/type schema key
one behavior per schema file
server-owned schema publishing
schema strongly bound to provider/model/type
invalid schema fails before generation submit
interactive username/password auth login
internal test-user full-access auth model
built-in serverUrl
init-downloaded schemas
SQLite asset identity vs file path separation
permanent server artifact URLs
no asset URL refresh flow
polling model for long-running tasks
stdout/stderr contract
server endpoint list
```

The required fixes listed below have been incorporated into the relevant architecture documents as of 2026-06-14. They remain here as an audit trail of the design review.

## Required Fixes Before Implementation

### 1. Server Update Flow Needs Install Path Policy

`install-and-init-flow.md` defines self-update, but binary replacement depends on where the binary is installed.

Ambiguity:

```text
~/.pi-pro/bin/pi-pro can be replaced by the user
/usr/local/bin/pi-pro may require elevated permissions
package-manager-managed binary should not be self-replaced
```

Recommendation:

```text
First implementation should install to ~/.pi-pro/bin/pi-pro by default.
Self-update only supports binaries under ~/.pi-pro/bin.
If current binary is outside managed install dir, return UPDATE_UNSUPPORTED_INSTALL_LOCATION.
```

### 2. install.sh Version Request Needs No-Local-Version Case

`POST /cli/version` must support first install before any local CLI version exists.

Ambiguity:

```text
install.sh may run before any pi-pro binary exists
```

Recommendation:

```text
Allow `localVersion=none` or omit `localVersion`.
Server should return latest binary for os/arch.
```

### 3. Init Manifest File URL Trust Boundary Needs Tightening

`GET /cli/init-manifest` returns file URLs.

Ambiguity:

```text
Can file URLs point to arbitrary hosts?
Can they use non-HTTPS?
```

Recommendation:

```text
First implementation should only allow HTTPS URLs from the same built-in server host, or relative file URLs resolved against serverUrl.
```

### 4. Asset Upload Requirement Should Be First-Implementation Conditional

`server-endpoint-contract.md` marks `POST /assets/upload` as required.

Ambiguity:

```text
If first schemas only use fileResolve: asset-db, upload is not immediately required.
If any first schema uses upload or asset-db-or-upload, upload is required.
```

Recommendation:

```text
Keep POST /assets/upload required only if first shipped schemas declare upload behavior.
Otherwise mark it "required when fileResolve upload is present".
```

### 5. Schema Refresh After Init Is Not Fully Defined

`pi-pro init` downloads schemas. Remote schema direct lookup is deferred.

Ambiguity:

```text
How does a user refresh schemas without reinstalling?
Is rerunning pi-pro init the only refresh mechanism?
```

Recommendation:

```text
Use pi-pro init as the first schema refresh command.
Defer pi-pro schema update or background refresh.
Document that rerunning init refreshes schemas from the latest manifest.
```

## Important Clarifications

### Auth Login Is Interactive by Design

This is correct:

```text
pi-pro auth login prompts username/password
generation/task commands never prompt
```

Implementation note:

```text
Password input must disable terminal echo.
If stdin is not a TTY, auth login should fail with INTERACTIVE_INPUT_REQUIRED unless a future non-interactive login mode is explicitly added.
```

### serverUrl Is Built In

This is correct for the first implementation and reduces configuration surface.

Implementation note:

```text
serverUrl should live in one Go package/config constant.
Do not duplicate it across install.sh and CLI if avoidable.
install.sh may query a public release URL, but CLI runtime serverUrl should remain canonical in the binary/package config.
```

### Schema Files Are Runtime Data

Current direction is consistent:

```text
schemas are downloaded by init
schemas are not embedded
schemas are not executable
```

Implementation note:

```text
Schema loader must validate metadata and reject unknown malformed top-level schema structures.
```

## YAGNI Check

Correctly deferred:

```text
browser login
remote schema direct resolution
schema cache
schema signatures
OS keychain
multi-profile
background updates
delta patching
debug/progress protocol
plugin system
```

Potential over-design to watch:

```text
asset hash computation for large files
self-update complexity
profile placeholders
schema cache abstraction before remote schemas exist
```

Recommendation:

```text
Keep interfaces small in Go. Add cache/profile/update subinterfaces only when the first implementation actually uses them.
```

## Missing Before Coding

One design module should still be added before implementation:

```text
Go Project Structure and Package Boundaries
```

Implementation sequencing is defined in [Development Roadmap](development-roadmap.md).

It should define:

```text
cmd/pi-pro
internal/commands
internal/config
internal/auth
internal/schema
internal/validation
internal/client
internal/task
internal/assets
internal/output
internal/init
internal/update
```

It should also define dependency direction so package imports stay clean.

## Implementation Readiness

Ready to start code after:

```text
1. Address required fixes in this review.
2. Add Go package boundary document.
3. Create Go project skeleton.
```

Do not start with all features at once. Suggested implementation order:

```text
1. output/error contract
2. config path and built-in serverUrl
3. init manifest download
4. schema registry from ~/.pi-pro/schemas
5. auth login/logout/status
6. validation pipeline
7. server client and task polling
8. asset SQLite flow
9. install.sh and update
```
