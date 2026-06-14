# Development Roadmap

Created: 2026-06-14

## Purpose

This roadmap connects the architecture documents into an implementation sequence for PI-Pro CLI. It should guide later development work so implementation follows the established boundaries instead of rediscovering decisions from scattered notes.

## Source Documents

Read these documents before implementation:

```text
architecture-review.md
project-structure-and-package-boundaries.md
version-comparison-flow.md
tdd-test-cases.md
cli-command-design.md
output-and-error-contract.md
install-and-init-flow.md
auth-and-config-flow.md
schema-contract.md
schema-registry-and-resolution-flow.md
validation-normalization-pipeline.md
asset-io-sqlite-flow.md
task-polling-and-server-client-flow.md
server-endpoint-contract.md
server-responsibilities.md
```

## Guiding Principles

```text
KISS: implement the shortest reliable path first
YAGNI: do not implement deferred features
DRY: centralize shared output, config, schema, and client logic
SOLID: keep command handlers thin and move behavior into focused services
```

The CLI should be implemented in Go.

## Mandatory Library Selection Rule

Before implementing any new feature, the developer must check whether the capability is already covered by stable existing tools.

Selection priority:

```text
1. mature official or open-source libraries when they significantly reduce code volume, edge-case handling, or orchestration complexity
2. Go standard library when it is simple, explicit, and does not force custom protocol/parser/storage logic
3. official Go packages maintained by the relevant project/vendor
4. small in-house implementation only when existing libraries do not materially reduce complexity or are unsuitable
```

Practical rule:

```text
Prefer a third-party library when it clearly makes the implementation smaller, safer, and easier to maintain.
Prefer the standard library when the feature is trivial or a dependency would add more operational cost than it removes.
```

This rule is mandatory for feature work that touches:

```text
CLI argument parsing
interactive prompts and hidden password input
HTTP client behavior
JSON schema validation
SQLite access
checksum and archive handling
file downloads/uploads
task retry/backoff
install script platform detection
```

When a dependency is considered, record the decision in the chapter implementation notes or PR summary:

```text
selected dependency or standard-library package
reason it fits the current feature
license compatibility
maintenance/activity signal
complexity reduction compared with in-house code
why simpler alternatives were rejected, if applicable
```

Do not add a dependency for trivial logic that is clearer with the standard library. Do not build a custom version of a mature protocol, parser, validator, CLI framework, retry mechanism, or storage layer when a reliable library can materially reduce implementation complexity.

## MVP Scope

In scope for the first implementation:

```text
Go binary CLI
install.sh installs binary only
pi-pro init downloads runtime files from server
pi-pro update supports explicit binary update
interactive pi-pro auth login
auth logout/status
types list/inspect from local initialized schemas
generateImage/generateVoice/generateVideo
input JSON and stdin support
schema-driven validation and normalization
SQLite asset mapping
task polling
artifact download
stable stdout JSON and stderr diagnostics
```

Out of scope for the first implementation:

```text
browser login
multi-profile support
quota/credit UI
remote direct schema lookup
schema cache
schema signatures
background update
delta patching
OS keychain
debug protocol
plugin system
```

## Chapter Development Plan

Development should proceed chapter by chapter. Each chapter must follow TDD:

```text
research standard library, official packages, and mature open-source options for the chapter scope
record the dependency decision before implementation
write or update the chapter tests first
run the tests and confirm they fail for the expected reason
implement the smallest production code needed
run the chapter tests
run go test ./...
only then move to the next chapter
```

Do not start implementation for a later chapter while the current chapter has failing tests.

### Chapter 0: Skeleton Baseline

Status:

```text
completed on 2026-06-14
server integration pending
```

Purpose:

```text
Keep the repository buildable and establish the command entry point.
```

Implementation scope:

```text
go.mod
cmd/pi-pro/main.go
internal/commands
internal/config local version
package boundary placeholders
```

Dependency decision:

