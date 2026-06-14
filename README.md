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

This repository is currently an initial CLI workspace. Implementation details, installation commands, and command references will be documented here as the CLI surface lands.

## Architecture Notes

- [Development roadmap](docs/architecture/development-roadmap.md)
- [Project structure and package boundaries](docs/architecture/project-structure-and-package-boundaries.md)
- [Version comparison flow](docs/architecture/version-comparison-flow.md)
- [TDD test cases](docs/architecture/tdd-test-cases.md)
- [CLI command design](docs/architecture/cli-command-design.md)
- [Schema contract](docs/architecture/schema-contract.md)
- [Schema registry and resolution](docs/architecture/schema-registry-and-resolution-flow.md)
- [Install and init flow](docs/architecture/install-and-init-flow.md)
- [Auth and config flow](docs/architecture/auth-and-config-flow.md)
- [Validation and normalization pipeline](docs/architecture/validation-normalization-pipeline.md)
- [Output and error contract](docs/architecture/output-and-error-contract.md)
- [Asset IO and SQLite flow](docs/architecture/asset-io-sqlite-flow.md)
- [Task polling and server client flow](docs/architecture/task-polling-and-server-client-flow.md)
- [Server responsibilities](docs/architecture/server-responsibilities.md)
- [Server endpoint contract](docs/architecture/server-endpoint-contract.md)
- [Architecture review](docs/architecture/architecture-review.md)

## Quick start

Clone the repository:

```sh
git clone git@github.com:a754962942/pi-pro-cli.git
cd pi-pro-cli
```

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
