# Fix Issue Skill

A fast-path workflow for diagnosing and fixing bugs in mxcli.

Each fix appends one finding to `findings/*.jsonl`. Those are **evidence, not
reading material**: 630 findings at ~1.7 KB each. The entry point for a diagnosis
is [`docs-wiki/bug-patterns/`](../../docs-wiki/bug-patterns/), which digests them
into failure classes; the findings are where you drill down for the instance.

## How to Use

1. **Start with the failure class.** Skim
   [`docs-wiki/bug-patterns/`](../../docs-wiki/bug-patterns/) — it is small, and it
   tells you which layer the bug lives in. Coverage is partial (three pages against
   630 findings), so a miss there means the finding has not been digested yet,
   **not** that it is new.
2. **Then find the instance** — see "Finding a prior fix" below. Go straight to the
   file the matching record names and follow its insight.
3. Write a failing test first, then implement.
4. **Verify at the layer the symptom lives in.** Parser → unit test. BSON we write →
   unit test on the encoded document. Files on disk after `mx` runs → integration test
   (`-tags integration`). The **rendered app's behaviour or appearance** → boot it and
   assert in a browser: [`verify-in-runtime.md`](./verify-in-runtime.md). A page can
   serialize to valid-looking BSON, pass `mx check`, build cleanly, and still render
   wrong.
5. **Prove the test detects the bug**: revert the fix and confirm it fails with the
   reported symptom. A test that has only ever run against fixed code has not been
   shown to detect anything.
6. **Build and run a real app if the fix's argument asserts what a Mendix tool
   accepts or rejects** — see "Two rules the checklist above does not cover".
   This fires on fixes step 4 does not catch: a version guard, a refusal, anything
   justified by "mxbuild/Studio Pro would (not) accept this".
7. After the fix: **append a finding** — see "Recording a fix" below.

> Before concluding that something **cannot be verified here**, read the second of
> those two rules. That claim ends the investigation, so it needs the evidence a
> fix would.

> **Why the findings are not in this file.** They were, as one Markdown table, and
> it reached 1.05 MB / 630 rows — past what fits in a context window and past what
> GitHub's web editor will open. It had also stopped being a table: only 227 of the
> 630 rows were still under the header, the other 403 having accreted in twelve
> separate runs through the document, most rendering as literal text.
>
> `.gitattributes` gave `merge=union` to the whole file, which kept two concurrent
> appends instead of conflicting. That was the right driver on the wrong unit: union
> applies file-wide, so two branches editing the same *prose* line would silently
> keep both. The note that shipped with it called this exact shot — "if that starts
> happening, split the table into its own file so `union` covers only append-only
> content" — and the union driver now sits on `findings/*.jsonl`, where every line is
> an independent record and keeping both sides is always correct.
>
> It never solved the conflicts anyway: GitHub's server-side merge does not run merge
> drivers, so every PR still had to merge `main` locally first. Sharding by area is
> what actually reduces them — two fixes now collide only when they touch the same
> area.

---

## Finding a prior fix

The findings live in `findings/*.jsonl`, one JSON object per line, sharded by
area. They are **data, not reading material**: 630 findings at ~1.7 KB each do
not fit in a context window, and the table they used to live in had grown past
the point where GitHub's web editor would open it.

Grep is the fast path — the shard name narrows it, and every line is
self-contained:

```bash
grep -il 'CE0463' .claude/skills/fix-issue/findings/*.jsonl        # which areas
grep -h  'CE0463' .claude/skills/fix-issue/findings/mdl-executor.jsonl | jq .
```

For anything with a shape to it, the records have fields — `area`, `symptom`,
`cause`, `file`, `insight`, plus `refs` / `ce` / `rules` where the text carried
them — and DuckDB reads the files in place, no import step:

```bash
duckdb -c "select symptom, file from '.claude/skills/fix-issue/findings/*.jsonl'
           where list_contains(ce, 'CE0463')"
```

A finding whose original row could not be split into four columns keeps its
`raw` line instead; `symptom` and friends are then absent, so filter on `raw` as
well when a query must cover everything.

`docs-wiki/bug-patterns/` is the layer above this: the *classes* of failure,
small enough to read. Start there, come here for the instance.

## Recording a fix

Append one line to the shard for the area you touched — a new shard is fine if
none fits. Keep it one line: `merge=union` in `.gitattributes` resolves two
concurrent appends by keeping both, which is correct for a file of independent
records and is not correct for prose.

```bash
cat >> .claude/skills/fix-issue/findings/mdl-executor.jsonl <<'JSON'
{"area":"mdl/executor","date":"2026-08-31","symptom":"...","cause":"...","file":"...","insight":"...","refs":["#123"]}
JSON
make check-findings
```

