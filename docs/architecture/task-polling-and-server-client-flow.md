# Task Polling and Server Client Flow

Created: 2026-06-13

## Context

PI-Pro CLI submits generation requests to a server and receives either an immediate result or a long-running task id. Video generation is expected to be long-running, so the CLI should use polling rather than callback handling.

The server owns provider callbacks, task persistence, provider status normalization, and final result storage. The CLI owns submission, polling, cancellation commands, timeout handling, and local result IO.

## Design Goals

- Keep CLI execution non-interactive and deterministic.
- Support both wait and submit-only workflows.
- Use bounded polling with backoff and jitter.
- Normalize server responses into stable CLI JSON.
- Keep provider-specific task behavior on the server.
- Make task status commands usable by agents.

## Server API Surface

Recommended minimal server endpoints:

```text
POST /generations
GET /tasks/:jobId
POST /tasks/:jobId/cancel
```

Optional future endpoints:

```text
GET /tasks/:jobId/result
GET /types
GET /types/:provider/:model/:type/schema
POST /assets/upload
```

Generation submit request shape:

```json
{
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "artifactKind": "video",
  "input": {
    "prompt": "A cinematic lake shot",
    "image": {
      "source": "asset-db",
      "url": "https://server.example/artifacts/image.png"
    },
    "duration": 5
  }
}
```

Submit response:

```json
{
  "jobId": "job_123",
  "status": "queued"
}
```

For synchronous providers, the server may return a terminal result directly:

```json
{
  "jobId": "job_123",
  "status": "succeeded",
  "artifacts": [
    {
      "url": "https://server.example/artifacts/image.png",
      "mime": "image/png",
      "kind": "image"
    }
  ]
}
```

## Task States

The CLI should only depend on a stable server task state model:

```text
queued
running
succeeded
failed
cancelled
expired
```

Terminal states:

```text
succeeded
failed
cancelled
expired
```

Non-terminal states:

```text
queued
running
```

Provider-specific states should be normalized by the server before the CLI sees them.

## Generation Command Flow

Default generation flow:

```text
1. Resolve auth and config.
2. Validate and normalize input.
3. Submit generation request to POST /generations.
4. If --no-wait is set, return submitted JSON with jobId.
5. If response is terminal, handle result immediately.
6. Otherwise poll GET /tasks/:jobId until terminal state or timeout.
7. If succeeded, optionally download artifacts through Asset IO flow.
8. Emit final JSON on stdout.
```

Submit-only output:

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

Completed output:

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

## Polling Defaults

Recommended defaults:

```text
wait: true
timeoutSeconds: 1800
pollIntervalSeconds: 2
pollMaxSeconds: 30
pollBackoff: 1.5
jitter: true
```

Schema may define task defaults, and command flags may override them:

```text
CLI polling flags > schema task defaults > global defaults
```

Recommended flags:

```text
--wait
--no-wait
--timeout <seconds>
--poll-interval <seconds>
--poll-max <seconds>
--poll-backoff <number>
```

## Backoff Strategy

Recommended polling delay:

```text
nextDelay = min(pollMaxSeconds, currentDelay * pollBackoff)
actualDelay = nextDelay with +/- 20% jitter
```

Jitter should be enabled by default to avoid synchronized polling spikes when many agents submit tasks at once.

The CLI should print progress diagnostics to stderr, never stdout.

Example stderr line:

```text
task job_123 running; polling again in 8.3s
```

## Task Commands

Recommended task command surface:

```sh
pi-pro task status <jobId>
pi-pro task wait <jobId>
pi-pro task cancel <jobId>
```

`task status` should make one request and return current state.

`task wait` should use the same polling loop as generation commands.

`task cancel` should request cancellation and return the server's resulting state.

## Timeout Behavior

If polling exceeds timeout:

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

Timeout does not imply server-side cancellation. The task may still continue on the server.

If cancellation on timeout is desired later, it should be explicit:

```text
--cancel-on-timeout
```

## Failure Behavior

If the server returns a terminal failed state:

The shared output and error contract is defined in [Output and Error Contract](output-and-error-contract.md).

```json
{
  "ok": false,
  "status": "failed",
  "jobId": "job_123",
  "error": {
    "code": "TASK_FAILED",
    "message": "Generation task failed.",
    "details": {
      "serverCode": "PROVIDER_QUOTA_EXCEEDED"
    }
  }
}
```

The CLI should preserve server-provided error codes in `details.serverCode` but keep CLI top-level error codes stable.

## Network Failure Behavior

Transient network errors during polling should be retried within the same timeout budget.

Recommended retryable categories:

```text
connection reset
timeout
HTTP 408
HTTP 429
HTTP 500
HTTP 502
HTTP 503
HTTP 504
```

Non-retryable categories:

```text
HTTP 400
HTTP 401
HTTP 403
HTTP 404
schema/request validation errors
```

HTTP 401 and 403 should map to auth errors:

```text
AUTH_INVALID
AUTH_EXPIRED
```

## Server Client Abstraction

Recommended abstraction:

```text
ServerClient
- submitGeneration(request)
- getTask(jobId)
- cancelTask(jobId)
- uploadAsset(file)
- downloadArtifact(artifact, targetPath)
```

The client should receive resolved config rather than reading config directly.

```text
ConfigResolver
  ↓
ServerClient(config)
  ↓
Generation / Task / Asset flows
```

This keeps auth/config resolution separate from transport behavior.

## Error Codes

Recommended task/client error codes:

```text
TASK_TIMEOUT
TASK_FAILED
TASK_CANCELLED
TASK_EXPIRED
TASK_NOT_FOUND
TASK_STATUS_INVALID
SERVER_REQUEST_FAILED
SERVER_RESPONSE_INVALID
NETWORK_RETRY_EXHAUSTED
AUTH_INVALID
AUTH_EXPIRED
```

## Design Decisions

- Use polling as the default long-task completion mechanism.
- Keep callback handling on the server.
- Default generation commands to `--wait`.
- Support `--no-wait` for agent-managed async workflows.
- Keep timeout separate from cancellation.
- Retry transient network failures within the timeout budget.
- Keep provider-specific task statuses hidden behind server-normalized states.
- Keep stdout JSON-only and send progress to stderr.
