# Schema Parameter Contract

Created: 2026-06-23

## Context

This document records the provider/model/type parameter contract that should guide future PI-Pro CLI schema and dry-run implementation.

The structure is inspired by the public `@lingjingai/lj-awb-cli` package maintained by `@starmoon77`, especially its agent-facing schema, model options, resource, constraint, and dry-run conventions. PI-Pro should use those ideas as design reference only. Do not copy provider-specific implementation details or platform-specific request mapping.

Related documents:

```text
schema-contract.md
schema-registry-and-resolution-flow.md
validation-normalization-pipeline.md
cli-command-design.md
asset-io-sqlite-flow.md
```

## Design Goals

- Let agents inspect real provider/model parameters before generation.
- Keep command handlers stable while provider parameters evolve through schema.
- Keep schema declarative and data-only.
- Keep provider payload construction on the server.
- Support dry-run as validation and execution preview, not task submission.
- Avoid forcing the CLI to hard-code every future model parameter.

## Source Design Lessons

`@lingjingai/lj-awb-cli` exposes two useful layers:

```text
schema --brief
  compact agent-facing command map, safety policy, workflow policy, output policy

model options / create-spec
  model-specific params, resources, constraints, input requirements, examples
```

PI-Pro should adapt this into:

```text
pi-pro schema --brief
pi-pro schema inspect --provider <provider> --model <model> --type <type>
```

The `types` command remains local provider/model/type discovery and raw schema inspection.

The `schema` command becomes the agent-facing execution contract.

## Provider Schema Extension

Each provider schema file still represents exactly one behavior:

```text
schemas/<provider>/<model>/<type>.json
```

Recommended top-level shape:

```json
{
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "schemaVersion": "1.0",
  "artifactKind": "video",
  "displayName": "SeedDance Image to Video",
  "description": "Generate a video from an image and prompt.",
  "input": {},
  "params": [],
  "resources": [],
  "constraints": [],
  "task": {},
  "artifacts": {},
  "safety": {},
  "workflow": {},
  "examples": []
}
```

`input` remains the validation source for normalized request fields.

`params`, `resources`, and `constraints` are agent-facing discovery and decision helpers. They should be derived from the same contract as `input`; they must not contradict `input`.

## Parameters

`params[]` describes controllable model parameters.

Recommended shape:

```json
{
  "key": "quality",
  "type": "string",
  "cliArg": "--quality",
  "genericModelParam": false,
  "values": ["720p", "1080p"],
  "defaultValue": "720p",
  "required": false,
  "affectsCost": true,
  "affectsOutput": true,
  "description": "Output quality or resolution tier."
}
```

Fields:

```text
key
type
cliArg
genericModelParam
values
defaultValue
required
affectsCost
affectsOutput
description
```

Rules:

- Values with `values` must be selected from that list.
- `defaultValue` is a candidate default for agents to show or use only when explicitly allowed by the user or calling context.
- Do not silently inject defaults for effect/cost-critical params unless the schema marks the field as safe for default injection through `input.properties.<field>.default`.
- Parameters not present in `params[]` or `input.properties` must not be invented.
- `false`, `0`, empty arrays, and empty objects are valid values and must not be dropped by truthiness checks.
- `null` and empty string are treated as missing when normalization policy allows omission.

`genericModelParam=true` is reserved for future generic model flags. The initial PI-Pro CLI can keep all params under normal schema fields and defer generic dynamic flag support until Chapter 12 implementation.

## Prompt Image Reference Rules

Some providers bind prompt text to image inputs by array order. These rules belong in the server-published schema, usually under `input.properties.prompt.description`, and the CLI must preserve them during validation, normalization, dry-run, and submission.

Seedance uses ordered references:

```text
@图1 -> images[0]
@图2 -> images[1]
@图N -> images[N-1]
```

For `two-image-to-video`, `@图1` is the start frame and `@图2` is the end frame. For `multi-image-to-video`, agents should describe image roles using the same order, for example `@图1为主要人物参考，@图2为场景与氛围参考`.

CLI requirements:

- Do not reorder `images` after reading user input.
- Do not translate Seedance prompt references to another syntax.
- Surface `prompt.description` in `schema inspect` so agents can follow the provider-specific reference rule.
- Treat prompt image references as semantic guidance; shape validation still comes from `input.required` and `input.properties`.

## Resources

`resources[]` describes model media inputs and their allowed usages.

Recommended shape:

