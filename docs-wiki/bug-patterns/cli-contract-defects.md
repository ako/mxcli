---
title: When the CLI Is the Defect
category: bug-pattern
last-synced: 5c24d899
covers:
  - cmd/mxcli
sources:
  - .claude/skills/fix-issue/findings/cmd-mxcli/
  - cmd/mxcli/syntax/
  - cmd/mxcli/script_set.go
  - cmd/mxcli/session_start.go
  - cmd/mxcli/cmd_describe.go
  - mdl/srctext/srctext.go
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
diagnostics on stderr. It came back across the whole query family because
choosing the executor's writer only works when the *command* prints the
payload; where the executor prints it, the split has to be made per line inside
the executor (answer vs. commentary) — and the empty result, the error, and a
command that silently ignores the root `--json` flag are where it breaks.

**Path handling is where portability bugs live.** Windows backslashes mangled by
escape processing, a relative `-p` rejected by a downstream tool with its own
error text, a test directory resolved relative to the wrong base, `-` not
accepted for stdin so MDL cannot be piped. Each is small and each stops a
workflow completely.

**Say what was skipped and why.** "Found 0 test(s) in 1 file(s)" counted a
directory as a file, so it read as though a file had been opened and rejected.
Naming the files that were skipped — and why — turns a dead end into a rename.

**An error message that recommends a flag is the only documentation of it — and
can be the only place it exists.** `--mxbuild-path` was named by the run-local
skill, by the code's own comment and by two resolution errors, and was never
registered on `run`; everything behind it was already wired. Guidance naming an
option the command does not accept is worse than none, because the user reads
the rejection as their own mistake — and on macOS it was the only advertised way
out of a platform mismatch. The same gap in the other direction: `mxcli version`
did not exist while `--version` did, and the code already special-cased a
`version` argument for warning suppression, which is evidence of an intended
command.

**Absence from `mxcli syntax` reads as absence from the language.** `RENAME
MICROFLOW` parsed, executed and was documented by `help`, and had no entry in the
syntax registry — so an agent concluded the statement did not exist. The
`describe` subcommand had the same shape from the other side: it keeps its own
hand-maintained map of type keyword to `DESCRIBE` statement, plus two auto-detect
maps, so a doctype added to the grammar never reaches it. Where a registry or a
subcommand mirrors the grammar, the guard reads the grammar rule and fails when
the two diverge.

**An argument is not always one argument.** A topic handed over as a single
string — a quoted copy-paste, a tool wrapper, `sh -c` — became one path segment,
so `mxcli syntax "workflow user-task targeting"` answered `Unknown topic` for a
path its own help advertises. The spaces in the reported message were the whole
diagnosis: the CLI joins on `.`, so a message containing spaces cannot have come
from the command as documented. Reading that as a paraphrase cost an hour.

**A flag's zero value is not its absence.** `-c ""` was indistinguishable from no
`-c`, so a generator spawning mxcli with an open stdin hung at the interactive
prompt. Use the flag library's "was it set" test whenever an empty value must
mean something other than not given.

**A framework hook is not "before anything".** Diagnostic session logging sat in
cobra's `PersistentPreRun`, described as running before argument validation. It
does not: `execute()` parses flags, answers `--help`/`--version`, then validates
args, *then* runs the hook — so every invocation that failed validation was
invisible to the very report built to count invocations. Anything that must see
every run belongs in `main()` before `Execute`, which also moves the problem of
closing it cleanly.

**Decode the bytes where they are read.** A script saved by Windows PowerShell
carries a UTF-8 BOM, and every reader passed raw bytes to a lexer that expects
none — so `check`, `exec`, `fmt` and `diff` all failed on an invisible character.
Stripping it in the parser alone would have been worse: a BOM also hides a
`mdl 1;` header from the version scan, so the language version would have been
silently wrong. One shared reader, and the enumeration is a grep for the read
calls rather than a fix to the reader the report named.

**One summary per run, over everything printed.** `check -p` printed three
summary lines from three tiers, so a warning line sat next to `✓ All references
valid` next to an error line, and none of the three was the total. A per-batch
formatter's summary is right only for a single-batch caller; a multi-stage report
needs its count at exit. The adjacent judgement call is severity: `exec` is
re-run constantly, so it filters info notes at *its* printer rather than
lowering the rule, because `check` is where a script is reviewed and must keep
every note — and the formatter's summary then has to say how many it hid.

**The exit code is a claim about the whole command.** `mxcli new` exited 1
because a best-effort final step could not download an auxiliary binary, after
six steps had already produced a usable project. Audit each step's failure branch
against whether the artefact is already usable, not against how that step reads
in isolation. The existing test had encoded the bug as the specification.

**The unit of work is sometimes the set, not the file.** `check a.mdl b.mdl`
judged each file against the project as it stands, so a reference to something an
earlier file creates was reported missing although `exec` on the pair succeeds —
and the reported kind (a widget datasource) was a red herring, not the blast
radius. `fmt --upgrade` had the mirror: a stub-then-real pair, upgraded one file
at a time, split, because a per-file header verdict is wrong when two files write
the same document. Both symptoms hide because each half is individually correct;
only the combination is wrong, so the test takes the set with the single-file
behaviour as its control.

**Shipped guidance is code.** The skills and skill packs mxcli embeds are
compiled in and versioned with the binary, so they go stale against it, and a
malformed one ships as-is if the test only checks that a file exists. Assert on
the rendered content, not on presence.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the individual
  flags, paths and messages
- [[test-runner-cannot-fail]] — the same "accepted but inert" shape, where its
  consequence is a false pass
- [[generated-mdl-on-the-authors-lines]] — where the text the tool complains
  about is the tool's own
- [[the-platform-ci-does-not-run]] — contract defects that only appear off the
  CI matrix
