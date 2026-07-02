# Current Development Status

Created: 2026-06-23
Updated: 2026-06-29

## Summary

PI-Pro CLI is active in server integration with pi-pro-server through HTTP/JSON APIs, remote capability discovery, and remote schema lookup.

Current implementation state:

```text
completed: Chapter 0 - Chapter 11
completed: Chapter 12 Agent Schema and Dry Run
partially completed: Chapter 13 MVP Integration Gate
verified real path: CLI -> Server -> Grok -> MinIO
verified real path: CLI -> Server -> Vidu -> MinIO
verified real path: CLI -> Server -> Qiling text-to-image/image-to-image -> MinIO
verified server provider path: Server -> Seedance-2.0 / Seedance-2.0-fast -> MinIO
verified server provider path: Server -> Qiling text-to-image/image-to-image -> MinIO for the currently advertised image capability matrix
```

## Implemented CLI Surface

Core commands:

```text
pi-pro --version
pi-pro init
pi-pro update
pi-pro auth login
pi-pro auth logout
pi-pro auth status
pi-pro types list
pi-pro types inspect --type <type>
pi-pro types inspect --provider <provider> --model <model> --type <type>
pi-pro schema --brief
pi-pro schema inspect --provider <provider> --model <model> --type <type>
pi-pro generateImage --provider <provider> --model <model> --type <type>
pi-pro generateVoice --provider <provider> --model <model> --type <type>
pi-pro generateVideo --provider <provider> --model <model> --type <type>
pi-pro task status <jobId>
pi-pro task wait <jobId>
pi-pro task cancel <jobId>
```

Server integration:

```text
POST /cli/version
GET /cli/init-manifest
GET /cli/files/*
POST /auth/login
GET /capabilities/types
GET /capabilities/types/:type/models
GET /cli/schema
POST /generations
GET /tasks/:jobId
POST /tasks/:jobId/cancel
```

Capability and schema behavior:

```text
types list prefers the remote capability API and falls back to local downloaded schemas
types inspect --type reads remote model/provider capability mappings
types inspect with provider/model/type reads the selected schema
schema --brief returns compact agent-readable capability/schema information
schema inspect returns the precise provider/model/type schema contract
generation commands resolve schemas through remote schema first and local fallback
schema validation remains the request-shape authority
provider-specific payload mapping remains server-side
```

Generation behavior:

```text
generateImage, generateVoice, and generateVideo share the generic generation pipeline
provider/model/type are required
input JSON can come from --input <file> or --input -
input JSON has priority over duplicate CLI flag values
schema defaults, required fields, unknown-field policy, basic types, enums, ranges, and string lengths are validated
file fields can resolve through the local SQLite asset database when schema declares fileResolve behavior
generation submits POST /generations
default behavior waits for task completion and polls GET /tasks/:jobId
--no-wait returns the submitted task result without polling
polling supports timeout, interval, max interval, backoff, and jitter controls
stdout stays machine-readable JSON; progress diagnostics go to stderr
```

Auth, config, and local state:

```text
config and runtime files live under ~/.pi-pro by default
PI_PRO_CONFIG_DIR isolates test/runtime state
auth login stores opaque sk-pipro-* tokens without parsing them
auth status redacts tokens
init downloads server-published files with checksum verification
assets.sqlite stores local asset metadata for schema-driven file resolution
install.sh installs binaries only and does not download schemas or runtime state
```

## Verified Integration

Latest verified behavior:

```text
CLI init/login/capability inspect/generateVideo works against pi-pro-server
CLI -> Server -> Grok -> MinIO real E2E passed
CLI -> Server -> Vidu -> MinIO real E2E passed
CLI -> Server -> Qiling text-to-image/image-to-image -> MinIO real E2E passed
Server -> Seedance-2.0 -> MinIO direct provider E2E passed
Server -> Seedance-2.0-fast -> MinIO direct provider E2E passed
```

Known verification commands:

```sh
go test ./... -count=1
```

Server-side real provider E2E tests are intentionally opt-in because they call external provider APIs and may consume paid or limited quota.

## Development Snapshot for Next Iteration

Recorded at:

```text
2026-06-29
```

Current readiness:

```text
CLI command surface is usable for the main remote-server workflow.
CLI should continue to stay provider-agnostic; provider-specific request mapping belongs in pi-pro-server.
Remote capability API is the source of truth for visible type/model/provider combinations.
Remote schema API is the source of truth for request validation shape.
Local schemas remain fallback only.
generate* commands validate provider/model/type against remote capability API before POST /generations.
```

