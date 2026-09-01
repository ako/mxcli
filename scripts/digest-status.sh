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

# Last bug-pattern sync: SYNC_LOG.md is the audit trail and the authority.
log = open("docs-wiki/SYNC_LOG.md").read()
dates = re.findall(r"^\| (\d{4}-\d{2}-\d{2}) \| bug-patterns/", log, re.M)
last = max(dates) if dates else "0000-00-00"
pages = sorted(glob.glob("docs-wiki/bug-patterns/*.md"))

areas = collections.Counter()
since = collections.Counter()
undated = 0
for fn in glob.glob(".claude/skills/fix-issue/findings/*.jsonl"):
    for line in open(fn):
        r = json.loads(line)
        a = r.get("area", "unfiled")
        areas[a] += 1
        d = r.get("date")
        if not d:
            # An undated finding is one this report cannot place. Counting it as
            # digested is the failure mode that matters: the headline would read
            # "0% outstanding" while a whole area went undigested. 249 records
            # reached the corpus that way before the date field was required.
            undated += 1
            since[a] += 1
        elif d > last:
            since[a] += 1

total, new = sum(areas.values()), sum(since.values())

if brief:
    if new:
        print("digest: %d of %d findings not yet digested (last bug-pattern sync %s, %d pages)"
              " — /mxcli-dev:wiki-sync bug-patterns/" % (new, total, last, len(pages)))
    sys.exit(0)

print("Bug-pattern digest status\n")
print("  pattern pages      %d" % len(pages))
print("  last sync          %s" % last)
print("  findings           %d" % total)
print("  not yet digested   %d  (%.0f%%)" % (new, 100.0 * new / total if total else 0))
if undated:
    print("    of which undated %d  (counted as not digested)" % undated)

# Which areas does the existing digest even mention? A heuristic, deliberately:
# the alternative is a covers: list in every page's frontmatter, which is one
# more thing to keep true and would silently rot the moment someone forgot it.
text = "\n".join(open(p).read() for p in pages)
def mentioned(a):
    # Only ask the question for a real package path. A bare first segment like
    # "model" or "scripts" matches as a substring of almost any prose and the
    # answer would be a false yes.
    if "/" not in a:
        return "-"
    return "yes" if a in text else "NO"

CUT = 5
head = [(a, n) for a, n in areas.most_common() if n >= CUT]
tail = [(a, n) for a, n in areas.most_common() if n < CUT]
print("\n  %-22s %9s %9s   %s" % ("area", "findings", "since", "in a page"))
for a, n in head:
    print("  %-22s %9d %9d   %s" % (a, n, since[a], mentioned(a)))
if tail:
    print("  %-22s %9d %9d   %s" % ("(%d areas < %d)" % (len(tail), CUT),
                                    sum(n for _, n in tail),
                                    sum(since[a] for a, _ in tail), "-"))

print("\n  Pages: %s" % ", ".join(p.split("/")[-1] for p in pages))
print("\n  This is advisory. Sync a page when its area has moved, or when a class of\n"
      "  failure keeps recurring — not to drive a number to zero.")
PY
