# TDD Test Cases

Created: 2026-06-14

## Purpose

This document defines the first implementation test cases for PI-Pro CLI. Development should follow TDD: write the failing test first, implement the smallest code to pass, then refactor.

The test cases are business-driven and aligned with the current architecture documents.

## Test Principles

```text
test public CLI behavior first
mock server endpoints
avoid real network
avoid real user home directory
use PI_PRO_CONFIG_DIR for isolated test state
assert stdout JSON exactly enough for contract stability
assert stderr never pollutes stdout
assert deferred features are not accidentally implemented
```

## Test Fixtures

Recommended fixture root:

```text
testdata/
  schemas/
  manifests/
  responses/
  assets/
```

Recommended shared fixtures:

```text
schema seeddance/v1/image-to-video.json
init manifest with one schema
valid auth login response
queued/running/succeeded task responses
failed task response
small png asset
small mp4 artifact placeholder
```

## Coverage Index

```text
Output contract: TC-001 to TC-004
Init/schema bootstrap: TC-010 to TC-016, TC-100 to TC-103
Auth: TC-020 to TC-024, TC-110 to TC-113
Schema registry: TC-030 to TC-033, TC-120 to TC-123
Validation: TC-040 to TC-045, TC-130 to TC-135
Asset IO: TC-050 to TC-053, TC-140 to TC-144
Task polling: TC-060 to TC-063, TC-150 to TC-154
Generation E2E: TC-070 to TC-073, TC-160 to TC-162
Install/update: TC-090 to TC-095
Deferred guards: TC-080 to TC-082
State/concurrency: TC-170 to TC-172
Server endpoint contract: TC-180 to TC-183
```

## Phase 1: Output and Error Contract

### TC-001 Success JSON Envelope

Given a command succeeds.

When the CLI writes output.

Then stdout contains valid JSON with:

```json
{
  "ok": true
}
```

And stderr may contain diagnostics.

And exit code is `0`.

### TC-002 Error JSON Envelope

Given a command fails with a known app error.

When the CLI writes output.

Then stdout contains:

```json
{
  "ok": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "..."
  }
}
```

And exit code is non-zero.

### TC-003 Progress Never Writes To Stdout

Given a polling command emits progress.

When the command runs.

Then progress appears on stderr.

And stdout contains only the final JSON object.

### TC-004 Secret Redaction

Given an error includes `authToken`, `password`, or `Authorization`.

When output is rendered.

Then stdout and stderr do not contain the raw secret.

## Phase 2: Init and Schema Bootstrap

### TC-010 Init Creates Local State

Given `PI_PRO_CONFIG_DIR` points to an empty temp directory.

And the mock server returns a valid init manifest.

When running:

```sh
pi-pro init
```

Then the CLI creates:

```text
config.json
assets.sqlite
init-state.json
schemas/seeddance/v1/image-to-video.json
```

And stdout returns:

```json
{
  "ok": true,
  "initialized": true
}
```

### TC-011 Init Is Idempotent

Given init already completed.

And local files match manifest checksums.

When running `pi-pro init` again.

Then no schema file is rewritten.

And stdout includes:

```json
{
  "ok": true,
  "changed": false
}
```

### TC-012 Init Replaces Changed Schema Atomically

Given a local schema exists with a checksum different from the manifest.

When running `pi-pro init`.

Then the CLI downloads the new schema.

And verifies checksum before replacing the old file.

And no partial file remains on failure.

### TC-013 Init Rejects Path Traversal

Given manifest contains:

```json
{
  "path": "../evil.json"
}
```

When running `pi-pro init`.

Then the command fails with:

```text
INIT_PATH_INVALID
```

And no file is written outside `PI_PRO_CONFIG_DIR`.

### TC-014 Init Rejects Checksum Mismatch

Given manifest checksum does not match downloaded file.

When running `pi-pro init`.

Then command fails with:

```text
INIT_FILE_CHECKSUM_MISMATCH
```

And the invalid file is not installed.

### TC-015 Init Checks Version

Given the server returns `updateRequired=false`.

When running `pi-pro init`.

Then the CLI calls `/cli/version`.

And continues to fetch `/cli/init-manifest`.

### TC-016 Init Stops On Required Unsupported Update

Given the server returns `updateRequired=true`.

And the current binary is not under the managed install directory.

When running `pi-pro init`.

Then command fails with:

```text
UPDATE_UNSUPPORTED_INSTALL_LOCATION
```

And init manifest is not downloaded.

## Phase 3: Auth

### TC-020 Auth Login Stores Token

