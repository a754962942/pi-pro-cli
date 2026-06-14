# Asset IO and SQLite Flow

Created: 2026-06-13

## Context

PI-Pro CLI needs IO capabilities because generation requests can reference local image, video, and audio files, while server and provider workflows operate on URLs.

Most local media files are expected to be artifacts previously returned by the server and downloaded by the CLI. Therefore, the CLI should maintain a local SQLite asset database that maps local file paths back to their server-side URLs.

Server-returned artifact URLs are expected to be permanent for the current product design. The CLI does not need an `assetId -> refresh URL` flow in the first implementation.

## Design Goals

- Resolve local file paths to server URLs without re-uploading known artifacts.
- Support explicit uploads for user-provided local assets.
- Keep file handling deterministic for agent calls.
- Preserve machine-readable output.
- Avoid hiding IO failures behind successful task responses.

## Business Rules

Server artifact URLs are permanent.

The CLI should store returned URLs directly and reuse them for future requests. If a URL later fails unexpectedly, the first implementation should surface a normal server or asset error rather than attempting URL refresh.

No `assetId -> refresh URL` endpoint is required.

## Local Asset Database

The CLI should maintain a SQLite database under the user config directory.

Recommended location:

```text
~/.pi-pro/assets.sqlite
```

The database should store local file metadata and source URL mappings.

File paths must not be treated as the asset identity. Paths are mutable because users and agents can move files. The stable identity should be a server asset id when available, with content hash as the local fallback.

Recommended table shape:

```text
assets
- id
- serverAssetId
- sourceUrl
- mime
- sizeBytes
- sha256
- provider
- model
- type
- jobId
- artifactKind
- createdAt
- updatedAt

asset_locations
- id
- assetId
- filePath
- lastSeenAt
- exists
- createdAt
- updatedAt
```

`filePath` should be stored as an absolute normalized path in `asset_locations`.

`sha256` is recommended for move detection, verification, and deduplication. For large media files, the CLI can compute it lazily only when path lookup fails or when recording a downloaded/uploaded artifact.

## File Resolution Flow

Default file fields use:

```text
fileResolve: asset-db
```

Resolution flow:

```text
1. Receive local file path from normalized input.
2. Resolve to absolute path.
3. Check that the file exists.
4. Query SQLite by absolute filePath in `asset_locations`.
5. If a matching asset exists, replace the input field with an asset reference.
6. If path lookup misses, compute file metadata and `sha256`.
7. Query SQLite by `sha256` and `sizeBytes`.
8. If a matching asset exists, add the new filePath to `asset_locations` and replace the input field with that asset reference.
9. If no asset exists, fail unless schema or command explicitly allows upload.
```

Recommended normalized asset reference:

```json
{
  "source": "asset-db",
  "assetId": "asset_123",
  "path": "/absolute/path/to/image.png",
  "url": "https://server.example/artifacts/image.png",
  "mime": "image/png"
}
```

Recommended error code when no mapping exists:

```text
ASSET_URL_NOT_FOUND
```

## Moved File Recovery

If a file has moved, path lookup will miss. The CLI should then fall back to content identity:

```text
1. Confirm the new path exists.
2. Read size and basic metadata.
3. Compute sha256.
4. Look up assets by sha256 and sizeBytes.
5. If exactly one asset matches, register the new path as another asset location.
6. If multiple assets match, choose the one with the same serverAssetId when available, otherwise fail as ambiguous.
7. If no asset matches, require explicit upload or fail with ASSET_URL_NOT_FOUND.
```

This makes path moves recoverable without re-uploading.

Recommended ambiguity error code:

```text
ASSET_MATCH_AMBIGUOUS
```

The CLI should not silently bind a moved path to a URL when the content identity is ambiguous.

## Explicit Upload Flow

For user-provided local files that are not known server artifacts, the CLI should support explicit upload.

Upload can be enabled by schema:

