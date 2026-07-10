# Release Skill

How to cut an mxcli release so every version reads the same way. This is a
**contributor** skill (dev-only) — it is not synced to user projects.

## The one rule that fixes everything

`CHANGELOG.md` is the **single source of truth**. You author the release notes
there, in Keep a Changelog form, and the GitHub release body is *derived* from
that section — never hand-written separately, and never produced by clicking
GitHub's "Generate release notes" button (that gives a flat PR dump with no
headline, which is why past releases drifted between two incompatible styles).

The GitHub body is: **the curated CHANGELOG section**, followed by an
auto-generated **New Contributors** block and a **Full Changelog** compare link.

---

## 1. Write the CHANGELOG section

Convert the top `## [Unreleased]` into `## [X.Y.Z] - YYYY-MM-DD` and leave a fresh
empty `## [Unreleased]` above it. Every release section has the **same shape**,
regardless of size:

```markdown
## [X.Y.Z] - YYYY-MM-DD

Headline: **<one sentence naming the release's theme>**. <1–3 sentences of context.>

### Added
- **Bold lead-in phrase** — explanation in 1–3 sentences, with `(#NNN)` refs.

### Changed
- ...

### Fixed
- **Bold lead-in** — what was broken and now works. Group related fixes under
  one parent bullet with indented sub-bullets when they share a theme.
```

Rules — these are what make releases consistent:

- **Headline is mandatory**, always exactly one line, even for a tiny release.
- **Section order is fixed**: `Added`, `Changed`, `Deprecated`, `Removed`,
  `Fixed`, `Security`. Omit any section that is empty — never keep an empty heading.
- **Entry style**: bold lead-in → em-dash → explanation. Reference issues/PRs as
  `(#NNN)`. One entry per user-facing change.
- **Group, don't list**: a wave of related fixes (e.g. several page-serialization
  bugs) becomes one parent bullet with sub-bullets, not ten flat lines.
- **Scale length to the release, not the shape**: a big release has more bullets;
  it does not get a different structure. A one-feature release is a headline plus
  a single `### Added` bullet.
- **Skip noise**: pure docs/test/proposal/CI-chore commits do not get their own
  entry unless they change user-visible behaviour. Dependency bumps that matter
  for security get a one-line `Changed`/`Fixed` entry (e.g. a CVE fix).

To see what changed since the last tag:

```bash
git log --oneline vX.Y-1.0..HEAD
```

Validate before committing: `make build && make test && make lint`.

---

## 2. Commit and tag

Release commit and tag live on `main` (the tag must point at the release commit):

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): release vX.Y.Z"     # add the Co-Authored-By trailer
git tag -a vX.Y.Z -m "Release vX.Y.Z"
```

If you need to fold a follow-up fix into the release, `git commit --amend` and move
the tag with `git tag -f -a vX.Y.Z -m "Release vX.Y.Z"` **before pushing**. Never
move a tag that has already been pushed.

Push the commit and tag together:

```bash
git push origin main --follow-tags
```

---

## 3. Create the GitHub release (curated body + auto tail)

Do **not** use the web "Generate release notes" button. Run this block — it
extracts the CHANGELOG section, appends the New Contributors block and a Full
Changelog compare link, and creates the release:

```bash
VER=v0.15.0          # this release
PREV=v0.14.0         # previous tag
REPO=mendixlabs/mxcli
BODY=$(mktemp)

# a) the curated CHANGELOG section (header line through the next "## [")
awk -v v="[${VER#v}]" '
  index($0,"## "v)==1 {p=1; print; next}
  p && /^## \[/ {exit}
  p {print}
' CHANGELOG.md > "$BODY"

# b) New Contributors block from GitHub's generator (empty if none), then our
#    own compare link — so the tail is always present and never duplicated
NEWC=$(gh api "repos/$REPO/releases/generate-notes" \
        -f tag_name="$VER" -f previous_tag_name="$PREV" -q .body \
        | awk '/^## New Contributors/{p=1} /^\*\*Full Changelog/{p=0} p')
{
  echo; echo "---"; echo
  [ -n "$NEWC" ] && { echo "$NEWC"; echo; }
  echo "**Full Changelog**: https://github.com/$REPO/compare/$PREV...$VER"
} >> "$BODY"

gh release create "$VER" --title "$VER" --notes-file "$BODY"
rm -f "$BODY"
```

Verify the rendered body on the releases page, then you're done.

---

## Checklist

- [ ] `## [Unreleased]` rolled into `## [X.Y.Z] - DATE`; fresh empty `[Unreleased]` left on top
- [ ] Headline present; sections in canonical order; no empty sections
- [ ] Entries use the bold-lead-in → em-dash style; related fixes grouped; `(#NNN)` refs
- [ ] `make build && make test && make lint` pass
- [ ] Commit `docs(changelog): release vX.Y.Z` + annotated tag on `main`
- [ ] `git push origin main --follow-tags`
- [ ] GitHub release created from the CHANGELOG section (not the web generator), with the auto tail
