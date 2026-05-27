#!/usr/bin/env python3
"""One-time migration: add YAML frontmatter to proposal files.

Derives status from inline **Status:** line, else the README table, else a
flagged default. Normalises to the canonical vocabulary. Dry-run by default;
pass --apply to write files.
"""
import os
import re
import sys

ROOT = "/workspaces/ModelSDKGo-docs"
ACTIVE = os.path.join(ROOT, "docs/11-proposals")
ARCHIVE = os.path.join(ACTIVE, "archive")
README = os.path.join(ACTIVE, "README.md")

APPLY = "--apply" in sys.argv

CANON = {"draft", "proposed", "partial", "done", "superseded", "abandoned", "reference"}


def normalise(raw: str) -> str:
    """Map a free-text status string to canonical vocabulary."""
    if not raw:
        return ""
    s = raw.strip().lower()
    if "supersed" in s:
        return "superseded"
    if "abandon" in s:
        return "abandoned"
    if "reference" in s:
        return "reference"
    if "all phases" in s:
        return "done"
    # "Phase 1 & 2", "Phase 1 Implemented", "Partially done" => still in progress
    if "phase" in s and ("implement" in s or "done" in s or "ship" in s):
        return "partial"
    if "partial" in s:
        return "partial"
    if "implement" in s or "ship" in s or s == "done" or s.startswith("done"):
        return "done"
    if "propos" in s:  # "Proposed" and the "Proposal" typo
        return "proposed"
    if "draft" in s:
        return "draft"
    return ""  # unknown -> flag


def parse_readme_map():
    """filename -> normalised status, from the README tables."""
    m = {}
    with open(README) as f:
        for line in f:
            if "|" not in line:
                continue
            cells = [c.strip() for c in line.split("|")]
            link_idx = None
            fname = None
            for i, c in enumerate(cells):
                mt = re.search(r"\[[^\]]+\]\((?:archive/)?([^)]+\.md)\)", c)
                if mt:
                    link_idx = i
                    fname = os.path.basename(mt.group(1))
                    break
            if fname and link_idx is not None and link_idx + 1 < len(cells):
                status = normalise(cells[link_idx + 1])
                if status:
                    m[fname] = status
    return m


def extract_inline(path):
    """(raw_status, date, title) from inline markers + first heading."""
    raw_status = date = title = ""
    with open(path) as f:
        head = f.read(4000)
    for line in head.splitlines():
        if not title and line.startswith("# "):
            title = line[2:].strip()
        # match **Status:** value | **Status: value** | > **Status: value**
        if "**" in line and "Status" in line and not raw_status:
            ms = re.search(r"Status\**\s*:\**\s*(.+)", line)
            if ms:
                raw_status = ms.group(1).strip().strip(">").strip().strip("*").strip()
        md = re.match(r"\*\*Date:\*\*\s*(.+)", line)
        if md and not date:
            date = md.group(1).strip()
    # clean title prefixes
    for pre in ("Proposal: ", "Proposal — ", "Implementation Plan: ",
                "Refactoring Proposal: ", "Proposal `", "Case Study: "):
        if title.startswith(pre):
            title = title[len(pre):].strip()
    title = title.strip("`").strip()
    return raw_status, date, title


def already_has_frontmatter(path):
    with open(path) as f:
        return f.readline().strip() == "---"


def main():
    readme_map = parse_readme_map()
    rows = []
    for folder, is_archive in ((ACTIVE, False), (ARCHIVE, True)):
        for name in sorted(os.listdir(folder)):
            if not name.endswith(".md") or name == "README.md":
                continue
            path = os.path.join(folder, name)
            if not os.path.isfile(path):
                continue
            if already_has_frontmatter(path):
                rows.append((name, "SKIP (has frontmatter)", "", is_archive))
                continue
            raw, date, title = extract_inline(path)
            status = normalise(raw)
            source = "inline"
            if not status:
                status = readme_map.get(name, "")
                source = "readme"
            if not status:
                status = "done" if is_archive else "draft"
                source = "default(archive=done)" if is_archive else "FLAG-default-draft"
            # archived files are terminal: a non-terminal status is stale -> done
            if is_archive and status not in ("done", "superseded", "abandoned"):
                source = f"archive-override(was {status})"
                status = "done"
            rows.append((name, status, f"{source}|{title}|{date}", is_archive))
            if APPLY:
                fm = ["---", f"title: {title or name[:-3]}", f"status: {status}"]
                if date:
                    fm.append(f"date: {date}")
                fm.append("---\n\n")
                with open(path) as f:
                    body = f.read()
                with open(path, "w") as f:
                    f.write("\n".join(fm) + body)
    # report
    for name, status, info, is_arch in rows:
        loc = "ARCH" if is_arch else "actv"
        print(f"{loc} {status:12} {name:55} {info}")
    flagged = [r for r in rows if "FLAG" in r[1] or "FLAG" in r[2]]
    print(f"\nTotal: {len(rows)} | flagged-default: {len(flagged)} | applied: {APPLY}")


if __name__ == "__main__":
    main()
