<div align="center">

# PI-Pro CLI

**The command-line companion for local AI filmmaking workflows.**

[![Utopai Studios](https://img.shields.io/badge/Utopai_Studios-4285F4?style=flat)](https://www.utopaistudios.com/)
[![Discord][discord-shield]][discord-url]
[![Follow @UtopaiStudios](https://img.shields.io/badge/Follow-%40UtopaiStudios-000000?style=flat&logo=x&logoColor=white)](https://x.com/UtopaiStudios)
<br />
[![Claude Code supported](https://img.shields.io/badge/Claude_Code-supported-2EA44F?style=flat&labelColor=D97757&logo=anthropic&logoColor=white)][claude-code-url]
[![Codex supported](https://img.shields.io/badge/Codex-supported-2EA44F?style=flat&labelColor=111111)][codex-url]

</div>

## What's PI-Pro CLI?

PI-Pro CLI is a lightweight command-line project for operating local AI filmmaking workflows from coding agents and terminal sessions.

It follows the same local-first direction as PAI-Pro:

- **Agent-driven workflows** for Claude Code, Codex, and other coding-agent environments.
- **Scriptable commands** that can become stable automation surfaces for generation, project setup, and asset operations.
- **Local project control** so prompts, files, and generated artifacts remain inspectable in the workspace.
- **PAI-Pro compatibility** as the CLI layer evolves alongside the broader filmmaking stack.

## Status

PI-Pro CLI now has the main command surface implemented and can connect to `pi-pro-server` for initialization, auth, capability discovery, remote schema inspection, generation submission, task polling, and cancellation.

Current verified paths:

- `CLI -> Server -> Grok -> MinIO`
- `CLI -> Server -> Vidu -> MinIO`
- `CLI -> Server -> Qiling text-to-image/image-to-image -> MinIO`
- `Server -> Seedance-2.0 / Seedance-2.0-fast -> MinIO` through server-side direct provider gates

Implemented generation safeguards include `generate* --dry-run`, capability preflight before `/generations`, and local artifact download to `--output` / `--output-dir` after a succeeded waited task. Still planned: full CLI-driven Seedance verification and release packaging validation.

## Release Notes

- [Release preflight](docs/release-preflight.md)
- [Current development status](docs/architecture/current-development-status.md)
- [Server responsibilities](docs/architecture/server-responsibilities.md)
- [Schema contract](docs/architecture/schema-contract.md)
- [Schema parameter contract](docs/architecture/schema-parameter-contract.md)

Historical design notes remain under `docs/architecture/`. They are useful for implementation context, but they may contain old placeholder hosts, version examples, or planned behavior from earlier design phases.

## Quick start

Install the latest CLI from the configured release server:

```sh
curl -fsSL https://raw.githubusercontent.com/a754962942/pi-pro-cli/main/scripts/install.sh | sh
```

Clone the repository:

```sh
git clone git@github.com:a754962942/pi-pro-cli.git
cd pi-pro-cli
```

Run the local tests:

```sh
go test ./... -count=1
```

Initialize CLI runtime files from the configured server:

```sh
pi-pro init
```

Log in:

```sh
pi-pro auth login
```

For CI or agents, pre-seed an isolated config directory instead of using interactive login:

```sh
export PI_PRO_CONFIG_DIR="$PWD/.pi-pro-ci"
mkdir -p "$PI_PRO_CONFIG_DIR"
printf '{"authToken":"%s","username":"%s"}\n' "$PI_PRO_AUTH_TOKEN" "$PI_PRO_USERNAME" > "$PI_PRO_CONFIG_DIR/config.json"
chmod 600 "$PI_PRO_CONFIG_DIR/config.json"
```

Inspect available generation capabilities and schemas:

```sh
pi-pro types list
pi-pro types inspect --type image-to-video
pi-pro schema --brief
pi-pro schema inspect --provider grok --model grok-video-1.5 --type image-to-video
```

Submit a video generation request:

```sh
pi-pro generateVideo \
  --provider grok \
  --model grok-video-1.5 \
  --type image-to-video \
  --input request.json
```

Poll or cancel a task:

```sh
pi-pro task status <jobId>
pi-pro task wait <jobId>
pi-pro task cancel <jobId>
```

`types` and `schema` prefer remote server data and fall back to local downloaded schemas when possible. Provider-specific request mapping is intentionally server-side.

Use your coding agent to inspect the project and continue setup from the current repository state:

> Read `README.md`, inspect the project structure, identify the available CLI entry points, install only the dependencies required by the repository, and verify the CLI with its local test or help command before making changes.

## Resources

- [Discord][discord-url] - questions, ideas, support, and show & tell
- [PAI-Pro](https://github.com/Utopai-Research/pai-pro) - local AI filmmaking workspace
- [Claude Code][claude-code-url] - coding-agent environment
- [Codex][codex-url] - OpenAI coding-agent environment
- [Issues](https://github.com/a754962942/pi-pro-cli/issues) - bug reports and feature requests

## License

PI-Pro CLI is released under the [PAI PRO Sustainable Use License](LICENSE.md), which permits personal use, non-commercial research, and internal business use. Commercial use of PAI-Pro Skills or enterprise-designated source code/Skills requires an explicit agreement; [enterprise licenses](mailto:enterprise@utopaistudios.com) are available.

[discord-shield]: https://img.shields.io/badge/Discord-Join-green?style=flat&logo=discord&logoColor=white
[discord-url]: https://discord.gg/CfjRGGwK
[claude-code-url]: https://code.claude.com/docs/en/overview
[codex-url]: https://developers.openai.com/codex/cli
