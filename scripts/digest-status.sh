#!/bin/bash
# digest-status.sh — how far the bug-pattern digest has fallen behind the findings.
#
# The findings under .claude/skills/fix-issue/findings/ are raw evidence; the
# read layer is docs-wiki/bug-patterns/, which digests them into failure classes.
# That digest is produced on demand by /mxcli-dev:wiki-sync, and nothing demanded
# it: the After-Every-Fix checklist feeds the input and no step consumes it. The
# result was three pattern pages, all synthesised on one day in May, against a
# corpus that kept growing.
#
# The backlog is counted PER AREA, against the last sync of a page that CLAIMS
# that area in its `covers:` frontmatter — not against the newest sync anywhere.
# A global date made a sync of any one page reset the clock for every area:
# digesting mdl/catalog and mdl/linter (51 findings) took the headline from 547
# outstanding to 0 while 289 mdl/executor findings stayed undigested, and the
# number read as an achievement. A partial sync is the normal case, so a global
# date is wrong almost always.
#
# `covers:` is explicit because the two signals already in the tree cannot carry
# a date claim, and both failures were measured:
#
#   - matching the area path against page TEXT over-credits. keyword-collisions
#     lists `mdl/executor/identifier_quoting.go` — one Go file — in its sources,
#     so a sync of that page zeroed all 913 mdl/executor findings.
#   - matching the findings SHARD in `sources:` under-credits. Shards and the
#     `area` field are different groupings (a mdl/catalog record is filed in the
#     sdk shard), and three pages name no shard at all.
#
# An area no page claims reads as never digested, so all of its findings are
# outstanding. That is the deliberate direction: this report's job is to be
# believed when it says there is nothing left, and the same choice is already
# made for undated findings below. A page with no `covers:` is listed under
# "pages making no coverage claim" rather than guessed at.
#
# Two limits worth knowing, because both make the number look BETTER than
# reality and neither is worth over-engineering away:
#
#   - `date` is day-resolution, and the comparison is strictly greater-than, so
#     findings appended on the same day as a sync count as digested. Measured:
#     256 of mdl/executor's findings carry the sync's own date, ~250 of which
#     landed after the pages were written.
#   - `date` is now required (check-findings enforces it), but the 249 records
#     that predate that requirement were backfilled from `git blame` — which
#     that very backfill then invalidated for every line. The field is the only
#     record from here on; there is no second source to recover it from.
##   - an area claimed by several pages takes the NEWEST of their syncs. The
#     alternative — the oldest — would mean an area split across two pages never
#     reads as digested when either is refreshed. The per-area `last sync` is
#     printed so the claim is checkable.
#
# So read the per-area `in a page` column as the actionable signal and the
# counts as a hint. This makes the gap a number. It is ADVISORY and always exits 0 — a stale digest
# must not block an unrelated fix. The teeth are that check-findings prints the
# one-line form on every run, so the number is in front of whoever just appended.

set -euo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

python3 - "${1:-}" <<'PY'
import glob, json, re, sys, collections

brief = sys.argv[1] == "--brief" if len(sys.argv) > 1 else False

# SYNC_LOG.md is the audit trail and the authority. Read it per PAGE: each row
# names the page it synced, so the newest row for a page is when that page last
# digested anything.
log = open("docs-wiki/SYNC_LOG.md").read()
rows = re.findall(r"^\| (\d{4}-\d{2}-\d{2}) \| (bug-patterns/\S+?\.md) \|", log, re.M)
page_synced = {}
for d, page in rows:
    name = page.split("/")[-1]
    if d > page_synced.get(name, ""):
        page_synced[name] = d
pages = sorted(glob.glob("docs-wiki/bug-patterns/*.md"))
page_text = {p.split("/")[-1]: open(p).read() for p in pages}

# Newest sync anywhere — context only, and labelled as such in the output. It is
# NOT what the backlog is measured against; see the header.
last = max((d for d, _ in rows), default="0000-00-00")

