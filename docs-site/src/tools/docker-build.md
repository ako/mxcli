# mxcli docker build

The `mxcli docker build` command builds a Mendix application using mxbuild in Docker, including support for PAD (Platform-Agnostic Deployment) patching.

## Usage

```bash
mxcli docker build -p app.mpr
```

## What It Does

1. **Downloads mxbuild** -- Automatically downloads the correct mxbuild version for your project if not already available
2. **Runs mxbuild** -- Executes the Mendix build toolchain in a Docker container
3. **PAD patching** -- Applies Platform-Agnostic Deployment patches (Phase 1) to the build output
4. **Produces artifact** -- Generates a deployable Mendix deployment package (MDA file)

## The Project Is Not Modified

`mx update-widgets`, `mx check` and MxBuild all write into the project they are
given: update-widgets rewrites the model (an MPRv1 `.mpr` in place, an MPRv2
project into MPRv1), mx check compiles the theme into `theme-cache/` and
`deployment/sass/`, and MxBuild regenerates the `javasource/` proxies, the
`.launch`, `.classpath` and `.project` files and all of `deployment/`.

So `docker build` copies the project to a temporary directory (in `$TMPDIR`)
and runs all three there. Widget definitions are normalised on that copy, so the
build uses the normalised model while the project keeps its own; run
`mxcli fix widgets` to apply the normalisation to the project. (So the build's
pre-build check does not report a stale widget definition; `mxcli docker check`,
which checks the project as stored, does.) The only thing
written is the output directory (`.docker/build/` by default, or `-o`). Build
output, caches and VCS folders (`deployment/`, `.docker/`, `.git/`,
`node_modules/`, ...) are not copied.

## PAD Patching

PAD (Platform-Agnostic Deployment) patching modifies the build output to be compatible with container-based deployment platforms. This is essential for deploying Mendix applications to Kubernetes, Cloud Foundry, or other container orchestrators.

The build command applies six patches automatically after MxBuild produces the PAD output:

| # | Patch | Applies To | Description |
|---|-------|-----------|-------------|
| 1 | **Set bin/start execute permission** | All versions | ZIP extraction does not preserve the `+x` flag. This patch runs `chmod 0755` on `bin/start` so the container can execute the start script. |
| 2 | **Fix Dockerfile CMD (start.sh -> start)** | 11.6.x only | MxBuild 11.6.x generates `CMD ["./bin/start.sh"]` but the actual script is named `start` (no `.sh` extension). |
| 3 | **Remove config argument** | All versions | Removes the `"etc/Default"` argument from the CMD line. The `bin/start` script accumulates args with `CONFIG_FILES="$CONFIG_FILES $1"`, which produces a leading space (`" etc/Default"`) that the runtime rejects. Without the argument, `bin/start` uses its own `DEFAULT_CONF` variable. |
| 4 | **Replace deprecated base image** | All versions | Replaces `FROM openjdk:21` with `FROM eclipse-temurin:21-jre`. The `openjdk` Docker images are deprecated and no longer maintained. |
| 5 | **Add HEALTHCHECK** | All versions | Inserts a `HEALTHCHECK` instruction before the `CMD` line: `curl -f http://localhost:8080/ || exit 1` with a 15s interval, 5s timeout, 30s start period, and 3 retries. |
| 6 | **Bind admin API to all interfaces** | All versions | Appends `admin.addresses = ["*"]` to the PAD config file (`etc/Default`). Without this, the admin API (port 8090) only listens on localhost inside the container and is unreachable from outside even when the port is mapped. |

Patches are applied idempotently -- if a patch has already been applied (e.g., from a previous build), it is skipped. The build output shows the status of each patch:

```
Applying patches...
  [applied] Set bin/start execute permission
  [skipped] Fix Dockerfile CMD (start.sh -> start)
  [applied] Remove config arg from Dockerfile CMD
  [applied] Replace deprecated openjdk base image
  [applied] Add HEALTHCHECK instruction
  [applied] Bind admin API to all interfaces
```

## Validation with mx check

To validate a project without building:

```bash
mxcli docker check -p app.mpr
```

This runs `mx check` against the project and reports any errors:

```
Checking app for errors...
The app contains: 0 errors.
```

## Auto-Download mxbuild

mxcli automatically downloads the correct mxbuild version for your project:

```bash
# Explicit setup
mxcli setup mxbuild -p app.mpr

# Or let docker build handle it automatically
mxcli docker build -p app.mpr
```

The downloaded mxbuild is cached at `~/.mxcli/mxbuild/{version}/` for reuse.
