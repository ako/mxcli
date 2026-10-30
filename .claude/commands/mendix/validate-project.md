---
description: Validate the project with mxbuild and report the errors
---

# Validate Project

Run Mendix's `mx check` command to validate the project for errors.

This is the same validation that Studio Pro performs - it checks for consistency errors (CE codes), warnings (CW codes), and other issues.

## Prerequisites

The `mx` command must be available; `mxcli docker check` does not download it. In a dev container, run `mxcli setup mxbuild -p app.mpr` once to download it.

**Where to find `mx`:**

| Environment | Path |
|-------------|------|
| Dev container | `~/.mxcli/mxbuild/{version}/modeler/mx` |
| Studio Pro install | `mx` (in PATH) |
| Manual download | `/path/to/mxbuild/modeler/mx` |

## Command

```bash
# After mxcli setup mxbuild -p app.mpr (docker check does not download mxbuild)
mxcli docker check -p app.mpr

# With an explicit MxBuild (still checks a temporary copy; bare `mx check` writes into the project)
mxcli docker check -p app.mpr --mxbuild-path /path/to/mxbuild
```

## Example Output

```
$ mx check MyApp.mpr
Mendix 10.6.0

Checking project...

MyFirstModule.Customer_Overview (Page):
  CE0123: The entity 'MyFirstModule.Customer' does not exist.

MyFirstModule.SaveCustomer (Microflow):
  CW0456: Variable '$customer' is declared but never used.

Found 1 error(s) and 1 warning(s)
```

## Common Error Codes

| Code | Type | Description |
|------|------|-------------|
| CE0xxx | Error | Consistency error - must be fixed |
| CW0xxx | Warning | Warning - should be reviewed |
| CD0xxx | Deprecation | Deprecated feature usage |

## Reading CE0463

`mxcli docker check` checks the project as stored, and labels each CE0463 ("The
definition of this widget has changed") after re-checking a temporary copy with the
widgets resynced:

- `stale widget definition — mxcli fix widgets fixes it`: the widget is older than its
  installed package. Run `mxcli fix widgets -p app.mpr` (never bare `mx update-widgets`
  on an MPR v2 project: it deletes `mprcontents/`).
- `persists after update-widgets`: a resync does not clear it; something in the stored
  widget is wrong. If mxcli wrote the widget, report it.

`--update-widgets` checks only the resynced copy and hides the stale ones, which still
fail MxBuild and `mxcli run --local`; do not use it to decide that the project builds.

## When to Use

- After making changes with mxcli to verify they're valid
- Before committing changes to version control
- After merging branches to check for conflicts
- To diagnose issues reported in Studio Pro

## Workflow

1. Make changes with an mxcli script that starts with `mdl 1;`, and execute it
   (`./mxcli exec changes.mdl -p MyApp.mpr`). `exec` writes the project as it
   runs; there is nothing to save or disconnect afterwards.
   ```sql
   mdl 1;
   create persistent entity Sales.Product (
     Name: string(200)
   );
   ```

2. Validate with mx check:
   ```bash
   mx check MyApp.mpr
   ```

3. Fix any errors and repeat

## Tips

- Run `mx check` after significant MDL changes
- Some warnings can be ignored (review case-by-case)
- Errors (CE codes) must be fixed before the project can run
- Use `mxcli check script.mdl` for MDL syntax validation (different from project validation)