Given the mock server accepts username/password.

When running:

```sh
pi-pro auth login
```

And the user enters username then password interactively.

Then the CLI calls `POST /auth/login`.

And stores:

```json
{
  "authToken": "sk-pipro-xxxxxxxxxxxxxxxxxxxxxxxx",
  "username": "user@example.com"
}
```

in user config.

And stdout does not contain the auth token.

### TC-021 Auth Login Hides Password Input

Given login prompts for password.

When the user types a password.

Then terminal echo is disabled for password input.

And password is never written to stdout/stderr.

### TC-022 Auth Login Requires TTY

Given stdin is not a TTY.

When running `pi-pro auth login`.

Then command fails with:

```text
INTERACTIVE_INPUT_REQUIRED
```

### TC-023 Auth Status Does Not Print Token

Given auth token exists in config.

When running:

```sh
pi-pro auth status
```

Then stdout includes `authenticated: true`.

And stdout does not contain the token.

### TC-024 Auth Logout Removes Token

Given auth token exists in config.

When running:

```sh
pi-pro auth logout
```

Then token is removed.

And subsequent `auth status` returns `authenticated: false`.

## Phase 4: Schema Registry

### TC-030 Types List Reads Local Init Schemas

Given `pi-pro init` downloaded schemas.

When running:

```sh
pi-pro types list
```

Then stdout lists:

```json
{
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video"
}
```

### TC-031 Types Inspect Returns Full Schema

Given schema exists at:

```text
schemas/seeddance/v1/image-to-video.json
```

When running:

```sh
pi-pro types inspect --provider seeddance --model v1 --type image-to-video
```

Then stdout returns the full schema.

### TC-032 Schema Metadata Mismatch Fails

Given schema path is:

```text
schemas/seeddance/v1/image-to-video.json
```

But schema content says:

```json
{
  "provider": "happy-horse"
}
```

When loading schema.

Then command fails with:

```text
SCHEMA_METADATA_MISMATCH
```

### TC-033 Invalid Schema Blocks Generation

Given schema is malformed or missing required schema fields.

When running a generation command using that schema.

Then command fails with:

```text
SCHEMA_INVALID
```

And `POST /generations` is not called.

## Phase 5: Validation and Normalization

### TC-040 Input JSON Has Priority

Given `request.json` contains:

```json
{
  "duration": 5
}
```

And CLI args include `--duration 10`.

When normalizing input.

Then final input uses:

```json
{
  "duration": 5
}
```

### TC-041 Stdin Input Is Supported

Given valid JSON is passed through stdin.

When running:

```sh
pi-pro generateVideo --provider seeddance --model v1 --type image-to-video --input -
```

Then CLI reads stdin as request JSON.

### TC-042 Required Field Missing

Given schema requires `prompt`.

And input does not include `prompt`.

When validating input.

Then command fails with:

```text
MISSING_REQUIRED_FIELD
```

And server is not called.

### TC-043 Unknown Field Rejected

Given schema `unknownPolicy` is `reject`.

And input includes an unknown field.

When validating input.

Then command fails with:

```text
UNKNOWN_FIELD
```

### TC-044 False And Zero Are Preserved

Given input contains:

```json
{
  "cameraFixed": false,
  "seed": 0
}
```

When normalizing.

Then both fields remain present.

### TC-045 Null And Empty String Are Treated As Missing

Given optional field has a default.

And input field is `null` or `""`.

When normalizing.

Then the default value is injected.

## Phase 6: Asset IO and SQLite

### TC-050 File Path Resolves To Existing URL

Given SQLite contains an asset location for `/tmp/a.png`.

When input references `/tmp/a.png`.

Then normalized input replaces the file path with the stored URL.

### TC-051 Moved File Recovers By Hash

Given SQLite has an asset with `sha256` and `sizeBytes`.

And the file is moved to a new path.

When input references the new path.

Then CLI computes hash, finds the asset, records the new location, and uses the existing URL.

### TC-052 Missing Asset Mapping Fails

Given a local file exists.

And no path or hash match exists in SQLite.

And schema uses `fileResolve: asset-db`.

When validating input.

Then command fails with:

```text
ASSET_URL_NOT_FOUND
```

### TC-053 Artifact Download Records Asset

Given generation succeeds and returns a permanent artifact URL.

And user provides `--output`.

When CLI downloads artifact.

Then SQLite records source URL, local path, mime, size, and hash.

## Phase 7: Task Polling

### TC-060 No Wait Returns Job

Given server returns:

```json
{
  "jobId": "job_123",
  "status": "queued"
}
```

