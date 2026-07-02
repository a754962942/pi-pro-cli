# Server Responsibilities

Created: 2026-06-13

## Context

PI-Pro CLI is designed for direct agent invocation. Long-running media generation, especially video generation, cannot reliably depend on callbacks into the CLI because the CLI does not provide a stable public API endpoint.

The server must therefore own long-running task lifecycle management, while the CLI submits jobs, polls task status, and handles local IO.

## Responsibility Boundary

### CLI Responsibilities

- Parse `generateImage`, `generateVoice`, and `generateVideo` commands.
- Resolve `type` to a provider-specific schema.
- Validate, clean, default, and map parameters according to that schema.
- Resolve authentication from CLI arguments, environment variables, project config, or user config.
- Submit generation requests to the server.
- Poll task status with timeout, exponential backoff, and jitter.
- Download returned artifacts when `--output` or `--output-dir` is provided.
- Emit stable machine-readable JSON on stdout.
- Emit progress and diagnostics on stderr.

Auth and config resolution is defined in [Auth and Config Flow](auth-and-config-flow.md).
Task submission, polling, and cancellation are defined in [Task Polling and Server Client Flow](task-polling-and-server-client-flow.md).
Server endpoints required by the CLI are summarized in [Server Endpoint Contract](server-endpoint-contract.md).
Server-side capability lessons from `@lingjingai/lj-awb-cli` are recorded in [Server Capability Analysis From LingjingAI](server-capability-analysis-from-lingjingai.md).

### Server Responsibilities

- Receive normalized generation requests from the CLI.
- Issue opaque API key auth tokens after successful login.
- Validate, revoke, and map auth tokens to server-side users.
- Route requests to the appropriate provider/model implementation.
- Own provider-specific long-running task submission.
- Persist task metadata and status.
- Expose task status query endpoints for polling.
- Normalize provider status values into a stable task state model.
- Persist or proxy final artifact URLs.
- Return final generation results in a stable response format.
- Own callback handling when upstream providers require callbacks.

## Auth Token Responsibilities

The server must own all auth token semantics.

Token format:

```text
sk-pipro-xxxxxxxxxxxxxxxxxxxxxxxx
```

The token is an opaque API key, not a JWT. It should not contain client-readable claims, expiration payloads, scopes, or user metadata.

Server responsibilities:

```text
generate high-entropy sk-pipro-* tokens
store only a secure hash or digest of the token
bind each token to an internal test user
validate Authorization: Bearer <authToken> on protected endpoints
reject missing, unknown, revoked, or disabled tokens
own expiration policy if expiration is introduced later
own permission, quota, and capability checks
support token revocation without requiring CLI changes
return stable AUTH_* errors for invalid auth states
```

CLI responsibilities:

```text
store authToken in local config with restrictive file permissions
send Authorization: Bearer <authToken>
redact token from stdout and stderr
never parse token contents
never infer expiration or permissions locally
```

## Task Model

The server should expose a task model that the CLI can poll.

Recommended task states:

- `queued`
- `running`
- `succeeded`
- `failed`
- `cancelled`
- `expired`

The CLI should treat `succeeded`, `failed`, `cancelled`, and `expired` as terminal states.

## Recommended Flow

```text
CLI validates and normalizes input
  ↓
CLI submits request to server
  ↓
Server creates provider task and returns jobId
  ↓
CLI polls server task status with backoff
  ↓
Server returns terminal result with artifact URL(s)
  ↓
CLI outputs JSON and optionally downloads artifacts
```

## Design Decision

The CLI should not act as a callback server by default.

Instead, provider callbacks, task persistence, and status normalization belong on the server. The CLI uses polling as the default long-task completion mechanism, with `--no-wait` support for submit-only workflows and task commands for later status checks.
