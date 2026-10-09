#!/usr/bin/env bash
# check-skill-mdl-semantics.sh — run every complete ```mdl / ```sql block in the
# given docs through the full `mxcli check`, and fail on any error it reports.
#
# check-skill-mdl.sh is the syntax gate: it splits blocks into statements and
# fails only on parse errors, on purpose, because a single statement lifted out
# of its block has lost its context. That left a hole: a whole example that
# parses and that `mxcli check` still rejects — `$response` created twice
# (MDL063, CE0111 at build), a view entity declaring String(400) over an OQL
# expression that returns String(200) (MDL031) — shipped marked as correct, and
# was found by a user who ran the pack through `mxcli check` themselves
# (mendixlabs/mxcli#1357). This gate runs that same check.
#
# The unit is the whole block, never a statement, so a variable declared in one
# statement and used in the next keeps its context. No project is loaded, so
# reference checks do not run: this is what `mxcli check script.mdl` alone
# reports, which is what a reader copying the example sees first.
#
# Skipped, because they are not complete scripts:
#   - a block that does not parse (MDL-SYNTAX) — a fragment, such as microflow
#     activities or an ALTER PAGE operation; check-skill-mdl.sh owns syntax;
#   - a ```sql block holding real SQL or raw OQL, which check refuses as
#     "produced no statements";
#   - a block marked as deliberately wrong (❌, -- BAD, INCORRECT, WRONG) or
#     opted out with `-- check-skip`.
# Ignored rules:
#   - MDL-DUPDEF: a block showing alternatives of one statement side by side
#     (the same demo user with and without `Entity:`) defines the name twice by
#     design. Nobody runs such a block as one script.
#
# Usage: scripts/check-skill-mdl-semantics.sh [mxcli-binary] [docs-dir-or-file]...
set -uo pipefail

MXCLI="${1:-bin/mxcli}"
shift || true
[ "$#" -gt 0 ] || set -- .claude/skills/mendix

if [ ! -x "$MXCLI" ]; then
	echo "error: mxcli binary not found or not executable: $MXCLI (run 'make build')" >&2
	exit 2
fi
command -v jq >/dev/null || { echo "error: jq is required" >&2; exit 2; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

FAILED=0
CHECKED=0
SKIPPED=0

while IFS= read -r md; do
	rm -f "$WORK"/block_* 2>/dev/null || true
	# One file per block, named after the line its body starts on.
	awk -v dir="$WORK" '
		/^```(mdl|sql)[ \t]*$/ { inb=1; fn=sprintf("%s/block_%06d.mdl", dir, NR+1); next }
		inb && /^```/ { inb=0; next }
		inb { print > fn }
	' "$md"

	for blk in "$WORK"/block_*.mdl; do
		[ -e "$blk" ] || continue
		line="$(basename "$blk" .mdl)"; line=$((10#${line#block_}))

		if grep -qE '❌|--[[:space:]]*BAD\b|\bINCORRECT\b|\bWRONG\b' "$blk" ||
			grep -qi 'check-skip' "$blk"; then
			SKIPPED=$((SKIPPED + 1)); continue
		fi

		json="$("$MXCLI" check --json "$blk" 2>"$WORK/stderr")"
		if ! printf '%s' "$json" | jq -e . >/dev/null 2>&1; then
			# Real SQL or raw OQL in a ```sql fence: not MDL at all, and check
			# says so instead of reporting. Anything else without a report (a
			# crash) is a failure.
			if grep -q 'produced no statements' "$WORK/stderr"; then
				SKIPPED=$((SKIPPED + 1)); continue
			fi
			FAILED=1
			echo "FAIL: $md:$line — mxcli check produced no JSON report"
			continue
		fi
		if printf '%s' "$json" | jq -e '[.violations[]? | select(.ruleId == "MDL-SYNTAX")] | length > 0' >/dev/null; then
			SKIPPED=$((SKIPPED + 1)); continue
		fi

		CHECKED=$((CHECKED + 1))
		errs="$(printf '%s' "$json" | jq -r '.violations[]?
			| select(.severity == "error" and .ruleId != "MDL-DUPDEF")
			| "    [\(.ruleId)] \(.message)"')"
		if [ -n "$errs" ]; then
			FAILED=1
			echo "FAIL: $md:$line"
			printf '%s\n' "$errs"
		fi
	done
done < <(find "$@" -name '*.md' | sort)

echo "---"
echo "Complete MDL blocks: checked $CHECKED, skipped $SKIPPED (fragments/illustrative)"
if [ "$FAILED" -ne 0 ]; then
	echo "Some complete MDL examples fail 'mxcli check' (see above)."
	exit 1
fi
echo "All complete MDL examples pass 'mxcli check'."