When running generation with `--no-wait`.

Then stdout returns `status: submitted`.

And no polling request is made.

### TC-061 Wait Polls Until Success

Given server returns `queued`, then `running`, then `succeeded`.

When running generation with default wait.

Then CLI polls until `succeeded`.

And returns final artifacts.

### TC-062 Timeout Does Not Cancel

Given server keeps returning `running`.

When timeout is reached.

Then CLI returns:

```text
TASK_TIMEOUT
```

And does not call cancel.

### TC-063 Failed Task Preserves Server Error

Given server returns:

```json
{
  "status": "failed",
  "error": {
    "code": "PROVIDER_QUOTA_EXCEEDED"
  }
}
```

When polling completes.

Then CLI returns `TASK_FAILED`.

And preserves server code in details.

## Phase 8: Generation End-to-End

### TC-070 Generate Video Happy Path

Given:

```text
schema exists
auth token exists
asset mapping exists
server accepts generation
task succeeds
```

When running:

```sh
pi-pro generateVideo --provider seeddance --model v1 --type image-to-video --input request.json --output video.mp4
```

Then CLI:

```text
loads schema
validates input
resolves file URL
POSTs /generations
polls task
downloads artifact
records SQLite mapping
prints ok=true JSON
```

### TC-071 Invalid Schema Prevents Generation

Given schema fails validation.

When running generation.

Then `POST /generations` is not called.

### TC-072 Auth Missing Prevents Generation

Given no auth token exists.

When running generation.

Then CLI fails with:

```text
AUTH_REQUIRED
```

And server is not called.

### TC-073 Download Failure Is Partial Failure

Given task succeeds.

And artifact download fails.

When generation command completes.

Then CLI returns:

```json
{
  "ok": false,
  "status": "succeeded",
  "error": {
    "code": "ARTIFACT_DOWNLOAD_FAILED"
  }
}
```

And preserves remote artifact URL.

## Phase 9: Deferred Feature Guard Tests

### TC-080 Browser Login Is Not Implemented

When running:

```sh
pi-pro auth login --browser
```

Then CLI returns a usage error.

### TC-081 Server URL Override Is Not Implemented

When running a command with:

```sh
--server-url
```

Then CLI returns a usage error.

### TC-082 Remote Schema Direct Lookup Is Not Used

Given local schema exists.

When running `types inspect`.

Then CLI reads local schema.

And does not call remote `/types/:provider/:model/:type/schema`.

## Phase 10: Install And Update

### TC-090 install.sh Uses Version Endpoint Without Current Version

Given no local `pi-pro` binary exists.

When `install.sh` checks for the latest binary.

Then it calls `POST /cli/version` with `localVersion=none` or omits `localVersion`.

And downloads the platform-specific binary from the response.

### TC-091 install.sh Does Not Download Runtime Files

Given `install.sh` completes successfully.

Then it installs only the binary.

And it does not create:

```text
schemas/
assets.sqlite
init-state.json
```

And it prints `pi-pro init` as the next step.

### TC-092 Update Already Latest

Given `/cli/version` returns `updateAvailable=false`.

When running:

```sh
pi-pro update
```

Then stdout returns:

```json
{
  "ok": true,
  "changed": false
}
```

### TC-093 Update Replaces Managed Binary

Given the current binary is under:

```text
~/.pi-pro/bin/pi-pro
```

And `/cli/version` returns a newer binary with a valid checksum.

When running `pi-pro update`.

Then the CLI downloads, verifies, and atomically replaces the binary.

And writes `version-state.json`.

### TC-094 Update Rejects Unmanaged Binary

Given the current binary is outside:

```text
~/.pi-pro/bin/
```

And update is required.

When running `pi-pro update`.

Then command fails with:

```text
UPDATE_UNSUPPORTED_INSTALL_LOCATION
```

### TC-095 Update Rejects Binary Checksum Mismatch

Given `/cli/version` returns a binary checksum.

And downloaded binary bytes do not match.

When running `pi-pro update`.

Then command fails with:

```text
UPDATE_CHECKSUM_MISMATCH
```

And current binary is not replaced.

## Phase 11: Init Manifest Security

### TC-100 Init Rejects Absolute Manifest Path

Given init manifest contains:

```json
{
  "path": "/tmp/evil.json"
}
```

When running `pi-pro init`.

Then command fails with:

```text
INIT_PATH_INVALID
```

### TC-101 Init Rejects Cross-Host File URL

Given built-in server host is `api.example.com`.

And manifest file URL points to `https://evil.example.com/schema.json`.

