# Schema Registry and Resolution Flow

Created: 2026-06-14

## Context

PI-Pro CLI uses `provider + model + type` to resolve one schema file. The first implementation downloads schemas during `pi-pro init` and reads them from the local config directory. The architecture should allow the server to manage and register schemas later.

Command handlers must not know whether a schema came from init-downloaded local files, cache, or the server.

## Design Goals

- Keep command handlers independent from schema storage.
- Support init-downloaded local schemas for the first implementation.
- Allow server-managed schemas later without changing command behavior.
- Keep a cache boundary available for future remote schemas.
- Never execute remote code from schema responses.
- Keep schema lookup deterministic for agents.

## Business Rules

Schema publishing is owned by the server.

Each schema is strongly bound to its model version:

```text
provider + model + type
```

If a schema is wrong, incompatible, malformed, or does not match its metadata, the CLI must fail validation/resolution and must not submit the generation request.

This rule intentionally prevents "best effort" calls with incorrect provider schemas. A failed schema is safer than sending an invalid or ambiguous request downstream.

## Schema Key

The schema lookup key is:

```text
provider + model + type
```

Local path:

```text
schemas/<provider>/<model>/<type>.json
```

Example:

```text
schemas/seeddance/v1/image-to-video.json
```

## Registry Boundary

Command handlers should depend on `TypeRegistry`, not on local files.

```text
Command Handler
  ↓
TypeRegistry.get(provider, model, type)
  ↓
SchemaSource
```

Recommended interface:

```text
TypeRegistry
- get(provider, model, type)
- list(filter)
- inspect(provider, model, type)
```

`get` returns a validated schema object.

`list` returns available schema summaries.

`inspect` returns full schema details for agent discovery.

## Schema Sources

Recommended source abstraction:

```text
SchemaSource
- get(provider, model, type)
- list(filter)
```

First implementation:

```text
LocalSchemaSource
```

Future implementation:

```text
RemoteSchemaSource
CachedSchemaSource
CompositeSchemaSource
```

Composite resolution order for future use:

```text
1. RemoteSchemaSource
2. CachedSchemaSource
3. LocalSchemaSource
```

For the first implementation, use only:

```text
LocalSchemaSource
```

## Local Schema Flow

```text
1. Receive provider, model, type.
2. Build local schema path under ~/.pi-pro/schemas.
3. Read JSON from schemas downloaded by pi-pro init.
4. Parse JSON.
5. Validate schema metadata matches path.
6. Validate schema contract shape.
7. Return schema to validation pipeline.
```

Metadata must match:

```text
schema.provider == provider
schema.model == model
schema.type == type
```

Mismatch should fail with:

```text
SCHEMA_METADATA_MISMATCH
```

Malformed or invalid schema should fail with:

```text
SCHEMA_INVALID
```

The generation command must stop before `POST /generations` when schema resolution or validation fails.

## Remote Schema Flow

Remote schema management is deferred, but the boundary should support it.

Future endpoints:

```text
GET /types
GET /types/:provider/:model/:type/schema
```

Future remote resolution:

```text
1. Resolve auth/config.
2. Request schema from server.
3. Validate response shape.
4. Validate provider/model/type metadata.
5. Cache schema when allowed.
6. Return schema to validation pipeline.
```

The CLI must treat remote schema as data only.

It must not:

```text
execute JavaScript from schema
load remote code
install mapper plugins dynamically
run shell commands declared by schema
```

Complex provider payload mapping remains a server responsibility.

## Cache Policy

Schema caching is useful after remote schemas exist.

Recommended cache location:

```text
~/.pi-pro/schema-cache/
```

Recommended cache key:

```text
<provider>/<model>/<type>.json
```

Recommended cache metadata:

```text
schemaVersion
fetchedAt
expiresAt
etag
source
```

First implementation can skip schema cache because `pi-pro init` materializes schemas into `~/.pi-pro/schemas`.

## Types List

`pi-pro types list` should return schema summaries.

Example:

```json
{
  "ok": true,
  "types": [
    {
      "provider": "seeddance",
      "model": "v1",
      "type": "image-to-video",
      "artifactKind": "video",
      "displayName": "SeedDance Image to Video"
    }
  ]
}
```

First implementation can list init-downloaded local schemas only.

Future implementation can merge remote and local schemas, with source metadata:

```json
{
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "source": "remote"
}
```

## Types Inspect

`pi-pro types inspect` should return the full schema.

Command:

```sh
pi-pro types inspect --provider seeddance --model v1 --type image-to-video
```

Output:

```json
{
  "ok": true,
  "schema": {
    "provider": "seeddance",
    "model": "v1",
    "type": "image-to-video",
    "schemaVersion": "1.0"
  }
}
```

This command helps agents discover required parameters before calling generation commands.

## Error Codes

Recommended schema registry errors:

```text
TYPE_NOT_FOUND
SCHEMA_LOAD_FAILED
SCHEMA_PARSE_FAILED
SCHEMA_INVALID
SCHEMA_METADATA_MISMATCH
SCHEMA_REMOTE_UNAVAILABLE
SCHEMA_CACHE_READ_FAILED
SCHEMA_CACHE_WRITE_FAILED
```

## YAGNI Filter

Keep for the first implementation:

```text
LocalSchemaSource
TypeRegistry abstraction
provider + model + type lookup
schema metadata validation
types list
types inspect
stable schema error codes
```

Defer until required:

```text
RemoteSchemaSource
schema cache
etag handling
cache ttl
remote/local merge conflict policy
schema signature verification
schema marketplace
dynamic mapper plugins
```

Rationale:

```text
The first implementation only needs deterministic schema discovery and validation from files downloaded by `pi-pro init`. The abstraction prevents command handlers from depending on local paths, so direct remote registration can be added later without rewriting generation commands.
```

## Design Decisions

- Use `provider + model + type` as the schema lookup key.
- Store one behavior per schema file.
- Keep command handlers dependent on `TypeRegistry`, not filesystem paths.
- Use init-downloaded local schemas for the first implementation.
- Defer remote schema registration until server support exists.
- Treat all schemas as data, not executable code.
- Keep provider payload mapping on the server.