Verified CLI -> Server paths:

```text
init -> auth login -> capability discovery -> schema inspect -> generation submit -> task polling
generateVideo -> Grok image-to-video -> MinIO
generateVideo -> Vidu image-to-video -> MinIO
generateImage -> Qiling text-to-image -> MinIO
generateImage -> Qiling image-to-image -> MinIO
task cancel returns CANCEL_UNSUPPORTED when provider cancellation is unavailable
```

Implemented generation type coverage from CLI perspective:

```text
text-to-video: discoverable and generatable when server exposes a matching provider/model/schema
image-to-video: discoverable and generatable
two-image-to-video: request path and schema validation are covered; needs real CLI-driven provider gate
multi-image-to-video: request path and schema validation are covered; needs real CLI-driven Seedance gate
text-to-image: discoverable and real CLI E2E verified
image-to-image: discoverable and real CLI E2E verified for server-advertised models
```

Next recommended CLI work order:

```text
P1: run full CLI-driven Seedance verification for text/image/two-image/multi-image video types
P2: implement real upload wiring when server schema declares file upload instead of URL/base64 input
P2: refresh README examples with image and video commands after dry-run/output semantics land
P2: validate release packaging and install/update flow against a release-like server
```

Known constraints for upcoming development:

```text
Do not hardcode the Qiling, Grok, Vidu, or Seedance payload mapping in CLI.
Do not expose provider base URLs or API keys in CLI capability output.
Do not treat gpt-image-2 image-to-image as supported unless server capability API advertises it again.
Do not make local schema fallback override remote capability decisions.
Do not make dry-run call provider, upload files, poll tasks, or download artifacts.
```

## Server Image Capability Sync

As of 2026-06-29, pi-pro-server exposes image capability discovery through the same remote capability/schema APIs already used by CLI:

```text
type: text-to-image -> 文生图
provider: qiling
models:
- gpt-image-2
- gpt-image-2-vip
- nano-banana-2
- nano-banana-2-2k-cl

type: image-to-image -> 图片编辑/参考生图
provider: qiling
models:
- gpt-image-2-vip
- nano-banana-2
- nano-banana-2-2k-cl
```

CLI status:

```text
generateImage discovers image capabilities through remote capability/schema APIs.
CLI -> Server -> Qiling -> MinIO E2E now verifies text-to-image and image-to-image.
gpt-image-2 image-to-image is intentionally not advertised by server capability API until the provider channel is verified stable.
```

## Partially Completed Chapters

### Chapter 12: Agent Schema and Dry Run

Status:

```text
completed
```

Completed:

```text
pi-pro schema --brief
pi-pro schema inspect --provider <provider> --model <model> --type <type>
remote schema/capability first, local schema fallback
schema command tests
generateImage --dry-run
generateVoice --dry-run
generateVideo --dry-run
dry-run normalized request preview
dry-run execution plan output
dry-run skips auth requirement, POST /generations, task polling, file resolution, upload, and artifact download
generation capability preflight rejects unsupported provider/model/type before POST /generations
--output downloads a single returned artifact after a succeeded waited task
--output-dir downloads multiple returned artifacts after a succeeded waited task
downloaded artifact paths are returned in command JSON and recorded in the local asset DB
```

### Chapter 13: MVP Integration Gate

Status:

```text
partially completed
```

Completed:

```text
init -> auth -> capability discovery -> schema inspect -> generateVideo -> task polling has been verified against pi-pro-server
real Grok and Vidu video generation paths have been verified through CLI/server/provider/MinIO
documentation now records Seedance prompt image references from server-published schemas
```

Pending:

```text
full CLI-driven Seedance path verification
README command examples against current real behavior
release-package verification
```

## Known Pending Work

High priority:

```text
real multipart upload wiring when a schema requires upload
full CLI-driven Seedance verification
README command examples for current commands
```

Before release:

```text
run shellcheck scripts/install.sh
build release binaries with ldflags-injected version
build Windows package with pi-pro.exe and pi-pro-updater.exe
verify install.sh against release metadata
verify init/auth/schema/generation/task flows against the release server
```

## Constraints To Preserve

```text
Do not add provider-specific payload mapping to the CLI.
Do not parse or introspect sk-pipro-* auth tokens in the CLI.
Do not execute remote schema mapper code in the CLI.
Do not auto-update during generation or task commands.
Do not make install.sh download schemas or runtime state.
Do not make dry-run submit generation, poll tasks, upload files, or download artifacts.
Keep stdout machine-readable JSON.
Keep progress diagnostics on stderr.
```
