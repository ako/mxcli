#!/bin/bash
# check-findings.sh — validate .claude/skills/fix-issue/findings/*.jsonl
#
# One JSON object per line, an `area`, and either the four structured fields or
# a `raw` row. A malformed line is not cosmetic: the shards are read by grep and
# by DuckDB, and DuckDB rejects the whole file on one bad line, so a typo in an
# appended finding takes out every query over that area.

set -euo pipefail

dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/.claude/skills/fix-issue/findings"

if [ ! -d "$dir" ]; then
    echo "findings directory not found: $dir" >&2
    exit 1
fi

shopt -s nullglob
files=("$dir"/*.jsonl)
if [ ${#files[@]} -eq 0 ]; then
    echo "no findings shards in $dir" >&2
    exit 1
fi

python3 - "${files[@]}" <<'PY'
import json, os, sys

bad = 0
total = 0
for path in sys.argv[1:]:
    with open(path) as f:
        for n, line in enumerate(f, 1):
            if not line.strip():
                print("%s:%d: blank line" % (path, n)); bad += 1; continue
            total += 1
            try:
                rec = json.loads(line)
            except Exception as e:
                print("%s:%d: not valid JSON: %s" % (path, n, e)); bad += 1; continue
            if not isinstance(rec, dict):
                print("%s:%d: not a JSON object" % (path, n)); bad += 1; continue
            if not rec.get("area"):
                print("%s:%d: missing `area`" % (path, n)); bad += 1
            structured = all(rec.get(k) for k in ("symptom", "cause", "file", "insight"))
            if not structured and not rec.get("raw"):
                print("%s:%d: needs either symptom/cause/file/insight or raw" % (path, n))
                bad += 1

if bad:
    print("\n%d problem(s) across %d findings" % (bad, total), file=sys.stderr)
    sys.exit(1)
print("findings OK: %d records in %d shards" % (total, len(sys.argv) - 1))
PY

# The digest gap, one line, on every run. Advisory: a stale digest never fails
# this check. It is printed HERE because this is the command the After-Every-Fix
# checklist already runs — a report nobody invokes is how the digest fell three
# months behind in the first place.
"$(dirname "${BASH_SOURCE[0]}")/digest-status.sh" --brief || true
