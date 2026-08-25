#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# DESCRIBE round-trip check for every mapping in a project.
#
#   scripts/mapping-census/roundtrip.sh app.mpr [bin/mxcli]
#
# For each import/export mapping: describe it, then re-parse the output with
# `mxcli check`. Prints one TSV row per mapping and a summary. A PARSE_OK row is
# NOT proof of fidelity — the output can parse and still rebuild a different
# mapping (issue #260); pair this with census.py, which reads the stored
# document directly.
set -uo pipefail

MPR=${1:?usage: roundtrip.sh <project.mpr> [mxcli]}
MXCLI=${2:-bin/mxcli}
OUT=$(mktemp -d -t mapping-roundtrip-XXXXXX)
trap 'rm -rf "$OUT"' EXIT

ok=0 fail=0 empty=0

for kind in import export; do
  # `show … mappings` prints a markdown table; take column 1, drop the header
  # and separator rows.
  "$MXCLI" -p "$MPR" -c "show ${kind} mappings" 2>/dev/null \
    | awk -F'|' 'NR>2 && NF>2 {gsub(/^[ \t]+|[ \t]+$/,"",$2); if ($2 != "") print $2}' \
    | while read -r qn; do
        f="$OUT/${kind}-${qn}.mdl"
        "$MXCLI" -p "$MPR" -c "describe ${kind} mapping ${qn}" 2>/dev/null \
          | grep -v '^WARNING' > "$f"
        if [ ! -s "$f" ]; then
          printf '%s\t%s\tEMPTY\n' "$kind" "$qn"
        elif "$MXCLI" check "$f" >/dev/null 2>&1; then
          printf '%s\t%s\tPARSE_OK\n' "$kind" "$qn"
        else
          printf '%s\t%s\tPARSE_FAIL\n' "$kind" "$qn"
        fi
      done
done | tee "$OUT/results.tsv"

ok=$(grep -c 'PARSE_OK$'   "$OUT/results.tsv" || true)
fail=$(grep -c 'PARSE_FAIL$' "$OUT/results.tsv" || true)
empty=$(grep -c 'EMPTY$'     "$OUT/results.tsv" || true)
echo
echo "parse ok: $ok   parse fail: $fail   empty: $empty"
[ "$fail" -eq 0 ] && [ "$empty" -eq 0 ]