```text
Use github.com/spf13/cobra for command tree and flag management.
Do not use the cobra-cli generator.
Do not introduce Viper for the MVP.
Keep Cobra usage thin and hand-written so generated boilerplate does not shape the architecture.
All business errors must still become AppError and render through internal/output as JSON.
```

Reasoning:

```text
PI-Pro CLI has nested commands such as auth login, task wait, and types inspect.
Using custom parsing for nested command flags would increase complexity and violate the mandatory library selection rule.
Cobra is a mature open-source Go CLI framework for command trees, flags, help, and completion.
Viper is unnecessary because the MVP has a small explicit config model and a built-in server URL.
```

Tests:

```text
go test ./...
go run ./cmd/pi-pro --help
go run ./cmd/pi-pro --version
```

Exit criteria:

```text
CLI starts
help text lists planned MVP commands
version command returns stable JSON
no business command is accidentally implemented
```

Verification:

```text
go test ./internal/commands
go test ./...
go run ./cmd/pi-pro --help
go run ./cmd/pi-pro --version
```

### Chapter 1: Output and Error Contract

Status:

```text
completed on 2026-06-14
business provider/model confirmation pending
server integration pending
```

Purpose:

```text
Make stdout/stderr and exit codes stable before business behavior exists.
```

Implementation scope:

```text
internal/output
internal/apperror
error code to exit code mapping
secret redaction
command-level error rendering
```

Dependency decision:

```text
Use the Go standard library.
encoding/json is sufficient for stable JSON output.
io.Writer keeps stdout and stderr testable without extra dependencies.
Secret redaction is intentionally small and local because the MVP only needs known CLI credential patterns.
Do not introduce logging frameworks before debug mode and structured diagnostics are designed.
```

Primary test cases:

```text
TC-001
TC-002
TC-003
TC-004
```

Recommended unit tests:

```text
internal/output/output_test.go
internal/apperror/apperror_test.go
internal/commands/commands_test.go
```

Exit criteria:

```text
success output is valid JSON
known errors use ok=false
stdout never contains progress logs
secrets are redacted from rendered output
go test ./... passes
```

Verification:

```text
go test -timeout 10s ./internal/output ./internal/apperror ./internal/commands
go test -timeout 20s ./...
```

### Chapter 2: Config and Local Paths

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Centralize all local filesystem paths and built-in server configuration.
```

Implementation scope:

```text
internal/config
PI_PRO_CONFIG_DIR test override
default ~/.pi-pro path resolver
config.json model
safe directory/file permissions
built-in server URL
```

Dependency decision:

```text
Use the Go standard library.
os, path/filepath, and encoding/json cover the current config scope with minimal code.
Viper/Koanf were not introduced because the MVP intentionally has a small config model and serverUrl must not be user-configurable.
A third-party config library can be reconsidered only if later chapters introduce multiple config sources with meaningful merge complexity.
```

Primary test cases:

```text
TC-010 partial
TC-100
TC-101
TC-170
```

Recommended unit tests:

```text
internal/config/config_test.go
```

Exit criteria:

```text
tests can isolate state without touching the real home directory
server URL is not user-configurable through CLI flags
path creation uses safe permissions
go test ./... passes
```

Verification:

```text
go test -timeout 10s ./internal/config
go test -timeout 20s ./...
```

### Chapter 3: Server Client Foundation

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Provide one HTTP boundary for every server call.
```

Implementation scope:

```text
internal/client
JSON request helpers
Bearer token injection
server error parsing
network error mapping
retry classification only where needed
```

Dependency decision:

```text
Use the Go standard library.
net/http, encoding/json, and context are sufficient for the current HTTP boundary.
Resty and similar HTTP clients were not introduced because this chapter needs explicit control over error mapping, auth header injection, and response preservation.
A third-party retry library can be reconsidered in the task polling chapter if backoff orchestration becomes non-trivial.
```

Primary test cases:

```text
TC-180
TC-181
TC-182
TC-183
```

Recommended unit tests:

```text
internal/client/client_test.go
```

Exit criteria:

