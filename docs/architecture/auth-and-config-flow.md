# Auth and Config Flow

Created: 2026-06-13

## Context

PI-Pro CLI is designed for direct agent invocation. Every server-facing operation needs predictable authentication and configuration resolution, including schema lookup, asset upload/download, generation submission, task polling, and task cancellation.

The initial auth flow should support username/password login from the CLI. The server exchanges credentials for an auth token, and the CLI stores that token for later server requests.

Generation, task, schema, upload, and download commands should be non-interactive. `pi-pro auth login` is the intentional exception: it is an initialization command and may prompt the user for credentials.

Browser-based login with external accounts is a future extension and should not be part of the first implementation.

The first implementation is for internal test users. After username/password login succeeds, the server treats the user as authorized for all CLI capabilities. The CLI does not need to model plans, credits, quotas, scopes, or permission checks.

## Design Goals

- Keep auth resolution deterministic.
- Avoid interactive prompts during generation and task commands.
- Support command-scoped overrides for agents.
- Store persistent user configuration locally.
- Keep secrets out of project files by default.
- Emit machine-readable errors when auth is missing or invalid.

## Business Rules

The first implementation targets internal test users.

Server-side auth behavior:

```text
valid username/password -> authToken
authToken -> full access to CLI-backed generation capabilities
```

The CLI should not implement:

```text
credit checks
quota display
plan display
scope negotiation
permission prompts
```

Provider quota or service failures may still be returned by the server as task/server errors, but they are not user-account quota features in the first implementation.

## Config Sources

Recommended config sources:

```text
CLI arguments
environment variables
user config
defaults
```

Recommended precedence:

```text
CLI arguments > environment variables > user config > built-in defaults
```

This differs from generation input precedence. Auth/config values are operational controls, so explicit CLI flags should override all other sources.

For the first implementation, `serverUrl` is not user-configurable. It should come from a CLI built-in default written into the package configuration at build/release time.

## Supported Values

Minimum required config values:

```text
authToken
```

Optional future values:

```text
profile
defaultProvider
defaultModel
schemaCacheTtlSeconds
assetDbPath
outputDir
```

## CLI Arguments

Recommended common auth/config options:

```text
--auth-token <token>
--profile <name>
```

`--auth-token` should be accepted for automation, but users should prefer environment variables or `pi-pro auth login` for persistent use.

## Environment Variables

Recommended environment variables:

```text
PI_PRO_AUTH_TOKEN
PI_PRO_PROFILE
PI_PRO_CONFIG_DIR
PI_PRO_ASSET_DB_PATH
```

`PI_PRO_CONFIG_DIR` allows CI, tests, and agents to isolate config from the user's real home directory.

## Config File Locations

User config:

```text
~/.pi-pro/config.json
```

Project config is deferred for the first implementation.

```json
{
  "defaultProvider": "seeddance",
  "defaultModel": "v1"
}
```

User config may store credentials:

```json
{
  "authToken": "sk-pipro-xxxxxxxxxxxxxxxxxxxxxxxx",
  "username": "user@example.com"
}
```

The token should use an opaque API key format similar to popular LLM provider tokens:

```text
sk-pipro-xxxxxxxxxxxxxxxxxxxxxxxx
```

The CLI should store and send the token only. It must not parse token payloads, infer expiration, infer permissions, or inspect scopes. Token validation, expiration policy, permission checks, rotation, and revocation belong to the server.

## Auth Commands

Recommended command surface:

```sh
pi-pro auth login
pi-pro auth logout
pi-pro auth status
```

`auth login` prompts for username first, then prompts for password with hidden input. It sends username/password to the server, receives an auth token, and writes user config.

Recommended prompt flow:

```text
Username: user@example.com
Password: ********
```

Username and password should not be exposed as public CLI flags. This avoids shell history leaks and keeps the login UX simple.

`auth logout` removes the stored auth token for the active profile or default config.

`auth status` should not print secrets. It should only report whether auth is configured.

Example status output:

```json
{
  "ok": true,
  "authenticated": true,
  "username": "user@example.com",
  "source": {
    "authToken": "user-config"
  }
}
```

## Login Flow

```text
1. Read built-in serverUrl from CLI package configuration.
2. Prompt for username.
3. Prompt for password with hidden input.
4. POST credentials to the server login endpoint.
5. Receive authToken.
6. Store authToken and username in user config.
7. Return a sanitized success JSON response.
```

Recommended login endpoint:

```text
POST /auth/login
```

Recommended request:

```json
{
  "username": "user@example.com",
  "password": "secret"
}
```

Recommended response:

```json
{
  "authToken": "sk-pipro-xxxxxxxxxxxxxxxxxxxxxxxx",
  "user": {
    "id": "user_123",
    "username": "user@example.com"
  }
}
```

Recommended sanitized CLI output:

```json
{
  "ok": true,
  "authenticated": true,
  "username": "user@example.com"
}
```

