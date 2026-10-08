#!/bin/bash
# check-findings.sh — validate .claude/skills/fix-issue/findings/<shard>/*.json
#
# One finding per file, one JSON object on one line: an `area`, a `date`, and
# either the four structured fields or a `raw` row. A malformed file is not
# cosmetic: the findings are read by grep and by DuckDB, and DuckDB rejects the
# whole glob on one bad file, so a typo in a new finding takes out every query.
#
# Findings used to be appended to one shared <shard>.jsonl per area. That made
# every merged fix-PR put every other open fix-PR into conflict, because GitHub's
# server-side merge does not run the merge=union driver that resolved it
# locally. A branch written before the split brings a <shard>.jsonl back on its
# next merge with main, so a stray one is refused here, with the converter named.

set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dir="$root/.claude/skills/fix-issue/findings"

if [ ! -d "$dir" ]; then
    echo "findings directory not found: $dir" >&2
    exit 1
fi

shopt -s nullglob
stray=("$dir"/*.jsonl)
if [ ${#stray[@]} -gt 0 ]; then
    for f in "${stray[@]}"; do echo "${f#$root/}: findings are one file per finding now" >&2; done
    echo "run scripts/split-findings.py to move these records into <shard>/<date>-<slug>.json files" >&2
    exit 1
fi

files=("$dir"/*/*.json)
if [ ${#files[@]} -eq 0 ]; then
    echo "no finding files in $dir/<shard>/" >&2
    exit 1
fi

python3 - "${files[@]}" <<'PY'
import json, os, re, sys

bad = 0
total = 0
name_re = re.compile(r"^\d{4}-\d{2}-\d{2}-[a-z0-9][a-z0-9-]*\.json$")
for path in sys.argv[1:]:
    rel = os.path.relpath(path)
    total += 1
    with open(path) as f:
        text = f.read()
    lines = text.splitlines()
    if not name_re.match(os.path.basename(path)):
        print("%s: name must be <YYYY-MM-DD>-<slug>.json (lowercase, digits, hyphens)" % rel); bad += 1
    if len(lines) != 1 or not text.endswith("\n"):
        # One line per record is what keeps `grep -h` returning whole records.
        print("%s: must be exactly one line (one JSON object) ending in a newline" % rel); bad += 1
        if not lines:
            continue
    try:
        rec = json.loads(text)
    except Exception as e:
        print("%s: not valid JSON: %s" % (rel, e)); bad += 1; continue
    if not isinstance(rec, dict):
        print("%s: not a JSON object" % rel); bad += 1; continue
    if not rec.get("area"):
        print("%s: missing `area`" % rel); bad += 1
    # A dateless finding is invisible to the digest report, which then
    # says the wiki is current when it is not. Required rather than
    # warned about: 249 records accumulated undated before this check
    # existed, and nobody saw the warning that was not there.
    if not rec.get("date"):
        print("%s: missing `date` (YYYY-MM-DD)" % rel); bad += 1
    structured = all(rec.get(k) for k in ("symptom", "cause", "file", "insight"))
    if not structured and not rec.get("raw"):
        print("%s: needs either symptom/cause/file/insight or raw" % rel)
        bad += 1

if bad:
    print("\n%d problem(s) across %d findings" % (bad, total), file=sys.stderr)
    sys.exit(1)
shards = len({os.path.basename(os.path.dirname(p)) for p in sys.argv[1:]})
print("findings OK: %d records in %d shards" % (total, shards))
PY

# The digest gap, one line, on every run. Advisory: a stale digest never fails
# this check. It is printed HERE because this is the command the After-Every-Fix
# checklist already runs — a report nobody invokes is how the digest fell three
# months behind in the first place.
"$(dirname "${BASH_SOURCE[0]}")/digest-status.sh" --brief || true
