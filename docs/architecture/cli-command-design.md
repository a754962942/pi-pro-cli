# CLI Command Design

Created: 2026-06-13

## Context

PI-Pro CLI is designed for direct invocation by coding agents. Commands must be stable, machine-readable, schema-driven, and safe to call from non-interactive automation.

The CLI command surface is intentionally small. Provider and model differences are expressed through provider/model options and schema resolution, not through separate provider-specific commands.

## Command Surface

Primary generation commands:

```sh
pi-pro generateImage --provider <provider> --model <model> --type <behavior>
pi-pro generateVoice --provider <provider> --model <model> --type <behavior>
pi-pro generateVideo --provider <provider> --model <model> --type <behavior>
```

Supporting commands:

```sh
pi-pro init
pi-pro update

pi-pro task status <jobId>
pi-pro task wait <jobId>
pi-pro task cancel <jobId>

pi-pro auth login
pi-pro auth logout
pi-pro auth status

pi-pro types list
pi-pro types inspect --provider <provider> --model <model> --type <type>
```

## Type Contract

`type` identifies the behavior inside a provider/model schema.

Recommended `type` format:

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

Local schema files should be grouped by provider and model:

```text
schemas/<provider>/<model>/<type>.json
```

This keeps files easy to inspect by concrete vendor and model while keeping `type` focused on behavior.

Schema discovery and inspection behavior is defined in [Schema Registry and Resolution Flow](schema-registry-and-resolution-flow.md).

## Input Methods

Agent-first input from a JSON file:

```sh
pi-pro generateVideo \
  --provider seeddance \
  --model v1 \
  --type image-to-video \
  --input request.json \
  --output ./outputs/video.mp4
```

Human/debug input:

```sh
pi-pro generateVideo \
  --provider seeddance \
  --model v1 \
  --type image-to-video \
  --prompt "A cinematic lake shot" \
  --image ./lake.png \
  --duration 5 \
  --output ./outputs/video.mp4
```

Input precedence:

```text
input JSON > CLI arguments > schema defaults
```

`--input -` should read JSON from stdin.

CLI arguments should not override fields already present in input JSON by default. Any override behavior should be explicit.

## Common Options

Recommended common options for generation commands:

```text
--provider <provider>     Required provider key
--model <model>           Required model key
--type <type>             Required behavior type
--input <file>            Read request JSON from file
--output <file>           Save a single returned artifact
--output-dir <dir>        Save multiple returned artifacts
--save-response <file>    Save full response JSON
--no-download             Return artifact URLs without downloading files
--overwrite               Allow overwriting local output files
--wait                    Wait for task completion
--no-wait                 Submit task and return immediately
--timeout <seconds>       Maximum wait time
--poll-interval <sec>     Initial polling interval
--poll-max <sec>          Maximum polling interval
--poll-backoff <number>   Polling backoff multiplier
--auth-token <token>      Override resolved auth token
```

Task polling behavior for `--wait`, `--no-wait`, and task commands is defined in [Task Polling and Server Client Flow](task-polling-and-server-client-flow.md).
Installation, initialization, and version update behavior is defined in [Install and Init Flow](install-and-init-flow.md).

## Output Contract

The CLI should emit stable machine-readable JSON on stdout.

Progress, warnings, and diagnostics should be emitted on stderr.

The shared output and error contract is defined in [Output and Error Contract](output-and-error-contract.md).

Successful submit-only response:

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

Successful completed response:

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
      "url": "https://example.com/video.mp4",
      "path": "./outputs/video.mp4",
      "mime": "video/mp4"
    }
  ]
}
```

Failed response:

```json
{
  "ok": false,
  "error": {
    "code": "UNKNOWN_ARGUMENT",
    "message": "Field `camera_fixed` is not allowed for provider `happy-horse`, model `v1`, type `image-to-video`."
  }
}
```

## Design Decisions

- Keep provider-specific commands out of the public CLI surface.
- Use `provider + model + type` as the schema selection key.
- Keep generation command names stable even as providers change.
- Default to JSON output because the primary caller is an agent.
- Default to non-interactive behavior.
- Support `--no-wait` plus `task wait` for long-running workflows.
- Keep provider parameter differences in schema and server-side provider adapters, not in command handlers.
