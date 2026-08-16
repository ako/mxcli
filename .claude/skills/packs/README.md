# Skill packs

Source of truth for skill **packs** — skills that carry more than prose.
`make sync-skill-packs` copies these into `cmd/mxcli/skillpacks/` for embedding
(that directory is gitignored and regenerated; **edit here, not there**).

A pack is a directory rather than a single Markdown file:

```
<pack-name>/
  pack.yaml            manifest — name must match the directory
  SKILL.md             frontmatter name + description, then the body
  references/*.md      loaded on demand, not in the prompt
  specs/  scripts/  mdl/   assets
```

Installed with `mxcli skill add <pack>`, which writes the tree into a project's
`.claude/skills/<pack-name>/`. Design and rationale:
[PROPOSAL_skill_packs.md](../../../docs/11-proposals/PROPOSAL_skill_packs.md).

## Packs are opt-in; the flat skills in `mendix/` are not

`.claude/skills/mendix/*.md` are pure prose and `mxcli init` writes every one of
them into every project — worst case an agent reads a page it did not need.

A pack is not free. `mendix-bulk-oql-dml` ships MDL that adds Java actions to the
model; a charting pack would need a widget installed. So nothing is installed
until asked for, and **copying a pack never touches the model** — `skill add`
writes files and prints the command that would apply the MDL, which the user runs
deliberately.

## Adding one

1. **Match the shape above.** `pack.yaml`'s `name` must equal the directory name;
   they are checked against each other, because `skill remove <name>` has to find
   what `skill add <name>` wrote.
2. **Installation steps that have been run**, not described from memory.
3. **Templates with sample inputs**, so the first use is copy-and-edit.
4. **A way to check the work without the full stack** — something with an exit
   code, runnable in seconds.
5. **Failure modes, symptoms first.** Every entry one that actually happened.
6. **Keep it project-neutral.** A pack carrying one project's module or widget
   namespace hands that namespace to everyone who installs it.

   For a **widget**, ship the source with `{{NAMESPACE}}` / `{{NAMESPACE_PATH}}`
   placeholders and list the files under `rewrite.files`; `mxcli skill add`
   substitutes the destination project's namespace, and `TestVendoredPacks*`
   fails the build if a real one is left in. For **MDL**, use a placeholder
   module name (`MyModule`) the user replaces.
7. **Any `mdl/*.mdl` is checked by `make check-skill-mdl`.** A pack whose own MDL
   is never checked is a pack that rots.

The shape and the first two packs come from
[mxcli-ledger](https://github.com/ako/mxcli-ledger/tree/main/.claude/skills).
