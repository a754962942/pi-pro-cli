# Capability API Iteration Plan

Created: 2026-06-27

## Purpose

PI-Pro Server now exposes explicit capability discovery APIs backed by its PostgreSQL capability tables. This document records how PI-Pro CLI uses those APIs while keeping the schema-bootstrap flow as a fallback.

Current CLI behavior:

```text
pi-pro init downloads /cli/init-manifest
pi-pro init downloads schema files from /cli/files/*
types list/inspect prefers /capabilities/* when a server context is configured
types list/inspect falls back to local schema files when the capability API is unavailable
schema --brief and schema inspect prefer remote schema/capability data and fall back to local schemas
commands submit generation payloads using the schema identity
```

Future capability API behavior:

```text
pi-pro init may still download schema files for validation and normalization
schema files remain the validation contract, not the only capability index
generation commands can add pre-submit type/model/provider validation against capability API
```

## Server API Contract

### GET /capabilities/types

Auth required:

```text
no
```

Response:

```json
{
  "types": [
    {
      "code": "image-to-video",
      "name": "首帧生视频",
      "artifactKind": "video"
    },
    {
      "code": "two-image-to-video",
      "name": "首尾帧生视频",
      "artifactKind": "video"
    }
  ]
}
```

Usage:

```text
list generation event types
render user-facing type names
avoid deriving event type availability only from schema file paths
```

### GET /capabilities/types/:type/models

Auth required:

```text
no
```

Response:

```json
{
  "models": [
    {
      "code": "grok-video-1.0",
      "name": "grok-video-1.0",
      "modality": "video",
      "supportedEventTypes": ["image-to-video", "two-image-to-video"],
      "providers": [
        {
          "code": "grok/grok-video-1.0",
          "providerCode": "grok",
          "providerName": "grok",
          "modelCode": "grok-video-1.0",
          "providerModelId": "grok-imagine-video-1.5-preview",
          "requestAdapter": "grok",
          "responseAdapter": "grok",
          "priority": 100,
          "weight": 1,
          "timeoutSeconds": 1800,
          "supportsCancel": false,
          "healthStatus": "healthy"
        }
      ]
    }
  ]
}
```

Security boundary:

```text
server must not return provider apiKey
server must not return provider baseUrl unless the CLI has a concrete user-facing need
CLI must treat providerModelId as an opaque server-provided supplier model identifier
```

## CLI Integration Strategy

### Phase 1: Read-only client support

Status:

```text
completed on 2026-06-29
```

Implementation scope:

```text
add client methods for GET /capabilities/types
add client methods for GET /capabilities/types/:type/models
add DTOs under the existing server client boundary
keep schema-bootstrap behavior unchanged
add tests for response decoding and server error preservation
```

Dependency decision:

```text
Use the existing internal/client HTTP helper and Go standard library JSON decoding.
Do not introduce a new HTTP client dependency.
Do not introduce Viper or remote config caching for this phase.
```

### Phase 2: types command capability source

Status:

```text
completed on 2026-06-29
```

Implementation scope:

```text
types list prefers capability API when server is reachable
types inspect can show models/providers for one event type
fallback to local schema registry when capability API is unavailable
do not break offline/local initialized schema inspection
```

TDD acceptance:

```text
types list renders image-to-video as 首帧生视频
types list renders two-image-to-video as 首尾帧生视频
types inspect image-to-video shows available model codes
types inspect image-to-video shows public provider codes only
types commands do not print apiKey, auth token, or baseUrl
server 404/5xx falls back to local schemas when local schemas exist
server 401/403 preserve existing auth error behavior if auth becomes required later
```

### Phase 3: generation command selection help

Implementation scope:

```text
generate commands can validate requested type/model/provider against capability API when online
CLI still validates payload shape through downloaded schema files
capability API helps choose model/provider; schema files validate request body
```

TDD acceptance:

```text
unsupported type/model/provider returns a stable CLI AppError before submit when capability data is available
schema validation remains authoritative for input fields
provider selection remains optional when server can route by type + model
```

## Non-goals

```text
no server-side cache in CLI MVP
no user-facing provider base URL display
no local mutation of capability data
no replacement of schema files for validation
no generated client code until API shape stabilizes
```

## Current Status

```text
server implemented: GET /capabilities/types
server implemented: GET /capabilities/types/:type/models
CLI implemented: client methods for capability types and models
CLI implemented: types list prefers capability API with local schema fallback
CLI implemented: types inspect --type shows model/provider mappings from capability API
CLI implemented: schema --brief summarizes remote capability/schema data with local fallback
CLI implemented: schema inspect reads the selected remote schema with local fallback
schema bootstrap flow: still supported and remains backward compatible
pending: generation command pre-submit validation against capability API
```