When running `pi-pro init`.

Then command fails with:

```text
INIT_MANIFEST_INVALID
```

### TC-102 Init Rejects Non-HTTPS File URL

Given manifest file URL uses `http://`.

When running `pi-pro init`.

Then command fails with:

```text
INIT_MANIFEST_INVALID
```

### TC-103 Init Keeps Existing Valid File On Download Failure

Given a local schema file exists and matches the previous init state.

And the new manifest download fails for that file.

When running `pi-pro init`.

Then the existing valid file remains in place.

And the command returns a structured init download error.

## Phase 12: Auth Failure Cases

### TC-110 Auth Login Invalid Credentials

Given server rejects username/password with HTTP 401.

When running `pi-pro auth login`.

Then command fails with:

```text
AUTH_INVALID
```

And no auth token is stored.

### TC-111 Auth Login Empty Username

Given the user submits an empty username.

When running `pi-pro auth login`.

Then the CLI fails before calling server with:

```text
VALIDATION_ERROR
```

### TC-112 Auth Login Empty Password

Given the user submits an empty password.

When running `pi-pro auth login`.

Then the CLI fails before calling server with:

```text
PASSWORD_REQUIRED
```

### TC-113 Generation Rejects Invalid Stored Token

Given user config contains an auth token.

And server returns HTTP 401 for `POST /generations`.

When running generation.

Then CLI returns:

```text
AUTH_INVALID
```

And does not retry with interactive login.

## Phase 13: Schema Contract Edge Cases

### TC-120 Type Not Found

Given no schema exists for:

```text
provider=seeddance model=v9 type=image-to-video
```

When running `types inspect`.

Then command fails with:

```text
TYPE_NOT_FOUND
```

### TC-121 Wrong Artifact Kind Rejects Command

Given schema has:

```json
{
  "artifactKind": "image"
}
```

When running:

```sh
pi-pro generateVideo --provider openai --model v1 --type text-to-image
```

Then command fails with:

```text
COMMAND_TYPE_MISMATCH
```

And `POST /generations` is not called.

### TC-122 Schema With Multiple Behaviors Is Invalid

Given a schema file contains a top-level `types` map.

When loading schema.

Then command fails with:

```text
SCHEMA_INVALID
```

Because one schema file must represent exactly one behavior.

### TC-123 Schema Cannot Declare Executable Mapper

Given a schema contains fields that request remote code execution, such as:

```json
{
  "mapperUrl": "https://example.com/mapper.js"
}
```

When loading schema.

Then command fails with:

```text
SCHEMA_INVALID
```

## Phase 14: Validation Edge Cases

### TC-130 Invalid Input JSON

Given `--input request.json` contains invalid JSON.

When running generation.

Then command fails with:

```text
INVALID_INPUT_JSON
```

### TC-131 Input File Missing

Given `--input missing.json`.

When running generation.

Then command fails with:

```text
INVALID_INPUT_JSON
```

or a more specific file read error mapped under validation/usage.

### TC-132 Enum Value Rejected

Given schema declares:

```json
{
  "resolution": {
    "enum": ["720p", "1080p"]
  }
}
```

And input uses:

```json
{
  "resolution": "4k"
}
```

Then command fails with:

```text
INVALID_ENUM_VALUE
```

### TC-133 Number Range Rejected

Given schema declares `duration` maximum `10`.

And input uses `duration: 30`.

Then command fails with:

```text
INVALID_RANGE
```

### TC-134 Required Null Still Fails

Given schema requires `prompt`.

And input contains:

```json
{
  "prompt": null
}
```

Then command fails with:

```text
MISSING_REQUIRED_FIELD
```

### TC-135 Empty String Required Field Fails

Given schema requires `prompt`.

And input contains:

```json
{
  "prompt": ""
}
```

Then command fails with:

```text
MISSING_REQUIRED_FIELD
```

## Phase 15: Asset IO Edge Cases

### TC-140 Hash Match Ambiguous

Given two asset records share the same `sha256` and `sizeBytes`.

And path lookup misses.

When resolving a moved file.

Then command fails with:

```text
ASSET_MATCH_AMBIGUOUS
```

### TC-141 Existing Output Path Refuses Overwrite

Given `--output video.mp4` already exists.

And `--overwrite` is not provided.

When artifact download would write to that path.

Then command fails with:

```text
OUTPUT_PATH_EXISTS
```

### TC-142 Overwrite Allows Existing Output Path

Given `--output video.mp4` already exists.

And `--overwrite` is provided.

When artifact download succeeds.

Then the local file is replaced.

