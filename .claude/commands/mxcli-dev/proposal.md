---
description: Create a structured feature proposal in docs/11-proposals/
argument-hint: <feature name>
---

# /mxcli-dev:proposal — Create Feature Proposal

Guide the contributor through creating a well-structured feature proposal for
mxcli. The proposal goes in `docs/11-proposals/PROPOSAL_<name>.md`.

## Process

Work through these phases **interactively** — ask the user questions, don't
guess. Each phase produces concrete output.

### Phase 1: Understand the Feature

Ask the user:

1. **What** do you want to add? (one sentence)
2. **Why** — what problem does this solve, or what user story does it enable?
3. **Which Mendix version** introduced this capability? (affects version-gating)
4. **Does this add MDL syntax?** (CREATE/ALTER/DROP/SHOW/DESCRIBE statements)
5. **Does this touch BSON serialization?** (reading or writing Mendix documents)

If the user isn't sure about version or BSON, help them find out:
- Version: check `reference/mendixmodellib/reflection-data/` or Mendix release notes
- BSON: check if similar features exist in `mdl/backend/modelsdk/*_read.go` or `*_write.go`

### Phase 2: BSON Investigation (if applicable)

**CRITICAL**: If the feature reads or writes Mendix documents, investigate the
BSON structure BEFORE designing syntax. Wrong assumptions here cause CE errors
in Studio Pro that are painful to debug.

1. **Find a working example** — ask the user:
   - "Do you have a test `.mpr` project with this feature already configured in Studio Pro?"
   - If yes: use `mxcli bson dump` or `mxcli bson discover` to extract the BSON structure
   - If no: ask them to create a minimal example in Studio Pro first

2. **Extract the BSON structure**:
   ```bash
   # Find the document type
   ./bin/mxcli -p project.mpr -c "SHOW STRUCTURE ALL" | grep -i <feature-name>

   # Dump the raw BSON
   ./bin/mxcli bson dump -p project.mpr --type "<BSON $Type>"

   # Or discover all instances of a type
   ./bin/mxcli bson discover -p project.mpr --pattern "<pattern>"
   ```

3. **Document the BSON structure** in the proposal:
   - Storage name (`$Type` field) — verify against `reference/mendixmodellib/reflection-data/`
   - All fields with types and observed values
   - Any fields that use Mendix-internal IDs (pointers to other documents)
   - Note any counter-intuitive naming (like Parent/Child pointer inversion)

4. **Check for storage-name vs qualified-name mismatches** — consult the table
   in CLAUDE.md under "BSON Storage Names vs Qualified Names". If the feature
   uses a type that has a known mismatch, document it prominently.

### Phase 3: Check for Overlap

Before writing the proposal, search for existing work:

```bash
# Existing proposals
ls docs/11-proposals/ | grep -i <feature>

# Existing implementations
grep -r "<feature>" mdl/executor/ mdl/backend/modelsdk/ --include="*.go" -l

# Existing test coverage
ls mdl-examples/doctype-tests/ | grep -i <feature>
```

If there's overlap, discuss with the user whether to extend the existing work
or start fresh.

### Phase 4: Design MDL Syntax (if applicable)

If the feature adds MDL statements, read `.claude/skills/design-mdl-syntax.md`
first, then design syntax that follows these principles:

- Uses standard verbs: `CREATE`, `ALTER`, `DROP`, `SHOW`, `DESCRIBE`
- Reads as English — a business analyst understands it
- Uses `Module.Element` qualified names everywhere
- Property format: `( Key: value, ... )` with colon separators
- One example is enough for an LLM to generate correct variants
- Adding one property is a one-line diff

Present the proposed syntax to the user and ask for feedback before proceeding.

### Phase 5: Write the Proposal

Create `docs/11-proposals/PROPOSAL_<snake_case_name>.md` with this structure:

```markdown
# Proposal: <Title>

**Status:** Draft
**Date:** <today's date>

## Problem Statement

<What problem does this solve? Who benefits?>

## BSON Structure

<Document the storage format. Include the $Type, all fields, pointer
relationships, and any gotchas. If not applicable, explain why.>

## Proposed MDL Syntax

<Show CREATE/ALTER/DROP/SHOW/DESCRIBE examples. If not MDL syntax,
describe the CLI commands or API changes.>

## Implementation Plan

<Which files need to change? What's the order of operations?>

### Files to modify/create

| File | Change |
|------|--------|
| `mdl/grammar/MDLParser.g4` | Add rule for ... |
| `mdl/ast/...` | New AST node |
| ... | ... |

## Version Compatibility

<Which Mendix version introduced this? Does it need version-gating?>

## Test Plan

<What test scripts go in mdl-examples/doctype-tests/? What roundtrip
tests are needed?>

## Open Questions

<What's unresolved? What needs user input or further investigation?>
```

### Phase 6: Confirm with User

Show the user the proposal and ask:
- "Does this look right?"
- "Anything I should add or change?"
- "Ready to commit?"

If confirmed, commit the proposal file.

---

## Important Reminders

- **Never guess BSON field names.** Always verify against a real `.mpr` file or
  the reflection data. Wrong field names cause silent data corruption.
- **Don't skip the BSON investigation** for features that touch Mendix documents.
  The proposal will be wrong without it, and the implementation will produce
  CE errors in Studio Pro.
- **Check existing proposals first.** The `docs/11-proposals/` directory has 30+
  proposals — some may cover the same ground or provide useful reference.
