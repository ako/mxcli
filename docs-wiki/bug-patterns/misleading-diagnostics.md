---
title: A Wrong Hint Is Worse Than No Hint
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-visitor.jsonl
  - mdl/visitor/visitor.go
---

> **Do not duplicate**: the individual hints and their trigger conditions live in
> the findings and in `mdl/visitor/`. This page is about why the class is graded
> higher than "the message could be clearer".

## What this is

ANTLR's raw errors name tokens, not intentions: `no viable alternative at input
'add<Name>'` for a missing keyword, `mismatched input '=' expecting ')'` for a
caption written with the wrong separator. mxcli adds hints on top. Six
`mdl/visitor` findings are those hints firing on the wrong thing and **sending
the reader somewhere the problem is not**.

A missing hint costs the reader a minute of confusion. A wrong one costs however
long they spend acting on it — and in the reported cases they blamed their
quoting, renamed an attribute that was fine, or concluded a construct was
unsupported.

## How it fits

**A pattern broad enough to be useful is broad enough to be wrong.** The
"unescaped apostrophe" hint matched any short lowercase word, so every genuine
parse error on `on`, `in`, `as`, `to` or `by` was diagnosed as a quoting mistake.
Narrowing it to the actual contraction suffixes — `s`, `t`, `d`, `m`, `re`, `ve`,
`ll` — keeps the real cases and drops the false ones. A hint's precision matters
more than its coverage, because its whole value is that the reader trusts it.

**Key the hint off the source line, not the parser message.** The token error for
a misplaced `index` clause lands on the index *name*, so the message never
mentions `index` at all and no message-matching hint can fire. Reading the
offending line is what makes the diagnosis possible — and it also lets the hint
discriminate against the valid form of the same construct.

**A hint that recommends a rename is a strong claim.** One offered three
alternative names for an attribute that parsed perfectly well elsewhere, because
it conflated two kinds of reserved word: an MDL *parser* keyword, which quoting
escapes and the model keeps, and a name the *platform* reserves, which quoting
does not help with at all. Conflating them is expensive in both directions —
advising a quote where the platform reserves the name produces something that
parses and then fails the build.

**Some errors are the platform's, and the hint should say so.** Mendix XPath
cannot compute values, so `[Seq = $Game/MoveSeq + 1]` is a limitation rather than
a syntax mistake; a bare `mismatched input '+'` reads as the latter. Naming the
constraint turns a dead end into a redesign.

**The reachable-form question.** Several of these are not really about wording:
the reader's next question is "then how *do* I write it?", and a hint that names
the working spelling answers it. Where there is no working spelling, saying so is
still better than a token error — that is the boundary with
[[capability-gap-as-parse-error]].

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — the hints,
  their triggers and their controls
- [[capability-gap-as-parse-error]] — when the honest hint is "you cannot"
- [[keyword-collisions]] — the two kinds of reserved word, kept apart
