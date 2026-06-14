# Project Structure and Package Boundaries

Created: 2026-06-14

## Purpose

This document fixes the initial Go project structure for PI-Pro CLI. It is the implementation map for the architecture documents and the TDD test matrix.

## Directory Layout

```text
cmd/pi-pro/
  main.go

cmd/pi-pro-updater/
  main.go

internal/
  apperror/
  assets/
  auth/
  client/
  commands/
  config/
  errdefs/
  generation/
  input/
  lifecycle/
  output/
  schema/
  serverapi/
  task/
  testutil/
  updater/
  validation/

scripts/
  install.sh

testdata/
  schemas/
  manifests/
  responses/
  assets/
```

## Package Responsibilities

### `cmd/pi-pro`

Process entry only.

Allowed responsibilities:

```text
read os.Args
wire stdout/stderr
return process exit code
```

Not allowed:

```text
business logic
HTTP calls
config reads
schema validation
```

### `cmd/pi-pro-updater`

Windows helper updater entry point.

Allowed responsibilities:

```text
load update-state.json
call internal/updater helper execution
return process exit code
```

Not allowed:

```text
normal CLI command handling
auth
server API calls
generation/task behavior
schema/config initialization
```

This binary is required only for platforms that cannot reliably replace the running main binary. If Windows is supported in the first release, `pi-pro-updater.exe` must be built and packaged with `pi-pro.exe`.

### `internal/commands`

CLI command dispatch and argument binding.

This package should stay thin. It translates CLI arguments into service calls and renders results through `internal/output`.

### `internal/output`

Stable stdout JSON and error rendering.

All machine-readable output must go through this package so agent callers receive predictable responses.

### `internal/apperror`

Application error taxonomy and exit-code categories.

Errors from lower packages should be converted into `AppError` before reaching command handlers.

### `internal/errdefs`

Centralized error codes and default user-facing messages.

All stable `error.code` values and their default messages should be defined here before they are used by command handlers or services.

### `internal/config`

Local config path resolution and config file model.

The built-in server URL belongs here. The first version must not expose `--server-url`.

### `internal/client`

HTTP client foundation.

Allowed responsibilities:

```text
server URL usage
Authorization header injection
request/response JSON handling
server error mapping
```

Not allowed:

```text
reading config files directly
provider-specific payload mapping
schema validation
```

### `internal/auth`

Login/logout/status flows and token storage abstraction.

The first version supports only interactive username/password login. Browser login is deferred.

### `internal/lifecycle`

`init` and `update` flows.

`init` downloads runtime files from the server into the local config directory. The Go binary does not embed schemas.

`update` coordinates version checks, binary downloads, checksum verification, and delegates platform-specific replacement to `internal/updater`.

### `internal/updater`

Platform-specific managed binary replacement.

Responsibilities:

```text
validate managed install location
stage update state
replace the managed binary on Unix-like platforms
start the Windows helper updater when required
execute Windows helper replacement flow
verify installed binary checksum
clean staged update files after success
```

Not allowed:

```text
version endpoint calls
binary downloads
schema/config initialization
auth token reads
generation/task orchestration
stdout JSON rendering
```

Recommended implementation structure:

```text
internal/updater/engine.go
internal/updater/engine_unix.go
internal/updater/engine_windows.go
internal/updater/state.go
```

The platform split should use Go build tags or OS-specific files so Windows-only process behavior does not leak into Unix update code.

### `internal/schema`

Local schema registry and schema resolution.

Schema lookup key is:

```text
provider + model + type
```

Each schema file represents one behavior only.

### `internal/serverapi`

Centralized server endpoint path constants and small endpoint path builders.

All CLI-server API paths such as `/cli/version`, `/cli/init-manifest`, `/auth/login`, `/generations`, and task endpoints should be defined here before use.

### `internal/validation`

Schema-driven validation, default injection, and normalization.

This package must not contain provider-specific mapper code. Provider payload mapping belongs on the server side.

### `internal/input`

Input source loading and merge rules.

Responsibilities:

```text
parse --input JSON
read stdin when requested
merge flags and JSON input
prefer input JSON when multiple sources provide the same field
```

### `internal/assets`

Local asset database and file path to URL resolution.

Responsibilities:

```text
resolve known local file paths to permanent server URLs
record downloaded artifact mappings
support explicit upload only when requested by schema
```

### `internal/task`

Long-running task polling and cancellation.

Progress diagnostics must go to stderr. Final task result must go to stdout JSON.

### `internal/generation`

Generate command orchestration.

This package coordinates:

```text
input loading
schema resolution
validation/normalization
asset resolution
POST /generations
task polling
artifact download
final response shaping
```

It must not implement provider-specific payload mapping.

### `internal/testutil`

Shared test helpers only.

Production packages must not import this package.

## Dependency Direction

```text
cmd/pi-pro
  -> internal/commands
    -> internal/output
    -> internal/apperror
    -> internal/errdefs
    -> internal/config
    -> internal/auth
    -> internal/lifecycle
    -> internal/schema
    -> internal/serverapi
    -> internal/validation
    -> internal/assets
    -> internal/task
    -> internal/generation
    -> internal/client
```

Rules:

```text
commands may depend on services
services may depend on client/config/apperror
lifecycle may depend on updater
output may depend on apperror
commands and services may depend on errdefs for stable error codes and default messages
client must not depend on commands
updater must not depend on client, auth, schema, assets, task, generation, or commands
validation must not depend on client
schema must not depend on client unless implementing remote schema lookup in a later phase
testutil must not be imported by production code
```

## TDD Placement

Prefer package-level tests next to implementation:

```text
internal/output/output_test.go
internal/config/config_test.go
internal/schema/registry_test.go
```

Use `testdata/` for shared fixtures that simulate server-published schemas, manifests, responses, and assets.

## First Implementation Gate

Before adding business behavior, the skeleton should satisfy:

```text
go test ./...
go run ./cmd/pi-pro --help
go run ./cmd/pi-pro --version
```
