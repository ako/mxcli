#!/usr/bin/env bash
# check-skill-mdl.sh — syntax-check the domain-model DDL blocks embedded in the
# user-facing skills AND the docs site, so invalid MDL can't drift into the docs.
# This is the class of drift seen in practice: associations documented with
# PARENT/CHILD or `[*] -> [1]`, enumerations with `= 'x'` or `as ( x: 'y' )`,
# ALTER ENTITY with `ADD (attr, ...)` instead of `ADD ATTRIBUTE attr: type`, etc.
#
# It recursively finds every *.md under the given directory, extracts each ```mdl
# and ```sql fenced block, and runs `mxcli check` (syntax only) on the individual
# statements whose first keyword is a domain-model DDL statement — CREATE/ALTER/DROP
# of an entity, association, enumeration, or constant. Those are reliably complete
# and standalone; a bad statement inside a larger multi-document example is still
# caught (extraction is per-statement, not per-block).
#
# Everything else is skipped on purpose, because skills legitimately show
# fragments that are NOT standalone top-level MDL: microflow activities
# (`change $x`, `show page X(...)`), page/REST snippets, and the import-mapping /
# JSON-structure mini-DSL (`create entity X { ... }`). Blocks are also skipped when
# they are clearly illustrative: placeholders (`...`), syntax templates (` | `
# alternation, `<name>`, `[optional]`), brace-based mini-DSL (`{`/`}`), or a
# deliberately-wrong example (❌ / INCORRECT / WRONG / -- BAD).
#
# Usage: scripts/check-skill-mdl.sh [mxcli-binary] [docs-dir]
set -uo pipefail

MXCLI="${1:-bin/mxcli}"
SKILLS_DIR="${2:-.claude/skills/mendix}"

if [ ! -x "$MXCLI" ]; then
	echo "error: mxcli binary not found or not executable: $MXCLI (run 'make build')" >&2
	exit 2
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# A checkable block's first statement must be a domain-model DDL statement. Scope
# is deliberately the drift-prone domain model — entity / association /
# enumeration / constant — not security (`create module role`), pages, or
# microflows, whose blocks are fragment-heavy and have their own syntax surface.
DDL_RE='^(create|alter|drop)( or (modify|replace))?( (persistent|non_persistent|view|external))? (entity|association|enumeration|constant)\b'

FAILED=0
CHECKED=0
SKIPPED=0

while IFS= read -r md; do
	[ -e "$md" ] || continue
	# Split the file into ```mdl / ```sql blocks, one file per block.
	rm -f "$WORK"/block_* 2>/dev/null || true
	awk -v dir="$WORK" '
		/^```(mdl|sql)[ \t]*$/ { inb=1; n++; fn=sprintf("%s/block_%03d.mdl", dir, n); next }
		inb && /^```/ { inb=0; next }
		inb { print > fn }
	' "$md"

	for blk in "$WORK"/block_*.mdl; do
		[ -e "$blk" ] || continue

		# Block-level skip: templates / placeholders, an explicit `-- check-skip`
		# opt-out, or a block that deliberately demonstrates a PARSE ERROR. Checked
		# on the whole block because these markers live in comments (stripped before
		# splitting). Note: "❌ INCORRECT" examples are NOT skipped — those are almost
		# always syntactically valid but semantically wrong (e.g. a reversed
		# association direction), so they pass a syntax check and their statements
		# should still be validated. Only genuinely syntax-broken demos are skipped.
		if grep -qE '\.\.\.| \| |<[a-zA-Z]|[$][{]' "$blk" ||
			grep -qiE 'check-skip|parse error' "$blk"; then
			SKIPPED=$((SKIPPED + 1)); continue
		fi

		# Split the block into individual statements (strip line comments and `/`
		# separators first, then split on `;`), so a bad DDL statement inside a
		# larger multi-document example is still caught.
		rm -f "$WORK"/stmt_* 2>/dev/null || true
		awk -v dir="$WORK" '
			{ sub(/[[:space:]]*--.*/, "") }              # drop line comments
			/^[[:space:]]*\/[[:space:]]*$/ { next }       # drop `/` separators
			{ buf = buf $0 "\n" }
			END {
				m = split(buf, parts, ";")
				for (i = 1; i <= m; i++)
					if (parts[i] ~ /[^[:space:]]/) {
						fn = sprintf("%s/stmt_%03d.mdl", dir, i)
						printf "%s;\n", parts[i] > fn
					}
			}' "$blk"

		for stmt in "$WORK"/stmt_*.mdl; do
			[ -e "$stmt" ] || continue
			first="$(grep -vE '^[[:space:]]*$' "$stmt" | head -1 | sed -E 's/^[[:space:]]+//' | tr 'A-Z' 'a-z')"
			# Only domain-model DDL statements are in scope.
			printf '%s' "$first" | grep -qE "$DDL_RE" || { SKIPPED=$((SKIPPED + 1)); continue; }
			# Skip the JSON-structure mini-DSL (`create entity X (NON_PERSISTENT)`).
			grep -qE 'NON_PERSISTENT\)' "$stmt" && { SKIPPED=$((SKIPPED + 1)); continue; }

			CHECKED=$((CHECKED + 1))
			out="$("$MXCLI" check "$stmt" 2>&1)"
			# Fail only on SYNTAX/parse errors — the drift this guard exists to
			# catch. Semantic validation (reserved attribute names, cross-references,
			# …) is context-dependent and noisy for isolated snippets, so it is not
			# enforced here.
			if printf '%s' "$out" | grep -qiE 'Syntax errors found|mismatched input|no viable alternative|extraneous input|expecting'; then
				FAILED=1
				echo "FAIL: $md — $(grep -vE '^[[:space:]]*$' "$stmt" | head -1 | sed -E 's/^[[:space:]]+//')"
				printf '%s\n' "$out" | grep -iE 'mismatched|expecting|no viable|extraneous' | grep -iv 'vibe' | sed 's/^/    /' | head -2
			fi
		done
	done
done < <(find "$SKILLS_DIR" -name '*.md' | sort)

echo "---"
echo "MDL blocks: checked $CHECKED, skipped $SKIPPED (illustrative/templates)"
if [ "$FAILED" -ne 0 ]; then
	echo "Some MDL blocks have invalid syntax (see above)."
	exit 1
fi
echo "All checkable MDL blocks pass 'mxcli check'."
