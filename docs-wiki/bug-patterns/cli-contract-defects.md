---
title: When the CLI Is the Defect
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli.jsonl
  - cmd/mxcli/syntax/
---

> **Do not duplicate**: individual flags, paths and messages live in the CLI help
> and the findings. This page is about the class where nothing is wrong with the
> model at all.

## What this is

Roughly fourteen `cmd/mxcli` findings are defects in the tool's own contract with
its caller — the flags it accepts, the paths it resolves, what it writes to
stdout, and what its help teaches. No Mendix document is involved. They matter
disproportionately because mxcli is driven by agents as often as by people, and
an agent takes the tool's word for everything.

## How it fits

**Help that teaches syntax the parser rejects is worse than no help.** `mxcli
syntax` documented spellings that fail to parse, so an agent following the
reference produced MDL that could not run. The mirror image is just as expensive:
a widget keyword the grammar accepts but the reference omits was concluded not to
exist and worked around at length — two days for a construct that already
existed. A syntax reference is an API surface and drifts like one.

**An unqualified success message is a claim.** `check` printed `Check passed!`
having resolved nothing against the project; a misspelled entity or page name
sailed through the command whose entire job is to catch it. Where a command's
depth depends on what it was given, the message has to say which check ran.

**Accepted-but-inert flags.** `--require-assertions` parsed, appeared in `--help`,
and was consulted in one of two code paths. A flag the tool advertises is a
promise, and the cheapest guard is that its effect has exactly one call site.

**stdout is a data channel.** `--format json | jq` failed because progress lines
shared the stream. Anything with a machine-readable format has to keep
diagnostics on stderr.

**Path handling is where portability bugs live.** Windows backslashes mangled by
escape processing, a relative `-p` rejected by a downstream tool with its own
error text, a test directory resolved relative to the wrong base, `-` not
accepted for stdin so MDL cannot be piped. Each is small and each stops a
workflow completely.

**Say what was skipped and why.** "Found 0 test(s) in 1 file(s)" counted a
directory as a file, so it read as though a file had been opened and rejected.
Naming the files that were skipped — and why — turns a dead end into a rename.

**Shipped guidance is code.** The skills and skill packs mxcli embeds are
compiled in and versioned with the binary, so they go stale against it, and a
malformed one ships as-is if the test only checks that a file exists. Assert on
the rendered content, not on presence.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  flags, paths and messages
- [[test-runner-cannot-fail]] — the same "accepted but inert" shape, where its
  consequence is a false pass
