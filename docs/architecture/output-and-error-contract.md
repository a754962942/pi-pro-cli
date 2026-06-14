# Output and Error Contract

Created: 2026-06-14

## Context

PI-Pro CLI is primarily called by agents. Agents need stable, machine-readable output and predictable process exit codes. Human-readable progress is still useful, but it must not pollute stdout.

This contract applies to generation commands, task commands, auth commands, init/update, schema inspection, upload/download, and validation failures.

## Design Goals

- Keep stdout machine-readable.
- Keep stderr available for progress and diagnostics.
- Use stable top-level JSON shapes.
- Use stable error codes across modules.
- Preserve server details without leaking secrets.
- Make process exit codes predictable for agents.

## Streams

stdout:

```text
final machine-readable JSON only
```

stderr:

```text
progress messages
warnings
retry notices
polling status
debug diagnostics when explicitly enabled
```

Do not write progress lines to stdout.

## Success Shape

All successful commands should return:

```json
{
  "ok": true
}
```

Command-specific fields can be added next to `ok`.

Generation success:

```json
{
  "ok": true,
  "status": "succeeded",
  "jobId": "job_123",
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "artifacts": [
    {
      "url": "https://server.example/artifacts/video.mp4",
      "path": "/absolute/path/to/video.mp4",
      "mime": "video/mp4",
      "kind": "video"
    }
  ]
}
```

Submit-only success:

```json
{
  "ok": true,
  "status": "submitted",
  "jobId": "job_123",
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video"
}
```

Init success:

```json
{
  "ok": true,
  "initialized": true,
  "changed": false,
  "manifestVersion": "2026-06-14"
}
```

## Error Shape

All failures should return:

```json
{
  "ok": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable summary.",
    "details": {}
  }
}
```

`details` is optional but should be structured when present.

Validation failure:

```json
{
  "ok": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Input validation failed.",
    "details": [
      {
        "code": "MISSING_REQUIRED_FIELD",
        "field": "prompt",
        "message": "Field `prompt` is required."
      }
    ]
  }
}
```

Task timeout:

```json
{
  "ok": false,
  "error": {
    "code": "TASK_TIMEOUT",
    "message": "Task did not reach a terminal state before timeout.",
    "details": {
      "jobId": "job_123",
      "lastStatus": "running"
    }
  }
}
```

## Partial Failure

Some flows can have a successful remote operation but failed local IO.

Example: generation succeeded, artifact download failed.

The CLI should return `ok: false` because the requested local command did not complete successfully, while preserving remote success details.

```json
{
  "ok": false,
  "status": "succeeded",
  "jobId": "job_123",
  "artifacts": [
    {
      "url": "https://server.example/artifacts/video.mp4",
      "mime": "video/mp4",
      "kind": "video"
    }
  ],
  "error": {
    "code": "ARTIFACT_DOWNLOAD_FAILED",
    "message": "Task succeeded, but one or more artifacts could not be downloaded.",
    "details": [
      {
        "url": "https://server.example/artifacts/video.mp4",
        "path": "/absolute/path/to/video.mp4"
      }
    ]
  }
}
```

## Exit Codes

Recommended exit codes:

```text
0  success
1  generic command failure
2  validation or usage error
3  auth/config error
4  network/server error
5  task failed, cancelled, expired, or timed out
6  local IO error
7  update/init error
```

The JSON error code is the primary contract. Exit codes are coarse process-level signals.

## Error Code Namespaces

Recommended error code groups:

```text
AUTH_*
CONFIG_*
SCHEMA_*
VALIDATION_*
TASK_*
SERVER_*
NETWORK_*
ASSET_*
UPLOAD_*
DOWNLOAD_*
OUTPUT_*
INIT_*
UPDATE_*
```

Error codes should be stable once released. If behavior changes, add a new code rather than changing the meaning of an existing code.

## Server Error Preservation

When the server returns a structured error, the CLI should map it to a stable CLI error code and preserve server information under `details`.

Example:

```json
{
  "ok": false,
  "error": {
    "code": "TASK_FAILED",
    "message": "Generation task failed.",
    "details": {
      "serverCode": "PROVIDER_QUOTA_EXCEEDED",
      "serverMessage": "Provider quota exceeded."
    }
  }
}
```

Do not expose raw server stack traces by default.

## Secret Redaction

The CLI must not print these values to stdout or stderr:

```text
authToken
password
Authorization header
raw credential request body
```

If diagnostic output needs to mention a token, redact it:

```text
pi_token_...redacted
```

## Debug Mode

Debug output should be opt-in.

Potential future flag:

```text
--debug
```

Debug output still goes to stderr and must redact secrets.

First implementation can omit debug mode.

## YAGNI Filter

Keep for the first implementation:

```text
stdout JSON only
stderr progress/diagnostics
ok true/false envelope
stable error.code
coarse exit codes
secret redaction
partial failure representation
server error preservation under details
```

Defer until required:

```text
debug mode
localized error messages
machine-readable progress stream
JSON lines progress protocol
rich tracing ids
structured warnings array
```

## Design Decisions

- Treat JSON stdout as the primary agent contract.
- Treat exit codes as coarse secondary signals.
- Keep progress off stdout.
- Use stable error codes and structured details.
- Redact secrets everywhere.
- Represent local IO failure after server success as `ok: false` with preserved artifact URLs.
