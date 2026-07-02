# Server Capability Analysis From LingjingAI

Created: 2026-06-23

## Context

This document analyzes the public `@lingjingai/lj-awb-cli` package and records what server-side capabilities PI-Pro may need when implementing a production-grade generation backend.

The goal is not to copy LingjingAI endpoint names or business concepts. The goal is to learn from the server responsibilities implied by a mature agent-facing media generation CLI.

Related PI-Pro documents:

```text
server-endpoint-contract.md
server-responsibilities.md
schema-contract.md
schema-parameter-contract.md
task-polling-and-server-client-flow.md
asset-io-sqlite-flow.md
```

## Observed LingjingAI Capability Domains

The `@lingjingai/lj-awb-cli` package shows these server-facing domains:

```text
auth and access key
account and team context
project group context
credits and usage accounting
model discovery
model options
model create-spec
upload credentials and signed object URLs
image generation fee estimate and task creation
video generation fee estimate and task creation
task status and task feed polling
asset group and asset registration
subject / element creation and lookup
voice creation and lookup
provider-specific media operations such as subtitle removal and super resolution
```

PI-Pro should not implement every domain in the first server version. The first server should keep the current internal-test assumptions:

```text
username/password login
opaque sk-pipro-* token
all internal users have full access
no credits or quotas
server-published schemas
generation submission
task polling
permanent artifact URLs
```

## Server Capability Map

### 1. Auth and Token Service

LingjingAI pattern:

```text
create login flow
query login flow status
validate access key
send access key in request header
```

PI-Pro MVP:

```text
POST /auth/login
Authorization: Bearer sk-pipro-*
```

Server responsibilities:

- Verify username and password for internal users.
- Issue high-entropy opaque `sk-pipro-*` tokens.
- Store token digests, not raw tokens.
- Validate bearer tokens on protected endpoints.
- Revoke or disable tokens server-side without CLI changes.
- Return stable auth errors.

Deferred:

- Browser login flow.
- Team switching.
- Token scopes.
- Token expiry surfaced to CLI.

### 2. CLI Release and Bootstrap Service

LingjingAI pattern:

```text
CLI checks update source
skill/bootstrap can install expected CLI version
runtime can check for update notices
```

PI-Pro MVP:

```text
POST /cli/version
GET  /cli/init-manifest
GET  /cli/files/*
```

Server responsibilities:

- Return latest release version by OS, arch, and channel.
- Return platform-specific binary URL and sha256.
- Return Windows helper updater URL and sha256.
- Return init manifest for schemas and bootstrap files.
- Serve init files as raw bytes.
- Keep init files data-only.
- Ensure manifest paths are relative and never contain `..`.

Deferred:

- Delta updates.
- Background schema refresh.
- Authenticated protected schema bootstrap.

### 3. Schema Publishing Service

LingjingAI pattern:

```text
schema --brief exposes agent command map
model options exposes params/resources/constraints
model create-spec exposes input requirement, intents, preflight, examples
```

PI-Pro MVP:

```text
schemas are published through /cli/init-manifest
CLI reads local schemas from ~/.pi-pro/schemas
```

Server responsibilities:

- Own schema publication.
- Bind schema to `provider + model + type + schemaVersion`.
- Publish one behavior per schema file.
- Include parameter, resource, constraint, safety, workflow, task, and artifact metadata.
- Reject generation requests for unknown or disabled schema identities.
- Keep provider mapping outside the CLI.

Future remote schema endpoints:

```text
GET /types
GET /types/:provider/:model/:type/schema
```

Important rule:

```text
The server may publish declarative schema data.
The server must not publish executable mapper code for the CLI to run.
```

### 4. Model Catalog and Capability Service

LingjingAI pattern:

```text
list models by usage
fetch model options
fetch model group info
show queue count and model descriptions
```

PI-Pro MVP:

```text
model catalog can be represented by schemas in init manifest
```

Server responsibilities for later phases:

- List available providers, models, and behavior types.
- Expose display names, descriptions, artifact kinds, and availability.
- Expose model parameter enums and defaults through schema.
- Expose resource capabilities through schema.
- Expose model status, disabled state, and queue/availability hints if needed.

YAGNI boundary:

- Do not add separate model browsing endpoints until the CLI needs remote discovery beyond init-downloaded schemas.
- For the first implementation, `pi-pro types list` can list local schemas only.

### 5. Generation Submission Service

LingjingAI pattern:

```text
image fee estimate
image create
video fee estimate
video create
provider-specific request construction
```

PI-Pro MVP:

```text
POST /generations
```

Server responsibilities:

- Authenticate request.
- Validate `provider + model + type` against server-supported schemas.
- Revalidate normalized input server-side.
- Select provider adapter.
- Map normalized CLI input to provider-specific payload.
- Submit provider job.
- Persist job metadata.
- Return stable `jobId` and initial status.
- Optionally return terminal result immediately for synchronous providers.

Provider adapter responsibilities:

```text
provider auth
provider payload construction
provider response parsing
provider task id mapping
provider callback verification
provider error normalization
```

### 6. Fee, Quota, and Usage Service

LingjingAI pattern:

```text
fee estimate before create
credits balance
credits records
project budget
usage aggregation
```

