# Server Endpoint Contract

Created: 2026-06-14

## Context

This document centralizes the server endpoints required by PI-Pro CLI. It is intended for implementing the server in a separate project without needing to reconstruct the contract from multiple CLI architecture documents.

The CLI uses a built-in `serverUrl`. Users do not configure server URLs in the first implementation.

## Endpoint Summary

Required for the first implementation:

```text
POST /auth/login
POST /cli/version
GET  /cli/init-manifest
GET  /cli/files/*
POST /generations
GET  /tasks/:jobId
POST /tasks/:jobId/cancel
POST /assets/upload
```

Deferred or optional:

```text
GET /types
GET /types/:provider/:model/:type/schema
GET /tasks/:jobId/result
```

## Auth

### POST /auth/login

Used by:

```text
pi-pro auth login
```

Auth required:

```text
no
```

Request:

```json
{
  "username": "user@example.com",
  "password": "secret"
}
```

Response:

```json
{
  "authToken": "sk-pipro-xxxxxxxxxxxxxxxxxxxxxxxx",
  "user": {
    "id": "user_123",
    "username": "user@example.com"
  }
}
```

The CLI stores `authToken` locally and sends it on later requests:

```text
Authorization: Bearer <authToken>
```

First implementation auth policy:

```text
valid internal test user credentials return authToken
authToken grants full access to CLI-backed generation capabilities
no credits, quotas, scopes, or plan checks are required
```

Token format:

```text
sk-pipro-xxxxxxxxxxxxxxxxxxxxxxxx
```

The token is an opaque API key. The CLI must not parse or validate it beyond treating it as a secret string.

## CLI Version

### POST /cli/version

Used by:

```text
install.sh
pi-pro init
pi-pro update
```

Auth required:

