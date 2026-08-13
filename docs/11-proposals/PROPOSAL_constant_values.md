---
title: Constant values — one precedence chain, and a slot for secrets
status: proposed
date: 2026-08-13
---

# Proposal: Constant values — one precedence chain, and a slot for secrets

**Status:** Proposed
**Date:** 2026-08-13

A Mendix constant has a value in four possible places, mxcli can write two of
them, and only one of those two reaches a running app. This proposal defines a
single precedence chain across every way mxcli boots an app, adds the missing
slot for a value that must not reach git, and closes a divergence where
`mxcli test --local` runs against different constant values than
`mxcli run --local`.

Everything measured below was measured on **Mendix 11.12.1**, against a live
standalone runtime. The measurements are in [Appendix A](#appendix-a--measurements).

## Problem Statement

### 1. A configuration override used to reach nothing (fixed, and the shape to avoid)

`alter settings constant 'M.C' value 'x' in configuration 'Default'` executed,
reported success, and round-tripped through `describe settings` — and the app ran
with the constant's *default*, because mxbuild writes each constant's default
into `deployment/model/config.json` and that map is what the runtime is handed.
An app ran for hours with an empty encryption key while the model said otherwise
(mxcli-chat FINDINGS §33).

That is fixed: `run --local` now resolves a configuration's shared constant
values and merges them over the defaults at boot, and prints what it applied in
every case including "nothing". The failure is worth restating because **every
remaining gap below has the same shape**: a layer reports success and the value
silently does not arrive.

### 2. `mxcli test --local` and `mxcli test --attach` disagree

`LocalAppOptions` — the headless boot used by both test runners — has no
`ConstantOverrides` field, so `test --local` boots with defaults only.
`test --attach` runs against an app booted by `run --local`, which *does* apply
the configuration's values. The same suite therefore sees different constants
depending on a flag that is documented as an optimisation ("no boot needed"),
not as a semantic change. Nothing errors; a test that depends on a constant just
quietly asserts against a different value.

### 3. There is no way to set a value for one run

Every route mxcli offers writes to the model, and therefore to git. For an API
key that is backwards. The only workaround is `alter settings constant …`, run,
then revert — and a forgotten revert commits the key.

`--runtime-setting MicroflowConstants={…}` looks like an escape hatch and is
not one: `RuntimeSettings` is applied *after* the constants map in
`runtimeConfigParams`, so it replaces the map mxcli built rather than adding to
it, and at boot there is nothing to fall back on for `BasePath`/`DatabaseName`
(neither is in `config.json`).

### 4. Mendix's own secret slot is unreachable headlessly

Mendix 10.9+ stores a **private** configuration value encrypted on the local
machine, readable only by that user account
([Configurations Tab](https://docs.mendix.com/refguide/configurations-tab/)).
The docs are explicit that a *shared* value "is stored as part of the app", so
committing it shares it with everyone who can read the repository, and that
relying on a constant's default or on shared configuration settings is unsafe
for exactly that reason
([App Setup Best Practices](https://docs.mendix.com/refguide/app-setup-best-practices/)).

So the right home for a secret exists — **but only where Studio Pro runs.** It
is per-user encrypted, off-model, and Windows/Mac. In a Linux devcontainer or a
Claude Code session there is no Studio Pro and no such store: nothing can write
it and nothing can read it. The only headless reader is
`mxbuild --export-secrets`, whose own help scopes it to
`target=portable-app-package` — i.e. reading secrets a Studio Pro user already
wrote on that machine.

**For a headless run the sensitive-settings slot is simply empty.** Defaults and
shared values both go to git; the one safe slot is off-platform. mxcli's current
refusal to write a private override is therefore correct, and for a stronger
reason than the one in the code comment ("the value is not in the model").

## Non-goals

- **Writing Mendix private values.** Out of reach (§4 above), and attempting it
  would mean reimplementing a per-user encryption scheme mxcli cannot verify.
  mxcli mirrors the *concept* with its own store instead, and says so.
- **Changing what `alter settings constant` does.** Shared per-configuration
  overrides stay exactly as they are; they remain the right place for a value
  the team *should* share.
- **A secrets manager.** No Vault/KMS integration, no encryption at rest beyond
  file permissions. The local store is a gitignored 0600 file — the same bar as
  `~/.mxcli/auth.json`, and honestly labelled.

## Proposed precedence chain

Highest first. Each layer is one sentence you can hold in your head.

| # | Layer | Set with | Lives in | In git? |
|---|-------|----------|----------|---------|
| 1 | **This run** | `--constant Module.Name=value` | nothing — the process only | no |
| 2 | **This machine** | `mxcli constant set Module.Name 'value' --local` | `<project>/.mxcli/constants.json` (0600) | **no** (already gitignored) |
| 3 | **This configuration** | `alter settings constant 'Module.Name' value '…' in configuration 'X'` | the model | yes |
| 4 | **Default** | `create [or modify] constant Module.Name … default '…'` | the model | yes |

Layer 2 is mxcli's answer to §4: same semantics as a Mendix private value
(per-machine, not shared, for secrets), different mechanism, and named as
mxcli's own rather than pretending to be Mendix's. `mxcli init` already writes
`.mxcli/` into the project `.gitignore`, so the home exists and is out of version
control by construction.

The chain applies **identically** to `run --local`, `test --local` and
`test --attach`, which is what fixes §2.

### Reporting

`run --local` already prints what it applied. It gains the layer each value came
from, and a `mxcli constant list` shows the resolved view:

```
$ mxcli constant list -p app.mpr --configuration Default
CONSTANT                      VALUE            FROM
MyModule.ApiKey               ****             machine (.mxcli/constants.json)
MyModule.ServiceUrl           https://…        configuration "Default"
MyModule.Retries              3                default
Encryption.EncryptionKey      (private)        Studio Pro — not in the model, default used
```

A layer-2 value is masked by default (`--show-values` to print it), because the
whole point of the layer is that it holds things you would not paste into a
terminal transcript.

## The live path — `update_configuration` + `reload_model`

Injecting a constant into an *already running* app is buildable, and is two
calls, not one. Measured (Appendix A):

- `update_configuration` is **staged, not applied**: the running app keeps the
  old value until the next `reload_model`, while answering `result:0`.
- The runtime treats the payload's `MicroflowConstants` as an **overlay on the
  deployment defaults** — constants omitted from the map still resolve.
- There is **no read-back**: `get_configuration`, `get_current_configuration`,
  `runtime_config` and `get_current_runtime_status` are all *"Action not found"*.

So:

```
mxcli constant set MyModule.ApiKey 'sk-…' --local --apply
```

writes layer 2 **and** applies it to a running dev loop, as
`update_configuration` (full payload, constants overlaid) → `reload_model` →
**verify by observation**. Verification is not optional: the admin API returned
success for the call that changed nothing, which is precisely the §33 shape.

This only works where mxcli owns the boot payload (`run --local`), because the
configuration cannot be read back to merge against. Against an app mxcli did not
boot, `--apply` refuses rather than sending a partial payload.

## BSON / storage

No new document type, and no new BSON. The model side is already implemented:

| Element | `$Type` | mxcli |
|---------|---------|-------|
| Constant | `Constants$Constant` | read + write (`create constant`, default value) |
| Per-configuration value | `Settings$ConstantValue` | read + write |
| Shared value | `Settings$SharedValue` (nested, carries `Value`) | read + write |
| Private value | `Settings$PrivateValue` (nested, **no properties at all**) | read (as a marker); **write refused** |

The `SharedValue` / `PrivateValue` distinction is the polymorphic-child trap
already recorded in CLAUDE.md: the variants differ in *arity*, not just field
values, so a write must dispatch on `$Type` before assigning `Value` — assigning
it to the marker corrupts the document into something `mx check` accepts and
Studio Pro cannot open. `mdl/settingsoverlay` already does this and this proposal
does not change it.

Layer 2's own file is not BSON. Proposed shape:

```json
{
  "version": 1,
  "constants": {
    "MyModule.ApiKey": "sk-…"
  }
}
```

Flat, per-project, no configuration dimension: this layer means "on this
machine", and a machine runs one thing at a time. If that proves wrong, a
`configurations` key can be added without breaking `version: 1` readers.

## Implementation plan

Four slices, each independently shippable and independently verifiable.

### Slice 1 — close the `test --local` gap (bug fix)

The smallest correct change, and the one with a user-visible bug behind it.

| File | Change |
|------|--------|
| `cmd/mxcli/docker/localapp.go` | `LocalAppOptions.ConstantOverrides`; pass to `StartLocalRuntime` |
| `cmd/mxcli/testrunner/runner_local.go`, `runner_endpoint.go` | resolve and pass the overrides |
|  `cmd/mxcli/cmd_test_run.go` | `--configuration` flag, mirroring `run` |

Test: a `.test.mdl` asserting a constant, run under `--local` and under
`--attach` against the same project, must agree. That test fails today.

### Slice 2 — layer 1, `--constant Module.Name=value`

| File | Change |
|------|--------|
| `cmd/mxcli/constants_resolve.go` (new) | the chain: layers 4→1, returning value + provenance |
| `cmd/mxcli/runconstants.go` | fold `resolveConstantOverrides` into the chain |
| `cmd/mxcli/cmd_run.go`, `cmd_test_run.go` | `--constant` (repeatable) |

Refuses an unknown constant name rather than passing it through: a typo'd
override is silently ignored by the runtime, which is the §33 shape again.

### Slice 3 — layer 2, the machine store

| File | Change |
|------|--------|
| `cmd/mxcli/constantstore/` (new) | load/save `<project>/.mxcli/constants.json`, 0600, atomic rename |
| `cmd/mxcli/cmd_constant.go` (new) | `mxcli constant set/unset/list` |
| `cmd/mxcli/init.go` | assert `.mxcli/` is in `.gitignore` (it is; make it a checked invariant) |

`constant set` refuses to write a name the project does not declare, and warns
when the same constant also has a shared override, naming which one wins.

### Slice 4 — `--apply` (the live path)

| File | Change |
|------|--------|
| `cmd/mxcli/docker/localboot.go` | export a `SetConstants(...)` that re-sends the full payload with constants overlaid, then reloads |
| `cmd/mxcli/cmd_constant.go` | `--apply`: locate the dev loop via `.mxcli/test-endpoint.json`-style handshake, apply, verify |

Verification is by observation, not by return code. The handshake file that
`run --local --test-endpoint` already publishes is the model for locating the
running loop; a dev loop without it can still be reached by admin port, but
`--apply` must then be given `--admin-port` explicitly rather than guessing.

## Version compatibility

Not version-gated. `deployment/model/config.json`, the M2EE
`update_configuration` action and `reload_model` are stable across the 10.x/11.x
range mxcli supports, and layers 1–2 are mxcli's own. The one version-specific
fact — Mendix private values existing from 10.9 — is a *non*-goal, so it gates
nothing; it only appears in the explanatory text of `constant list`.

`sdk/versions/mendix-*.yaml` needs no new entry.

## Test plan

| Layer the symptom lives in | Test |
|---|---|
| Precedence resolution | unit tests in `cmd/mxcli/` — each layer wins over the one below; provenance is reported; an unknown name is refused |
| The machine store | unit tests — 0600, atomic write, absent file is not an error, malformed file is a named error not a panic |
| Files on disk | `.mxcli/constants.json` is matched by the generated `.gitignore` (assert, don't assume) |
| **The value a running app actually uses** | **`.claude/skills/verify-in-runtime.md`** — boot `run --local --test-endpoint`, read the constant through a microflow over the test endpoint. This is the only layer that can prove the value arrived, and every bug in this area has been invisible at every layer above it |
| The live path | the same, with the control that matters: change the value, read **without** a reload, assert it is *unchanged*; then reload and assert it changed |

MDL examples: `mdl-examples/doctype-tests/09-constant-examples.mdl` already
covers layers 3–4 and needs no change. Layers 1–2 are CLI, not MDL, so they get
CLI tests rather than doctype tests.

The runtime test must include the negative control. A test that only asserts
"after set + apply the value is X" passes against an implementation that applies
the value at boot and ignores `--apply` entirely.

## Open questions

1. **Should `--constant` exist at all, given layer 2?** A flag puts the secret in
   shell history and in `ps` output. The argument for keeping it is CI, where the
   value comes from the runner's secret store and there is no persistent machine.
   An env route (`MXCLI_CONSTANT_<Module>.<Name>`) avoids `ps` but not the
   environment. **Recommendation:** ship `--constant` in slice 2 for the
   ephemeral/CI case, document the exposure in its own flag help, and let layer 2
   be the recommended path for a developer machine.
2. **Per-configuration layer 2?** Deliberately flat above. Someone running two
   configurations against one checkout would need it; nobody has asked.
3. **Does `mxcli docker run` get the same chain?** It should, for consistency, but
   the container boots through a different path and this proposal does not cover
   it. Worth a follow-up rather than a rushed slice.
4. **Masking in `constant list`.** Masking every layer-2 value is safe but
   annoying for non-secrets. An explicit `--secret` marker on `constant set`
   would let non-secrets print — at the cost of a decision on every set.

## Appendix A — measurements

Mendix 11.12.1, `mxcli run --local --test-endpoint`, values read by invoking a
microflow that returns `@MyFirstModule.ApiKey` over the test endpoint by raw
HTTP. Reading this way is load-bearing: `mxcli test --attach` rebuilds and
hot-reloads on every run, which would have made the reload the confound.

| # | Action | Value the running microflow returned |
|---|--------|--------------------------------------|
| 0 | boot with a configuration `Default` override | `RUNTIME-KEY` |
| 1 | `update_configuration` → INJECTED, then `reload_model` | `INJECTED-KEY` |
| **2** | **`update_configuration` → LIVE, no reload** | **`INJECTED-KEY` — unchanged** |
| 3 | `reload_model`, no further config call | `LIVE-KEY` |
| 4 | payload carrying **one** constant, then reload | `PARTIAL-KEY`; the omitted constant still resolved to its default; the database still worked |

Row 0 is a live re-confirmation that the §33 configuration-override fix works
end to end. Row 2 is the control that proves the call is staged. Row 4 corrected
a comment in `localboot.go` that claimed the admin action "REPLACES rather than
merges" — at the runtime level, for constants, it does not.

`get_configuration`, `get_current_configuration`, `runtime_config` and
`get_current_runtime_status` all returned `{"result":-5,"message":"Action not
found."}`.

**Not measured:** a runtime *restart* with a partial configuration still in
place — the `shutdown` used to test it ended the JVM. In practice mxcli always
sends the full payload at boot, so a partial configuration never survives a
restart.

## References

- mxcli-chat FINDINGS §33 — the silent no-op this builds on
- `cmd/mxcli/runconstants.go` — layer 3 resolution as it exists today
- `cmd/mxcli/docker/localboot.go` — the boot payload and what the admin action does
- `mdl/settingsoverlay/settingsoverlay.go` — shared/private guard on the model side
- [Configurations](https://docs.mendix.com/refguide/configuration/) ·
  [Configurations Tab](https://docs.mendix.com/refguide/configurations-tab/) ·
  [Constants](https://docs.mendix.com/refguide/constants/) ·
  [App Setup Best Practices](https://docs.mendix.com/refguide/app-setup-best-practices/)