# `covers:` is each page's own claim about which findings areas it digests. It is
# read from the frontmatter block, which ends at the closing `---`, so a path
# appearing later in the prose cannot be mistaken for a claim.
def covers_of(text):
    # The frontmatter is the block between the leading `---` and the next one.
    # Splitting on "\n---" puts the OPENING delimiter in element 0 (it has no
    # preceding newline), so the frontmatter is element 0 with its own first
    # line dropped — not element 1, which is the body. Getting this wrong read
    # the prose instead and every page reported no claim.
    if not text.startswith("---\n"):
        return []
    block = text[4:].split("\n---", 1)[0]
    m = re.search(r"^covers:\n((?:[ \t]*-[^\n]*\n?)+)", block, re.M)
    if not m:
        return []
    return [ln.strip().lstrip("-").strip() for ln in m.group(1).splitlines() if ln.strip()]

page_covers = {n: covers_of(t) for n, t in page_text.items()}
unclaimed_pages = sorted(n for n, c in page_covers.items() if not c)

def pages_claiming(a):
    return [n for n, c in page_covers.items() if a in c]

def area_last_sync(a):
    """When this area was last digested: the newest sync of a page claiming it.

    An area no page claims has never been digested, so it gets the zero date and
    every one of its findings counts as outstanding — which is the point."""
    return max((page_synced.get(n, "0000-00-00") for n in pages_claiming(a)),
               default="0000-00-00")

areas = collections.Counter()
for fn in glob.glob(".claude/skills/fix-issue/findings/*/*.json"):
    for line in open(fn):
        areas[json.loads(line).get("area", "unfiled")] += 1

area_last = {a: area_last_sync(a) for a in areas}

since = collections.Counter()
undated = 0
for fn in glob.glob(".claude/skills/fix-issue/findings/*/*.json"):
    for line in open(fn):
        r = json.loads(line)
        a = r.get("area", "unfiled")
        d = r.get("date")
        if not d:
            # An undated finding is one this report cannot place. Counting it as
            # digested is the failure mode that matters: the headline would read
            # "0% outstanding" while a whole area went undigested. 249 records
            # reached the corpus that way before the date field was required.
            undated += 1
            since[a] += 1
        elif d > area_last[a]:
            since[a] += 1

total, new = sum(areas.values()), sum(since.values())

if brief:
    if new:
        # Name the areas rather than one date: the backlog is per area, and a
        # single "last sync" here read as what it was measured against.
        items = ["%s %d" % (a, since[a]) for a, _ in areas.most_common() if since[a]]
        shown, rest = items[:3], len(items) - 3
        worst = ", ".join(shown) + (" and %d more areas" % rest if rest > 0 else "")
        print("digest: %d of %d findings not yet digested across %d pattern pages"
              " (%s) — /mxcli-dev:wiki-sync bug-patterns/"
              % (new, total, len(pages), worst))
    sys.exit(0)

print("Bug-pattern digest status\n")
print("  pattern pages      %d" % len(pages))
print("  newest sync        %s  (any page; the backlog below is per area)" % last)
print("  findings           %d" % total)
print("  not yet digested   %d  (%.0f%%)" % (new, 100.0 * new / total if total else 0))
if undated:
    print("    of which undated %d  (counted as not digested)" % undated)

def mentioned(a):
    if "/" not in a:
        return "-"
    return "yes" if pages_claiming(a) else "NO"

def shown_last(a):
    # "-" where the question does not apply, so a bare segment is not reported
    # as never-digested when it is only unanswerable.
    if "/" not in a:
        return "-"
    d = area_last[a]
    return "never" if d == "0000-00-00" else d

CUT = 5
head = [(a, n) for a, n in areas.most_common() if n >= CUT]
tail = [(a, n) for a, n in areas.most_common() if n < CUT]
print("\n  %-22s %9s %12s  %-10s  %s"
      % ("area", "findings", "outstanding", "last sync", "claimed"))
for a, n in head:
    print("  %-22s %9d %12d  %-10s  %s" % (a, n, since[a], shown_last(a), mentioned(a)))
if tail:
    print("  %-22s %9d %12d  %-10s  %s" % ("(%d areas < %d)" % (len(tail), CUT),
                                           sum(n for _, n in tail),
                                           sum(since[a] for a, _ in tail), "-", "-"))

if unclaimed_pages:
    print("\n  Pages making no coverage claim (add `covers:` on their next sync — until\n"
          "  then the areas they digest read as outstanding):")
    for n in unclaimed_pages:
        print("    %s" % n)

print("\n  Pages: %s" % ", ".join(p.split("/")[-1] for p in pages))
print("\n  This is advisory. Sync a page when its area has moved, or when a class of\n"
      "  failure keeps recurring — not to drive a number to zero.")
PY
