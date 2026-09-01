#!/bin/bash
# check-wiki-pages.sh — the wiki's page list must describe the wiki.
#
# .claude/skills/maintain-wiki/pages.md is the table of contents, and it is
# hand-maintained: every sync is supposed to append a row. It drifted anyway —
# architecture/mcp-backend.md and models/ped-mutation-constraints.md were added
# in June 2026 and were still missing from it in September, which nothing
# noticed because a table of contents has no failure mode of its own.
#
# Checks BOTH directions. A page absent from the list is invisible to anyone
# choosing what to sync; a row naming no file is a page someone believes exists.

set -euo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

list=".claude/skills/maintain-wiki/pages.md"
bad=0

while IFS= read -r f; do
    rel="${f#docs-wiki/}"
    case "$rel" in README.md|SYNC_LOG.md) continue ;; esac
    grep -q "\`$rel\`" "$list" || { echo "$rel exists but is not in $list"; bad=1; }
done < <(find docs-wiki -name '*.md' | sort)

while IFS= read -r rel; do
    [ -e "docs-wiki/$rel" ] || { echo "$list names docs-wiki/$rel, which does not exist"; bad=1; }
done < <(grep '^| `' "$list" | sed 's/^| `\([^`]*\)`.*/\1/')

if [ "$bad" -ne 0 ]; then
    echo
    echo "The wiki page list and docs-wiki/ disagree. Add the row (or the page)." >&2
    exit 1
fi
echo "wiki pages OK: $(grep -c '^| `' "$list") listed, all present"
