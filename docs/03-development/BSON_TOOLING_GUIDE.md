# BSON Tooling Guide

How to use mxcli's BSON tools when adding support for a new Mendix document type or debugging serialization issues. This guide covers which tool to use at each stage.

## Tool inventory

| Tool | Command | Best for |
|------|---------|----------|
| **bson dump** | `mxcli bson dump` | Seeing raw BSON structure of any document |
| **bson compare** | `mxcli bson compare` | Diffing two documents (same or cross-project) |
| **bson discover** | `mxcli bson discover` | Finding which BSON fields your MDL covers |
| **TUI** | `mxcli tui -p app.mpr` | Interactive browsing and side-by-side comparison |
| **Python bson diff** | See debug-bson.md | Finding crash-causing extra fields across all documents |
| **mx check** | `mx check app.mpr` | Validating BSON output against Mendix runtime |

## Part 1: Adding a new document type

### Stage 1 -- Understand the BSON structure

You need a reference project that has the document type created in Studio Pro.

**List all instances of a type:**

```bash
mxcli bson dump -p app.mpr --type workflow --list
```

Output: names of all workflows in the project. Supported types: `page`, `microflow`, `nanoflow`, `enumeration`, `snippet`, `layout`, `constant`, `workflow`, `imagecollection`, `javaaction`, `javascriptaction`, `entity`, `association`.

**Dump a single document as JSON:**

```bash
mxcli bson dump -p app.mpr --type workflow --object "Module.MyWorkflow"
```

This shows the complete BSON tree with `$type`, `$ID`, nested objects, and arrays. Use this output to understand which fields exist and how they're structured.

**Dump as NDSL (normalized DSL) for a cleaner view:**

```bash
mxcli bson dump -p app.mpr --type workflow --object "Module.MyWorkflow" --format ndsl
```

NDSL renders the BSON as a structured text format that's easier to scan: type headers, alphabetized fields, array markers. Useful for sharing with others or feeding to an LLM.

**When to use Python instead:** Only if you need to scan across ALL documents in a project (e.g., collecting every `$type` and its fields) or if the type isn't supported by `bson dump`. See the Python script in `.claude/skills/debug-bson.md`.

### Stage 2 -- Compare against a known-good baseline

After you've written parser/writer code and created a document via MDL, compare your output against a Studio Pro-created reference.

**Same project, two different documents:**

```bash
mxcli bson compare -p app.mpr --type workflow StudioProWorkflow MdlWorkflow
```

**Cross-project comparison (reference vs your output):**

```bash
mxcli bson compare -p reference.mpr -p2 test.mpr --type workflow MyWorkflow
```

The diff output shows:
- **OnlyInLeft**: fields present in the reference but missing from your output (you need to add these)
- **OnlyInRight**: fields you're writing that shouldn't be there (remove these)
- **ValueMismatch**: fields with different values (check defaults)

By default, structural fields (`$ID`, `PersistentId`, `RelativeMiddlePoint`, `Size`) are skipped. Use `--all` to include them.

**NDSL diff format:**

```bash
mxcli bson compare -p reference.mpr -p2 test.mpr --type workflow MyWorkflow --format ndsl
```

Shows the two documents side by side in NDSL format instead of a structured diff.

### Stage 3 -- Check field coverage

After implementing DESCRIBE for a type, check how many BSON fields your MDL output covers:

```bash
mxcli bson discover -p app.mpr --type workflow
mxcli bson discover -p app.mpr --type workflow --object "Module.MyWorkflow"
```

Output shows per-`$type` coverage:
- How many instances exist
- Each field's status: covered (appears in MDL output), uncovered, or default value
- Sample values for uncovered fields
- Fields categorized as semantic, structural, or layout

Use this to find fields you forgot to handle in your DESCRIBE implementation, or to prioritize which fields to support next.

### Stage 4 -- Interactive exploration with the TUI

For browsing and comparing documents interactively:

```bash
mxcli tui -p app.mpr
```

- Navigate the project tree to find your document
- Press **`b`** to view raw BSON in NDSL format
- Press **`c`** to enter compare mode, then:
  - **`1`** for NDSL vs NDSL (raw BSON side by side)
  - **`2`** for NDSL vs MDL (BSON vs your DESCRIBE output)
  - **`3`** for MDL vs MDL
- Press **`s`** to toggle synchronized scrolling
- Press **`D`** for a diff view

The TUI is best for exploratory work -- when you're browsing multiple documents to understand patterns, or visually comparing your MDL output against the raw BSON to spot gaps.

### Stage 5 -- Validate with mx check

After writing BSON, always validate:

```bash
# if mx is cached from setup
mxcli docker check -p app.mpr

# or via mxcli
mxcli docker check -p app.mpr
```