```text
client does not read config directly
auth header is injected when token exists
401 and 403 map to auth errors
server error code/message are preserved
go test ./... passes
```

Verification:

```text
go test -timeout 10s ./internal/client
go test -timeout 20s ./...
```

### Chapter 4: Init and Update

Status:

```text
completed on 2026-06-14
requires follow-up before Windows first-release support is considered complete
```

Purpose:

```text
Bootstrap runtime files from the server without embedding schema in the binary.
```

Implementation scope:

```text
internal/lifecycle
internal/updater
cmd/pi-pro-updater for Windows helper update
pi-pro init
pi-pro update
POST /cli/version
/cli/init-manifest
/cli/files/*
checksum verification
atomic writes
assets.sqlite initialization
platform-specific binary replacement
Windows helper updater packaging requirement
```

Dependency decision:

```text
Use modernc.org/sqlite for SQLite initialization.
SQLite support is not available in the Go standard library.
modernc.org/sqlite is a mature pure-Go driver, avoids cgo setup, and creates a real SQLite database instead of a fragile custom placeholder file.
Use Go standard library packages for HTTP file download, sha256 checksum verification, relative path validation, and atomic file writes because those flows are small and explicit.
```

Updater architecture note:

```text
The initial lifecycle implementation must not grow OS-specific replacement logic inline.
Refactor binary replacement behind internal/updater before implementing Windows support.
Unix-like platforms may replace the managed binary directly after staging and checksum verification.
Windows must use a separate pi-pro-updater.exe helper because the running pi-pro.exe cannot reliably overwrite itself.
If Windows is part of the first release target, pi-pro-updater.exe is mandatory and every Windows package must include it.
```

Version behavior:

```text
local CLI version comes from internal/config
server release version comes from POST /cli/version
commit is not part of public version output
mock server should be used in tests
comparison rules are defined in version-comparison-flow.md
```

Primary test cases:

```text
TC-010
TC-011
TC-012
TC-013
TC-014
TC-015
TC-016
TC-090
TC-091
TC-092
TC-093
TC-094
TC-095
TC-100
TC-101
TC-102
TC-103
```

Recommended unit tests:

```text
internal/lifecycle/init_test.go
internal/lifecycle/update_test.go
internal/commands/init_command_test.go
```

Exit criteria:

```text
init creates the local runtime state
init is idempotent
bad manifest paths are rejected
checksum mismatch never installs a file
install.sh remains binary-only
Windows release package includes pi-pro.exe and pi-pro-updater.exe when Windows is supported
go test ./... passes
```

Verification:

```text
go test -timeout 30s ./internal/lifecycle ./internal/commands
go test -timeout 60s ./...
```

### Chapter 4.1: Update Lifecycle Hardening

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Close the lifecycle gaps found during review before auth and generation depend on update state.
```

Implementation scope:

```text
internal/updater boundary
managed install location validation
platform-specific update strategy selection
version-state.json persistence
update-specific error codes
init manifest shape validation
Windows helper updater decision point
```

Dependency decision:

```text
Use Go standard library packages for executable path discovery, filepath comparison, process spawning, JSON state files, and checksum checks.
Do not introduce a self-update framework yet because the first updater requirement is small and tightly constrained to ~/.pi-pro/bin.
If Windows helper process handling grows beyond simple spawn-and-exit orchestration, revisit mature updater libraries before Chapter 4.2.
```

Behavior requirements:

```text
pi-pro update must only update the managed binary path.
If the current executable is outside the managed install path, return UPDATE_UNSUPPORTED_INSTALL_LOCATION.
Init and update must write version-state.json after successful version checks.
Update downloads must use UPDATE_DOWNLOAD_FAILED.
Update checksum failures must use UPDATE_CHECKSUM_MISMATCH.
Update replacement failures must use UPDATE_REPLACE_FAILED.
Invalid init manifests must fail with INIT_MANIFEST_INVALID before downloading files.
Windows update must branch away from direct self-replacement.
If Windows helper support is not fully implemented in Chapter 4.1, Windows update must fail explicitly with UPDATE_HELPER_MISSING or UPDATE_UNSUPPORTED_PLATFORM instead of pretending to update.
```

Recommended unit tests:

```text
internal/lifecycle/lifecycle_test.go
internal/updater/updater_test.go
```

Exit criteria:

```text
version-state.json is written by init and update
update validates managed install location
update uses update-specific errors
invalid manifests are rejected before file download
platform updater selection is covered by unit tests
go test ./... passes
```

Verification:

```text
go test -timeout 30s ./internal/lifecycle ./internal/updater
go test -timeout 60s ./...
```

Chapter 4.1 implementation note:

```text
Unix-like direct replacement is implemented through internal/updater.
Windows update now branches away from direct replacement and fails explicitly with UPDATE_HELPER_MISSING.
Full Windows helper process orchestration is deferred to Chapter 4.2.
```

### Chapter 4.2: Windows Helper Updater

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Complete Windows self-update by shipping and invoking a separate helper updater binary.
```

