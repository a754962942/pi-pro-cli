# Validation and Normalization Pipeline

Created: 2026-06-13

## Context

PI-Pro CLI is agent-facing. Generation commands receive structured input from CLI arguments, `--input` JSON files, or stdin, then validate and normalize that input according to the selected provider/model/type schema.

The pipeline stops at a stable server request. It does not construct final provider API payloads.

## Input Sources

Supported input sources:

```text
CLI arguments
--input <file>
--input -
stdin
schema defaults
```

`--input -` means read JSON from stdin.

Recommended examples:

```sh
pi-pro generateVideo --provider seeddance --model v1 --type image-to-video --input request.json
```

```sh
cat request.json | pi-pro generateVideo --provider seeddance --model v1 --type image-to-video --input -
```

## Input Precedence

Default precedence:

```text
input JSON > CLI arguments > schema defaults
```

Rationale:

- Agent calls should prefer a single structured request body.
- CLI arguments remain useful for human debugging and small calls.
- Avoid silently mutating an agent-generated JSON request through extra flags.

CLI arguments should not override fields already present in `--input` JSON by default.

If override behavior is needed later, it should be explicit, for example:

```text
--override duration=10
```

or:

```text
--allow-cli-overrides
```

The default should stay non-overriding.

## Pipeline Steps

```text
1. Parse command and common options
2. Read input JSON from file or stdin when provided
3. Merge input using precedence rules
4. Resolve schema by provider, model, and type through TypeRegistry
5. Verify schema artifact kind matches invoked command
6. Apply raw normalization
7. Validate required fields
8. Validate field types and constraints
9. Apply default values
10. Resolve file fields through local asset database
11. Build stable server request
12. Submit request or return validation error
```

## Command and Schema Match

The selected schema must be compatible with the invoked public command.

Example invalid case:

```sh
pi-pro generateVideo --provider openai --model v1 --type text-to-image
```

If the schema declares:

```json
{
  "artifactKind": "image"
}
```

the CLI should fail before validation.

Recommended error code:

```text
COMMAND_TYPE_MISMATCH
```

The schema metadata must also match the requested provider, model, and type. Mismatches should fail before validation.

## Normalization Rules

Default raw normalization:

```text
remove undefined
remove null
remove empty strings
preserve false
preserve 0
preserve empty arrays unless schema rejects them
preserve empty objects unless schema rejects them
do not trim strings by default
```

`trimString` should be opt-in through schema or a future explicit CLI option. It should not be enabled globally by default because prompts and model parameters can be whitespace-sensitive.

## Default Value Rules

Defaults are applied when:

```text
field is missing
field is null and null omission is enabled
field is an empty string and empty-string omission is enabled
```

Defaults should only be injected for fields that are not required.

If a required field is missing after normalization, the CLI should fail with `MISSING_REQUIRED_FIELD`.

## Validation Rules

Validation is schema-driven.

Recommended validation categories:

```text
required field
unknown field
field type
enum value
number range
string length
file path
file media type or extension
command/artifact kind match
```

Unknown fields should follow the schema's `unknownPolicy`.

Recommended default:

```text
reject
```

## File Field Resolution

Most file inputs are expected to be local files that were previously downloaded from server artifact URLs. The CLI should use a local SQLite asset database to resolve file paths back to their source URLs.

Detailed asset IO and SQLite behavior is defined in [Asset IO and SQLite Flow](asset-io-sqlite-flow.md).

Recommended lookup:

```text
filePath -> sourceUrl
```

For these fields, the CLI should:

```text
validate that the file path exists
lookup the file path in SQLite
replace the file path with the known URL reference
fail if the file has no URL mapping and upload is not explicitly requested
```

Recommended normalized shape:

```json
{
  "image": {
    "source": "asset-db",
    "path": "/absolute/path/to/image.png",
    "url": "https://server.example/artifacts/image.png"
  }
}
```

## Explicit Upload Flow

Some files may be user-provided local assets that do not yet exist on the server.

The CLI should provide upload capability for those cases, separate from ordinary file resolution.

Recommended behavior:

```text
1. CLI uploads the file to the server.
2. Server returns a URL.
3. CLI downloads or verifies the server-side artifact if needed.
4. CLI writes filePath -> url mapping into SQLite.
5. CLI uses that URL in normalized input.
```

This keeps future calls stable because the same local file path can be resolved without re-uploading.

Schema should distinguish these two cases explicitly:

```text
fileResolve: asset-db
fileResolve: upload
fileResolve: asset-db-or-upload
```

Default recommendation:

```text
asset-db
```

## Server Request Shape

The exact provider schema is not fixed yet, so the server request shape should remain minimal and stable.

Recommended temporary shape:

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
      "path": "/absolute/path/to/image.png",
      "url": "https://server.example/artifacts/image.png"
    },
    "duration": 5
  }
}
```

The server owns provider-specific payload construction.

## Error Model

Validation and normalization failures should produce stable JSON.

The shared output and error contract is defined in [Output and Error Contract](output-and-error-contract.md).

Recommended shape:

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

Recommended error codes:

```text
TYPE_NOT_FOUND
SCHEMA_LOAD_FAILED
COMMAND_TYPE_MISMATCH
INVALID_INPUT_JSON
MISSING_REQUIRED_FIELD
UNKNOWN_FIELD
INVALID_FIELD_TYPE
INVALID_ENUM_VALUE
INVALID_RANGE
INVALID_FILE
ASSET_URL_NOT_FOUND
UPLOAD_FAILED
VALIDATION_ERROR
```

## Design Decisions

- Support `--input -` for stdin JSON.
- Prefer input JSON over CLI arguments.
- Do not allow implicit CLI argument overrides for fields already present in input JSON.
- Remove undefined, null, and empty strings during normalization.
- Preserve `false` and `0`.
- Do not trim strings by default.
- Apply defaults only to non-required missing fields.
- Treat null and empty string as missing when default injection applies.
- Resolve file fields through SQLite filePath-to-url mapping by default.
- Provide explicit upload behavior for files that are not already known server artifacts.
- Keep provider payload construction on the server.