```text
fileResolve: upload
fileResolve: asset-db-or-upload
```

or by a future explicit command option:

```text
--upload
```

Upload flow:

```text
1. Resolve local path to an absolute path.
2. Validate file existence and media type.
3. Upload file to the server.
4. Server returns sourceUrl and metadata.
5. CLI records serverAssetId/sourceUrl/content hash and filePath location in SQLite.
6. CLI replaces the input file path with an asset reference.
```

Recommended normalized uploaded reference:

```json
{
  "source": "upload",
  "assetId": "asset_456",
  "path": "/absolute/path/to/local.png",
  "url": "https://server.example/uploads/local.png",
  "mime": "image/png"
}
```

## Download Flow

When a generation task succeeds, the server returns one or more artifact URLs.

CLI behavior:

```text
1. If neither --output nor --output-dir is provided, return URLs only.
2. If --output is provided, download a single artifact to that path.
3. If --output-dir is provided, download all artifacts into that directory.
4. Refuse to overwrite existing files unless --overwrite is provided.
5. Record each downloaded artifact in SQLite with serverAssetId/sourceUrl/content hash and filePath location.
6. Return final JSON with both url and local path when downloaded.
```

Recommended output artifact shape:

```json
{
  "url": "https://server.example/artifacts/video.mp4",
  "path": "/absolute/path/to/outputs/video.mp4",
  "mime": "video/mp4",
  "kind": "video"
}
```

## Output Path Rules

`--output` is valid when exactly one artifact is expected or returned.

`--output-dir` is recommended for multiple artifacts.

If multiple artifacts are returned with `--output`, the CLI should fail with:

```text
OUTPUT_PATH_AMBIGUOUS
```

Default naming under `--output-dir` should be deterministic:

```text
<jobId>-<index><extension>
```

Example:

```text
job_123-1.mp4
job_123-2.mp4
```

## Download Failure Semantics

Generation success and local download success are separate outcomes.

If the server task succeeds but local download fails, the CLI should return `ok: false` with a specific IO error, while preserving the remote artifact URLs in error details.

Recommended error shape:

```json
{
  "ok": false,
  "error": {
    "code": "ARTIFACT_DOWNLOAD_FAILED",
    "message": "Task succeeded, but one or more artifacts could not be downloaded.",
    "details": [
      {
        "url": "https://server.example/artifacts/video.mp4",
        "path": "/absolute/path/to/video.mp4"
      }
    ]
  }
}
```

This keeps agent behavior explicit: a completed generation is not silently treated as a successful local IO operation.

## SQLite Update Rules

The CLI should write SQLite records when:

```text
an uploaded file receives a server URL
a remote artifact is downloaded locally
an existing local file is verified against a known source URL
a moved file is matched by sha256 and registered at a new path
```

The CLI should not write a mapping when:

```text
download fails
upload fails
the local path does not exist
the server response lacks a durable URL
```

## Error Codes

Recommended IO and asset error codes:

```text
INVALID_FILE
ASSET_URL_NOT_FOUND
ASSET_MATCH_AMBIGUOUS
UPLOAD_FAILED
DOWNLOAD_FAILED
ARTIFACT_DOWNLOAD_FAILED
OUTPUT_PATH_AMBIGUOUS
OUTPUT_PATH_EXISTS
ASSET_DB_OPEN_FAILED
ASSET_DB_WRITE_FAILED
```

## Design Decisions

- Use SQLite as the local asset mapping source of truth.
- Treat file paths as mutable locations, not asset identity.
- Store absolute normalized file paths in an asset locations table.
- Prefer server asset id as stable identity when available.
- Use sha256 and sizeBytes to recover mappings after file moves.
- Resolve known files through path lookup first, then hash lookup.
- Upload only when schema or a future explicit option allows it.
- Download artifacts only when `--output` or `--output-dir` is provided.
- Record downloaded artifacts in SQLite for future reuse.
- Treat task success and local IO success as separate outcomes.
