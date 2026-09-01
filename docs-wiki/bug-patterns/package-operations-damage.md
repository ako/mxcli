---
title: Package Operations That Damage the Project
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli.jsonl
  - cmd/mxcli/marketplace/update.go
---

> **Do not duplicate**: the marketplace command surface lives in the CLI help and
> `PROPOSAL_marketplace_module_upgrade.md`; the per-operation repairs live in the
> findings. This page describes the shared hazard.

## What this is

Installing or updating a marketplace module, or running `mx update-widgets`,
hands the project to a tool mxcli does not control — and several of those tools
change things nobody asked them to. Eleven `cmd/mxcli` findings are damage from
that hand-off, and the damage is usually reported as success.

## How it fits

**The v2 → v1 collapse is the worst of them.** Mendix's own tooling rewrites an
MPR v2 project (`.mpr` plus a `mprcontents/` tree) into a single-file v1
database. `mx check` afterwards reports **0 errors** and looks like a clean run;
what has actually happened is that `mprcontents/` is gone, the `.mpr` has grown
from tens of kilobytes to tens of megabytes, and per-document diffing no longer
works. Nothing about the output says so. mxcli's repair commands exist because of
this: let the tool convert, read the units back, restore v2, and write only the
changed ones through mxcli's own writer.

**Version numbers are not identity.** Matching an installed module against a
marketplace release by version *number* is ambiguous — a blank project ships two
different modules that both published a 4.1.0. The module's `AppStoreGuid` is the
version UUID and is the thing to match on. Getting this wrong produces
"version not found" for a module the project demonstrably has.

**An update can move a dependency backwards.** Installing one module rolled a
project's widgets from 3.11.3 to 3.4.0 because an unrelated package bundled an
older copy, and `mx check` stayed clean throughout. Anything that copies bundled
files needs to compare versions rather than overwrite, and to *report* what it
skipped — "the package wanted a different version" is exactly the fact that was
invisible.

**A model that builds can still be running the old code.** After an update the
model referenced new widget definitions while `widgets/` held the old binaries;
`show modules` reported the new version. The model and the deployed artefacts are
separate, and only one of them is what `mx check` inspects.

**Report what could not be determined, never "unchanged".** Drift detection
compares an installed module against a freshly imported reference, and an element
that cannot be described is reported **unknown**. Reporting it as unchanged would
turn a gap in coverage into a claim about the user's project — and the same
report also has to say when "no modifications found" is not a conclusion.

**Environment failures masquerade as project corruption.** A path too long, an
`.mpk` built by a newer Studio Pro, a `mx` invocation resolving to a different
cached version — each leaves output that looks like a broken install. The
remedies are mostly diagnosis: say which version, which path, which binary.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  repairs and their measurements
- [[mpr-read-write]] — what v1 and v2 actually are, and why the collapse matters
- `PROPOSAL_marketplace_module_upgrade.md` — the drift-detection design
