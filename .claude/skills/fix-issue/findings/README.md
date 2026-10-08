# Findings

One file per finding, in a directory per area:
`<area>/<YYYY-MM-DD>-<slug>.json`, holding one JSON object on one line. The
records were extracted from the symptom table that used to live inside
`fix-issue.md` — see that file for how to search them and how to add one.

## Why one file per finding

The findings started as a Markdown table inside `fix-issue.md`, which grew to
1.05 MB: past a context window, past what GitHub's web editor will open, and
past what `wiki-sync` can consume. They then became nine `<area>.jsonl` shards,
one record per line, with `merge=union` in `.gitattributes` so two fixes
appending to the same shard kept both lines.

That fixed the size and did not fix the conflicts. GitHub's server-side merge
does not run merge drivers, so every fix-PR that appended to a shard conflicted
with every other open one the moment one of them merged — measured on
2026-10-08, eight open PRs re-conflicting after each merge, each needing a
local merge of `main`, a rebuild and a re-test. Sharding only lowered the odds;
almost every fix lands in `mdl-executor`. And `union` had quietly done damage:
six records were in that shard twice, byte for byte, from merges that kept both
sides of the same line. The split dropped the duplicates.

With one file per finding, two fixes add two different files. There is nothing
to merge, so nothing conflicts, on GitHub or anywhere else. The cost the shard
design was avoiding, "a directory nobody can scan", is not one: nobody scans it.
The findings are looked up by `grep` or DuckDB, both of which take a glob.

The split (`scripts/split-findings.py`) wrote each original line verbatim as a
file, so it is lossless by construction: the multiset of file contents equals
the multiset of unique original lines, checked byte for byte.

## Fields

| field | |
|---|---|
| `area` | package the fix landed in, e.g. `mdl/executor`. The directory is the coarse shard (`mdl-executor/`); `area` is the precise package |
| `date` | when the finding was last touched, `YYYY-MM-DD`. Also the file name's prefix when the file was written. The older records were backfilled by `git blame` over the table they came from, so for those it is *last touched*, not *first written*. Used by `make digest-status` to measure how far the wiki digest has fallen behind |
| `symptom` | what the user saw — the thing you match against |
| `cause` | the mechanism |
| `file` | where to look first |
| `insight` | what would have made it cheaper to find; the part worth reading |
| `refs`, `ce`, `rules` | issue/PR refs, Mendix `CE####` codes, MDL rule ids — extracted from the text, present only where the text carried them |
| `raw` | the original table row, for the records whose row could not be split into four columns (an unescaped `\|` in the text, or a row that never had four cells). `symptom`/`cause`/`file`/`insight` are absent on these |

A record has **either** the four structured fields **or** `raw`. Queries that
must cover everything need to account for both.

## Naming

`<date>-<slug>.json`: the record's `date`, then a slug of its symptom
(lowercase words joined by hyphens, about 60 characters at most). The name is
for a human skimming `ls`; nothing parses it beyond the pattern check. If the
name is taken, add `-2`.

## Guard

`make check-findings` (`scripts/check-findings.sh`, run in CI) validates every
file: the name pattern, exactly one line, one JSON object, an `area`, a `date`,
and either the four fields or `raw`. It also **refuses any `*.jsonl` left in
this directory**. A branch written before the split brings its shard file back
on its next merge with `main`; `scripts/split-findings.py` converts the lines
into files, skipping those that already exist. It prints one line saying how
far `docs-wiki/bug-patterns/` has fallen behind these findings; see `make
digest-status` for the breakdown.

## On DuckDB

DuckDB reads the files in place over a glob, with no import, schema or build
step: `select … from read_json_auto('.claude/skills/fix-issue/findings/*/*.json')`.
It is not vendored and not required: `grep` plus `jq` covers the common lookup,
and the guard above only needs `python3`.
