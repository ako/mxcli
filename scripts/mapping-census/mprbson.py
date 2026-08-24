# SPDX-License-Identifier: Apache-2.0
"""Minimal BSON reader for Mendix .mpr units.

Enough of the BSON spec to walk a Mendix document: the codec never emits
JavaScript-with-scope, regex, or DBPointer, so those raise rather than being
silently skipped. Binary values (element $IDs) are kept as hex so a document is
JSON-serialisable.

Typed arrays are decoded as [marker, doc, doc, ...] — Mendix prefixes a list
with an int marker (1 or 3, see ADR notes on list markers), so callers filter to
dicts. `kids()` does that.
"""

import glob
import os
import sqlite3
import struct


def _cstr(b, i):
    j = b.find(b"\x00", i)
    return b[i:j].decode("utf-8", "replace"), j + 1


def decode(b, i=0):
    """Decode the BSON document at offset i. Returns (dict, next_offset)."""
    n = struct.unpack_from("<i", b, i)[0]
    end = i + n - 1
    i += 4
    out = {}
    while i < end:
        t = b[i]
        i += 1
        if t == 0:
            break
        name, i = _cstr(b, i)
        if t == 0x01:
            v = struct.unpack_from("<d", b, i)[0]; i += 8
        elif t in (0x02, 0x0D, 0x0E):
            ln = struct.unpack_from("<i", b, i)[0]; i += 4
            v = b[i:i + ln - 1].decode("utf-8", "replace"); i += ln
        elif t == 0x03:
            v, i = decode(b, i)
        elif t == 0x04:
            d, i = decode(b, i)
            v = [d[k] for k in sorted(d, key=lambda x: int(x) if x.isdigit() else x)]
        elif t == 0x05:
            ln = struct.unpack_from("<i", b, i)[0]
            v = {"$binary": b[i + 5:i + 5 + ln].hex(), "$sub": b[i + 4]}
            i += 5 + ln
        elif t == 0x07:
            v = {"$oid": b[i:i + 12].hex()}; i += 12
        elif t == 0x08:
            v = bool(b[i]); i += 1
        elif t == 0x09:
            v = {"$date": struct.unpack_from("<q", b, i)[0]}; i += 8
        elif t == 0x0A:
            v = None
        elif t == 0x10:
            v = struct.unpack_from("<i", b, i)[0]; i += 4
        elif t in (0x11, 0x12):
            v = struct.unpack_from("<q", b, i)[0]; i += 8
        else:
            raise ValueError("unhandled BSON type 0x%02x for %r at %d" % (t, name, i))
        out[name] = v
    return out, end + 1


def loads(b):
    return decode(b, 0)[0]


def kids(v):
    """The child documents of a Mendix typed array, dropping the list marker."""
    return [x for x in v if isinstance(x, dict)] if isinstance(v, list) else []


def units(mpr):
    """Yield (unit_id, container_id, containment_name, document) for a project.

    Handles both storage formats: v1 keeps the BSON in Unit.Contents, v2 keeps
    only the row and puts the document in mprcontents/<xx>/<yy>/<uuid>.mxunit.
    """
    con = sqlite3.connect("file:%s?mode=ro" % mpr, uri=True)
    cols = {r[1] for r in con.execute("pragma table_info(Unit)")}
    if "Contents" in cols:  # v1
        q = "select UnitID, ContainerID, ContainmentName, Contents from Unit"
        for uid, cid, cname, blob in con.execute(q):
            yield bytes(uid), bytes(cid) if cid else None, cname, loads(blob or b"")
        con.close()
        return
    rows = list(con.execute("select UnitID, ContainerID, ContainmentName from Unit"))
    con.close()
    root = os.path.join(os.path.dirname(os.path.abspath(mpr)), "mprcontents")
    by_id = {}
    for f in glob.glob(os.path.join(root, "**", "*.mxunit"), recursive=True):
        by_id[os.path.splitext(os.path.basename(f))[0].replace("-", "")] = f
    for uid, cid, cname in rows:
        # The row's UnitID is the .NET GUID byte order; the filename is the
        # canonical text form. Match on the file's hex instead of re-ordering.
        f = by_id.get(_guid_hex(bytes(uid)))
        if f is None:
            continue
        with open(f, "rb") as fh:
            yield bytes(uid), bytes(cid) if cid else None, cname, loads(fh.read())


def _guid_hex(b):
    """.NET GUID little-endian first three fields -> canonical hex."""
    return (b[3::-1] + b[5:3:-1] + b[7:5:-1] + b[8:]).hex()
