# Install and Init Flow

Created: 2026-06-14

## Context

PI-Pro CLI will be implemented as a Go binary and installed through `install.sh`. The binary should not embed schemas or other mutable runtime assets. Instead, `pi-pro init` should download the required initialization files from the built-in server.

This keeps the installer small and lets the server control schema/config bootstrap content without requiring a new CLI binary for every schema update.

## Design Goals

- Keep `install.sh` focused on binary installation.
- Keep mutable runtime files out of the Go binary.
- Let `pi-pro init` own application initialization.
- Download initialization files from the built-in server.
- Make init idempotent and safe to rerun.
- Check CLI version compatibility and support safe binary updates.
- Keep generation/task commands independent from install details.

## Responsibility Split

`install.sh` responsibilities:

```text
detect OS/arch
download matching Go binary
verify checksum
install binary
ensure executable bit
print next step: pi-pro init
```

`pi-pro init` responsibilities:

```text
create config directory
check CLI version against server
update CLI binary when the server requires a compatible version
download init manifest from server
download required schema/config assets
verify checksums
write files into local config directory
initialize SQLite asset database
record init metadata
return machine-readable JSON
```

## Local Directory Layout

Recommended first-implementation layout:

```text
~/.pi-pro/
  bin/
    pi-pro
  config.json
  assets.sqlite
  init-state.json
  version-state.json
  schemas/
    <provider>/
      <model>/
        <type>.json
```

`schemas/` is populated by `pi-pro init`, not by `install.sh` and not embedded in the binary.

`version-state.json` stores the latest successful version check metadata.

## Version Check

The CLI should support version checking against the built-in server.

Recommended endpoint:

```text
POST /cli/version
```

Request body:

```json
{
  "localVersion": "0.1.0",
  "os": "darwin",
  "arch": "arm64",
  "channel": "internal"
}
```

For first-time install, `install.sh` may omit `localVersion` or send `localVersion=none`.

Recommended response:

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

Version check result should be written to:

```text
~/.pi-pro/version-state.json
```

Recommended state shape:

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

## Auto Update Policy

The CLI should not silently replace itself during generation, task polling, upload, or download commands.

Safe auto-update entry points:

```text
pi-pro init
pi-pro update
```

Rationale:

```text
generation/task commands should remain deterministic once started
binary replacement during a long-running command can make debugging difficult
init/update are explicit lifecycle commands where mutation is expected
```

Recommended behavior:

```text
pi-pro init:
  check version first
  if localVersion != releaseVersion, stop and require pi-pro update before downloading init files

pi-pro update:
  check version
  if updateAvailable, download and replace binary
  if already latest, return ok with changed=false

generation/task commands:
  optionally read recent version-state
  do not auto-update
  if server rejects old client, return UPDATE_REQUIRED
```

## Binary Update Flow

`pi-pro update` is a terminal lifecycle command. After a successful update it must finish the current process instead of continuing into another CLI workflow. The updated binary is used on the next invocation.

```text
1. Read current binary path.
2. Request POST /cli/version with local version, OS, arch, and channel.
3. If no update is available, return changed=false.
4. Download new binary to a temp path under ~/.pi-pro/tmp.
5. Verify sha256.
6. chmod executable.
7. Backup current binary.
8. Atomically replace current binary when supported by the platform.
9. Write version-state.json.
10. Return update result JSON.
```

Recommended output:

```json
{
  "ok": true,
  "changed": true,
  "previousVersion": "0.1.0",
  "localVersion": "0.1.0",
  "releaseVersion": "0.1.1"
}
```

If already latest:

```json
{
  "ok": true,
  "changed": false,
  "localVersion": "0.1.1",
  "releaseVersion": "0.1.1"
}
```

If atomic replacement is not possible on a platform, the CLI should leave the downloaded binary staged and return an actionable error instead of partially replacing itself.

First implementation should only self-update binaries installed under:

```text
~/.pi-pro/bin/pi-pro
```

If the current binary is outside the managed install directory, return:

```text
UPDATE_UNSUPPORTED_INSTALL_LOCATION
```

## Cross-platform Updater Strategy

