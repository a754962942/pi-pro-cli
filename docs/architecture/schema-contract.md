# Schema Contract

Created: 2026-06-13

## Context

PI-Pro CLI uses `provider`, `model`, and `type` to select a provider-specific schema. The schema defines how agent-provided parameters are validated, cleaned, defaulted, and normalized before the CLI submits a request to the server.

The CLI must not couple command execution to local schema files. Local schemas are downloaded by `pi-pro init` for the first implementation. Later, schemas may be resolved directly from the server.

Schema lookup and source resolution are defined in [Schema Registry and Resolution Flow](schema-registry-and-resolution-flow.md).
Agent-facing model parameter, resource, constraint, and dry-run contract details are defined in [Schema Parameter Contract](schema-parameter-contract.md).
Install and initialization behavior is defined in [Install and Init Flow](install-and-init-flow.md).

## Design Goals

- Keep provider parameter differences outside command handlers.
- Let the CLI validate agent-facing input without owning provider API payload construction.
- Keep complex provider-specific mapping on the server.
- Allow most agent-facing parameter changes to be handled by schema updates.
- Support local schemas now and server-managed schemas later.
- Avoid executing remote code from server-provided schemas.
- Keep the CLI stable for agent invocation.

## Business Rules

Schema publishing is owned by the server.

Schemas are strongly bound to a concrete `provider + model + type` combination. The CLI should treat that combination as the schema identity.

If a schema is invalid, mismatched, or cannot be loaded, generation must not continue. Incorrect schemas must fail fast rather than allowing a best-effort downstream call.

## Schema Location

Local schemas should be organized by provider and model:

```text
schemas/
  <provider>/
    <model>/
      <type>.json
```

Example:

```text
schemas/
  seeddance/
    v1/
      image-to-video.json
      text-to-video.json
  happy-horse/
    v2/
      image-to-video.json
```

This structure keeps provider and model ownership visible in the filesystem.

## Type Format

Each schema file represents exactly one behavior. Inside the schema file, `type` should only describe that concrete behavior:

```text
<behavior>
```

Examples:

```text
image-to-video
text-to-video
text-to-image
text-to-speech
```

The full schema selection key is:

```text
provider + model + type
```

## Example Schema

Path:

```text
schemas/seeddance/v1/image-to-video.json
```

```json
{
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "schemaVersion": "1.0",
  "artifactKind": "video",
  "displayName": "SeedDance Image to Video",
  "description": "Generate a video from a reference image and prompt.",
  "input": {
    "unknownPolicy": "reject",
    "required": ["prompt", "image"],
    "properties": {
      "prompt": {
        "type": "string",
        "minLength": 1,
        "description": "Text prompt used for generation."
      },
      "image": {
        "type": "file",
        "accept": ["image/png", "image/jpeg", "image/webp"],
        "fileResolve": "asset-db",
        "description": "Local reference image path."
      },
      "duration": {
        "type": "number",
        "default": 5,
        "enum": [5, 10]
      },
      "resolution": {
        "type": "string",
        "default": "720p",
        "enum": ["720p", "1080p"]
      },
      "cameraFixed": {
        "type": "boolean",
        "default": false
      },
      "seed": {
        "type": "integer",
        "minimum": 0
      }
    }
  },
  "normalization": {
    "omitNull": true,
    "omitEmptyString": true
  },
  "task": {
    "mode": "async",
    "defaultWait": true,
    "timeoutSeconds": 1800,
    "pollIntervalSeconds": 2,
    "pollMaxSeconds": 30,
    "pollBackoff": 1.5,
    "jitter": true
  },
  "artifacts": {
    "expected": [
      {
        "kind": "video",
        "mime": "video/mp4",
        "extension": ".mp4"
      }
    ]
  }
}
```

## Contract Sections

### Metadata

```text
schemaVersion
provider
model
type
artifactKind
displayName
description
```

