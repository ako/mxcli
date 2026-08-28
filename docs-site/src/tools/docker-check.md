# mxcli docker check

The `mxcli docker check` command validates a Mendix project without building it. It runs the Mendix `mx check` tool against the project and reports errors, warnings, and deprecations.

## Usage

```bash
mxcli docker check -p app.mpr
```

## What It Does

1. **Resolves the `mx` binary** -- Looks for `mx` in the mxbuild directory, PATH, or cached installations at `~/.mxcli/mxbuild/<version>/modeler/mx`
2. **Runs `mx check`** -- Executes the Mendix project checker against the `.mpr` file
3. **Reports results** -- Outputs errors, warnings, and deprecations to the console

If the project has errors, the command exits with a non-zero status code.

## Auto-Download

If mxbuild is not installed locally, you can download it first:

```bash
# Explicit setup (downloads matching mxbuild version)
mxcli setup mxbuild -p app.mpr

# Then check
mxcli docker check -p app.mpr
```

The downloaded mxbuild (including the `mx` binary) is cached at `~/.mxcli/mxbuild/{version}/` for reuse.

## Example Output

A clean project:

```
Using mx: /home/vscode/.mxcli/mxbuild/11.6.3.12345/modeler/mx
Checking project /workspaces/app/app.mpr...
The app contains: 0 errors.
Project check passed.
```

A project with errors:

```
Using mx: /home/vscode/.mxcli/mxbuild/11.6.3.12345/modeler/mx
Checking project /workspaces/app/app.mpr...
CE0001: Entity 'MyModule.Customer' has no access rules configured.
CE0463: Widget definition changed for 'DataGrid2'.
The app contains: 2 errors.
project check failed: exit status 1
```

## When to Use

`mxcli docker check` is faster than a full build (`mxcli docker build`) because it only validates the project structure without compiling or packaging. This makes it useful for:

- **Quick validation** after making MDL changes
- **Pre-build gate** -- `mxcli docker build` runs `mx check` automatically before building (use `--skip-check` to bypass)
- **CI pipelines** -- fail fast on invalid projects before spending time on a full build
- **Iterative development** -- check after each MDL script execution to catch errors early

## Integration with mxcli docker build

The `mxcli docker build` command runs `mx check` automatically as a pre-build step. If the check fails, the build is aborted. You can skip this with:

```bash
mxcli docker build -p app.mpr --skip-check
```

## Integration with the TUI

When using mxcli in interactive REPL mode, the TUI can auto-check the project on file changes, giving immediate feedback on whether MDL modifications introduced errors.

## Related Pages

- [Docker Build](docker-build.md) -- Building a Mendix application with PAD patching
- [Docker Run](docker-run.md) -- Building and running the app in Docker
- [Testing](testing.md) -- MDL test framework
- [Linting](linting.md) -- MDL lint rules