The update implementation must branch by platform behind a small updater boundary. The lifecycle flow should not contain OS-specific replacement details.

Recommended package boundary:

```text
internal/lifecycle:
  version check, download, checksum verification, update orchestration, JSON result

internal/updater:
  platform-specific binary replacement strategy
  managed install location checks
  staged update state handling

cmd/pi-pro-updater:
  helper process used by Windows update flow
```

Unix-like systems:

```text
1. pi-pro update downloads the new binary to ~/.pi-pro/tmp or ~/.pi-pro/updates.
2. CLI verifies sha256.
3. CLI chmods executable.
4. CLI replaces ~/.pi-pro/bin/pi-pro using rename semantics.
5. CLI writes version-state.json.
6. CLI returns update JSON and exits.
```

Windows:

```text
1. pi-pro.exe update downloads the new binary to ~/.pi-pro/updates/pi-pro-<version>.exe.
2. CLI verifies sha256.
3. CLI writes update-state.json with current binary path, staged binary path, expected sha256, target version, and parent process id.
4. CLI verifies that pi-pro-updater.exe exists in the managed bin directory.
5. CLI starts pi-pro-updater.exe with the update-state.json path.
6. CLI exits immediately after the helper starts.
7. pi-pro-updater.exe waits for the parent pi-pro.exe process to exit.
8. Helper replaces ~/.pi-pro/bin/pi-pro.exe with the staged binary.
9. Helper verifies the installed binary checksum.
10. Helper removes staged files and marks update-state.json as completed.
```

Windows requires a helper because a running `.exe` cannot be reliably overwritten by itself. If the first release supports Windows, the helper updater is mandatory and must be implemented before declaring update support complete on Windows.

Helper updater responsibilities:

```text
read and validate update-state.json
reject paths outside the managed ~/.pi-pro directory
wait for the main pi-pro.exe process to exit
replace only the managed ~/.pi-pro/bin/pi-pro.exe binary
verify sha256 after replacement
preserve enough failure state for troubleshooting
clean staged files after successful replacement
exit with a process status that install/update scripts can inspect later if needed
```

Helper updater must not:

```text
perform version checks
call server APIs
download binaries
parse CLI business flags
read auth tokens
modify schemas, config.json, or assets.sqlite
```

Packaging requirement:

```text
Windows release archives must include both pi-pro.exe and pi-pro-updater.exe.
install.sh or the platform installer must install both files into ~/.pi-pro/bin.
pi-pro update on Windows must fail with UPDATE_HELPER_MISSING if pi-pro-updater.exe is not present.
```

## Init Manifest

`pi-pro init` should fetch an initialization manifest from the built-in server.

Recommended endpoint:

```text
GET /cli/init-manifest
```

Recommended manifest shape:

```json
{
  "version": "2026-06-14",
  "files": [
    {
      "path": "schemas/seeddance/v1/image-to-video.json",
      "url": "https://api.example.com/cli/files/schemas/seeddance/v1/image-to-video.json",
      "sha256": "abc123",
      "required": true
    }
  ],
  "sqlite": {
    "assetDbSchemaVersion": 1
  }
}
```

The manifest should only describe files under the CLI config directory. Paths must be relative and must not contain `..`.

Manifest file URLs should be HTTPS URLs from the built-in server host, or relative URLs resolved against the built-in server URL.

## Init Flow

```text
1. Read built-in serverUrl.
2. Create ~/.pi-pro with restrictive permissions.
3. Check CLI version.
4. If localVersion != releaseVersion, stop with UPDATE_REQUIRED and ask the user to run pi-pro update.
5. Fetch /cli/init-manifest.
6. Validate manifest shape.
7. For each required file:
   - validate relative path
   - download file
   - verify sha256
   - write atomically to ~/.pi-pro/<path>
8. Initialize or migrate assets.sqlite.
9. Write init-state.json with manifest version and file metadata.
10. Return init result JSON.
```

Recommended output:

```json
{
  "ok": true,
  "initialized": true,
  "changed": true,
  "configDir": "/Users/example/.pi-pro",
  "manifestVersion": "2026-06-14",
  "files": {
    "downloaded": 12,
    "skipped": 0
  },
  "version": {
    "checked": true,
    "updated": false,
    "localVersion": "0.1.1",
    "releaseVersion": "0.1.1"
  }
}
```