Implementation scope:

```text
cmd/pi-pro-updater
update-state.json contract
Windows helper process spawn from pi-pro.exe update
helper waits for parent process exit
helper replaces managed pi-pro.exe
helper verifies installed checksum
helper cleanup and failure state
Windows packaging includes pi-pro.exe and pi-pro-updater.exe
```

Implementation note:

```text
cmd/pi-pro-updater is implemented as a narrow helper entry point.
internal/updater owns update-state.json, Windows helper process spawning, helper-side replacement, checksum verification, path containment checks, and cleanup.
Windows release packaging must build both:
  go build -o pi-pro.exe ./cmd/pi-pro
  go build -o pi-pro-updater.exe ./cmd/pi-pro-updater
```

Exit criteria:

```text
Windows update does not direct-replace the running pi-pro.exe
missing helper returns UPDATE_HELPER_MISSING
helper replaces the managed binary after the parent process exits
helper rejects paths outside ~/.pi-pro
go test ./... passes
```

### Chapter 5: Auth and Config Persistence

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Implement internal-test login without exposing username/password flags.
```

Implementation scope:

```text
internal/auth
pi-pro auth login
pi-pro auth logout
pi-pro auth status
interactive username prompt
hidden password prompt
token persistence in config.json
token redaction
```

Dependency decision:

```text
Use golang.org/x/term for hidden password input.
It is an official Go extended package and avoids hand-rolled terminal echo handling across platforms.
Keep prompting behind internal/auth.Prompter so tests do not depend on real terminal state.
Use the existing internal/client and internal/config packages for server calls and persistence.
Do not introduce Viper, keychain storage, browser login, profiles, or token parsing in this chapter.
```

Primary test cases:

```text
TC-020
TC-021
TC-022
TC-023
TC-024
TC-110
TC-111
TC-112
TC-113
```

Recommended unit tests:

```text
internal/auth/auth_test.go
internal/commands/auth_command_test.go
```

Exit criteria:

```text
login prompts username before password
password is not accepted through --password
status never prints token
logout removes token safely
go test ./... passes
```

Verification:

```text
go test -timeout 30s ./internal/auth ./internal/commands
go test -timeout 60s ./...
```

Integration note:

```text
Chapter 5 unit and command behavior is complete.
End-to-end login connectivity is deferred until the server implements POST /auth/login.
Before release, run pi-pro auth login against the real server and verify config.json receives authToken and username.
```

### Chapter 6: Schema Registry

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Resolve server-published local schemas by provider, model, and type.
```

Implementation scope:

```text
internal/schema
local schema filesystem scanner
schema metadata validation
types list
types inspect
one file per behavior
```

Dependency decision:

```text
Use the Go standard library for local schema scanning, JSON decoding, path validation, and deterministic sorting.
Do not introduce a JSON Schema validator in this chapter because Chapter 6 only validates registry metadata and the minimal schema contract shape.
Deep input validation belongs to Chapter 7.
Keep command handlers dependent on the registry boundary rather than direct schema file reads.
```

