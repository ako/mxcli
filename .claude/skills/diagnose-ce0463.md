# Diagnosing CE0463 "the definition of this widget has changed"

CE0463 names the *widget version* but is caused by almost anything in the widget's
stored BSON. That mismatch between what the error says and what it means is why this
class of bug burns time. Work in this order — each step is cheap and eliminates a
whole family of causes.

Related: [`debug-bson.md`](debug-bson.md) for the general BSON diff workflow;
[`WIDGET_BSON_VERSION_COMPATIBILITY.md`](../../docs/03-development/WIDGET_BSON_VERSION_COMPATIBILITY.md)
for what is version-fragile and the per-minor onboarding record.

## Before you measure anything: `docker check` clears CE0463 by default

`mxcli docker check` runs **`mx update-widgets` before `mx check`**, deliberately — it
normalizes pluggable widget definitions to suppress the Case A noise below. So the plain
command reports **0 errors** on a project that genuinely has a CE0463, and the repair
also *mutates the project*, leaving it clean for any later check too.

```bash
mxcli docker check -p app.mpr --no-update-widgets    # the only form that can see CE0463
```

This is documented in `docker check --help` and is not a defect, but it is the first
thing that goes wrong in a CE0463 investigation: you check, you see zero errors, and you
conclude there is nothing to find. Measured on 11.13.0: a page mxcli wrote reported
`0 errors / Project check passed` with the default and **1 error, CE0463** with the flag.

**`mxcli check` is not an alternative.** It validates an MDL *script* — syntax,
references, creation order, expression types — and never invokes mxbuild, so it cannot
produce CE0463 at all.

## Step 0 — Establish which of two bugs you have

These look identical in `mx check` output and have unrelated causes.

| | Case A: package upgraded | Case B: authored fresh and still wrong |
|---|---|---|
| Trigger | widget package changed *after* the widgets were created | widgets created *against the current* package |
| Cause | a stored instance carries a property the new definition dropped | mxcli emits something the package does not accept |
| Owner | **not an mxcli bug** — this is what "Update all widgets" is for | mxcli |

**Two controls settle it. Neither is optional.**

1. **Do Studio Pro's own widgets fail too?** A blank project ships `dataGrid2_*`,
   `gallery1/2`, `drop_downFilter1/2` on its template pages. If those fail alongside
   yours, the tool is not the variable.
2. **Does `mx update-widgets` clear it?** If yes, the BSON was structurally valid and
   correct for the version it was written against → Case A. Genuine template bugs do
   *not* clear this way (the Image stale default and the number-filter markerless
   array both needed template fixes).

```bash
cp -r proj proj-ref && mx update-widgets proj-ref/App.mpr   # ALWAYS on a copy
mx check proj-ref/App.mpr | grep -c CE0463
```

`mx update-widgets` **destroys `mprcontents/`** on MPR v2, collapsing the project to
the v1 single-file layout. Only ever run it on a throwaway copy.

## Step 1 — Measure against an untouched control

The doctype fixtures live in a *blank project whose own widgets are already failing*
for Case A reasons. A raw CE0463 count therefore mixes both bugs.

```bash
# control: same project, same package, mxcli never ran
mx check control/App.mpr | grep CE0463 | sed 's/.*at //' | sort -u > /tmp/base.txt
mx check proj/App.mpr    | grep CE0463 | sed 's/.*at //' | sort -u > /tmp/mine.txt
comm -13 /tmp/base.txt /tmp/mine.txt      # ← the only failures that are yours
```

Skipping this produces a confident wrong answer. It did during #716: counting raw
totals in a blank project attributed template widgets to mxcli and led to
"DataGrid2 is clean" — which a real project immediately disproved.

## Step 2 — Exhaustive path diff FIRST, before any hypothesis

Do not pattern-match against previous CE0463 fixes until you know the diff. Flatten
both widget subtrees to `path → value` maps and list every difference. This takes
minutes and usually bounds the problem to a handful of paths.

```python
def flat(o, path, out):
    if isinstance(o, dict):
        for k, v in sorted(o.items()):
            if k == '$ID': continue          # regenerated UUIDs, never meaningful
            flat(v, path + '/' + k, out)
    elif isinstance(o, list):
        for i, v in enumerate(o): flat(v, path + '[%d]' % i, out)
    else:
        out[path] = '<bin>' if isinstance(o, bytes) else o
```

Diff **the whole widget node**, not just `Type` and `Object` — `Appearance`,
`LabelTemplate` and the other sibling fields live above them and have differed in
practice.

## Step 3 — Patch each difference in isolation, then in combination

Apply one candidate to the stored BSON, re-run `mx check`, revert. A difference that
does not move the count is not the cause, however plausible it looks. Then apply the
survivors together — CE0463 compares the whole widget, so a combination can matter
where no single item does.

Apply to the **failing widget only**. A project-wide patch changes other widgets and
makes the count unreadable.

## Step 4 — Bisect by splice when the diff comes up empty