If already initialized and files match:

```json
{
  "ok": true,
  "initialized": true,
  "changed": false,
  "manifestVersion": "2026-06-14"
}
```

## Idempotency

`pi-pro init` must be safe to rerun.

Recommended behavior:

```text
rerunning init fetches the latest init manifest
if local file exists and checksum matches, skip download
if local file exists and checksum differs, replace atomically
if download fails, keep existing valid file
if manifest cannot be fetched, fail unless a valid previous init exists and --offline is supported later
```

For the first implementation, rerunning `pi-pro init` is also the schema refresh mechanism. A separate schema update command is deferred.

First implementation can omit `--offline`; keep it as a future extension.

## Auth Policy

`pi-pro init` should not require auth for public bootstrap files.

Rationale:

```text
users need init before login can reliably work
schema/config bootstrap should not require auth unless the server explicitly requires protected schemas later
```

If protected schemas are needed later, add authenticated schema refresh as a separate flow rather than blocking first-run init.

## Security Rules

The init flow must protect the local filesystem:

```text
only write under ~/.pi-pro
reject absolute manifest paths
reject paths containing ..
verify sha256 before replacing files
write to temp file then atomic rename
use restrictive permissions for config directory
do not execute downloaded files
schemas are data only
```

## install.sh Flow

Recommended install script behavior:

```text
1. Detect platform.
2. Download the platform release package or binary metadata.
3. Verify checksums.
4. Install pi-pro to ~/.pi-pro/bin/pi-pro by default.
5. On Windows, also install pi-pro-updater.exe to ~/.pi-pro/bin/pi-pro-updater.exe.
6. Print PATH instructions if needed.
7. Print: Run `pi-pro init`.
```

`install.sh` should not download schemas, configs, or SQLite files.

`install.sh` may use the same `/cli/version` metadata to locate the latest binary, but it should not duplicate init logic.

It may optionally offer:

```text
--run-init
```

But first implementation can skip auto-init to keep installer behavior predictable.

## Error Codes

Recommended install/init error codes:

```text
INIT_MANIFEST_FETCH_FAILED
INIT_MANIFEST_INVALID
INIT_FILE_DOWNLOAD_FAILED
INIT_FILE_CHECKSUM_MISMATCH
INIT_FILE_WRITE_FAILED
INIT_PATH_INVALID
INIT_SQLITE_FAILED
VERSION_CHECK_FAILED
UPDATE_REQUIRED
UPDATE_DOWNLOAD_FAILED
UPDATE_CHECKSUM_MISMATCH
UPDATE_REPLACE_FAILED
UPDATE_UNSUPPORTED_INSTALL_LOCATION
UPDATE_HELPER_MISSING
UPDATE_HELPER_FAILED
```

## YAGNI Filter

Keep for the first implementation:

```text
Go binary install through install.sh
pi-pro init command
server init manifest
schema download into ~/.pi-pro/schemas
checksum verification
atomic file writes
SQLite initialization
idempotent rerun
version check endpoint
pi-pro update command
init blocks with UPDATE_REQUIRED when localVersion differs from releaseVersion
platform-specific updater boundary
Windows helper updater when Windows is supported in the first release
```

Defer until required:

```text
schemas embedded in binary
install.sh downloading all runtime files
offline init
multiple release channels
authenticated schema bootstrap
schema delta patching
automatic background schema updates
rollback command
plugin installation
silent self-update during generation/task commands
multi-channel update selection
delta binary patching
background helper updater on non-Windows platforms
```

## Design Decisions

- Use Go for the CLI binary.
- Keep schemas out of the binary.
- Keep install.sh limited to binary installation.
- Use `pi-pro init` to download runtime initialization files.
- Check CLI version during `pi-pro init`.
- Support `pi-pro update` for explicit binary updates.
- Use a helper updater for Windows binary replacement.
- Package `pi-pro-updater.exe` with every Windows release.
- Fetch init files from the built-in server.
- Do not silently replace the binary during generation/task commands.
- Make init safe, idempotent, checksum-verified, and data-only.
