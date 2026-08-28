# Dev Container Setup

mxcli supports development inside VS Code Dev Containers, providing a consistent, pre-configured development environment with all tools installed.

## What Is a Dev Container?

A dev container is a Docker-based development environment defined by a `devcontainer.json` configuration file. When you open a project in VS Code with the Dev Containers extension, it builds and starts a container with all development tools pre-installed.

## mxcli in Dev Containers

When running inside a dev container, mxcli tools and the `mx` binary are available at known paths:

| Tool | Path |
|------|------|
| mxcli | Available on PATH |
| mx binary | `~/.mxcli/mxbuild/{version}/modeler/mx` |
| mxbuild | `~/.mxcli/mxbuild/{version}/` |

## Setting Up mxbuild

To download the correct mxbuild version for your project:

```bash
mxcli setup mxbuild -p app.mpr
```

This downloads mxbuild to `~/.mxcli/mxbuild/{version}/` and makes the `mx` tool available for validation and project creation.

## Validating Projects

```bash
# Find the mx binary
MX=~/.mxcli/mxbuild/<version>/modeler/mx

# Check a project
$MX check /path/to/app.mpr

# Create a fresh project for testing
cd /tmp/test-workspace
$MX create-project
```

## Project Initialization

Initialize mxcli for your project inside the dev container:

```bash
# Initialize with Claude Code support
mxcli init -p app.mpr

# This creates:
# - .claude/settings.json
# - .claude/commands/
# - .claude/lint-rules/
# - .ai-context/skills/
# - CLAUDE.md
# - VS Code MDL extension (auto-installed)
```

## Container Runtime (Docker or Podman)

For `mxcli docker build`, `mxcli docker run`, and `mxcli test` (which require a container runtime), the dev container must have Docker-in-Docker or Podman-in-Podman support enabled.

### Docker-in-Docker (default)

```json
{
  "features": {
    "ghcr.io/devcontainers/features/docker-in-docker:2": {}
  }
}
```

### Podman-in-Podman

For organizations that cannot use Docker Desktop due to licensing:

```json
{
  "features": {
    "ghcr.io/devcontainers/features/podman-in-podman:1": {}
  },
  "containerEnv": {
    "MXCLI_CONTAINER_CLI": "podman"
  }
}
```

When running `mxcli init`, use `--container-runtime podman` to generate this configuration automatically. Requires Podman 4.7+ (ships `podman compose` with Docker Compose V2 compatibility).

## Typical Dev Container Workflow

1. Open the project in VS Code
2. VS Code prompts to reopen in dev container
3. Inside the container, run `mxcli setup mxbuild -p app.mpr`
4. Run `mxcli init` to set up AI assistant integration
5. Use `mxcli` commands as normal (REPL, exec, lint, test, etc.)
