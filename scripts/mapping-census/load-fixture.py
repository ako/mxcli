#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Load an extracted mapping fixture into a project.

    scripts/mapping-census/load-fixture.py --project App.mpr \
        --fixture mdl/executor/testdata/mapping-fixtures [--mxcli bin/mxcli]

Runs deps.mdl through mxcli, then transplants each `.bson` document verbatim as
a new unit under its module. The documents are NOT re-encoded — the point of the
fixture is that they are byte-for-byte what Studio Pro wrote.

Module creates in deps.mdl are expected to fail when the module already exists
(FeedbackModule ships in every blank app); that is not an error.
"""

import argparse
import glob
import json
import os
import sqlite3
import subprocess
import sys
import uuid

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mprbson import _guid_hex, loads, units  # noqa: E402


def unit_identity(blob):
    """(UnitID bytes, canonical uuid) for a document, taken from its own $ID.

    A unit's UnitID *is* the document's `$ID` — the modelsdk backend resolves a
    document's module with `moduleNameFor(doc.ID)`, so a transplant that mints a
    fresh unit id is listed by `show` and then not found by `describe`.
    """
    doc = loads(blob)
    raw = bytes.fromhex(doc["$ID"]["$binary"])
    return raw, uuid.UUID(_guid_hex(raw))


MAP_TYPES = ("ImportMappings$ImportMapping", "ExportMappings$ExportMapping")


def survey(mpr):
    """(module name -> UnitID, set of mapping names already present)."""
    mods, present = {}, set()
    for uid, _cid, _cn, d in units(mpr):
        t = d.get("$Type")
        if t in ("Projects$Module", "Projects$ModuleImpl"):
            mods[d.get("Name")] = uid
        elif t in MAP_TYPES:
            present.add((t, d.get("Name")))
    return mods, present


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--project", required=True)
    ap.add_argument("--fixture", required=True)
    ap.add_argument("--mxcli", default="bin/mxcli")
    args = ap.parse_args()

    manifest = json.load(open(os.path.join(args.fixture, "manifest.json")))

    # Modules first, one statement at a time: a module the project already has
    # (FeedbackModule ships in every blank app) must not abort the whole file —
    # `mxcli exec` refuses a script with any known error rather than applying it
    # partly.
    for mod in manifest.get("modules", []):
        subprocess.run([args.mxcli, "-p", args.project, "-c",
                        "create module %s" % mod],
                       capture_output=True, text=True)

    deps = os.path.join(args.fixture, "deps.mdl")
    p = subprocess.run([args.mxcli, "exec", deps, "-p", args.project],
                       capture_output=True, text=True)
    out = p.stdout + p.stderr
    if "Refusing to execute" in out or "Error:" in out:
        bad = [l for l in out.splitlines()
               if "Error:" in l or "Refusing to execute" in l or l.lstrip().startswith("\u2717")]
        print("deps.mdl failed:\n  " + "\n  ".join(bad[:12]), file=sys.stderr)
        return 1

    mods, present = survey(args.project)
    con = sqlite3.connect(args.project)
    v2 = "Contents" not in {r[1] for r in con.execute("pragma table_info(Unit)")}
    root = os.path.join(os.path.dirname(os.path.abspath(args.project)), "mprcontents")

    n = skipped = 0
    for m in manifest["mappings"]:
        # A base project that already ships the document (FeedbackModule's two
        # mappings are in every blank app) must not end up with two documents of
        # the same qualified name.
        if (m["type"], m["name"]) in present:
            print("skip %s.%s — already in the project" % (m["module"], m["name"]))
            skipped += 1
            continue
        container = mods.get(m["module"])
        if container is None:
            print("no module %s in project — run deps.mdl first" % m["module"],
                  file=sys.stderr)
            return 1
        with open(os.path.join(args.fixture, m["file"]), "rb") as f:
            blob = f.read()
        uid, u = unit_identity(blob)
        if v2:
            d = os.path.join(root, str(u)[:2], str(u)[2:4])
            os.makedirs(d, exist_ok=True)
            with open(os.path.join(d, "%s.mxunit" % u), "wb") as f:
                f.write(blob)
            con.execute("insert into Unit (UnitID, ContainerID, ContainmentName)"
                        " values (?,?,?)", (uid, container, "Documents"))
        else:
            con.execute("insert into Unit (UnitID, ContainerID, ContainmentName,"
                        " Contents) values (?,?,?,?)",
                        (uid, container, "Documents", blob))
        n += 1
    con.commit()
    con.close()
    print("loaded %d mapping(s)%s into %s"
          % (n, (" (%d already present)" % skipped) if skipped else "", args.project))
    return 0


if __name__ == "__main__":
    sys.exit(main())
