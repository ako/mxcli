#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Extract real Studio Pro mapping documents into a round-trip test fixture.

The regression test for issue #260 needs mappings mxcli **cannot author** — which
is circular unless the documents come from somewhere else. This pulls them out of
a real project verbatim, together with the MDL that recreates everything they
reference.

    scripts/mapping-census/extract-fixture.py \
        --project /path/to/App.mpr --out mdl/executor/testdata/mapping-fixtures \
        KrogerAPI.IM_AccessToken OpenAI_API.IM_OpenAI

Output per run:

    <name>.bson     the mapping unit's BSON, byte-for-byte as Studio Pro wrote it
    deps.mdl        modules, JSON structures, entities, associations, microflows
    manifest.json   each mapping's module, type, and the constructs that block it

A mapping document references entities, attributes, associations, structures and
microflows by **qualified name**, never by ID, so a document dropped into a project
that declares the same names is intact. That is what makes the transplant sound —
and why deps.mdl must keep the original module and element names.
"""

import argparse
import json
import os
import re
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from census import blockers  # noqa: E402
from mprbson import kids, loads, units  # noqa: E402

MAP_TYPES = ("ImportMappings$ImportMapping", "ExportMappings$ExportMapping")

# Documents transplanted verbatim alongside the mappings rather than recreated
# from MDL. A JSON structure rebuilt from its snippet is NOT the same document:
# mxcli derives an array item's name as "<Name>Item" where Studio Pro
# singularises ("Data" -> "Datum"), and gives the root MinOccurs 0 where Studio
# Pro writes 1 (issue #272). Those differences leak into every mapping built over
# the structure, so a round-trip fixture that regenerates them measures the
# structure builder instead of the mapping describer. Message definitions have no
# MDL at all (#263).
VERBATIM_TYPES = ("JsonStructures$JsonStructure",
                  "MessageDefinitions$MessageDefinition")

# Modules every project already has, and which must never be authored: System is
# the platform module, and describing its entities emits things like
# `FileID: AutoNumber` with no seed that mxcli's own pre-flight then refuses.
PLATFORM_MODULES = {"System"}


def _walk(elems):
    for e in elems:
        yield e
        for sub in _walk(kids(e.get("Children"))):
            yield sub


def describe(mxcli, mpr, stmt):
    p = subprocess.run([mxcli, "-p", mpr, "-c", stmt],
                       capture_output=True, text=True)
    out = "\n".join(l for l in p.stdout.splitlines() if not l.startswith("WARNING"))
    if p.returncode != 0 or "Parse error" in out or "Error:" in out:
        return None
    return out.strip()


# Lines in a `describe entity` that pull in dependencies a mapping fixture does
# not need: access rules need module roles, and event handlers need the
# microflows they call (and, transitively, everything those call).
_NOISE = re.compile(r"^\s*(grant |on (before|after) )")


def _strip_noise(text):
    return "\n".join(l for l in text.splitlines() if not _NOISE.match(l))


def stub_microflow(text):
    """A microflow reduced to its signature.

    The fixture needs the converter and custom-handler microflows to EXIST with
    the right signature — nothing calls them. Keeping their real bodies drags in
    the transitive closure of everything they call (a Community-Commons helper,
    a module-reflection microflow, …), none of which has anything to do with
    mapping fidelity.
    """
    head, sep, _body = text.partition("\nbegin")
    if not sep:
        return text
    returns = re.search(r"^returns\s+(.+)$", head, re.M)
    body = "  @start(100, 200)\n"
    if returns:
        body += "  @position(300, 200)\n  return empty;\n"
    return head + "\nbegin\n" + body + "end;"


def references(doc):
    """Everything a mapping document names, grouped by kind."""
    ref = {"entity": set(), "association": set(), "microflow": set(),
           "structure": set()}
    for key in ("JsonStructure", "XmlSchema"):
        if doc.get(key):
            ref["structure"].add(doc[key])
    if doc.get("MessageDefinition"):
        # "Module.Document.MessageName" — the document is the first two parts.
        ref["structure"].add(".".join(doc["MessageDefinition"].split(".")[:2]))
    def keep(qn):
        return qn.split(".")[0] not in PLATFORM_MODULES

    for e in _walk(kids(doc.get("Elements"))):
        if e.get("Entity") and keep(e["Entity"]):
            ref["entity"].add(e["Entity"])
        if e.get("Association") and keep(e["Association"]):
            ref["association"].add(e["Association"])
        if e.get("Converter") and keep(e["Converter"]):
            ref["microflow"].add(e["Converter"])
        if e.get("Attribute"):
            # Module.Entity.Attr — the declaring entity may differ from the
            # element's own (inherited members, see #703).
            parts = e["Attribute"].split(".")
            if len(parts) == 3 and keep(e["Attribute"]):
                ref["entity"].add(".".join(parts[:2]))
        ch = e.get("CustomHandlerCall")
        if isinstance(ch, dict) and ch.get("Microflow") and keep(ch["Microflow"]):
            ref["microflow"].add(ch["Microflow"])
            for p in kids(ch.get("ParameterMappings")):
                q = (p.get("Parameter") or "").split(".")
                if len(q) == 3 and keep(p["Parameter"]):
                    ref["microflow"].add(".".join(q[:2]))
    return ref


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--project", required=True, help="source .mpr")
    ap.add_argument("--out", required=True, help="fixture directory")
    ap.add_argument("--mxcli", default="bin/mxcli")
    ap.add_argument("--append", action="store_true",
                    help="add to an existing fixture instead of replacing it")
    ap.add_argument("mappings", nargs="+", help="Module.MappingName")
    args = ap.parse_args()

    wanted = {}
    for qn in args.mappings:
        mod, _, name = qn.rpartition(".")
        wanted[(mod, name)] = qn

    os.makedirs(args.out, exist_ok=True)
    raw = {}
    found = {}
    all_units = {uid: (cid, d) for uid, cid, _c, d in units(args.project)}

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

    # Re-read the raw bytes: the fixture must be Studio Pro's document, not a
    # re-encode of our decode.
    for uid, (cid, d) in all_units.items():
        if d.get("$Type") not in MAP_TYPES:
            continue
        key = (module_of(uid), d.get("Name"))
        if key in wanted:
            found[key] = (uid, d)

    missing = [qn for k, qn in wanted.items() if k not in found]
    if missing:
        print("not found in %s: %s" % (args.project, ", ".join(missing)),
              file=sys.stderr)
        return 1

    raw = _raw_bytes(args.project, [uid for uid, _d in found.values()])

    # Index the verbatim-transplant candidates by qualified name.
    verbatim_units = {}
    for uid, (cid, d) in all_units.items():
        if d.get("$Type") in VERBATIM_TYPES:
            verbatim_units["%s.%s" % (module_of(uid), d.get("Name"))] = (uid, d)

    refs = {"entity": set(), "association": set(), "microflow": set(),
            "structure": set()}
    manifest = []
    for (mod, name), (uid, d) in sorted(found.items()):
        with open(os.path.join(args.out, "%s.%s.bson" % (mod, name)), "wb") as f:
            f.write(raw[uid])
        r = references(d)
        for k in refs:
            refs[k] |= r[k]
        manifest.append({
            "module": mod, "name": name, "type": d["$Type"],
            "file": "%s.%s.bson" % (mod, name),
            "source": os.path.basename(args.project),
            "blockers": sorted(blockers({"doc": d, "type": d["$Type"]})),
        })

    dep_path = os.path.join(args.out, "deps.mdl")
    existing = ""
    if args.append and os.path.exists(dep_path):
        existing = open(dep_path).read()

    mdl = []
    if not existing:
        mdl += ["-- GENERATED by scripts/mapping-census/extract-fixture.py",
                "-- Recreates everything the fixture mappings reference, by the SAME",
                "-- qualified names — the transplanted documents resolve by name.", ""]
    # An entity's attribute can be typed by an enumeration in the same module,
    # which the mapping document never names — collect them from the entity MDL
    # itself and emit them first, or the entity fails with "unknown type".
    entity_mdl = {}
    for qn in sorted(refs["entity"]):
        entity_mdl[qn] = describe(args.mxcli, args.project, "describe entity %s" % qn)
    enums = set()
    qualified = re.compile(
        r"Enumeration\(([A-Za-z_]\w*\.[A-Za-z_]\w*)\)"          # Enumeration(M.E)
        r"|^\s*\w+:\s*([A-Za-z_]\w*\.[A-Za-z_]\w*)", re.M)     # Attr: M.E
    for text in entity_mdl.values():
        for a, b in qualified.findall(text or ""):
            qtype = a or b
            if qtype and qtype.split(".")[0] not in PLATFORM_MODULES:
                enums.add(qtype)
    for qn in sorted(enums):
        text = describe(args.mxcli, args.project, "describe enumeration %s" % qn)
        if text and text not in existing:
            mdl.append(text)
            mdl.append("")
    refs["enumeration"] = enums
    modules = sorted({q.split(".")[0] for s in refs.values() for q in s}
                     - PLATFORM_MODULES)

    # Structures and message definitions go in verbatim; anything not found that
    # way (an XML schema, say) still falls back to a describe.
    documents = []
    for qn in sorted(refs["structure"]):
        hit = verbatim_units.get(qn)
        if hit is None:
            continue
        uid, d = hit
        blob = _raw_bytes(args.project, [uid])[uid]
        fname = "%s.bson" % qn
        with open(os.path.join(args.out, fname), "wb") as f:
            f.write(blob)
        documents.append({"module": qn.split(".")[0], "name": d.get("Name"),
                          "type": d["$Type"], "file": fname,
                          "source": os.path.basename(args.project)})
    refs["structure"] -= set(verbatim_units)

    for kind, stmt in (("structure", "describe json structure %s"),
                       ("entity", "describe entity %s"),
                       ("association", "describe association %s"),
                       ("microflow", "describe microflow %s")):
        for qn in sorted(refs[kind]):
            text = entity_mdl[qn] if kind == "entity" else \
                describe(args.mxcli, args.project, stmt % qn)
            if text is not None and kind == "microflow":
                text = stub_microflow(text)
            if text is None:
                mdl.append("-- UNRESOLVED %s %s (describe failed)" % (kind, qn))
                continue
            if text in existing:
                continue
            text = _strip_noise(text)
            mdl.append(text)
            mdl.append("")

    with open(dep_path, "a" if args.append else "w") as f:
        f.write("\n".join(mdl) + "\n")
    mpath = os.path.join(args.out, "manifest.json")
    prev = {"modules": [], "mappings": []}
    if args.append and os.path.exists(mpath):
        prev = json.load(open(mpath))
    seen_docs = {d["file"] for d in prev.get("documents", [])}
    doc = {"modules": sorted(set(prev.get("modules", [])) | set(modules)),
           "documents": prev.get("documents", [])
           + [d for d in documents if d["file"] not in seen_docs],
           "mappings": prev.get("mappings", []) + manifest}
    with open(mpath, "w") as f:
        json.dump(doc, f, indent=1)
        f.write("\n")
    print("wrote %d mapping(s) + deps.mdl to %s" % (len(doc["mappings"]), args.out))
    return 0


def _raw_bytes(mpr, uids):
    """The stored bytes for each unit, from Unit.Contents (v1) or the file (v2)."""
    import glob
    import sqlite3
    from mprbson import _guid_hex
    con = sqlite3.connect("file:%s?mode=ro" % mpr, uri=True)
    cols = {r[1] for r in con.execute("pragma table_info(Unit)")}
    out = {}
    if "Contents" in cols:
        for uid, blob in con.execute("select UnitID, Contents from Unit"):
            if bytes(uid) in uids:
                out[bytes(uid)] = bytes(blob)
        con.close()
        return out
    con.close()
    root = os.path.join(os.path.dirname(os.path.abspath(mpr)), "mprcontents")
    by_id = {os.path.splitext(os.path.basename(f))[0].replace("-", ""): f
             for f in glob.glob(os.path.join(root, "**", "*.mxunit"), recursive=True)}
    for uid in uids:
        f = by_id.get(_guid_hex(uid))
        if f:
            with open(f, "rb") as fh:
                out[uid] = fh.read()
    return out


if __name__ == "__main__":
    sys.exit(main())