`mx check` is the definitive validator. If it passes, Studio Pro will accept the document. If it fails, note the error code -- common ones are documented in `.claude/skills/debug-bson.md`.

## Part 2: Debugging BSON issues

### Symptom: Studio Pro crashes on open

**Error:** `System.InvalidOperationException: Sequence contains no matching element` at `MprProperty..ctor`

**Cause:** Your BSON contains a field that doesn't exist on the `$type`. Studio Pro's type cache crashes on unrecognized fields.

**Diagnosis with Python** (best tool for this case -- you need to diff ALL fields across all types):

```python
import bson, os
from collections import defaultdict

type_props = defaultdict(set)

def walk_bson(obj, tp):
    if isinstance(obj, dict):
        t = obj.get("$type", "")
        if t:
            for k in obj.keys():
                if k not in ("$type", "$ID"):
                    tp[t].add(k)
        for v in obj.values():
            walk_bson(v, tp)
    elif isinstance(obj, list):
        for item in obj:
            walk_bson(item, tp)

# Collect from your broken project
for root, dirs, files in os.walk("broken-project/mprcontents"):
    for f in files:
        if f.endswith(".mxunit"):
            with open(os.path.join(root, f), "rb") as fh:
                walk_bson(bson.decode(fh.read()), type_props)

# Collect from a known-good project the same way (baseline_props)
# then compare:
for t, props in type_props.items():
    if t in baseline_props:
        extra = props - baseline_props[t]
        if extra:
            print(f"{t}: EXTRA props = {sorted(extra)}")
```

**Fix:** Remove the extra fields from your writer function.

**Why Python and not `bson compare`?** Because the crash could be caused by ANY document in the project, not a specific one you know about. The Python script scans everything.

### Symptom: mx check reports CE errors

**Use `bson compare` to find the difference:**

```bash
# create the same element in Studio Pro, then compare
mxcli bson compare -p studio-pro.mpr -p2 mdl-generated.mpr --type page MyPage
```

The diff shows exactly which fields differ. Common causes:

| Error | Typical cause | Fix |
|-------|--------------|-----|
| CE0463 | Widget property values inconsistent with mode rules | Check editorConfig.js visibility rules |
| CE0642 | Required property missing | Add the missing field with its default value |
| CE1613 | Association stored as attribute (or vice versa) | Check resolveMemberChange logic |
| CE0066 | Access rule on wrong entity | Check ParentPointer/ChildPointer semantics |

### Symptom: DESCRIBE output doesn't roundtrip

Your DESCRIBE output should be re-parseable by `mxcli check` and produce the same document when executed.

**Use the TUI to compare NDSL vs MDL:**

```bash
mxcli tui -p app.mpr
# Navigate to the document, press c, then 2 for NDSL | MDL view
```

This shows the raw BSON on the left and your MDL output on the right. Scan for BSON fields that don't appear in the MDL -- those are gaps in your DESCRIBE implementation.

**Or use `bson discover` for a quantitative check:**

```bash
mxcli bson discover -p app.mpr --type workflow --object "Module.MyWorkflow"
```

## Decision tree: which tool to use

```
What are you trying to do?
|
|-- "See the BSON structure of a document"
|     --> mxcli bson dump --format json (or ndsl)
|
|-- "Compare my output against Studio Pro's"
|     --> mxcli bson compare -p2 reference.mpr
|
|-- "Find which BSON fields my DESCRIBE misses"
|     --> mxcli bson discover
|
|-- "Browse and explore interactively"
|     --> mxcli tui (press b for BSON, c for compare)
|
|-- "Find crash-causing fields across ALL documents"
|     --> Python bson diff script (walk all .mxunit files)
|
|-- "Validate that my BSON output is correct"
|     --> mx check app.mpr
```

## Reflection data reference

The Mendix type definitions live in `reference/mendixmodellib/reflection-data/`. Each JSON file defines one metamodel domain with:
- Type names and their storage names (`$type` values)
- Properties with types, defaults, and whether they're required
- Inheritance hierarchy

Check these when you're unsure whether a field belongs on a type. For example, `DomainModels.json` shows that `ParentConnection` exists on `DomainModels$association` but not on `DomainModels$CrossAssociation`.

The generated Go metamodel in `generated/metamodel/types.go` mirrors these definitions and is used by `bson discover` for field coverage analysis.

## Related documentation

- [Implement MDL Feature](../../.claude/skills/implement-mdl-feature.md) -- full pipeline from investigation to testing
- [Debug BSON](../../.claude/skills/debug-bson.md) -- widget-specific debugging (CE0463, templates, mpk inspection)
- [PAGE_BSON_SERIALIZATION.md](PAGE_BSON_SERIALIZATION.md) -- page/widget BSON format reference
- [MDL_PARSER_ARCHITECTURE.md](MDL_PARSER_ARCHITECTURE.md) -- ANTLR parser pipeline
