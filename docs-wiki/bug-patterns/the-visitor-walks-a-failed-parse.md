---
title: The Visitor Walks a Failed Parse
category: bug-pattern
last-synced: a918689d
covers:
  - mdl/visitor
sources:
  - .claude/skills/fix-issue/findings/mdl-visitor/
  - mdl/visitor/visitor.go
  - mdl/visitor/visitor_workflow.go
  - mdl/visitor/visitor_verbs.go
  - mdl/visitor/visitor_string_escapes.go
---

> **Do not duplicate**: the individual guards and their controls live in the
> findings and in `mdl/visitor/`; what each statement's grammar requires is in
> the `.g4` files. This page is about the one design decision that makes a whole
> class of crash reachable.

## What this is

`visitor.Build()` walks the parse tree **even when the parse failed**. That is
deliberate and valuable: it is what lets `mxcli check` report more than the
first syntax error, which is the difference between one round trip per mistake
and one per file. ANTLR's error recovery — single-token deletion, insertion of a
"missing" token, resynchronisation to a follow set — hands the walk a tree whose
shape the grammar says is impossible.

So in this codebase **a required grammar child is not a guarantee inside the
visitor.** Every `ctx.X().GetText()` or `.GetSymbol()` on a child the rule marks
mandatory is a latent nil dereference, reachable from ordinary malformed input
rather than from anything exotic. Three have shipped as hard crashes: an
unquoted value in a workflow parameter mapping, a stray word after `show
project`, and a missing string on a published REST property. `check`, `check
--references` and `exec` all took the same SIGSEGV.

## How it fits

**The severity is what sets this apart from the other visitor classes.** A
wiring gap loses a property; a translation error changes a number. This one kills
the process, which means no diagnostics at all — not even the syntax error that
*caused* the recovery and would have told the user exactly what to fix. The user
is left with a stack trace for a typo.

**Guard the shared helper, not each site.** The first instance's own advice was
to grep for unguarded `STRING_LITERAL().GetText()`. That grep missed the next
one, because most sites reach the token through a shared unquoting helper the
pattern never matches. A nil there always means a parse that already failed, so
returning the empty string is safe by construction — the program cannot reach
`exec` — and one guard covers every rule that uses it, including the rules
nobody has written yet. Per-site guards are a treadmill; the helper is a fence.

**Deprecation and gate listeners run on recovered trees too.** It is tempting to
think of this as a problem for the statement builders only, but the listeners
that record deprecated spellings and language notes walk the same tree. One
called `GetSymbol()` on a token its rule makes mandatory, and `show project
version` panicked — while the *truncated* form, `show project` alone, did not,
because ANTLR conjured the missing token there. The lesson for the test is
precise: **exercise the "one wrong word" form, not the truncated one**, since
the two take different recovery paths and only one of them produces the
impossible tree.

**A check that reads a statement's last tokens must stand down on a line that
already failed.** Under `mdl 1` a missing terminator is an error, and the check
reads the end of each statement — but after single-token deletion the statement
*ends* wherever recovery left it, so `drop microflow M.F if exists;` reported a
missing `;` that is plainly present in the source. The parse finishes before the
walk, so the error listener's line numbers are available to every exit: a
builder-level check on a line that already carries a syntax error must say
nothing, or it reports the recovery as a second mistake and sends the reader
after a phantom.

**The enabling condition is worth stating positively.** None of this argues for
abandoning the walk-on-failure design — the alternative costs every user an
error-at-a-time loop. It argues that the visitor's contract is weaker than the
grammar's, and that the weakening is invisible at every call site. Reading
`ctx.STRING_LITERAL().GetText()` in a rule that requires `STRING_LITERAL` looks
total. It is not, and the type system agrees with the grammar rather than with
the runtime.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — each crash,
  its input, and the guard that now covers it
- [[visitor-wiring-gaps]] — the same bridge failing quietly instead of loudly
- [[misleading-diagnostics]] — where recovery produces a wrong message rather
  than a crash
- [[mdl-execution]] — the pipeline, and why `check` wants more than one error