PI-Pro MVP:

```text
not required
internal users have full access
no credits, quota, or usage accounting gate
```

Deferred responsibilities:

- Estimate generation cost before submission.
- Track user usage.
- Enforce quota or billing policy.
- Expose balance and usage records.
- Return stable insufficient-credit errors.

YAGNI boundary:

- Do not add fee endpoints until product explicitly needs credits or quota.
- Keep schema `safety.costsCredits` as declarative future metadata only.

### 7. Asset Upload and Object Storage Service

LingjingAI pattern:

```text
fetch upload secret
upload local file to object storage
return backendPath
fetch signed object URL
reuse backendPath in generation resources
```

PI-Pro MVP:

```text
POST /assets/upload
```

Server responsibilities:

- Accept file uploads when schema permits `fileResolve=upload` or `asset-db-or-upload`.
- Validate file type and size.
- Store file in durable object storage.
- Return permanent URL and stable asset id.
- Return metadata for CLI SQLite mapping.
- Make returned artifact URLs permanent for first implementation.

Important PI-Pro difference:

```text
Most files should be resolved from the CLI asset SQLite mapping first.
Upload is only needed when the schema explicitly permits upload and no mapping exists.
```

Deferred:

- Direct-to-object-storage signed upload flow.
- Temporary upload area lifecycle.
- URL refresh.
- Asset groups.
- Asset review.

### 8. Task Lifecycle Service

LingjingAI pattern:

```text
task info
task feed pull
task wait
record polling
terminal task error handling
result URL extraction
```

PI-Pro MVP:

```text
GET  /tasks/:jobId
POST /tasks/:jobId/cancel
```

Server responsibilities:

- Persist internal job id and provider task id.
- Normalize provider states into:

```text
queued
running
succeeded
failed
cancelled
expired
```

- Expose polling endpoint.
- Persist progress if available.
- Persist terminal artifacts.
- Persist structured terminal errors.
- Cancel provider task when possible.
- Own provider callback endpoints when providers use callbacks.

Important rule:

```text
The CLI is not a callback server.
The server receives provider callbacks and the CLI polls the server.
```

### 9. Artifact and Result URL Service

LingjingAI pattern:

```text
task result includes resultUrl values
uploaded objects are referenced by backendPath
signed URLs can be fetched for object names
```

PI-Pro MVP:

```text
task succeeded response includes artifacts[].url
URLs are permanent
```

Server responsibilities:

- Normalize final provider output into artifacts.
- Persist artifact metadata:

```text
assetId
url
mime
kind
sizeBytes
sha256 when available
```

- Ensure URLs returned to CLI are durable.
- Avoid requiring URL refresh in the first implementation.

Deferred:

- Signed URL refresh.
- Artifact expiry policy.
- Artifact library management.

### 10. Subject, Voice, and Reusable Asset Services

LingjingAI pattern:

```text
create subject
wait for subject externalId
create subject voice
wait for voice externalId
reuse subject/voice as generation resources
```

PI-Pro MVP:

```text
not required unless first schemas need reusable subject or voice assets
```

Deferred responsibilities:

- Create reusable subject assets.
- Create reusable voice assets.
- Resolve subject/voice IDs in generation schemas.
- Expose status endpoints for subject/voice creation.

YAGNI boundary:

- Do not add these endpoints until concrete provider/model schemas require them.
- For initial generation, represent reusable references as normal asset URLs or asset ids when possible.

## Recommended PI-Pro MVP Server Modules

Based on the analysis, the first server implementation should contain these modules:

```text
auth
release
initManifest
schemaPublication
generation
providerAdapters
tasks
assets
artifactStorage
errorNormalization
```

Suggested responsibility split:

```text
auth
  login, token issue, token validation

release
  /cli/version, binary metadata, helper updater metadata

initManifest
  /cli/init-manifest, /cli/files/*

schemaPublication
  publish schemas into init manifest, validate schema files

generation
  /generations, server-side schema revalidation, adapter routing

providerAdapters
  provider payload mapping, provider task submission, provider status sync

tasks
  /tasks/:jobId, /tasks/:jobId/cancel, task persistence, status normalization

assets
  /assets/upload, file validation, object storage persistence

artifactStorage
  final artifact persistence, permanent URL generation

errorNormalization
  stable API errors for auth, schema, validation, provider, task, asset, update
```

## Current Endpoint Contract Fit

The existing PI-Pro endpoint contract already covers the MVP server backbone:

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

The LingjingAI analysis suggests future additions only after business need is confirmed:

```text
GET /types
GET /types/:provider/:model/:type/schema
POST /generations/estimate
GET /usage
GET /assets/:assetId
POST /subjects
GET /subjects/:id
POST /voices
GET /voices/:id
```

These should remain deferred until concrete PI-Pro schemas or product requirements require them.

## Design Decisions

- Keep the first server small and aligned with the existing CLI contract.
- Server owns provider adapters and long-running task callbacks.
- CLI owns local validation, normalization, asset-db lookup, polling, and JSON output.
- Server must revalidate everything because the CLI is not a trust boundary.
- Server-published schemas are data-only.
- Credits, project groups, teams, subject libraries, voice libraries, and asset review are deferred.