And SQLite is updated.

### TC-143 Multiple Artifacts With Single Output Fails

Given server returns two artifacts.

And user provided `--output one-file`.

Then command fails with:

```text
OUTPUT_PATH_AMBIGUOUS
```

### TC-144 Output Dir Uses Deterministic Names

Given server returns two artifacts.

And user provided `--output-dir outputs`.

Then files are written as:

```text
<jobId>-1<extension>
<jobId>-2<extension>
```

## Phase 16: Server Contract And Network Cases

### TC-150 Retry Transient Polling Error

Given `GET /tasks/:jobId` returns HTTP 503 once.

And then returns `running`.

When polling.

Then CLI retries within timeout budget.

### TC-151 Non-Retryable Bad Request

Given server returns HTTP 400 for `POST /generations`.

When generation runs.

Then CLI returns:

```text
SERVER_REQUEST_FAILED
```

And does not retry.

### TC-152 Task Not Found

Given `GET /tasks/job_missing` returns HTTP 404.

When running:

```sh
pi-pro task status job_missing
```

Then CLI returns:

```text
TASK_NOT_FOUND
```

### TC-153 Invalid Server Response

Given server returns malformed JSON.

When CLI expects a JSON response.

Then CLI returns:

```text
SERVER_RESPONSE_INVALID
```

### TC-154 Update Required From Server

Given server rejects a generation request because the CLI version is too old.

When generation runs.

Then CLI returns:

```text
UPDATE_REQUIRED
```

And does not attempt silent self-update.

## Phase 17: Command Surface Tests

### TC-160 Generate Image Requires Image Artifact Kind

Given schema artifact kind is `video`.

When running `generateImage` with that schema.

Then command fails with:

```text
COMMAND_TYPE_MISMATCH
```

### TC-161 Generate Voice Happy Path

Given a valid `text-to-speech` schema exists.

And server returns a voice artifact.

When running:

```sh
pi-pro generateVoice --provider minimax --model v1 --type text-to-speech --input request.json --output voice.mp3
```

Then CLI returns `ok: true`.

And records the voice artifact in SQLite.

### TC-162 Generate Image Happy Path

Given a valid `text-to-image` schema exists.

And server returns an image artifact.

When running:

```sh
pi-pro generateImage --provider openai --model v1 --type text-to-image --input request.json --output image.png
```

Then CLI returns `ok: true`.

And records the image artifact in SQLite.

### TC-163 Unknown Command Fails With Usage Error

When running:

```sh
pi-pro generateAudio
```

Then CLI exits with usage/validation exit code.

And stdout returns an error JSON if the command framework supports JSON for usage errors.

## Phase 18: State Isolation And Concurrency

### TC-170 Config Dir Isolation

Given `PI_PRO_CONFIG_DIR` points to a temp directory.

When running init/auth/types.

Then no files are written under the real home directory.

### TC-171 Concurrent Init Does Not Corrupt Files

Given two `pi-pro init` processes run at the same time against the same config dir.

When both finish.

Then schema files are valid.

And SQLite is not corrupted.

At least one process may return a lock/try-again error if locking is implemented.

### TC-172 Concurrent Asset Writes Are Safe

Given two artifact downloads record assets concurrently.

When both complete.

Then SQLite remains valid.

And both asset records exist or duplicate records are safely deduplicated.

## Phase 19: Server Endpoint Contract Tests

### TC-180 Login Endpoint Contract

Given `pi-pro auth login` is executed.

Then the mock server receives:

```text
POST /auth/login
```

With username/password JSON.

### TC-181 Init Endpoint Contract

Given `pi-pro init` is executed.

Then the mock server receives:

```text
POST /cli/version
GET /cli/init-manifest
GET /cli/files/*
```

In that order, except file downloads may be parallelized later.

### TC-182 Generation Endpoint Contract

Given valid generation input.

When generation submits.

Then mock server receives:

```text
POST /generations
Authorization: Bearer <authToken>
```

And request body contains:

```text
provider
model
type
artifactKind
input
```

### TC-183 Task Endpoint Contract

Given generation waits for completion.

Then mock server receives:

```text
GET /tasks/:jobId
```

Until terminal state.

## TDD Execution Order

Recommended order:

```text
TC-001 to TC-004
TC-010 to TC-016
TC-020 to TC-024
TC-030 to TC-033
TC-040 to TC-045
TC-050 to TC-053
TC-060 to TC-063
TC-070 to TC-073
TC-080 to TC-082
TC-090 to TC-183
```

Do not implement a later phase before its contract tests exist.
