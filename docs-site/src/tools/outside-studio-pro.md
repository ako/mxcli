# Working Outside Studio Pro

A project edited with mxcli, git and an editor is sometimes opened in Studio Pro
afterwards. Two kinds of state that Studio Pro depends on are invisible to
`mx check`, and mxcli checks both.

## The ContentsHash index: `mxcli fix hashes`

In an MPR v2 project (Mendix 10.18 and later) the `.mpr` file indexes every
`mprcontents/**/*.mxunit` file by its hash: `Unit.ContentsHash` is
`base64(SHA-256(file))`. mxcli's writer and Studio Pro keep the two in step.
Anything else that changes a `.mxunit` does not: restoring a unit with
`git checkout`, `git restore`, a revert or a merge leaves the index describing
bytes that are no longer on disk. `mx check` and MxBuild read the files and do
not notice; Studio Pro uses the index to decide what changed.

```bash
mxcli fix hashes -p app.mpr            # verify; exits 1 when something is off
mxcli fix hashes -p app.mpr --repair   # rewrite mismatched hashes from the files
```

| Reported | Meaning | `--repair` |
|----------|---------|------------|
| `MISMATCH` | the file's hash differs from the stored one | rewrites the stored hash from the file |
| `MISSING` | the `.mpr` indexes a unit whose file does not exist | reported only — restore the file |
| `ORPHAN` | a `.mxunit` file no unit in the `.mpr` indexes | reported only — `mxcli diag --check-units --fix` removes it |

The repair treats the files as the source of truth and never changes them. It
updates every hash in one SQLite transaction, and, like every mxcli write, is
refused while Studio Pro has the project open (the `.mpr.lock` beside it). An
MPR v1 project keeps unit contents inside the `.mpr`, has no index to drift,
and the command says so.

`mxcli exec` and `mxcli docker check` run the verify too and print a one-line
warning when it finds a mismatch or a missing file. It never fails them;
measured on a 900-unit project, the whole verify takes about 0.2 s.

## Git states that crash Studio Pro

Studio Pro 11.13 fails to open a project with *"Unable to find 'system'
property in 'system'"* when the project folder is a git repository and

- the checked-out branch has **no upstream** — remedy:
  `git push -u origin <branch>` before opening the project (or
  `git remote add origin <url>` first, if the repository has no remote); or
- git reports **"detected dubious ownership"** — typical for a folder shared
  between the host and a devcontainer, owned by a different user on each side.
  Remedy, on the machine that runs Studio Pro:
  `git config --global --add safe.directory <path>`.

`mxcli docker check`, `mxcli run --local` and `mxcli diag -p app.mpr` warn about
both. The warnings are advice and never fail a command. They are silent:

- outside a git repository, or when git is not installed;
- on a **detached HEAD** — how CI systems and `git checkout <sha>` leave a
  repository; there is no branch to push, and nobody opens such a checkout in
  Studio Pro;
- when the `CI` environment variable is set, or `MXCLI_NO_GIT_WARNINGS=1`.

Dubious ownership is judged by the git mxcli runs. Inside a devcontainer that is
the container's git, which may see ownership differently from the host's Studio
Pro — so a clean report from inside the container does not rule it out on the
host.

## One report: `mxcli diag -p`

```bash
mxcli diag -p app.mpr
```

prints mxcli's diagnostics followed by a *Project* section with both checks.

## Commit metadata: `mxcli git note`

Studio Pro attaches a git note under `refs/notes/mx_metadata` to every commit
it makes:

```json
{"BranchName":"feature-x","ModelerVersion":"11.13.0",
 "ModelChanges":[{"Status":"Modified","UnitID":"…","UnitType":"Forms$Page",
                  "UnitName":"Home_Web","Module":"MyModule"}],
 "RelatedStories":[],"SolutionVersion":"","MPRFormatVersion":"Version2","HasModelerVersion":true}
```

It reads the note to check that a revision's Mendix version is compatible and to
show what the revision changed. A commit made with plain git has none, and
Studio Pro 11.13 back-fills one with `"(unknown)"` values and no changes.

```bash
mxcli git note -p app.mpr            # preview the notes for commits not yet pushed
mxcli git note -p app.mpr --write    # attach them
git push origin <branch> refs/notes/mx_metadata
```

The note is computed from the commit: the `.mpr` unit index and the
`mprcontents` units at the commit and at its parent. With no commit argument it
covers `@{upstream}..HEAD`, or `HEAD` when the branch has no upstream. A note
Studio Pro wrote is kept unless you pass `--force`; a placeholder is replaced.

| Status | Meaning |
|--------|---------|
| `Added` / `Deleted` | the unit is new / gone. A deleted module is listed once, as its `Projects$ModuleImpl` |
| `Modified` | the unit's content changed. A re-serialisation that only mints new element IDs is not a change |
| `Moved` | the unit moved to another folder (its container in the `.mpr` changed) |

Measured against Studio Pro's own notes on a Team Server repository: for
ordinary commits the note is identical, header and change list. For a Mendix
**version upgrade** the header is identical but the change list is longer than
Studio Pro's, because an upgrade re-serialises units in ways only Studio Pro
knows to ignore. Upgrades are done in Studio Pro, which writes its own note. MPR
v1 projects are not supported.

**Push the notes ref soon.** `git push` does not push notes, and while the
project is open Studio Pro fetches every few minutes and force-replaces the local
`refs/notes/mx_metadata` with the Team Server's copy, discarding notes that were
not pushed. If that happens, `mxcli git note --write` again: the discarded notes
have become placeholders, which it replaces.

The whole workflow (server branches, what to stage, never merging model history
with git) is in the `teamserver-git` skill that `mxcli init` installs.
