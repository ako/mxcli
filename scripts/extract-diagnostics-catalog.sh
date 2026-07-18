#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
#
# extract-diagnostics-catalog.sh — mine Mendix's consistency-diagnostics catalog
# (CE/CW/CI codes) from Mendix.Modeler.Texts.dll into structured, tiered data.
#
# The SCRIPT is ours; its OUTPUT is Mendix's proprietary message corpus — do NOT
# commit the generated catalog.{csv,json}. See PROPOSAL_check_diagnostics_catalog.md.
#
# Usage:   scripts/extract-diagnostics-catalog.sh [VERSION] [LOCALE] [OUTDIR]
# Example: scripts/extract-diagnostics-catalog.sh 11.12.1 en-US /tmp/diagcat
# Requires: curl, tar, monodis (apt-get install -y mono-utils), python3
set -euo pipefail

VERSION="${1:-11.12.1}"
LOCALE="${2:-en-US}"
OUTDIR="${3:-./diag-catalog-$VERSION}"
DLL="modeler/Mendix.Modeler.Texts.dll"
URL="https://cdn.mendix.com/runtime/mxbuild-${VERSION}.tar.gz"

command -v monodis >/dev/null || { echo "ERROR: monodis not found (apt-get install -y mono-utils)" >&2; exit 1; }
mkdir -p "$OUTDIR"; cd "$OUTDIR"

echo "[1/3] streaming $DLL from $URL ..."
# tar --occurrence=1 exits after the one member, closing the pipe early; curl then
# gets a (harmless) SIGPIPE/write-failure. Tolerate it and verify by file existence.
set +o pipefail
curl -fsSL "$URL" 2>/dev/null | tar -xz --occurrence=1 "$DLL" || true
set -o pipefail
[ -f "$DLL" ] || { echo "ERROR: failed to extract $DLL from $URL" >&2; exit 1; }

echo "[2/3] extracting embedded PO resources via monodis ..."
mkdir -p po && ( cd po && monodis --mresources "../$DLL" >/dev/null 2>&1 )
PO="$(ls po/*problem-descriptions_${LOCALE}.po | grep -vE 'deprecated|test' | head -1)"
[ -n "$PO" ] || { echo "ERROR: primary $LOCALE PO not found" >&2; exit 1; }

echo "[3/3] parsing + tiering $PO ..."
python3 - "$PO" "$VERSION" <<'PY'
import re, json, csv, sys, collections
po, version = sys.argv[1], sys.argv[2]
SEV = {"CE": "error", "CW": "warning", "CI": "info"}
# static-checkability tiers (heuristic: message text + source subsystem)
D = ['version control','marketplace','app store','environment','deployment','license',
     'certificate','migration','team server','commit','branch','merge','governance']
C = ['data source','react client','not supported in','offline','native','reachable',
     'end event','unreachable','navigation','home page','loop']
B = ['xpath','expression','type of','of type','cannot be converted','incompatible',
     'return type','data type','parameter','must return','regular expression','template']
def tier(blob):
    if any(k in blob for k in D): return 'D'
    if any(k in blob for k in C): return 'C'
    if any(k in blob for k in B): return 'B'
    return 'A'

code_re = re.compile(r'^(C[EWI])(\d+)__(.*)$')
rows, loc = [], []
lines = open(po, encoding='utf-8').read().splitlines()
i = 0
while i < len(lines):
    ln = lines[i]
    if ln.startswith('# LOCATION:'):
        loc.append(re.sub(r'\s*\(in project.*', '', ln.split('LOCATION:', 1)[1]).strip())
    elif ln.startswith('# NOTE:') and 'MX_DOCS_CAT' in ln:
        docs = ln.split('MX_DOCS_CAT', 1)[1].lstrip(':(colon) ').strip()
    elif ln.startswith('msgid "'):
        mid = ln[7:-1]
        j = i + 1
        while j < len(lines) and not lines[j].startswith('msgstr'): j += 1
        msg = ''
        if j < len(lines):
            msg = lines[j][8:].rstrip('"')
            k = j + 1
            while k < len(lines) and lines[k].startswith('"'):
                msg += lines[k].strip().strip('"'); k += 1
        m = code_re.match(mid)
        if m:
            params = sorted(set(re.findall(r'\{([A-Z0-9_]+)\}', msg)))
            rows.append(dict(code=m.group(1)+m.group(2), severity=SEV[m.group(1)],
                             slug=m.group(3), message=msg, params=params,
                             locations=sorted(set(loc)),
                             docs_cat=locals().get('docs'),
                             tier=tier((m.group(3) + ' ' + msg + ' ' + ' '.join(loc)).lower())))
        loc, docs = [], None
    i += 1

json.dump({"version": version, "codes": rows}, open("catalog.json", "w"), indent=1)
with open("catalog.csv", "w", newline='') as f:
    w = csv.writer(f); w.writerow(["code","severity","tier","message","params","source","docs_cat"])
    for r in rows:
        w.writerow([r['code'], r['severity'], r['tier'], r['message'],
                    ';'.join(r['params']), ';'.join(r['locations']), r['docs_cat'] or ''])

sev = collections.Counter(r['severity'] for r in rows)
tiers = collections.Counter(r['tier'] for r in rows)
print(f"  codes: {len(rows)}  ({dict(sev)})")
print(f"  tiers: {dict(sorted(tiers.items()))}  (A=structural B=expr/type C=cross-doc D=runtime)")
print(f"  parameterised: {sum(1 for r in rows if r['params'])}/{len(rows)}")
print(f"  wrote catalog.json + catalog.csv  (LOCAL ONLY — Mendix's corpus, do not commit)")
PY
echo "Done: $OUTDIR/catalog.{json,csv}"