## Non-Interactive Behavior

Generation and task commands must not prompt for missing credentials.

If auth is missing, return a structured error:

```json
{
  "ok": false,
  "error": {
    "code": "AUTH_REQUIRED",
    "message": "Missing auth token. Provide --auth-token, PI_PRO_AUTH_TOKEN, or run `pi-pro auth login`."
  }
}
```

If the server rejects credentials:

```json
{
  "ok": false,
  "error": {
    "code": "AUTH_INVALID",
    "message": "The configured auth token was rejected by the server."
  }
}
```

## Secret Storage

Initial implementation can store the auth token in `~/.pi-pro/config.json` with restrictive file permissions.

Recommended permissions:

```text
directory: 0700
config file: 0600
```

Future implementations may support OS keychain storage. The command and resolver interfaces should not depend on the storage mechanism.

Recommended abstraction:

```text
ConfigResolver
- resolve()

ConfigStore
- readUserConfig()
- writeUserConfig()
- readProjectConfig()

SecretStore
- getAuthToken(profile)
- setAuthToken(profile, authToken)
- deleteAuthToken(profile)
```

## Profile Policy

Profiles are useful, but they add complexity to every command.

Recommended first implementation:

```text
single default profile
```

Keep `--profile` and `PI_PRO_PROFILE` as reserved future-facing inputs, but do not require profile support for the MVP unless multiple server accounts become a near-term requirement.

## Server URL Policy

The first implementation should use a built-in `serverUrl`. Users should not need to configure it with command flags, environment variables, project config, or user config.

The built-in `serverUrl` should be validated during development and release packaging.

Future server URL override support can add validation rules:

```text
must be http or https
must not be empty
must not include query string
trim trailing slash for canonical storage
```

## Config Resolution Flow

```text
1. Read CLI auth/config flags.
2. Read environment variables.
3. Read user config from ~/.pi-pro/config.json.
4. Merge using auth/config precedence.
5. Attach built-in serverUrl.
6. Resolve authToken.
7. Return resolved config with source metadata.
```

Resolved config shape:

```json
{
  "serverUrl": "https://api.example.com",
  "authToken": "sk-pipro-xxxxxxxxxxxxxxxxxxxxxxxx",
  "source": {
    "serverUrl": "built-in",
    "authToken": "env"
  }
}
```

The CLI should never include raw `authToken` or password in normal command output.

## Server Request Usage

The resolved auth token should be sent as an authorization header:

```text
Authorization: Bearer <authToken>
```

The CLI should avoid passing credentials in URLs or request bodies unless a future server contract explicitly requires it.

## Error Codes

Recommended auth/config error codes:

```text
AUTH_REQUIRED
AUTH_INVALID
AUTH_EXPIRED
CONFIG_READ_FAILED
CONFIG_WRITE_FAILED
INVALID_SERVER_URL
SECRET_STORE_FAILED
LOGIN_FAILED
PASSWORD_REQUIRED
```

## Future Browser Login

Browser-based external account login is intentionally deferred.

Future shape may look like:

```sh
pi-pro auth login --browser
```

Possible flow:

```text
1. CLI asks server for a device/browser login session.
2. CLI opens or prints a browser URL.
3. User completes external account login in the browser.
4. CLI polls the server for completion.
5. Server returns an auth token.
6. CLI stores the token using the same SecretStore path as username/password login.
```

This should reuse the same stored `authToken` output contract, so generation and task commands do not need to know which login method produced the token.

## YAGNI Filter

Keep for the first implementation:

```text
interactive username/password login
authToken storage
PI_PRO_AUTH_TOKEN override
built-in serverUrl
auth status/logout
Authorization: Bearer
single default profile
```

Defer until required:

```text
browser login
external OAuth provider details
username/password public flags
non-interactive credential login flags
serverUrl override flags
serverUrl environment variable
project-level server config
multi-profile account switching
refresh token rotation
OS keychain integration
device-code login
team/org selection
fine-grained token scopes
automatic token renewal
```

Rationale:

```text
The first implementation only needs a reliable way for the CLI to obtain and reuse a server auth token. Everything else can be added behind the same AuthStore/SecretStore boundary later.
```

## Design Decisions

- Use CLI args, env vars, user config, and built-in defaults as first-implementation config sources.
- Use `CLI arguments > environment variables > user config > built-in defaults` precedence for first-implementation auth/config.
- Do not prompt during generation, task, schema, upload, or download commands.
- Support interactive username/password login for the initial implementation.
- Do not expose username/password public flags.
- Store the resulting auth token in `~/.pi-pro/config.json` for the initial implementation.
- Use a built-in server URL for the first implementation.
- Keep future project config secret-free by default.
- Reserve profile support but start with a single default profile.
- Send credentials through `Authorization: Bearer`.
- Defer browser/external-account login until the server login contract requires it.
