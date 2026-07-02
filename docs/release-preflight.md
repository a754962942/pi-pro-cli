# PI-Pro CLI Release Preflight

Updated: 2026-07-01 CST

## Conclusion

Do not push the CLI as an operations-ready release yet. The command surface and remote flow work, but the first GitHub release workflow run still needs to be verified after push.

## Verified

- `go test ./... -count=1` passed.
- Local release-style build passed with injected values:
  `LocalVersion=0.1.0`, `BuiltInServerURL=https://api.pi-pro.org`.
- `scripts/build-release.sh` self-checks the host-platform binary so the injected `LocalVersion` matches the generated manifest `releaseVersion`.
- Managed update behavior is covered by lifecycle/updater tests:
  managed install replacement, unmanaged install rejection, checksum mismatch handling, and Windows helper state.
- GitHub release workflow is configured to test, build release artifacts, publish workflow artifacts, and create or update `v*` GitHub Releases.
- Remote smoke already passed for:
  - `pi-pro init`
  - `auth login`
  - `types list`
  - `schema inspect`
  - `generateImage --dry-run`
  - real `generateImage` with artifact download through `assets.pi-pro.org`

## Known Excluded Gap

- First tag-driven GitHub Actions release run is not verified yet.
- Server still needs an operations step to consume the generated GitHub Release manifest and artifact tarball.
- Automation auth is documented as a pre-seeded `PI_PRO_CONFIG_DIR/config.json`; no separate non-interactive login command is required yet.

## Must Fix Before Operations

1. Review and commit the stabilized diff before release.
   - The CLI repo has broad but intentional changes across commands, lifecycle, generation, assets, docs, tests, and release scripts.
   - Commit only this intended feature set before any push.

2. Keep local runtime state out of source control.
   - `PI_PRO_CONFIG_DIR` and local `assets.sqlite` are correctly designed as runtime state.
   - Keep `.cache/`, `dist/`, `bin/`, and generated config out of commits.

## Not Blockers

- Voice generation is explicitly unsupported; do not treat `generateVoice` as a release requirement.
- Local schema fallback can remain as a safety path because remote capability/schema APIs are already preferred.
