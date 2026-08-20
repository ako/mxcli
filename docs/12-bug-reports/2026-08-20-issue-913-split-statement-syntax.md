# #913 — MDL decision (split) syntax is inconsistent

- **Reported**: 2026-08-17 by @mkrouwel, against mxcli v0.18
- **Investigated**: 2026-08-20, against `766cdf6` (also reproduced on `c134251`)
- **Upstream**: [mendixlabs/mxcli#913](https://github.com/mendixlabs/mxcli/issues/913)
- **Proposal**: [PROPOSAL_split_statement_syntax_alignment.md](../11-proposals/archive/PROPOSAL_split_statement_syntax_alignment.md) — **implemented**

## What was reported

MDL renders Mendix's three decision types in three unrelated shapes:

| Mendix decision | MDL |
|---|---|
| Boolean | `if … then … else … end if;` |
| Enumeration | `case $x when _a then (…) when (empty) then (…) end case;` |
| Object type | `split type $x case Module.EntityA (…) else return; end split;` |

The reporter asks for alignment on a Java/TypeScript `switch`, with `case` for
branches, `default`/`empty` for the catch-all, and "right indentation".

## Verdict

The complaint is valid; the proposed remedy is not. Investigation found **three
defects**, only one of which the issue names, plus one interaction worth routing
elsewhere. The `switch` proposal is rejected — see the proposal document for why
and for the alternative that satisfies the same complaint.

## Reproduction

Environment: mxbuild 11.13.0, blank project, `bin/mxcli` at `766cdf6`.

```bash
mxcli exec i913.mdl -p Repro.mpr        # author the splits
mxcli -p Repro.mpr -c "describe microflow I913.X"
~/.mxcli/mxbuild/11.13.0/modeler/mx check Repro.mpr
```

Fixtures used are reproduced inline below; each finding names the exact command
that produced its evidence.

---

## Finding 1 — `else` on a `split type` is not a default branch (it is `(empty)`)

**Severity: high.** Silently wrong semantics, no diagnostic, contradicts the
keyword's meaning everywhere else in MDL.

`else` on an inheritance split maps to the outgoing flow with **no**
`InheritanceCase` — which on a Mendix object-type decision is the `(empty)`
flow, taken when the object is **null**. It is not "any other type". An author
who reads `else` the way `if`'s `else` reads gets a branch that never fires for
a real object.

This is stated in the builder
(`mdl/executor/cmd_microflows_builder_actions.go`, the `addBranch("", s.ElseBody)`
call and its comment) but nowhere a user would look.

**Proof.** One case plus `else`, on a three-entity hierarchy:

```mdl
split type $Animal
  case I913.Dog
    return 'dog';
  else
    return 'other';
end split;
```

`mx check` on the resulting project:

```
[error] [CE0090] "The 'I913.Animal' value should be configured for an outgoing flow." at Object type decision ''
[error] [CE0090] "The 'I913.Cat' value should be configured for an outgoing flow." at Object type decision ''
The app contains: 2 errors.
```

mxbuild demands a flow for `Cat` and for the base `Animal` **even though an
`else` is present**. If `else` were a default, neither error would be raised.

**Positive control.** Give every type its own case, keep the `else`:

```mdl
split type $Animal
  case I913.Dog    return 'dog';
  case I913.Cat    return 'cat';
  case I913.Animal return 'animal';
  else             return 'other';
end split;
```

```
The app contains: 0 errors.
```

The `else` contributes nothing to type coverage in either run. It is the
`(empty)` flow and only that.

Related: [`.claude/skills/fix-issue.md`](../../.claude/skills/fix-issue.md) already
records that this flow is load-bearing (dropping it fails **CE0089**) and that
`else` cannot substitute for the base entity's case (**CE0090**). What is missing
is that MDL spells it `else`, which reads as the opposite.

---

## Finding 2 — DESCRIBE indents the two splits inconsistently, and neither matches `if`

**Severity: medium.** Output is ambiguous to read and contradicts mxcli's own
documented examples. No test pins the current behaviour, so it is a free fix.

Column-annotated `describe microflow`, single-value branches (nothing else in
play):

```
col2 |   case $Status
col4 |     when Open then
col4 |     return 'open';          <- body at the SAME column as `when`
col4 |     when Pending then
col4 |     return 'pending';
col2 |   end case;
```

```
col2 |   split type $Animal
col2 |   case I913.Dog             <- branch at the SAME column as `split type`
col4 |     declare $y String = 'woof';
col2 |   case I913.Cat
col2 |   else
col4 |     return 'other';
col2 |   end split;
```

```
col2 |   if $Flag then             <- the correct reference
col4 |     return 'yes';
col2 |   else
col4 |     return 'no';
col2 |   end if;
```

Two bugs pointing opposite ways:

- **enum split** — `when` is indented from `case`, but the branch body is *not*
  indented from `when`. `mdl/executor/cmd_microflows_show_helpers.go:1575` writes
  `when` at `indentStr+"  "` and then traverses the body at `indent+1`, which is
  the same column.
- **inheritance split** — `case` is *not* indented from `split type`, sitting
  flush with `split type` and `end split`, while its body is indented.
  Same file, line 1612 vs 1626.

Neither matches `if/else`. The enum form also contradicts every hand-written
example in the repo — `mdl-examples/bug-tests/907-case-enum-split-is-supported.mdl`
puts `case` at 2, `when` at 4, body at 6.

**Why it matters beyond taste.** Nest an `if` inside a case branch and the
output cannot be parsed by eye:

```
col2 |   case $Status
col4 |     when Open then
col4 |     if $Flag then
col6 |       return 'open-yes';
col4 |     else                    <- reads as the case's else, which is MDL008
col6 |       return 'open-no';
col4 |     end if;
col4 |     when (empty) then
col4 |     return 'unset';         <- (empty) body, or a sibling of the `when`s?
col2 |   end case;
```

The `else` on line 4 belongs to the nested `if`, but sits at exactly the column
a reader expects a `case` branch — and an `else` on a `case` is an **MDL008**
error. DESCRIBE output is the template LLM authors copy from; this shape teaches
a spelling `mxcli check` rejects.

---

## Finding 3 — `case` has two meanings

**Severity: low on its own, but it is the issue's actual point and it is correct.**

| Site | Role of `case` | Branch keyword |
|---|---|---|
| `caseExpression` (`MDLSettings.g4:312`) | subject / searched introducer | `when … then` |
| enum split (`MDLMicroflow.g4:186`) | subject introducer | `when … then` |
| inheritance split (`MDLMicroflow.g4:208`) | **branch** introducer | — |

Two of three agree. The inheritance split is the outlier, and it is the one the
reporter singles out ("not use case where switch/split is meant"). The enum
split's `case … when … then … end case` is SQL's statement-`CASE` verbatim and
is what [ADR-0003](../13-decisions/0003-mdl-is-sql-shaped.md) commits the language to.

---

## Observation — multi-value `when` branches render as a non-nesting graph

**Not in scope here; routing to [#923](https://github.com/mendixlabs/mxcli/issues/923).**

`when Pending, Closed then` — a form mxcli documents and pins in
`907-case-enum-split-is-supported.mdl` — produces two flows to one destination.
DESCRIBE renders the branch **empty** and hoists its body past `end case;`:

```mdl
case $Status
  when Open then          return 'open';
  when Pending, Closed then  return 'other';
  when (empty) then       return 'unset';
end case;
```

describes as

```
  -- WARNING: the decision at (360, 200) has 4 branches that do not nest …
  --          This description is NOT equivalent to the microflow and must not
  --          be re-executed over it … (mxcli #923, recombinable)
  case $Status
    when Open then
    return 'open';
    when Pending, Closed then
    when (empty) then
    return 'unset';
  end case;
  return 'other';
```

Two notes for whoever owns #923:

1. Replacing the multi-value branch with three single-value branches removes the
   warning entirely, so the trigger is the shared destination, not the split.
2. The warning says the description "must not be re-executed". Measured, it
   round-trips: describe → exec → describe differs only in the split's
   `@position` (360,200 → 530,200), with identical statement content. For this
   shape the wording looks stronger than the behaviour. Layout drift across a
   roundtrip is its own question.

---

## Documentation drift found along the way

| File | Problem |
|---|---|
| `docs/11-proposals/archive/PROPOSAL_microflow_enum_split_statement.md` | Its worked example includes an `else` branch on a `case`. MDL008 rejects that. Proposal is `status: done`, so the example ships as the reference. |
| `docs/11-proposals/PROPOSAL_microflow_inheritance_split_statement.md` | "The optional `else` branch maps to the outgoing flow without an inheritance case" — true and useless. Does not say that flow is `(empty)`, i.e. the null-object case. Still `status: draft` though the feature shipped. |
| `.claude/skills/mendix/write-microflows.md` | `split type` example puts `case` flush with `split type`, matching the buggy emitter rather than `if`. |
| `docs/01-project/MDL_QUICK_REFERENCE.md` | Type split row says "Runtime specialization branches" with no mention of what `else` means. |

## Resolution

All three findings are fixed. The type split now takes the enumeration split's
branch syntax:

```mdl
split type $Animal
  when Zoo.Dog then      …
  when Zoo.Animal then   …
  when (empty) then      …   -- what `else` always was: the null-object flow
end split;
```

| Finding | Fix |
|---|---|
| 1 — `else` means `(empty)` | Branch renamed to `when (empty) then`, which says what it does. `else` still parses and warns **MDL065**, whose message states the `(empty)`/null semantics and the CE0090 consequence rather than reading as a rename. |
| 2 — indentation | Branch bodies now render one level in from their branch keyword, in **both** splits and in **both** emitters (`cmd_microflows_show_helpers.go` for DESCRIBE, `cmd_diff_mdl.go` for diff — the second had the identical bug and was found only by grepping for the pattern). |
| 3 — `case` overloaded | `case` now introduces a subject everywhere and never a branch. |

The old spelling is kept indefinitely: scripts in the wild use it, and both
spellings build the identical flow — measured by describing two projects built
from the two spellings and diffing (identical), with `mx check` 0 errors on each.

**Controls.** The indentation test was run against reverted emitters and fails
with the reported symptom (`branch body indent = 4, want 6`) before passing
after. The three DESCRIBE tests that pinned the old spelling failed on the
change and were updated deliberately rather than loosened.

Not taken: `switch` (rejected, see the proposal), `case type` as a replacement
for `split type`, multi-value type branches, and `else` as an enum-split alias.