Metadata identifies the schema and supports `pi-pro types list` and `pi-pro types inspect`.

`provider`, `model`, and `type` should match the schema file path. The CLI should reject mismatches to avoid loading the wrong provider contract.

Each schema file must contain exactly one behavior. Do not group multiple behaviors in a single JSON file.

`artifactKind` is used to verify that a schema is compatible with the invoked public command:

```text
generateImage -> image
generateVoice -> voice
generateVideo -> video
```

The public command remains useful as the CLI entry point, but it is not part of schema lookup.

### Input

`input` defines the CLI-facing parameters:

```text
unknownPolicy
required
properties
```

Recommended `unknownPolicy` values:

```text
reject       Fail on unknown fields. Recommended default.
strip        Remove unknown fields.
passthrough  Send unknown fields downstream.
```

### Properties

Each property can define:

```text
type
required membership
default
enum
minimum / maximum
minLength / maxLength
accept
fileResolve
description
```

`fileResolve` defines how local file paths are converted into URL references before request submission.

Recommended values:

```text
asset-db
upload
asset-db-or-upload
```

The default should be `asset-db`, because most local media files are expected to be downloaded server artifacts with an existing filePath-to-url mapping in the CLI SQLite database.

### Normalization

`normalization` controls how raw input becomes a clean server request.

The CLI should only perform safe declarative normalization:

```text
remove null values
remove empty strings when configured
inject defaults
validate files
resolve local file paths to URL references when configured
preserve field names from the schema
```

The CLI should not construct final provider API payloads. Provider-specific payload construction belongs on the server.

The CLI should not download and execute mapper code from the server.

### Task

`task` defines long-running behavior for this type.

Recommended modes:

```text
sync
async
```

For `async`, the CLI submits the request, receives a job id, and polls server task status with the configured polling defaults.

### Artifacts

`artifacts` describes expected outputs so the CLI can choose file extensions, validate downloads, and produce stable JSON.

## Program Decoupling

Command handlers should depend on a registry abstraction, not local schema files.

Recommended boundary:

```text
Command Handler
  ↓
TypeRegistry.get(provider, model, type)
  ↓
SchemaSource
```

Recommended interfaces:

```text
TypeRegistry
- get(provider, model, type)
- list(filter)
- inspect(provider, model, type)

SchemaSource
- fetch(provider, model, type)
- list(filter)

SchemaCache
- read(provider, model, type)
- write(provider, model, type, schema)
- invalidate(provider, model, type)
```

Schema sources can be composed:

```text
CompositeSchemaSource
  1. RemoteSchemaSource
  2. LocalSchemaSource
  3. CachedSchemaSource fallback
```

Initial implementation can use local schemas only, but command code should not know that.

## Server-Side Provider Mapping

After CLI normalization, the server receives a stable request:

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
      "path": "/absolute/path/to/input.png",
      "url": "https://server.example/artifacts/input.png"
    },
    "duration": 5,
    "resolution": "720p",
    "cameraFixed": false
  }
}
```

The server then owns provider-specific mapping:

```text
normalized CLI input
  ↓
server provider adapter
  ↓
SeedDance / Happy Horse / other provider payload
```

This keeps provider API changes centralized on the server and avoids requiring the CLI to understand every provider's final request structure.

## Future Server Registration

Future server-managed schema APIs may look like:

```text
GET /types
GET /types/:provider/:model/:type
GET /types/:provider/:model/:type/schema
```

The CLI can then resolve schemas from the server while keeping the same command flow.

Recommended behavior:

- Cache remote schemas locally.
- Include schema version in cache records.
- Prefer explicit `model` values over mutable latest aliases.
- Do not execute remote mapper code.
- Fall back to cached schema when the server is unavailable, if the schema is marked cacheable.

## Design Decision

The schema contract is the CLI's provider adaptation boundary.

Commands remain stable. Agent-facing parameter evolution is handled by schema updates. Complex provider API mapping is handled by server-side provider adapters.
