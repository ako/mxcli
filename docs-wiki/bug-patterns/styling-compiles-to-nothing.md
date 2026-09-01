---
title: Styling That Compiles to Nothing
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli.jsonl
  - cmd/mxcli/theme/block.go
---

> **Do not duplicate**: where SCSS compiles and the Atlas layering rules are
> canonical in CLAUDE.md; the theme commands are in the skills. This page
> describes why the failures are invisible.

## What this is

Eleven `cmd/mxcli` findings are styling that was written correctly, built
successfully, and had no effect — or had an effect nobody could read. There is no
validator for CSS. `mx check` says nothing, the build succeeds, and the rules are
simply absent from the compiled stylesheet, which in a browser is
indistinguishable from a specificity problem.

## How it fits

**Location decides whether anything compiles at all.** mxbuild walks the model's
modules to find theme sources; it never globs the directory. SCSS written to a
folder that does not match a real module is skipped without a warning. The
same applies in reverse to files imported once per module, where a rule — as
opposed to a declaration — is emitted N times.

**A token nobody reads is indistinguishable from a design not applied.** Writing
an unrecognised `--mxt-*` name produces a theme that applies cleanly and renders
exactly as before. That is why an unknown token is refused rather than written:
the alternative is a success message for a no-op.

**Contrast is a correctness property, and nothing measures it.** Several findings
are text at 1.02:1 or 1.13:1 — invisible, and reported as "contrast is low"
rather than as a defect in a specific rule. Two Atlas rules assume a dark
navigation rail and paint topbar text with a fixed colour, so a theme that
lightens the rail loses its own labels. A literal colour outside the palette
survives a theme swap and is wrong under every theme but one.

**The compile-time failures are outside Go's reach.** A Sass variable holding a
selector must be a quoted string; a bare selector is not a Sass expression, and
the failure happens in mxbuild's SCSS compiler where no Go test and no `mx check`
can see it. The only signal is the build log of a real build.

**Writing into files the project already owns needs a fence, not a rewrite.**
`theme/web/main.scss` is Mendix's own three lines and `custom-variables.scss`
carries Atlas defaults, so mxcli replaces only a digest-fenced region and refuses
a block a human has edited. The record lives in the file rather than in sidecar
state, which is what keeps it from drifting — and it is the same guard-don't-drop
rule as [[rewrite-drops-unauthored-state]], applied to files instead of BSON.
Its failure mode is the mirror image: a project that has silently taken ownership
of a file, reported as `skipped` forever after.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  selectors, tokens and measurements
- [[rewrite-drops-unauthored-state]] — the same guard, applied to model documents
- `.claude/skills/verify-in-runtime.md` — the only way most of this class is
  observable
