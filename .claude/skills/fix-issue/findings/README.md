# Findings

One JSON object per line, sharded by area. 630 records, extracted from the
symptom table that used to live inside `fix-issue.md` — see that file for how to
search them and how to add one.

## Why JSONL, and why sharded

The table had grown to 1.05 MB. That is past a context window, past what
GitHub's web editor will open, and past what the wiki's own `wiki-sync` can
consume (its Phase 2 requires reading a source in full) — so the digest step that
turns findings into `docs-wiki/bug-patterns/` pages had been blocked since the
day it was created.

Line-oriented records fix all three at once. A shard is small enough to open and
grep; `merge=union` is *correct* on a file of independent records, where on the
old mixed prose-and-table file it was a hazard; and a query can pull one area at
a time instead of the whole corpus.

Nine shards rather than one file per finding: 630 files would trade a
too-large file for a directory nobody can scan, and the conflicts that motivated
sharding only happen between fixes touching the *same* area.

## Fields

| field | |
|---|---|
| `area` | package the fix landed in, e.g. `mdl/executor`. Decides the shard |
| `date` | when the finding was last touched, `YYYY-MM-DD`. Backfilled by `git blame` over the table this came from, so it is *last touched*, not *first written* — a row that was later corrected carries the correction's date. Used by `make digest-status` to measure how far the wiki digest has fallen behind |
| `symptom` | what the user saw — the thing you match against |
| `cause` | the mechanism |
| `file` | where to look first |
| `insight` | what would have made it cheaper to find; the part worth reading |
| `refs`, `ce`, `rules` | issue/PR refs, Mendix `CE####` codes, MDL rule ids — extracted from the text, present only where the text carried them |
| `raw` | the original table row, for the 68 records whose row could not be split into four columns (an unescaped `\|` in the text, or a row that never had four cells). `symptom`/`cause`/`file`/`insight` are absent on these |

A record has **either** the four structured fields **or** `raw`. Queries that
must cover everything need to account for both.

## Guard

`make check-findings` (`scripts/check-findings.sh`) validates every line: one
JSON object, an `area`, a `date`, and either the four fields or `raw`. It prints
one line saying how far `docs-wiki/bug-patterns/` has fallen behind these
findings — see `make digest-status` for the breakdown.

It runs in CI as of the change that added this sentence. It did not before,
for the two weeks this file claimed it did — which is the same shape as every
other finding here, so it is recorded rather than quietly corrected.

The extraction was verified lossless by regenerating every table row from the
records and diffing against the original 630 — byte-identical as a multiset.
Order carries no meaning: these are looked up by matching a symptom.

## On DuckDB

The files are newline-delimited JSON, which DuckDB reads in place over a glob —
no import, no schema, no build step. It is not vendored and not required: `grep`
plus `jq` covers the common lookup, and the guard above only needs `python3`.
