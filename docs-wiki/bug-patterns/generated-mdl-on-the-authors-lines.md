---
title: When mxcli Checks Its Own Generated MDL
category: bug-pattern
last-synced: 5c24d899
covers:
  - cmd/mxcli
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli/
  - cmd/mxcli/testrunner/check_source.go
  - cmd/mxcli/testrunner/parser.go
  - cmd/mxcli/testrunner/upgrade_source.go
  - cmd/mxcli/lsp_completion.go
---

> **Do not duplicate**: the rendering's own rationale and measurements are
> canonical in `check_source.go`'s package comment; the `.test.mdl` format is in
> `.claude/skills/test-microflows.md` and `docs-site`; the per-defect fixes are
> in the findings. This page is about what changes when the MDL being checked is
> mxcli's.

## What this is

A `.test.mdl` file is not a sequence of MDL statements. Each block is a
**microflow body**, wrapped in a doc comment and separated by `/`, so `check`,
`fmt` and the LSP cannot parse it directly — each one *renders* it into real MDL
first and checks the rendering. The same is true elsewhere: the LSP's completion
snippets are MDL that ships inside the binary, and the runner's generators emit
MDL that is executed rather than checked.

That makes mxcli both the author and the critic, and it breaks the assumption
every diagnostic rests on: **that the text being complained about is the text the
user wrote.** The rendering is deliberately laid out on the author's own lines —
blank-padded, with the wrapper on the lines the doc comment and the separator
occupied — so no diagnostic needs remapping. The cost of that choice is that
every defect in the wrapper arrives in the user's editor, on their line, looking
like their mistake.

## How it fits

**Generated MDL is subject to the deprecation registry and the language notes,
like any other MDL.** A wrapper spelled `CREATE OR REPLACE MICROFLOW` put
MDL-DEPR001 on the doc comment of every `@test` block, for a spelling the author
never typed — and it counted against the conformance gate. The wrapper's closing
`/` did the same with MDL-V1-SLASH. So the rule is stronger than "the rendering
must parse": **the wrapper has to be canonical under every language version**,
and a guard asserting no deprecations is only half of it — it must assert the
language notes too, which is exactly how the second instance got through after
the first was fixed.

**A rendering checked against a project must declare everything the runner would
create.** `--references` on a test file failed with `module not found: MxTest`,
because the wrapper names a module that only a *run* creates. The interesting
part is the second-order effect: the early return on the unresolvable container
suppressed every finding inside it, so the false positive was also a false
negative — a body calling a microflow that does not exist was never reported.
A `CREATE MODULE` on the first wrapper's line fixes both and keeps the line
numbers.

**In a format that is not parsed as one script, every consumer that splits the
text must take the header out first.** `mdl 1;` at the top of a `.test.mdl` was
not read by anything, so it stayed as body text in the first `/`-chunk — which
stopped that chunk's `/** @test */` from being a *leading* doc comment, and the
first test silently vanished: `--list` found 41 of 42 with no message. A header
is not a flag to pass along; in a chunked format it is content in whichever chunk
it lands in. The same "leading doc comment" rule had already eaten a test once
before.

**Behaviour and documentation can both be intended, so settle which before
touching code.** `fmt --upgrade` adds `mdl 1;` to a test file while `--help` and
the skill said test files take no header. The CHANGELOG entry that introduced the
behaviour said outright that it does, so the fix was text-only — changing `fmt`
would have regressed the change that added it. The tell for a stale sentence is a
word the feature's own history contradicts.

**Completion text is MDL that ships in the binary**, held to what documentation
is held to. The LSP's snippets were hand-written once, never parsed, and the
grammar moved on underneath them — so the editor offered insertions that parse
under no language version. The guard writes itself: expand every snippet with its
defaults, parse it under the current header, and require no deprecation.

**The reporter's diagnosis can be precise, confident and wrong, and the pipeline
answers faster than the code.** An OQL follow-set error (`expecting {GROUP_BY,
SELECT, HAVING}` on a `RETRIEVE … LIMIT`) was reported as the runner generating a
broken microflow. It was not: the generated flow parsed fine against a real
project, and the message came from the *rendering*. Dumping the generator's
output and feeding it to the visitor settled in seconds what an hour of reading
had not.

**Scraping another command's human-readable output is this bug one layer out.**
The runner read the project's after-startup microflow by parsing `DESCRIBE
SETTINGS` text, splitting on `=`. A describe rewrite changed the format to
`Key: 'value'`, so the whole line became the value, and cleanup restored a
mangled setting into the user's project. A parser of another command's prose
breaks silently when that prose changes; the table test has to carry the line the
producer emits *today*, and the durable fix is to read the value through the
backend instead.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — each wrapper
  defect and the guard that now pins it
- [[test-runner-cannot-fail]] — the same runner, where the consequence is a false
  pass rather than a false diagnostic
- [[cli-contract-defects]] — the neighbouring class where the tool's own contract
  with its caller is the defect
- [[misleading-diagnostics]] — what a wrong message costs once it is believed