Primary test cases:

```text
TC-030
TC-031
TC-032
TC-033
TC-120
TC-121
TC-122
TC-123
```

Recommended unit tests:

```text
internal/schema/registry_test.go
internal/commands/types_command_test.go
```

Exit criteria:

```text
schema path and metadata must match
unknown provider/model/type fails before generation
invalid schema cannot be used
types inspect returns the selected schema
go test ./... passes
```

Verification:

```text
go test -timeout 30s ./internal/schema ./internal/commands
go test -timeout 60s ./...
```

Integration note:

```text
Chapter 6 local registry and types command behavior is complete.
Business-side provider and model configuration is not finalized yet.
After provider/model/type schemas are confirmed and the server exposes the related schema/type endpoints, run integration tests against server-published schemas.
Before release, verify pi-pro init downloads real schemas and pi-pro types list/inspect resolves those schemas correctly.
```

### Chapter 7: Input, Validation, and Normalization

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Turn agent input into a clean provider-agnostic request.
```

Implementation scope:

```text
internal/input
internal/validation
--input JSON
--input -
CLI flag merge
schema default injection
null and empty string omission
false and 0 preservation
required field enforcement
```

Dependency decision:

```text
Use the Go standard library for JSON input loading, source merging, normalization, and the first schema-driven validation pass.
Do not introduce a JSON Schema validator in this chapter because the schema contract is custom and only requires a focused subset now: required fields, unknown field policy, basic type checks, enum, ranges, string length, and defaults.
Do not implement provider-specific mapping in the CLI.
Defer file path to URL resolution to Chapter 8.
```

Primary test cases:

```text
TC-040
TC-041
TC-042
TC-043
TC-044
TC-045
TC-130
TC-131
TC-132
TC-133
TC-134
TC-135
```

Recommended unit tests:

```text
internal/input/input_test.go
internal/validation/validation_test.go
```

Exit criteria:

```text
input JSON has priority over duplicate CLI fields
missing optional fields can receive schema defaults
null and empty string are treated as missing
false and 0 are preserved
provider-specific mapping is not implemented in CLI
go test ./... passes
```

Verification:

```text
go test -timeout 30s ./internal/input ./internal/validation
go test -timeout 60s ./...
```

### Chapter 8: Asset IO and SQLite

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Resolve local workspace files to permanent server URLs and persist artifact mappings.
```

Implementation scope:

```text
internal/assets
SQLite schema
file path lookup
sha256 + size fallback for moved files
download mapping persistence
explicit upload hook
```

Dependency decision:

```text
Use modernc.org/sqlite already selected by Chapter 4 for the local asset database.
Use the Go standard library for path normalization, file metadata, sha256, and resolver orchestration.
Do not implement HTTP multipart upload in this chapter; expose an internal/assets.Uploader hook so Chapter 10 can wire server upload behavior only when schemas require fileResolve upload.
Keep artifact download behavior deferred to Chapter 11.
```

Primary test cases:

```text
TC-050
TC-051
TC-052
TC-053
TC-140
TC-141
TC-142
TC-143
TC-144
TC-170
TC-171
TC-172
```

Recommended unit tests:

```text
internal/assets/store_test.go
internal/assets/resolve_test.go
```

Exit criteria:

```text
path changes can recover through content identity
permanent URLs are reused
unknown local files fail unless upload is explicitly allowed
downloaded artifacts are recorded
go test ./... passes
```

Verification:

```text
go test -timeout 30s ./internal/assets ./internal/lifecycle
go test -timeout 60s ./...
```