`check-findings` prints one line saying how far `docs-wiki/bug-patterns/` has
fallen behind; `make digest-status` breaks it down by area. **If the class of
failure keeps recurring, sync its pattern page** (`/mxcli-dev:wiki-sync
bug-patterns/<page>.md`). Nothing else will ask: the digest is on demand, and
every page in it was written on one day in May while the findings went on
accumulating — 607 of the 631 arrived afterwards. Neither number is a target to
drive to zero; they are there so the decision is made deliberately rather than
by default.

Write the **insight**, not the changelog: what would have made this cheaper to
find, what measurement settled it, and which plausible-sounding wrong turn to
skip. Position carries no meaning — these are looked up by matching a symptom,
never read in order.


---

## TDD Protocol

Never implement before the test exists:

```
Step 1: write a failing test at the layer the symptom lives in
Step 2: confirm it fails — and that it fails with the REPORTED symptom
Step 3: implement the minimum code to make it pass
Step 4: go test ./...   (or the packages you touched)
Step 5: append a finding to findings/<area>.jsonl
```

Where the test goes, by layer:

| Layer | Package |
|-------|---------|
| grammar / parse tree | `mdl/visitor/` |
| BSON we encode or decode | `modelsdk/codec/`, `modelsdk/mpr/` |
| backend mutation | `mdl/backend/modelsdk/` |
| executor handler | `mdl/executor/` (with `MockBackend`) |
| version gating | `sdk/versions/` |

`sdk/mpr` is **gone** — the legacy engine was deleted once its importer count
reached zero, so a test written there is a compile error rather than a wrong
answer. Earlier versions of this skill sent tests there.

---

## Two rules the checklist above does not cover

Both were paid for on mendixlabs/mxcli#1121, where everything else in this skill
was followed and the fix still shipped with a false claim in its PR body.

### Run the real thing when the argument is about what a Mendix tool accepts

`verify-in-runtime.md` asks whether the symptom is a property of the *running
app*. That is the right question for a rendering bug (#812) and it does not fire
here: #1121 was a version gate, and its unit tests were sound — they proved the
refusal was gone and the right keys were written.

What they could not touch is the claim the fix *rests* on. The guard omits two
11.5-only properties below 11.5 because writing them is supposed to be unsafe.
Is it? Nothing in the test suite can say. Building a real 10.24.25 app with the
keys forced back in answers it in one run:

```
The app contains: 0 errors.
```

mxbuild accepts them silently — which is what makes the guard load-bearing
rather than decorative, and is not something any amount of reasoning establishes.

**So: add a full run whenever the fix's justification asserts that a Mendix tool
would (or would not) catch something.** `mx check` tolerating a malformed
document is the most common shape of this, and it is exactly the case where a
green build is mistaken for evidence. Cheapest form: two copies of a real
project, one with the fix and one with the fault forced back in, `mxcli docker
check` on both.

### "Cannot be verified here" is a claim, and needs the same evidence as a fix

It is the claim that *ends* an investigation, so it gets the least scrutiny and
does the most damage.

On #1121: `mxbuild-10.24.25.tar.gz` 404'd. So did a dozen sibling 9.x and 10.x
versions. 11.6.0, 11.12.1 and 11.13.0 all returned 200 from the same host and
the same path. The conclusion — Mendix 10 is not downloadable from this
environment — went into a PR body as fact.

It was false. Mendix 9 and 10 publish **four-part** artifact names carrying a
build number: the real file is `mxbuild-10.24.25.122571.tar.gz`. One CDN listing
call (`?list-type=2&prefix=runtime/mxbuild-10.24.`) would have shown it, and did,
the moment someone asked the right question. `mxcli` now resolves this itself.

The tell was in the shape of the evidence: **a negative that is uniform across an
entire class is evidence about the query, not about the class.** Nothing about
"Mendix 10 was withdrawn from the CDN" predicts that *every* 10.x patch fails
identically while every 11.x succeeds; a wrong filename predicts exactly that.

Before writing "cannot":

1. Say what a positive result would have looked like.
2. Check the probe could have produced one — try the thing you are sure works
   (the 11.x control), and try a different *shape* of query, not another value.
3. If it still holds, state the limitation with the evidence attached, so the
   next reader can attack it. Do not state it as a property of the environment.

---

## After Every Fix — Checklist

- [ ] Failing test written before implementation, at the layer the symptom lives in
- [ ] Test proven to detect the bug: revert the fix, confirm it fails with the reported symptom
- [ ] Full run done if the fix's argument asserts what a Mendix tool accepts or rejects
- [ ] Any "cannot be verified" claim carries its evidence, and a control that could have falsified it
- [ ] `make test && make lint` pass
- [ ] `mdl-examples/bug-tests/<issue>-<description>.mdl` added for the regression case
- [ ] New finding appended to `findings/<area>.jsonl` (if not already covered), and `make check-findings` passes
- [ ] PR title: `fix: <one-line description matching the symptom>`
