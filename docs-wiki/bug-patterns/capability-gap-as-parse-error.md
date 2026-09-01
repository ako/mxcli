---
title: A Missing Capability Looks Like a Syntax Error
category: bug-pattern
last-synced: ced830e0
sources:
  - .claude/skills/fix-issue/findings/mdl-grammar.jsonl
  - mdl/grammar/MDLParser.g4
  - docs/13-decisions/0003-mdl-is-sql-shaped.md
---

> **Do not duplicate**: the syntax itself lives in `MDL_QUICK_REFERENCE.md` and
> the skills; each construct's grammar rule and BSON shape live in the findings.
> This page is about what a parse error costs when it means "not implemented".

## What this is

Roughly twenty of the `mdl/grammar` findings are the same event: someone tries to
express something Mendix supports, the parser says `no viable alternative`, and
they conclude **the feature is impossible**. It is not — the capability is simply
unspelled, and a parse error is indistinguishable from a mistake the user made.

That confusion is the expensive part. The reports in this class are rarely
"MDL should support X"; they are workarounds. An admin screen hand-rolled as five
pages with bespoke SCSS because a tab container was believed not to exist. "Use a
Java action" for a binary upload. "Go to Studio Pro" as the standing answer to
translating an app. A headless pipeline that ends with a manual step because
anonymous access has no statement.

## How it fits

**The gap is bidirectional.** MDL not being able to *write* a construct is only
half of it: `DESCRIBE` of a document that has one has to do something, and
dropping it silently is the common outcome. Worse, in the mapping findings,
DESCRIBE emitted MDL that **parsed and rebuilt a different document** — output
that looks like a successful round trip and is not. When adding a spelling, the
read side is part of the same change, not a follow-up.

**A narrow/wide pair is a whitelist.** Where two statements can set the same
property and one accepts less than the other — `SET` versus `REPLACE` here — the
narrow one gets extended a bug report at a time, and the documented workaround
routes users through the wide one, which rebuilds more than they asked. The fix
that holds is to reuse the wide rule and its builder, not to add another case to
the narrow one.

**"Not in the metamodel" is not a conclusion until the namespace is right.** A
search under one prefix "disproved" a capability that lives under another —
`Microflows$…RequestHandling` rather than `Rest$…Body`. Asking for a Studio Pro
example settled in minutes what a metamodel grep had ruled out, and CLAUDE.md's
rule applies: when the shape is unknown, get a reference document rather than
reason from an absence.

**With no reference available, derive from something already proven.** Where no
example could be obtained, the shapes came from the generated metamodel and from
sibling writers that already ship the same by-name reference form — not from
guessing and not from stopping. Where a reference *is* available, pin against it:
a guess got four things wrong at once about one element — the type's namespace, a
property that does not exist, append-versus-sort ordering, and a flag's default.

**A reference is a reference, not a string.** An icon, an entity, a microflow —
anything that names a model element is spelled as a qualified name, which is what
makes the language consistent (ADR-0003) and what forces the quoting question
into the describe emitter as well.

**The documentation is part of the surface.** Several of these were found by
running the statements quoted in mxcli's own skills through `mxcli check` rather
than by reading them. Advice that cannot parse is the same defect as a missing
rule, and it is the version an agent will follow confidently.

## See also

- [fix-issue findings](../../.claude/skills/fix-issue/findings/) — each construct,
  its rule, and the reference document that settled its shape
- [[mdl-as-sql]] — why the language is shaped the way it is
- [[describe-round-trip-gaps]] — the read half of every gap here
