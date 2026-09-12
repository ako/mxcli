#!/usr/bin/env python3
"""Classify what a describe -> exec round trip changed, per microflow.

Top-level document properties are reported separately from the flow graph
(ObjectCollection / Flows): they are different concerns — one is the
microflow's settings, the other its shape.
"""
import json, os, sys, collections

GRAPH_KEYS = {"ObjectCollection", "Flows"}


def to_plain(v):
    """mxcli bson dump emits [{Key,Value},...]; normalise to dicts, ids away."""
    if isinstance(v, list):
        if v and all(isinstance(e, dict) and set(e) == {"Key", "Value"} for e in v):
            return {e["Key"]: to_plain(e["Value"]) for e in v if e["Key"] != "$ID"}
        return [to_plain(e) for e in v]
    if isinstance(v, dict):
        if "Data" in v and "Subtype" in v:
            return "<id>"
        return {k: to_plain(x) for k, x in v.items() if k != "$ID"}
    return v


def load(path):
    with open(path) as f:
        return to_plain(json.loads("[" + f.read()))


def main(root):
    lost = collections.Counter()      # key present before, absent after
    changed = collections.Counter()   # key present both, value differs
    added = collections.Counter()     # key absent before, present after
    examples = {}
    graph_changed = []
    seen = 0

    for name in sorted(os.listdir(f"{root}/before")):
        mf = name[:-5]
        before, after = load(f"{root}/before/{name}"), load(f"{root}/after/{name}")
        if not isinstance(before, dict) or not isinstance(after, dict):
            continue
        seen += 1
        for k in set(before) | set(after):
            if k in GRAPH_KEYS:
                if before.get(k) != after.get(k):
                    graph_changed.append(mf)
                continue
            b, a = before.get(k, "<absent>"), after.get(k, "<absent>")
            if b == a:
                continue
            if a == "<absent>":
                lost[k] += 1
            elif b == "<absent>":
                added[k] += 1
            else:
                changed[k] += 1
            examples.setdefault(k, (mf, b, a))

    print(f"== {root}: {seen} microflows")
    for label, counter in (("LOST", lost), ("CHANGED", changed), ("ADDED", added)):
        if not counter:
            continue
        print(f"\n  {label}")
        for k, n in counter.most_common():
            mf, b, a = examples[k]
            bs, as_ = json.dumps(b)[:70], json.dumps(a)[:70]
            print(f"    {k:<28} {n:>3}/{seen}   {bs} -> {as_}")
            print(f"    {'':<28}       e.g. {mf}")
    print(f"\n  flow graph changed in {len(set(graph_changed))}/{seen}")


if __name__ == "__main__":
    main(sys.argv[1])