### Chapter 9: Task Polling

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Support long-running generation tasks through server-side task query endpoints.
```

Implementation scope:

```text
internal/task
task status
task wait
task cancel
polling loop
exponential backoff with jitter
timeout handling
stderr progress
```

Primary test cases:

```text
TC-060
TC-061
TC-062
TC-063
TC-150
TC-151
TC-152
TC-153
TC-154
```

Recommended unit tests:

```text
internal/task/polling_test.go
internal/commands/task_command_test.go
```

Dependency decision:

```text
Use Cobra for task status/wait/cancel command routing.
Use the existing internal/client JSON client and internal/serverapi endpoint constants.
Use the Go standard library for timers, context cancellation, and jitter calculation.
Expose sleeper/jitter injection points for deterministic unit tests without adding test-only dependencies.
```

Exit criteria:

```text
terminal states stop polling
timeout does not cancel remote task
progress appears only on stderr
cancel calls the server endpoint
go test ./... passes
```

Verification:

```text
go test -timeout 30s ./internal/task ./internal/commands
go test -timeout 60s ./...
```

### Chapter 10: Generation Commands

Status:

```text
generic implementation completed on 2026-06-14; downstream server and concrete provider schema integration pending
```

Purpose:

```text
Wire the full generateImage/generateVoice/generateVideo path.
```

Implementation scope:

```text
internal/generation
generateImage
generateVoice
generateVideo
provider/model/type required flags
input loading
schema resolution
validation
asset resolution
POST /generations
poll or no-wait
artifact download
```

Current boundary:

```text
Implement generic generation pipeline using local schemas, normalized input, asset-db file resolution, POST /generations, --no-wait, and task polling against mockable server endpoints.
Do not implement concrete provider schema behavior in this chapter.
Do not perform real downstream server integration in this chapter.
Keep concrete schema and server联调 pending until service-side endpoints and provider/model schemas are finalized.
Artifact download remains deferred until the artifact output chapter.
```

Primary test cases:

```text
TC-070
TC-071
TC-072
TC-073
TC-160
TC-161
TC-162
```

Recommended unit tests:

```text
internal/generation/generation_test.go
internal/commands/generate_command_test.go
```

Exit criteria:

```text
server receives normalized request only
wrong schema cannot submit generation
--no-wait returns job metadata
default wait polls task and returns final artifact URLs
file fields can resolve through asset-db mappings
auth missing prevents generation before server call
real downstream server联调 waits for server endpoints and concrete schemas
go test ./... passes
```

Verification:

```text
go test -timeout 30s ./internal/generation ./internal/commands
go test -timeout 60s ./...
```

### Chapter 11: Install Script and Release Packaging

Status:

```text
completed on 2026-06-14
```

Purpose:

```text
Install the Go binary without coupling install.sh to runtime schemas.
```

Implementation scope:

```text
scripts/install.sh
platform detection
binary download
checksum verification
install under ~/.pi-pro/bin
PATH guidance
release artifact naming
```

Primary test cases:

```text
TC-090
TC-091
TC-092
TC-093
TC-094
TC-095
```

Recommended tests:

```text
shellcheck scripts/install.sh when available
script dry-run tests where feasible
go test ./...
```

Dependency decision:

```text
Use POSIX sh for broad installer portability.
Use curl for HTTP downloads and standard system checksum tools in priority order: shasum, sha256sum, openssl.
Keep JSON parsing minimal and limited to the stable /cli/version install response fields: binary.url, binary.sha256, releaseVersion.
Do not introduce jq because it is not guaranteed on a first-time install target.
```

Exit criteria:

```text
install.sh never downloads schemas
install.sh prints pi-pro init as the next step
binary install path is deterministic
checksum failure aborts install
go test ./... passes
```

Verification:

```text
go test -timeout 30s ./internal/installscript
go test -timeout 60s ./...
shellcheck scripts/install.sh not run: shellcheck is not installed in the current environment
```

### Chapter 12: MVP Integration Gate

Status:

```text
pending; wait for downstream server endpoints and concrete provider/model schemas before continuing
```

Purpose:

```text
Verify all chapters work together against mocked server endpoints.
```

Implementation scope:

```text
end-to-end CLI tests with httptest server
deferred feature guard tests
README command examples
```

Primary test cases:

```text
TC-080
TC-081
TC-082
TC-160
TC-161
TC-162
TC-180
TC-181
TC-182
TC-183
```

Recommended tests:

```text
go test ./...
mocked init -> auth -> types inspect -> generateVideo flow
```

Exit criteria:

```text
MVP happy path works with a mock server
deferred features remain unavailable
stdout/stderr contract holds across commands
architecture documents match implemented behavior
```

Current stop point:

```text
Development is intentionally paused after Chapter 11.
Chapter 0 through Chapter 11 are completed for the current CLI-only scope.
Chapter 10 contains a generic generation pipeline only; real provider schema behavior and downstream server联调 remain pending.
Chapter 12 should resume only after server endpoints, auth behavior, release metadata, init files, concrete schemas, generation submission, task polling, and artifact responses are available or mockable at the agreed contract level.
```

## Legacy Phase Mapping

The phase list below remains as module-level guidance. The chapter plan above is the execution order for implementation work.

### Phase 1: Go Project Skeleton

Goal:

```text
Create a minimal buildable Go CLI project.
```

Recommended outputs:

```text
go.mod
cmd/pi-pro/main.go
internal/config LocalVersion
internal/output
internal/apperror
project-structure-and-package-boundaries.md
```

Acceptance:

```text
pi-pro --version works
pi-pro --help works
all command responses use stdout JSON where applicable
```

Reference:

```text
cli-command-design.md
output-and-error-contract.md
architecture-review.md
```

### Phase 2: Output and Error Foundation

Goal:

```text
Implement shared JSON response and error handling before business commands.
```

Recommended outputs:

```text
internal/output
internal/apperror
exit code mapping
secret redaction helpers
```

Acceptance:

```text
success responses use ok=true
failure responses use ok=false + error.code
progress never writes to stdout
exit codes follow the contract
```

Reference:

```text
output-and-error-contract.md
```

### Phase 3: Config Paths and Built-In Server URL

Goal:

```text
Centralize config directory, built-in server URL, and local state paths.
```

Recommended outputs:

```text
internal/config
config dir resolver
~/.pi-pro path creation
PI_PRO_CONFIG_DIR override
built-in serverUrl constant
```

Acceptance:

```text
serverUrl is not user-configurable
config path can be isolated in tests
~/.pi-pro uses safe permissions
```

Reference:

```text
auth-and-config-flow.md
install-and-init-flow.md
```

### Phase 4: Server Client Base

Goal:

```text
Implement HTTP client foundation with auth injection and server error mapping.
```

Recommended outputs:

```text
internal/client
request helpers
Bearer auth injection
server error parsing
retry classifier
```

Acceptance:

```text
client receives resolved config
client does not read config directly
401/403 map to auth errors
server errors preserve serverCode/serverMessage
```

Reference:

```text
server-endpoint-contract.md
task-polling-and-server-client-flow.md
output-and-error-contract.md
```

### Phase 5: Init and Update

Goal:

```text
Implement lifecycle commands before generation commands.
```

Recommended outputs:

```text
pi-pro init
pi-pro update
version check client
init manifest client
checksum verification
atomic file writes
assets.sqlite initialization
```

Acceptance:

```text
pi-pro init creates ~/.pi-pro
schemas download into ~/.pi-pro/schemas
init is idempotent
checksum mismatch fails
pi-pro update updates only managed binaries under ~/.pi-pro/bin
```

Reference:

```text
install-and-init-flow.md
server-endpoint-contract.md
```

### Phase 6: Auth

Goal:

```text
Implement interactive username/password login and token storage.
```

Recommended outputs:

```text
pi-pro auth login
pi-pro auth logout
pi-pro auth status
internal/auth
token store in ~/.pi-pro/config.json
hidden password input
```

Acceptance:

```text
auth login prompts username then hidden password
auth token is stored with safe permissions
auth status never prints token
generate/task commands never prompt for auth
```

Reference:

```text
auth-and-config-flow.md
server-endpoint-contract.md
output-and-error-contract.md
```

### Phase 7: Schema Registry

Goal:

```text
Load schemas downloaded by pi-pro init.
```

Recommended outputs:

```text
internal/schema
LocalSchemaSource
TypeRegistry
pi-pro types list
pi-pro types inspect
schema metadata validation
```

Acceptance:

```text
schema key is provider + model + type
one behavior per schema file
metadata mismatch fails
invalid schema blocks generation
types inspect returns full schema
```

Reference:

```text
schema-contract.md
schema-registry-and-resolution-flow.md
server-endpoint-contract.md
```

### Phase 8: Validation and Normalization

Goal:

```text
Convert agent input into stable server requests.
```

Recommended outputs:

```text
internal/validation
input JSON reader
stdin reader
CLI arg merger
unknown field policy
default injection
file field hooks
```

Acceptance:

```text
input JSON > CLI arguments > schema defaults
undefined/null/empty strings omitted
false and 0 preserved
required fields enforced
schema errors stop before POST /generations
```

Reference:

```text
validation-normalization-pipeline.md
schema-contract.md
output-and-error-contract.md
```

### Phase 9: Asset IO and SQLite

Goal:

```text
Resolve local files to permanent server URLs and record downloaded artifacts.
```

Recommended outputs:

```text
internal/assets
SQLite schema
asset identity table
asset locations table
path lookup
sha256 fallback
download recording
upload client hook
```

Acceptance:

```text
file paths are not asset identity
moved files recover by sha256 + sizeBytes
asset URLs are treated as permanent
downloaded artifacts are recorded
missing mapping fails unless upload is explicitly allowed
```

Reference:

```text
asset-io-sqlite-flow.md
server-endpoint-contract.md
```

### Phase 10: Task Polling

Goal:

```text
Implement task status, wait, cancel, and shared polling loop.
```

Recommended outputs:

```text
internal/task
pi-pro task status
pi-pro task wait
pi-pro task cancel
polling backoff with jitter
timeout handling
```

Acceptance:

```text
stable states only
timeout does not cancel
--no-wait returns submitted job
progress goes to stderr
terminal states map to stable JSON
```

Reference:

```text
task-polling-and-server-client-flow.md
server-endpoint-contract.md
output-and-error-contract.md
```

### Phase 11: Generation Commands

Goal:

```text
Wire schema, validation, assets, server client, polling, and output together.
```

Recommended outputs:

```text
pi-pro generateImage
pi-pro generateVoice
pi-pro generateVideo
```

Acceptance:

```text
provider/model/type required
--input file supported
--input - stdin supported
--output and --output-dir supported
--no-wait supported
server receives normalized request only
successful artifacts can be downloaded and recorded
```

Reference:

```text
cli-command-design.md
validation-normalization-pipeline.md
task-polling-and-server-client-flow.md
asset-io-sqlite-flow.md
```

### Phase 12: install.sh

Goal:

```text
Install the Go binary safely.
```

Recommended outputs:

```text
install.sh
platform detection
binary download
checksum verification
install to ~/.pi-pro/bin/pi-pro
PATH guidance
```

Acceptance:

```text
install.sh does not download schemas
install.sh does not create runtime config beyond install dir
install.sh prints next step pi-pro init
first install can use POST /cli/version with localVersion=none
```

Reference:

```text
install-and-init-flow.md
server-endpoint-contract.md
```

## Testing Strategy

Minimum test coverage:

The detailed TDD case list is defined in [TDD Test Cases](tdd-test-cases.md).

```text
output/error JSON shape
config path resolution
init manifest path validation
checksum verification
schema metadata validation
input normalization
asset path/hash resolution
auth token storage redaction
task polling timeout/retry
server error mapping
generation command happy path with mocked server
```

## Completion Criteria

The first implementation is complete when:

```text
install.sh installs pi-pro
pi-pro init downloads schemas and initializes SQLite
pi-pro auth login stores token
pi-pro types inspect works
pi-pro generateVideo can submit and poll a mocked server task
artifact download records SQLite mapping
stdout/stderr contract is respected
all MVP deferred items remain unimplemented
```
