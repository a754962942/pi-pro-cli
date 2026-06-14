# Version Comparison Flow

Created: 2026-06-14

## Purpose

This document defines how PI-Pro CLI compares the local CLI version with the server release version.

The version flow must be designed before implementation because it affects:

```text
pi-pro --version
pi-pro init
pi-pro update
install.sh
server endpoint contract
version-state.json
```

## Fixed Decisions

```text
local CLI version comes from CLI package configuration
server release version comes from the server
commit is not part of the public version output
version checking uses POST /cli/version
tests should use httptest mock server instead of real network
```

## Version Concepts

### Local Version

The local version is the version of the currently running CLI binary.

Source:

```text
internal/config
```

Recommended variable:

```go
var LocalVersion = "0.0.0-dev"
```

Release builds must inject this value with `ldflags`:

```sh
go build -ldflags "-X github.com/a754962942/pi-pro-cli/internal/config.LocalVersion=0.1.0"
```

Command code should read it from the config layer rather than hard-code it in command handlers. `LocalVersion` must be a `var`, not a `const`, because Go `ldflags -X` can only set string variables.

### Release Version

The release version is the latest server-approved CLI version for the current platform and release channel.

Source:

```text
POST /cli/version
```

The CLI should not infer release availability from Git tags, commit hashes, or local metadata.

### Minimum Supported Version

The server may declare the oldest CLI version that is still allowed to call runtime APIs.

Source:

```text
POST /cli/version response
```

This enables the server to force upgrades for incompatible CLI versions.

## Endpoint Contract

Request:

```http
POST /cli/version
Content-Type: application/json
```

Body:

```json
{
  "localVersion": "0.1.0",
  "os": "darwin",
  "arch": "arm64",
  "channel": "internal"
}
```

Response:

```json
{
  "localVersion": "0.1.0",
  "releaseVersion": "0.1.1",
  "minSupportedVersion": "0.1.0",
  "updateAvailable": true,
  "updateRequired": false,
  "binary": {
    "url": "https://api.example.com/cli/releases/0.1.1/darwin-arm64/pi-pro",
    "sha256": "abc123"
  },
  "initManifestVersion": "2026-06-14"
}
```

## Comparison Rules

The server is the source of truth for `updateAvailable` and `updateRequired`.

The CLI should still validate the response for consistency:

```text
releaseVersion must be present
minSupportedVersion must be present
updateRequired=true requires binary.url and binary.sha256
updateAvailable=true should include binary.url and binary.sha256 for update flows
localVersion in response should match the request when present
```

Client behavior:

```text
if updateRequired=true, block init/generation/task runtime operations until update
if updateAvailable=true and updateRequired=false, warn on stderr and continue unless command is pi-pro update
if updateAvailable=false, continue
if version check fails during pi-pro --version, return localVersion plus a structured version check error
if version check fails during pi-pro init, fail because init depends on server compatibility
```

## `pi-pro --version` Output

`pi-pro --version` should call `POST /cli/version` when a server URL is available.

Success:

```json
{
  "ok": true,
  "localVersion": "0.1.0",
  "releaseVersion": "0.1.1",
  "updateAvailable": true,
  "updateRequired": false
}
```

No `commit` field should be returned.

If the server cannot be reached, the command should still return local version information and expose the remote failure in a structured way:

```json
{
  "ok": true,
  "localVersion": "0.1.0",
  "releaseVersion": null,
  "versionCheck": {
    "ok": false,
    "code": "VERSION_CHECK_FAILED",
    "message": "Unable to check server release version."
  }
}
```

Rationale:

```text
--version is an inspection command, not a runtime compatibility gate
users and agents still need to know the installed local version when offline
```

## `pi-pro init` Behavior

`pi-pro init` must check version before downloading runtime files.

Flow:

```text
read local version from config
POST /cli/version
validate response
if updateRequired=true, fail with UPDATE_REQUIRED
write version-state.json
continue init manifest download
```

## `pi-pro update` Behavior

`pi-pro update` uses the same version response.

Flow:

```text
read local version from config
POST /cli/version
if updateAvailable=false, return ok=true changed=false
download binary.url
verify binary.sha256
replace managed binary atomically
write version-state.json
```

## `install.sh` Behavior

`install.sh` may also call `POST /cli/version`.

For first install, request body should omit `localVersion` or use:

```json
{
  "localVersion": "none",
  "os": "darwin",
  "arch": "arm64",
  "channel": "internal"
}
```

`install.sh` still must not download schemas or initialize runtime files.

## State File

Version check result should be written to:

```text
~/.pi-pro/version-state.json
```

Recommended shape:

```json
{
  "checkedAt": "2026-06-14T12:00:00Z",
  "localVersion": "0.1.0",
  "releaseVersion": "0.1.1",
  "minSupportedVersion": "0.1.0",
  "updateAvailable": true,
  "updateRequired": false
}
```

## Testing Requirements

Use `httptest.NewServer` for version tests.

Required cases:

```text
localVersion has a development default
localVersion can be injected through the same variable used by ldflags
POST request is used
localVersion is sent from config
releaseVersion is read from server response
commit is not returned
updateAvailable=false returns no update action
updateAvailable=true and updateRequired=false warns but allows non-update runtime flow
updateRequired=true blocks init/runtime flow
server failure keeps --version local output but marks versionCheck failed
invalid version response fails init/update
```
