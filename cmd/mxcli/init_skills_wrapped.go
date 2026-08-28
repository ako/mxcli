// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// init_skills_wrapped.go writes the embedded skills for the tools that need
// each skill wrapped in their own frontmatter (OpenCode, Vibe), rather than
// copied verbatim the way `.claude/` and `.ai-context/` get them.
//
// It exists because both callers had their own copy of the same walk, and both
// copies read the skill's name off the FILE:
//
//	skillName := strings.TrimSuffix(d.Name(), ".md")
//
// That was right while skills were flat `<name>.md`. Once they moved to the
// Agent Skills layout (`<name>/SKILL.md`), every skill's base name became
// SKILL.md, so all 70 resolved to the same name and wrote to the same path —
// 70 skills collapsed into one `SKILL/`, last writer winning, its frontmatter
// reading `name: 'SKILL'`. The same walk also had no notion of a skill's
// supporting files, so every `references/*.md` inside a skill folder was
// promoted to a top-level skill of its own: `.opencode/skills/` held 19 entries,
// of which one was the collapsed skill and eighteen were fragments.
//
// The name therefore comes from the DIRECTORY, and only `SKILL.md` marks a
// skill root. Anything else in the folder is a resource the skill refers to and
// is copied through unwrapped — wrapping it would announce a fragment as a
// skill, which is the bug above wearing a different hat.

// skillWrapper renders a skill's SKILL.md for a specific tool.
type skillWrapper func(skillName string, content []byte) []byte

// writeWrappedSkills mirrors the embedded skills into destDir as
// <name>/SKILL.md, wrapping each root with wrap and copying every supporting
// file verbatim. It returns the number of skills (SKILL.md files) written.
func writeWrappedSkills(destDir string, wrap skillWrapper) (int, error) {
	count := 0
	err := fs.WalkDir(skillsFS, "skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel("skills", filepath.FromSlash(p))
		if relErr != nil {
			return relErr
		}
		// The tree's own README is not a skill.
		if rel == "README.md" {
			return nil
		}
		// A file directly under skills/ is not a skill root either: the layout
		// is <name>/SKILL.md, so anything at depth 1 is stray.
		dir := filepath.Dir(rel)
		if dir == "." {
			return nil
		}
		content, err := skillsFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading embedded skill %s: %w", p, err)
		}
		target := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		// Only the root is wrapped; resources are the skill's own files.
		out := content
		if filepath.Base(rel) == "SKILL.md" {
			// The skill's name is its folder — for a nested resource folder the
			// top-level folder is still the skill.
			out = wrap(topSkillName(rel), content)
			count++
		}
		if err := os.WriteFile(target, out, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
		return nil
	})
	return count, err
}

// topSkillName returns the skill folder a path belongs to — the first segment
// of a skills-relative path.
func topSkillName(rel string) string {
	for {
		dir := filepath.Dir(rel)
		if dir == "." || dir == string(filepath.Separator) {
			return rel
		}
		rel = dir
	}
}