```json
{
  "key": "image",
  "mediaType": "IMAGE",
  "usage": ["first_frame", "last_frame"],
  "valueShapes": ["file", "url", "asset"],
  "fileResolve": "asset-db",
  "fileTypes": ["image/png", "image/jpeg", "image/webp"],
  "maxFiles": 1,
  "required": true,
  "supportLastFrameOnly": false,
  "minDurationMs": null,
  "maxDurationMs": null,
  "description": "First frame or last frame image input."
}
```

Recommended `mediaType` values:

```text
IMAGE
VIDEO
AUDIO
SUBJECT
```

Recommended `usage` values:

```text
reference
first_frame
last_frame
keyframe
source
```

Recommended `valueShapes`:

```text
file
url
asset
platformPath
```

PI-Pro file handling rules:

- `fileResolve=asset-db` means a local absolute path is resolved through the CLI SQLite filePath-to-url mapping.
- `fileResolve=upload` means the CLI may upload the file to the server when generation is not dry-run.
- `fileResolve=asset-db-or-upload` means the CLI resolves existing mappings first and uploads only when explicitly allowed.
- Dry-run must never upload unknown files.
- If a resource accepts audio input, it must be modeled as a resource such as `mediaType=AUDIO usage=reference`; do not overload boolean output controls such as `needAudio`.
- Subject-like references should use `mediaType=SUBJECT` and `valueShapes=["asset"]`.

The existing `input.properties.<field>.type=file` contract remains valid for simple single-file schemas. `resources[]` is the richer discovery layer for agents and future multi-resource behavior.

## Constraints

`constraints[]` describes conditional parameter/resource narrowing.

Recommended shape:

```json
{
  "target": "params.ratio",
  "conditions": [
    {
      "field": "resources.image.usage",
      "operator": "contains",
      "value": "first_frame"
    }
  ],
  "effect": "allow_values",
  "values": ["16:9", "9:16"],
  "description": "When first-frame image is used, only selected ratios are available."
}
```

Resource limit override example:

```json
{
  "target": "resources.image",
  "conditions": [
    {
      "field": "type",
      "operator": "eq",
      "value": "image-to-video"
    }
  ],
  "effect": "resource_limit_overrides",
  "limits": {
    "maxFiles": 1
  }
}
```

Recommended operators:

```text
eq
neq
in
not_in
contains
exists
missing
```

Recommended effects:

```text
allow_values
deny_values
no_selectable_values
resource_limit_overrides
required_when
forbidden_when
```

Rules:

- Constraints are declarative. They must not contain executable code.
- If constraints make a parameter unavailable, the CLI should omit that parameter rather than submit an invalid value.
- If constraints cannot be evaluated safely, generation must fail before `POST /generations`.

## Safety

`safety` lets agents decide whether a command can run automatically.

Recommended shape:

```json
{
  "safeToAutoRun": false,
  "remoteWrite": true,
  "localStateWrite": false,
  "costsCredits": true,
  "destructive": false,
  "requiresConfirmation": false,
  "supportsDryRun": true,
  "longRunning": true
}
```

Generation commands should normally be:

```text
remoteWrite=true
costsCredits=true when server enables billing
supportsDryRun=true
longRunning=true for async video or audio generation
```

For the current internal-test version, billing/credit accounting is out of scope, but the schema should keep the field available as a declaration.

## Workflow

`workflow` describes agent execution order and output expectations.

Recommended shape:

```json
{
  "recommendedPreflight": ["schema inspect", "dry-run"],
  "outputKind": "generation_task",
  "nextActions": [
    "submit generation",
    "wait for task when requested",
    "download artifacts unless --no-download is set"
  ],
  "defaultWait": true
}
```

Rules:

- `schema inspect` is read-only and safe to auto-run.
- `dry-run` validates and previews one request.
- `dry-run` is not a parameter exploration loop.
- Formal generation should use user-provided or already-confirmed parameters.

## Agent Brief Output

`pi-pro schema --brief` should be compact enough to read before planning.

Recommended shape:

