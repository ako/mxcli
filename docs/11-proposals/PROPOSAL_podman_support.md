---
title: Podman Support as Docker Alternative
status: partial
date: 2026-03-27
---

# Proposal: Podman Support as Docker Alternative

**Status:** Implemented (Phase 1 & 2)
**Date:** 2026-03-27

## Motivation

Docker Desktop requires a paid subscription for larger organizations. Some users prefer or are required to use [Podman](https://podman.io/) instead — due to licensing, corporate policy, or preference for daemonless rootless containers.

The goal is a **unified Podman-only experience** — Podman on the host running the devcontainer, and Podman inside the devcontainer running the Mendix app + PostgreSQL stack. No Docker Desktop anywhere in the chain.

> **Note:** Users who run Podman on the host but are fine with Docker inside the devcontainer can already do that today — the devcontainers spec supports Podman as the outer engine, and the existing `docker-in-docker:2` feature works inside. This proposal goes further: Podman all the way down.

## Current State

All container invocations go through 6 call sites in Go, all using `exec.Command("docker", ...)`:

| Call site | File | Usage |
|-----------|------|-------|
| `runCompose()` | `cmd/mxcli/docker/runtime.go:281` | All compose operations (up, down, logs, shell) |
| `runComposeOutput()` | `cmd/mxcli/docker/runtime.go:264` | Compose with captured output |
| `status()` | `cmd/mxcli/docker/runtime.go:208` | `docker compose ps --format json` |
| `CallM2EE()` | `cmd/mxcli/docker/m2ee.go:166` | `docker compose exec` for admin API |
| `testrunner` (2 sites) | `cmd/mxcli/testrunner/runner.go:313,445` | Compose for test execution |

The devcontainer uses `ghcr.io/devcontainers/features/docker-in-docker:2`, and `mxcli init` generates devcontainer configs with the same feature.

## Design

### Principle: single abstraction point

Introduce a `containerCLI()` function that returns the container runtime binary name. All call sites replace the hardcoded `"docker"` with this function. No other code changes are needed because:

- **Podman v4.7+** ships `podman compose` natively with full Docker Compose v2 compatibility
- Podman accepts the same CLI flags as Docker for the commands we use (`compose up/down/logs/exec/ps`)

### Runtime detection

```go
// containerCLI returns the container runtime binary ("docker" or "podman").
// Resolution order:
//   1. MXCLI_CONTAINER_CLI env var (explicit override)
//   2. "docker" if available on path
//   3. "podman" if available on path
//   4. "docker" as fallback (will fail with a clear error at exec time)
func containerCLI() string {
    if cli := os.Getenv("MXCLI_CONTAINER_CLI"); cli != "" {
        return cli
    }
    if _, err := exec.LookPath("docker"); err == nil {
        return "docker"
    }
    if _, err := exec.LookPath("podman"); err == nil {
        return "podman"
    }
    return "docker"
}
```

Docker remains the default for backwards compatibility. Users with only Podman installed get automatic detection. The env var provides an escape hatch.

### Changes by area

#### 1. Container runtime calls (Go code)

**File: `cmd/mxcli/docker/runtime.go`** — new `containerCLI()` function, update 3 call sites:

```go
// before
cmd := exec.Command("docker", append([]string{"compose"}, args...)...)

// after
cmd := exec.Command(containerCLI(), append([]string{"compose"}, args...)...)
```

**File: `cmd/mxcli/docker/m2ee.go`** — update 1 call site (same pattern).

**File: `cmd/mxcli/testrunner/runner.go`** — update 2 call sites (same pattern).

Total: ~6 one-line changes after adding the `containerCLI()` function.

#### 2. Compose compatibility

Podman Compose supports Docker Compose v2 format, which is what `cmd/mxcli/docker/templates/docker-compose.yml` already uses. No changes needed to the compose template.

One consideration: `docker compose ps --format json` output differs slightly between Docker and Podman. The `status()` function in `runtime.go` parses this JSON. We need to verify and potentially handle both output formats.

#### 3. Devcontainer — Podman-in-Podman

Add a parallel devcontainer config for Podman users. The outer container is run by Podman on the host; the inner containers (Mendix app, PostgreSQL) are run by Podman inside the devcontainer.

Create `.devcontainer/podman/devcontainer.json`:

```jsonc
{
  "name": "ModelSDKGo (Podman)",
  "build": { "dockerfile": "../Dockerfile" },
  "features": {
    // Installs Podman inside the container for Podman-in-Podman
    "ghcr.io/devcontainers/features/podman-in-podman:1": {}
  },
  "forwardPorts": [8080, 8090, 5432],
  "containerEnv": {
    "MXCLI_CONTAINER_CLI": "podman"
  }
}
```

The existing `.devcontainer/devcontainer.json` (Docker-in-Docker) remains the default. Users select the Podman variant when opening in VS Code via the "Reopen in Container" picker, which lists both configurations.

The `containerEnv` setting ensures mxcli uses `podman compose` for all inner container operations without needing additional flags.

#### 4. `mxcli init` — generated devcontainer for user projects

Update `cmd/mxcli/init.go` and `cmd/mxcli/tool_templates.go` to accept a `--container-runtime podman` flag (default: `docker`). When set to `podman`:

- Use `ghcr.io/devcontainers/features/podman-in-podman:1` instead of `docker-in-docker:2`
- Set `MXCLI_CONTAINER_CLI=podman` in `containerEnv`
- Add a note in the generated CLAUDE.md about the Podman setup

#### 5. Documentation

- Add a section to `docs-site/src/tools/devcontainer.md` explaining the Podman setup
- Update `.claude/skills/mendix/docker-workflow.md` to mention Podman compatibility
- Add `MXCLI_CONTAINER_CLI` to the environment variable reference

## Known Podman Differences to Handle

| Area | Docker | Podman | Impact |
|------|--------|--------|--------|
| Compose subcommand | `docker compose` (built-in v2) | `podman compose` (v4.7+) | Require Podman 4.7+; older `podman-compose` (Python) has Compose v2 gaps |
| `ps --format json` | Array of objects | May differ in field names | Test and normalize in `status()` |
| Rootless networking | N/A | Rootless containers can't bind to ports <1024 | Default ports (8080, 8090, 5432) are all >1024 — no issue |
| Named volumes | `docker volume` | `podman volume` | Compose handles this transparently |
| Health checks | `HEALTHCHECK` in Dockerfile | Supported since Podman 3.0 | No issue |

## Implementation Plan

### Phase 1 — Runtime abstraction + devcontainer (enables full Podman-in-Podman)

1. Add `containerCLI()` to `cmd/mxcli/docker/runtime.go`
2. Replace all 6 hardcoded `"docker"` exec calls
3. Add `--container-runtime` flag to `mxcli docker` root command (sets env var for subcommands)
4. Add `.devcontainer/podman/devcontainer.json` for this repo
5. Test with Podman 4.7+ on Linux — full `mxcli docker run` workflow

### Phase 2 — `mxcli init` + documentation

6. Update `mxcli init` to support `--container-runtime podman`
7. Documentation updates (devcontainer guide, docker-workflow skill, env var reference)

### Phase 3 — Validation across platforms

8. Verify `compose ps` JSON parsing works with both runtimes
9. Test `mxcli test` with Podman
10. Test Podman-in-Podman devcontainer on macOS (via Podman Machine) and Linux

## Scope Exclusions

- **Podman on Windows**: Podman Desktop on Windows uses WSL2 (like Docker Desktop). It should work but is not a priority for initial testing.
- **Podman remote**: Podman supports a remote client mode. Out of scope — the local socket is sufficient.
- **Kubernetes/pods**: Podman can create Kubernetes YAML from containers. Interesting but out of scope.
- **`podman-compose` (Python)**: We target `podman compose` (Go, built into Podman 4.7+). The older Python-based `podman-compose` has compatibility gaps with Compose v2 features.

## Effort Estimate

- Phase 1: Small — ~50 lines of Go + devcontainer JSON
- Phase 2: Small — init flag + docs
- Phase 3: Medium — testing across runtimes and platforms