```text
no
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

Example:

```text
POST /cli/version
```

For first-time install, `localVersion` may be omitted or set to `none`.

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

Windows response should include helper updater metadata when the platform requires it:

```json
{
  "localVersion": "0.1.0",
  "releaseVersion": "0.1.1",
  "minSupportedVersion": "0.1.0",
  "updateAvailable": true,
  "updateRequired": false,
  "binary": {
    "url": "https://api.example.com/cli/releases/0.1.1/windows-amd64/pi-pro.exe",
    "sha256": "abc123",
    "helper": {
      "url": "https://api.example.com/cli/releases/0.1.1/windows-amd64/pi-pro-updater.exe",
      "sha256": "def456"
    }
  },
  "initManifestVersion": "2026-06-14"
}
```

Server responsibilities:

```text
return platform-specific binary URL
return sha256 for binary verification
return helper updater URL and sha256 for Windows releases
declare minSupportedVersion
set updateRequired=true when old clients must update before continuing
publish Windows release packages with both pi-pro.exe and pi-pro-updater.exe
```

## Initialization

### GET /cli/init-manifest

Used by:

```text
pi-pro init
```

Auth required:

```text
no
```

Response:

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

Server responsibilities:

```text
only return relative paths
never return paths containing ..
include sha256 for every file
ensure files are data-only
make public bootstrap files available before login
```

### GET /cli/files/*

Used by:

```text
pi-pro init
```

Auth required:

```text
no for first implementation
```

Response:

```text
raw file bytes
```

The CLI verifies each file against the manifest `sha256` before writing it locally.

## Generation

### POST /generations

Used by:

```text
pi-pro generateImage
pi-pro generateVoice
pi-pro generateVideo
```

Auth required:

```text
yes
```

Request:

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
      "assetId": "asset_123",
      "url": "https://server.example/artifacts/image.png",
      "mime": "image/png"
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

Terminal response is also allowed:

```json
{
  "jobId": "job_123",
  "status": "succeeded",
  "artifacts": [
    {
      "assetId": "asset_456",
      "url": "https://server.example/artifacts/video.mp4",
      "mime": "video/mp4",
      "kind": "video"
    }
  ]
}
```

Server responsibilities:

```text
validate auth token
route by provider/model/type
perform provider-specific payload mapping
submit provider task
persist task metadata
normalize provider task state
return jobId
```

If the submitted schema identity is unknown or not supported by the server, the server should reject the request. The CLI should normally prevent this earlier through schema validation, but the server remains the final authority.

## Tasks

Stable task states:

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

### GET /tasks/:jobId

Used by:

```text
generation polling
pi-pro task status
pi-pro task wait
```

Auth required:

```text
yes
```

Response:

```json
{
  "jobId": "job_123",
  "status": "running",
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "artifactKind": "video",
  "progress": {
    "percent": 42,
    "message": "Generating"
  }
}
```

Succeeded response:

```json
{
  "jobId": "job_123",
  "status": "succeeded",
  "artifacts": [
    {
      "assetId": "asset_456",
      "url": "https://server.example/artifacts/video.mp4",
      "mime": "video/mp4",
      "kind": "video"
    }
  ]
}
```

Failed response:

```json
{
  "jobId": "job_123",
  "status": "failed",
  "error": {
    "code": "PROVIDER_QUOTA_EXCEEDED",
    "message": "Provider quota exceeded."
  }
}
```

Server responsibilities:

```text
hide provider-specific states
return only stable task states
return durable artifact URLs when succeeded
return structured error when failed
```

### POST /tasks/:jobId/cancel

Used by:

```text
pi-pro task cancel
```

Auth required:

```text
yes
```

Response:

```json
{
  "jobId": "job_123",
  "status": "cancelled"
}
```

Server responsibilities:

```text
cancel provider task when possible
mark task cancelled or return current terminal state
return stable task status
```

## Assets

### POST /assets/upload

Required when first shipped schemas use `fileResolve: upload` or `fileResolve: asset-db-or-upload`.

Used by:

```text
fileResolve: upload
fileResolve: asset-db-or-upload
future explicit upload flow
```

Auth required:

```text
yes
```

Request:

```text
multipart/form-data
file=<binary>
```

Response:

```json
{
  "assetId": "asset_123",
  "url": "https://server.example/uploads/local.png",
  "mime": "image/png",
  "sizeBytes": 123456,
  "sha256": "abc123"
}
```

Server responsibilities:

```text
store uploaded asset
return durable URL
return stable asset id
return metadata useful for local SQLite mapping
```

Returned URLs are expected to be permanent. No URL refresh endpoint is required in the first implementation.

## Deferred Schema Endpoints

Remote schema resolution is not required for the first implementation because `pi-pro init` downloads schemas into `~/.pi-pro/schemas`.

Future endpoints:

```text
GET /types
GET /types/:provider/:model/:type/schema
```

These endpoints must return schema data only. They must not return executable code.

## Error Shape

Server errors should be structured:

```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable message."
  }
}
```

The CLI maps server errors to stable CLI error codes and preserves server fields under `error.details`.

## Auth Failures

Recommended HTTP status mapping:

```text
401 -> AUTH_INVALID or AUTH_EXPIRED
403 -> AUTH_INVALID
404 task not found -> TASK_NOT_FOUND
429 -> retryable server/network error when appropriate
5xx -> retryable server/network error when appropriate
```

## YAGNI Filter

Required for first implementation:

```text
POST /auth/login
POST /cli/version
GET /cli/init-manifest
GET /cli/files/*
POST /generations
GET /tasks/:jobId
POST /tasks/:jobId/cancel
POST /assets/upload
```

Deferred:

```text
GET /types
GET /types/:provider/:model/:type/schema
GET /tasks/:jobId/result
provider-specific public endpoints
webhook callback endpoint exposed by CLI
```

## Design Decisions

- Server owns provider-specific mapping.
- Server owns provider callbacks and task persistence.
- Server normalizes task states before returning them to the CLI.
- CLI uses polling; CLI does not expose a callback server.
- CLI init files are public for first implementation.
- Generation, task, and asset upload endpoints require Bearer auth.