```json
{
  "schemaVersion": 1,
  "kind": "agent_brief",
  "cli": {
    "name": "pi-pro",
    "version": "0.1.0"
  },
  "commands": ["generateImage", "generateVoice", "generateVideo"],
  "schemaKey": "provider + model + type",
  "providers": [
    {
      "provider": "seeddance",
      "models": [
        {
          "model": "v1",
          "types": ["image-to-video", "text-to-video"]
        }
      ]
    }
  ],
  "outputPolicy": {
    "stdout": "machine-readable JSON",
    "stderr": "progress, warnings, diagnostics"
  },
  "safetyPolicy": {
    "supportsDryRun": true,
    "dryRunDoesNotSubmit": true
  },
  "lookup": {
    "preciseSchema": "pi-pro schema inspect --provider <provider> --model <model> --type <type>",
    "rawSchema": "pi-pro types inspect --provider <provider> --model <model> --type <type>"
  }
}
```

## Schema Inspect Output

`pi-pro schema inspect` should return one precise execution contract.

Recommended shape:

```json
{
  "schemaVersion": 1,
  "kind": "execution_contract",
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "artifactKind": "video",
  "requiredFields": ["prompt", "image"],
  "params": [],
  "resources": [],
  "constraints": [],
  "normalization": {},
  "task": {},
  "artifacts": {},
  "safety": {},
  "workflow": {},
  "examples": []
}
```

It must not return executable mapper code.

## Dry-Run Output

Dry-run should preview the normalized request and execution plan.

Recommended shape:

```json
{
  "ok": true,
  "status": "dry-run",
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "artifactKind": "video",
  "request": {
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
      }
    }
  },
  "plan": {
    "wouldSubmit": true,
    "wouldWait": true,
    "wouldPoll": false,
    "wouldUpload": false,
    "wouldDownload": false,
    "uploadBlockedByDryRun": false
  }
}
```

Dry-run must not:

```text
POST /generations
poll tasks
upload files
download artifacts
write asset mappings
consume credits
```

## Examples

Image-to-video schema excerpt:

```json
{
  "provider": "seeddance",
  "model": "v1",
  "type": "image-to-video",
  "schemaVersion": "1.0",
  "artifactKind": "video",
  "input": {
    "unknownPolicy": "reject",
    "required": ["prompt", "image"],
    "properties": {
      "prompt": {
        "type": "string",
        "minLength": 1
      },
      "image": {
        "type": "file",
        "accept": ["image/png", "image/jpeg", "image/webp"],
        "fileResolve": "asset-db"
      },
      "duration": {
        "type": "number",
        "enum": [5, 10]
      },
      "quality": {
        "type": "string",
        "enum": ["720p", "1080p"]
      }
    }
  },
  "params": [
    {
      "key": "duration",
      "type": "number",
      "cliArg": "--duration",
      "values": [5, 10],
      "defaultValue": 5,
      "required": false,
      "affectsCost": true,
      "affectsOutput": true
    },
    {
      "key": "quality",
      "type": "string",
      "cliArg": "--quality",
      "values": ["720p", "1080p"],
      "defaultValue": "720p",
      "required": false,
      "affectsCost": true,
      "affectsOutput": true
    }
  ],
  "resources": [
    {
      "key": "image",
      "mediaType": "IMAGE",
      "usage": ["first_frame"],
      "valueShapes": ["file", "url", "asset"],
      "fileResolve": "asset-db",
      "fileTypes": ["image/png", "image/jpeg", "image/webp"],
      "maxFiles": 1,
      "required": true
    }
  ],
  "safety": {
    "remoteWrite": true,
    "costsCredits": false,
    "supportsDryRun": true,
    "longRunning": true
  }
}
```

Audio reference rule example:

```json
{
  "key": "audio",
  "mediaType": "AUDIO",
  "usage": ["reference"],
  "valueShapes": ["file", "url", "asset"],
  "fileResolve": "asset-db-or-upload",
  "fileTypes": ["audio/mpeg", "audio/wav"],
  "maxFiles": 1,
  "required": false
}
```

`needAudio` or similar booleans should only represent generated output audio, not audio input.

## Server Boundary

The CLI sends normalized declarative input:

```text
provider
model
type
artifactKind
input
```

The server owns:

```text
provider adapter selection
final provider API payload mapping
provider-specific auth
provider-specific default translation
task creation and callback handling
task status query
artifact URL persistence
```

The CLI must not download or execute remote mapper code.

## Future Implementation Notes

Chapter 12 should implement only the generic shape:

```text
schema --brief
schema inspect
dry-run validation and preview
```

Do not implement provider-specific schema behavior until concrete server-published schemas exist.

If server-managed schema registration is added later, it should publish the same data-only contract. The CLI can cache and inspect that data, but provider mapping remains server-side.
