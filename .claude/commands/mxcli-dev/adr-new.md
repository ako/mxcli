# /mxcli-dev:adr-new — Create a New Architecture Decision Record

Guide the contributor through writing a new ADR in `docs/13-decisions/`.

**Read `docs/13-decisions/README.md` first.** It defines the template,
the naming convention, the when-to-write checklist, and the immutability
rules. This command is the workflow; the README is the contract.

## Process

### Phase 1: Confirm an ADR is the right home

Ask the user:

1. **What decision are you recording?** (one sentence)
2. **Is it cross-cutting?** Does it shape more than one feature?
3. **Is it durable?** Expected to hold for years, not weeks?
4. **Would the *why* be hard to reconstruct from PR history later?**

If any answer is "no", route to the correct artifact:

| If... | Use instead |
|-------|-------------|
| Feature-specific design | `/mxcli-dev:proposal` |
| Active rule for everyday work | Add a line to CLAUDE.md |
| Step-by-step procedure | A skill in `.claude/skills/` |
| Synthesized current model | A wiki page (use `/mxcli-dev:wiki-sync` after) |

Do not write an ADR for trivia. The README's checklist exists to prevent
the folder from filling with low-value records.

### Phase 2: Check for existing ADRs

```bash
ls docs/13-decisions/
grep -l "<topic keyword>" docs/13-decisions/
```

Specifically:
- Is there an ADR on this topic already? If so, does the user want to
  **supersede** it (new decision changes the previous one) or **amend**
  it (the existing one is fine, no new ADR needed)?
- Is the topic listed in the README's "Candidates to back-fill"? If so,
  remove that bullet when you create the ADR.

### Phase 3: Investigate the decision

Before drafting, gather context:

1. **What forces are at play?** Performance, compatibility, ergonomics,
   external constraints (Mendix versions, library availability)?
2. **What alternatives were on the table?** A good ADR documents the
   roads not taken, with one sentence each on why they were rejected.
3. **Who is affected?** Contributors, end users, both?
4. **What evidence supports the decision?** Link to proposals, PRs,
   benchmarks, prior bugs.

If the user is back-filling an old decision, ask them to dig up the
relevant PR(s) or issue(s) for the context section. Don't synthesize
context from memory.

### Phase 4: Pick the number

The next ADR number is the highest existing number plus one. No gaps,
no reuse. Confirm by listing the folder:

```bash
ls docs/13-decisions/ | grep -E '^[0-9]{4}-' | sort | tail -1
```

### Phase 5: Draft the ADR

Create `docs/13-decisions/NNNN-<short-slug>.md` using the template from
the README:

```markdown
# ADR-NNNN: <Decision title>

- **Status**: Proposed
- **Date**: <today>
- **Deciders**: <optional>
- **Related**: <optional — proposal #, PR #, issue #>

## Context

<What forces are at play? What problem? Why now?>

## Decision

<One paragraph — the decision itself, not the justification.>

## Consequences

<Positive, negative, neutral. Be honest about the downsides.>

## Alternatives considered

<What else was on the table, and why was it not chosen?>
```

Slug guidance:
- Names the **decision**, not the topic. `backend-abstraction` (decision)
  is good; `executor` (topic) is too broad.
- Lowercase, hyphenated, short — 2–5 words usually.

### Phase 6: Wire cross-references

After drafting, propose these edits (do not apply without asking):

1. **README index** — add the new ADR row to the table.
2. **README back-fill list** — remove the bullet if back-filling.
3. **CLAUDE.md** — if the ADR introduces a load-bearing rule, add a
   one-line pointer: `See ADR-NNNN: <slug>.`
4. **Wiki** — if a `rationale/` page is affected, queue a
   `/mxcli-dev:wiki-sync rationale/<page>.md` run with the new ADR in
   `sources:`.

### Phase 7: Confirm with user

Show the user:
- The drafted ADR.
- The proposed cross-reference edits.
- A note that ADR status starts as **Proposed**; flip to **Accepted**
  when the team signs off (typically in the PR review).

If confirmed, commit the ADR + cross-reference edits in one commit.

## Important reminders

- **ADRs are immutable once Accepted.** Typo fixes only — content changes
  go in a new ADR that supersedes the old one.
- **One decision per ADR.** If the user describes two decisions, write
  two ADRs.
- **Be honest about consequences.** A future contributor weighing whether
  to revisit needs the real downsides, not a marketing summary.
- **Don't bulk back-fill.** The README has a candidate list, but each
  back-fill needs real research to capture context accurately. Drive-by
  ADRs without context are worse than no ADRs.
