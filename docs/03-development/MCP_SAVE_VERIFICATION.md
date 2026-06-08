# Verifying `--mcp-save` from Concord (in-Studio-Pro test)

`--mcp-save` flushes PED-authored, in-memory writes to disk via Concord's
`save_all` (PED has no save tool). `save_all` synthesizes a macOS Cmd-S
(osascript → System Events), so its reliability depends on the environment — see
the `save_all` caveats in [`PED_MCP_CAPABILITIES.md`](PED_MCP_CAPABILITIES.md).

Testing it from a **devcontainer** is inconclusive: with two Studio Pro instances
sharing one bundle id, the synthetic Cmd-S targets the wrong window and silently
no-ops (`{"status":"save_command_sent"}` with no disk change). The authoritative
test is from **Claude Code running in the Concord terminal inside Studio Pro on
macOS**, with a single active instance — there the keystroke has an unambiguous
target.

## The test prompt

Paste this into Claude Code in the Concord terminal (Studio Pro, macOS):

> **Test whether mxcli's `--mcp-save` actually persists a change to disk.**
>
> You're in the Concord terminal inside Studio Pro on macOS. Two local MCP
> servers: **PED** (Studio Pro's built-in authoring) on `localhost:7782`, and
> **Concord** on `localhost:7783`. mxcli is built from `main`. The MCP backend
> reads from the local `.mpr` and writes via PED; `--mcp-save` flushes via
> Concord's `save_all`. Everything is local — **no socat/port-forwarding**.
>
> Goal: confirm a PED-authored entity, flushed with `--mcp-save`, lands on disk.
>
> 1. **Find the open project.** It's the project Studio Pro has open. Set:
>    ```bash
>    MPR="<path to the open .mpr>"      # e.g. .../mx-test-projects/test7-app/test7.mpr
>    DIR=$(dirname "$MPR")
>    ```
>    Pick an **existing user module** in it (e.g. `MyFirstModule` in test7) — call it `MOD`.
>
> 2. **Record disk state before:**
>    ```bash
>    stat -f '%m %N' "$DIR/mprcontents"     # note this mtime
>    ```
>
> 3. **Create an entity via PED and save via Concord:**
>    ```bash
>    mxcli -p "$MPR" \
>      --mcp http://localhost/mcp --mcp-dial localhost:7782 \
>      --mcp-concord http://localhost/mcp --mcp-concord-dial localhost:7783 \
>      --mcp-save --mcp-check \
>      -c "create entity $MOD.SaveProbeMac (Code: string)"
>    ```
>    If it can't connect to PED, retry with bare `--mcp http://localhost:7782/mcp`
>    (and same for `--mcp-concord` on 7783). Note any `warning: --mcp-save failed …` line.
>
> 4. **Verify it persisted** (these are the same files Studio Pro saves to):
>    ```bash
>    stat -f '%m %N' "$DIR/mprcontents"               # mtime should have advanced
>    grep -rl SaveProbeMac "$DIR/mprcontents/"         # should find the entity
>    ```
>
> 5. **Report:**
>    - Did mxcli print `Created entity`? Any `--mcp-save failed` warning? What did `--mcp-check` print?
>    - Did the `mprcontents` mtime change, and is `SaveProbeMac` on disk?
>    - **Conclusion: did `--mcp-save` persist the change?**
>
> Important conditions (the whole point of this test):
> - Run with **only ONE Studio Pro instance** open — `save_all` sends a synthetic
>   Cmd-S, which is ambiguous if two instances share the bundle id.
> - Make sure **Studio Pro is the frontmost/active app** when the command runs.
> - If it didn't persist, do a **manual Cmd-S** in Studio Pro and re-check — that
>   isolates "the automation didn't land" from "the project can't save."

## Interpreting the result

- **mtime advances + `SaveProbeMac` on disk** → `--mcp-save` works; the
  devcontainer failure was purely the two-instance ambiguity. Update the
  `save_all` caveat in `PED_MCP_CAPABILITIES.md` accordingly.
- **No disk change, but manual Cmd-S does save** → Concord's `save_all` keystroke
  isn't reaching Studio Pro even single-instance; report upstream to Concord (its
  own error text invites this).
- **Even manual Cmd-S doesn't save** → a Studio Pro / project issue unrelated to
  mxcli.
