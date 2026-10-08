#!/usr/bin/env python3
"""split-findings.py — turn findings/<shard>.jsonl lines into one file per finding.

The findings used to be appended to one shared file per area. Every PR that
fixed a bug in that area appended to the same file, and GitHub's server-side
merge does not run the `merge=union` driver that resolved this locally — so
each merged fix put every other open fix-PR into conflict. One file per finding
removes the shared write: two fixes now add two different files.

This script did the one-time migration, and it is also the remedy for a branch
written before it: a branch that still appends to `<shard>.jsonl` gets that file
back on its next merge with main (a modify/delete conflict, or a re-created
file). `make check-findings` refuses any `*.jsonl` left in the findings
directory and points here.

    scripts/split-findings.py            # convert every findings/*.jsonl
    scripts/split-findings.py --check    # report what would be written, change nothing

Each line is written VERBATIM (plus a newline) to
`findings/<shard>/<date>-<slug>.json`, where <shard> is the .jsonl file's name
and <slug> comes from the record's symptom (or raw row). Writing the original
bytes rather than re-serialising is what makes the migration provably lossless:
the multiset of file contents equals the multiset of original lines. A line
whose exact bytes already exist as a finding file is skipped, so re-running it
over a branch that re-created a shard file only adds that branch's new records.
The .jsonl file is removed once its lines are all written.
"""

import glob
import json
import os
import re
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
FINDINGS = os.path.join(ROOT, ".claude", "skills", "fix-issue", "findings")

STOP = {"a", "an", "the", "is", "are", "was", "of", "on", "in", "to", "and", "or",
        "for", "with", "it", "its", "as", "at", "by", "be", "that", "this", "when"}


def slug_for(rec):
    text = rec.get("symptom") or rec.get("raw") or rec.get("cause") or "finding"
    # Drop markdown/code punctuation, keep words.
    words = re.findall(r"[A-Za-z0-9]+", text.lower())
    words = [w for w in words if w not in STOP] or ["finding"]
    out = ""
    for w in words:
        nxt = (out + "-" + w) if out else w
        if len(nxt) > 60:
            break
        out = nxt
    return out or words[0][:60]


def existing_contents():
    seen = set()
    for path in glob.glob(os.path.join(FINDINGS, "*", "*.json")):
        with open(path, "rb") as f:
            seen.add(f.read())
    return seen


def main():
    check = "--check" in sys.argv[1:]
    shards = sorted(glob.glob(os.path.join(FINDINGS, "*.jsonl")))
    if not shards:
        print("no findings/*.jsonl to split")
        return 0
    seen = existing_contents()
    written = skipped = 0
    for shard_path in shards:
        shard = os.path.splitext(os.path.basename(shard_path))[0]
        outdir = os.path.join(FINDINGS, shard)
        with open(shard_path, "rb") as f:
            lines = [l.rstrip(b"\r\n") for l in f.read().split(b"\n")]
        for n, line in enumerate(lines, 1):
            if not line.strip():
                continue
            content = line + b"\n"
            if content in seen:
                skipped += 1
                continue
            try:
                rec = json.loads(line)
            except ValueError as e:
                print("%s:%d: not valid JSON (%s); fix it first" % (shard_path, n, e), file=sys.stderr)
                return 1
            date = rec.get("date") or "undated"  # check-findings then refuses the record: date is required
            base = "%s-%s" % (date, slug_for(rec))
            name, i = base + ".json", 2
            while os.path.exists(os.path.join(outdir, name)):
                name, i = "%s-%d.json" % (base, i), i + 1
            if check:
                print("would write %s/%s" % (shard, name))
            else:
                os.makedirs(outdir, exist_ok=True)
                with open(os.path.join(outdir, name), "wb") as f:
                    f.write(content)
            seen.add(content)
            written += 1
        if not check:
            os.remove(shard_path)
    verb = "would write" if check else "wrote"
    print("%s %d finding file(s); %d line(s) already present" % (verb, written, skipped))
    return 0


if __name__ == "__main__":
    sys.exit(main())
