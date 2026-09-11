#!/bin/bash
# Round-trip audit: for every microflow in a project, describe -> exec -> dump,
# and record what changed. Per-microflow so one failure does not hide the rest.
set -u
if [ $# -ne 3 ]; then
  echo "usage: $0 <project-dir> <App.mpr> <out-dir>   (MXCLI= to override the binary)" >&2
  exit 2
fi
MXCLI="${MXCLI:-$(cd "$(dirname "$0")/../.." && pwd)/bin/mxcli}"
SRC="$1"          # source project dir
MPR="$2"          # mpr filename
OUT="$3"          # output dir

rm -rf "$OUT"; mkdir -p "$OUT/before" "$OUT/after" "$OUT/mdl"
WORK="$OUT/work"; rm -rf "$WORK"; cp -r "$SRC" "$WORK"

mapfile -t MFS < <("$MXCLI" bson dump -p "$WORK/$MPR" --type microflow --list 2>/dev/null \
  | sed -n 's/^  \([A-Za-z0-9_.]*\) (Microflows\$Microflow)$/\1/p')
echo "microflows: ${#MFS[@]}" >&2

: > "$OUT/exec_failures.txt"
for mf in "${MFS[@]}"; do
  "$MXCLI" bson dump -p "$WORK/$MPR" --type microflow --object "$mf" 2>/dev/null \
    | tail -n +2 > "$OUT/before/$mf.json"
  "$MXCLI" -p "$WORK/$MPR" -c "describe microflow $mf" 2>/dev/null > "$OUT/mdl/$mf.mdl"
  if ! MXCLI_ALWAYS_WRITE=1 "$MXCLI" exec "$OUT/mdl/$mf.mdl" -p "$WORK/$MPR" >"$OUT/mdl/$mf.log" 2>&1; then
    echo "$mf" >> "$OUT/exec_failures.txt"
  elif grep -qi "parse error\|Error:" "$OUT/mdl/$mf.log"; then
    echo "$mf" >> "$OUT/exec_failures.txt"
  fi
  "$MXCLI" bson dump -p "$WORK/$MPR" --type microflow --object "$mf" 2>/dev/null \
    | tail -n +2 > "$OUT/after/$mf.json"
done
echo "done: $OUT" >&2
