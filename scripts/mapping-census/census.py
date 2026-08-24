#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Census of import/export mappings against what MDL can express.

Reads every ImportMappings$ImportMapping / ExportMappings$ExportMapping in one
or more projects and reports, per construct, how many documents mxcli cannot
author or round-trip today. Accepts .mpr files (v1 or v2) and .mpk packages.

    scripts/mapping-census/census.py mx-test-projects/*.mpk
    scripts/mapping-census/census.py --json app.mpr > census.json

The classification is on the decoded document, never on `describe` output, so it
stays independent of the DESCRIBE defects it is used to measure (issue #260).
Each construct below is a filed issue; see
docs/11-proposals/PROPOSAL_mapping_coverage.md for the analysis.
"""

import argparse
import collections
import json
import os
import shutil
import subprocess
import sys
import tempfile
import zipfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mprbson import kids, units  # noqa: E402

# construct -> the issue that tracks it
ISSUES = {
    "root: array": 248,
    "source: message definition": 263,
    "object handling: custom microflow": 264,
    "value converter microflow": 266,
    "primitive-array wrapper element": 268,
    "path through primitive-array marker": 268,
    "object element with no entity": 262,
    "root: nested schema element": 267,
    "object handling: find + ignore": 261,
    "object handling: find + error": 261,
    "object handling: create + error": 261,
    "object handling: allow override": 261,
    "mapping input parameter": 265,
    "value element with no attribute": 265,
    "source: XML schema": None,
    "xml attribute binding": None,
    "unspellable JSON member name": 260,
}

MAP_TYPES = ("ImportMappings$ImportMapping", "ExportMappings$ExportMapping")
MARKERS = {"(Object)", "(Array)", "(Wrapper)", "(Value)"}


def _walk(elems, root=True):
    for e in elems:
        yield e, root
        for sub in _walk(kids(e.get("Children")), False):
            yield sub


def _identifier(seg):
    return seg and (seg[0].isalpha() or seg[0] == "_") and all(
        c.isalnum() or c == "_" for c in seg)


def mappings(mpr, project):
    """Every mapping document in a project, with its owning module."""
    all_units = {uid: (cid, d) for uid, cid, _, d in units(mpr)}

    def module_of(uid):
        seen = set()
        while uid in all_units and uid not in seen:
            seen.add(uid)
            cid, d = all_units[uid]
            if d.get("$Type") in ("Projects$Module", "Projects$ModuleImpl"):
                return d.get("Name")
            if cid is None:
                return None
            uid = cid
        return None

    for uid, (cid, d) in all_units.items():
        if d.get("$Type") in MAP_TYPES:
            yield {"project": project, "module": module_of(uid),
                   "name": d.get("Name"), "type": d["$Type"], "doc": d}


def blockers(m):
    """The constructs in this mapping that MDL cannot express today."""
    d, out = m["doc"], set()
    imp = m["type"].startswith("Import")

    if d.get("XmlSchema"):
        out.add("source: XML schema")
    if d.get("MessageDefinition"):
        out.add("source: message definition")
    pt = d.get("ParameterType")
    if isinstance(pt, dict) and pt.get("$Type") != "DataTypes$UnknownType":
        out.add("mapping input parameter")

    roots = kids(d.get("Elements"))
    for r in roots:
        jp = r.get("JsonPath", "")
        if d.get("XmlSchema"):
            continue
        if jp in ("(Array)", "(Array)|(Object)"):
            out.add("root: array")
        elif jp not in ("(Object)", ""):
            out.add("root: nested schema element")

    for e, _root in _walk(roots):
        jp = e.get("JsonPath", "") or ""
        if "(Wrapper)" in jp or jp.endswith("|(Value)"):
            out.add("path through primitive-array marker")
        for seg in jp.split("|"):
            if seg and seg not in MARKERS and not _identifier(seg):
                out.add("unspellable JSON member name")
        if "Object" in e["$Type"]:
            oh, ob = e.get("ObjectHandling"), e.get("ObjectHandlingBackup")
            if oh == "Custom":
                out.add("object handling: custom microflow")
            elif imp and oh in ("Find", "Create") and ob in ("Error", "Ignore"):
                out.add("object handling: %s + %s" % (oh.lower(), ob.lower()))
            if e.get("ObjectHandlingBackupAllowOverride"):
                out.add("object handling: allow override")
            if e.get("ElementType") == "Wrapper":
                out.add("primitive-array wrapper element")
            if not e.get("Entity"):
                out.add("object element with no entity")
        else:
            if e.get("Converter"):
                out.add("value converter microflow")
            if e.get("IsXmlAttribute") or e.get("IsContent"):
                out.add("xml attribute binding")
            if not e.get("Attribute"):
                out.add("value element with no attribute")
    return out


def collect(path, tmp):
    """(mpr_path, label) for a .mpr or an extracted .mpk."""
    if path.endswith(".mpr"):
        return path, os.path.basename(path)
    label = os.path.basename(path)[:-4]
    out = os.path.join(tmp, label)
    os.makedirs(out, exist_ok=True)
    with zipfile.ZipFile(path) as z:
        names = [n for n in z.namelist()
                 if n.endswith(".mpr") and "mprcontents" not in n]
        if not names:
            return None, label
        # A .mpk is v1 (single .mpr); v2 packages carry mprcontents, which the
        # zip flattening below would break — extract the tree in that case.
        if any("mprcontents" in n for n in z.namelist()):
            z.extractall(out)
            return os.path.join(out, names[0]), label
        with open(os.path.join(out, os.path.basename(names[0])), "wb") as f:
            f.write(z.read(names[0]))
        return os.path.join(out, os.path.basename(names[0])), label


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("projects", nargs="+", help=".mpr or .mpk paths")
    ap.add_argument("--json", action="store_true", help="emit JSON instead of a table")
    ap.add_argument("--examples", type=int, default=3,
                    help="named examples to print per construct (default 3)")
    args = ap.parse_args()

    tmp = tempfile.mkdtemp(prefix="mapping-census-")
    docs = []
    try:
        for p in args.projects:
            mpr, label = collect(p, tmp)
            if mpr is None:
                print("skip (no .mpr): %s" % p, file=sys.stderr)
                continue
            docs.extend(mappings(mpr, label))
    finally:
        shutil.rmtree(tmp, ignore_errors=True)

    rows = [(m, sorted(blockers(m))) for m in docs]
    if args.json:
        json.dump([{"project": m["project"], "module": m["module"],
                    "name": m["name"], "type": m["type"], "blockers": b}
                   for m, b in rows], sys.stdout, indent=1)
        print()
        return

    clean = [1 for _m, b in rows if not b]
    imports = sum(1 for m, _b in rows if m["type"].startswith("Import"))
    print("mappings: %d  (import %d / export %d)"
          % (len(rows), imports, len(rows) - imports))
    if not rows:
        return
    print("expressible in MDL today: %d (%.0f%%)"
          % (len(clean), 100.0 * len(clean) / len(rows)))
    print()

    count = collections.Counter()
    example = collections.defaultdict(list)
    for m, bs in rows:
        for b in bs:
            count[b] += 1
            if len(example[b]) < args.examples:
                example[b].append("%s / %s.%s" % (m["project"], m["module"], m["name"]))
    print("%-42s %5s  %s" % ("construct MDL cannot express", "docs", "issue"))
    for k, v in count.most_common():
        iss = ISSUES.get(k)
        print("%-42s %5d  %s" % (k, v, ("#%d" % iss) if iss else "-"))
    print()
    for k, _v in count.most_common():
        print("### %s" % k)
        for e in example[k]:
            print("    %s" % e)


if __name__ == "__main__":
    main()