Replace mxcli's node with the reference node and confirm the error clears; then
narrow. This proves *where* the cause is even when you cannot see *what* it is.

**Trap that produces a false positive:** swapping only `Type` or only `Object`
desynchronises `TypePointer` and makes the project fail to **load**. `mx check` then
prints `0 errors` because it never got far enough to check anything. Always read the
output tail, never just the error count:

```bash
mx check proj/App.mpr | tail -3     # "The app contains: N errors." or a stack trace?
```

## What normalising hides

Every convenience in a comparison script is a place the answer can hide. During #716
the following were all confirmed identical to the reference, and the cause was still
somewhere the tooling flattened:

| Normalisation | What it masks |
|---|---|
| dropping `$ID` | pointer *identity* — check `TypePointer` → `PropertyKey` mapping separately |
| `bytes → '<bin>'` | every binary field value (attribute/entity refs, pointers) |
| `sorted(o.items())` | BSON key order — a documented CE0463 cause (`b1f4de3a`) |
| comparing sets not sequences | property ordering within `PropertyTypes` / `Properties` |

When a structural comparison says "identical" but the behaviour differs, the answer
is in what you normalised. Go to bytes.

## Known cause families

Ordered by how often they have actually been the answer.

1. **A value, not the schema.** Four of the five CE0463 fixes in the v0.13 cycle were
   value-shaped while the error named the widget version: an empty `TextTemplate`
   header where Studio Pro wants the attribute name (`3cb8ab6`), an empty
   `Forms$ClientTemplate` where Studio Pro stores `null` (`455c43a`), placeholder
   `" "` ClientTemplates in object-list items (`4ea402c2`), an unset String as `" "`
   (`abba773`).
2. **A stale default in the embedded template** — the Image widget's
   `Atlas_Core.Content.Mendix` (`549c44f`).
3. **A markerless empty array** — `"Items": []` instead of `"Items": [3]`. Mendix
   ≤ 11.11 tolerates it, 11.12 fails the whole project load.
4. **Property-set drift against the package.** Real, but a weak predictor: `datagrid`
   needs 19 additions and 1 removal against Data Widgets 3.10 and passes, while
   `datagrid-dropdown-filter` is byte-for-byte in sync and fails.
5. **Augmentation never ran.** Before theorising about a widget's stored BSON,
   confirm the reconciliation reached it. `AugmentTemplate`'s "nothing to add or
   remove" guard returned before six value-level passes appended after it, so any
   widget whose property set already matched its package was emitted unreconciled
   (#716's drop-down filter). A one-line probe settles it — call `augmentFromMPK`
   directly and count `ValueTypes with AllowUpload`: 0/25 for the broken widget
   against 44/44 for a working one localised this in a single step.
6. **A mis-defaulted definition attribute in the `.mpk` parser.** The widget XML
   schema defaults `required` to **true**; reading a missing attribute as `false`
   put the wrong `Required` on 24 of DataGrid2 3.4's 40 properties. Two things make
   this family easy to miss: the value is only wrong for properties that *omit* the
   attribute (so the packages that spell it out look fine), and `sdk/widgets/mpk`
   and `modelsdk/widgets/mpk` are parallel copies — a fix in one leaves the other
   latent until something starts consuming the value. Grep the sibling package
   before concluding a defect is engine-specific.

## Rules of thumb

- **Read the tail, not the count.** `0 errors` can mean "did not load".
- **Assert the artifact exists before trusting a zero.** A run that exhausted disk
  created no widgets at all and scored a clean `mx check` — read as "the pre-fix
  binary is fine" and cost a wrong conclusion. `mxcli -p <proj> -c 'show widgets'
  | grep <name>` before every measurement.
- **Establish the control before the first measurement**, not after the tenth.
- **A hypothesis that survives only because you have not tested it is not evidence.**
  Patch it, measure, and write down the negative result — the elimination list is
  worth as much as the fix.
- **A difference from the reference is not automatically a cause.** With a synced
  DataGrid2's `Type` byte-identical to `mx update-widgets` output, 25 value-level
  differences remained. Three were patched in isolation — the `[3]` marker on empty
  `DesignProperties`, an explicit null `LabelTemplate`, and the `GridSortBar`
  `SortDirection`→`SortOrder` + marker migration — and **none moved the count**
  (33 → 33 each). Real differences, not the cause. Budget for this: the diff bounds
  the search, it does not rank it.
- **A value fix that is right for NEW properties is usually wrong applied to all.**
  `mx update-widgets` stores `TextTemplate: null` on a property it has just added,
  where mxcli's authoring path builds a populated `Forms$ClientTemplate` (correct
  there — an empty required textTemplate is CE4899). Nulling *every* TextTemplate
  value in a synced project took CE0463 from **33 to 127**: instances that
  legitimately carry a caption need it. Scope such a fix to the properties the
  operation itself introduced.
- **Test any candidate fix against the bundled package too.** Pruning the fields the
  `update-widgets` reference omits fixes 2 widgets on Data Widgets 3.10 and takes the
  bundled 3.4 from **0 → 139**.
