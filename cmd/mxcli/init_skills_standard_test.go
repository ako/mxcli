// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Issue #906: the bundled skills were flat `<name>.md` files with no frontmatter,
// routed by a hand-maintained table in the generated CLAUDE.md that had drifted
// to 12 of 68. Nothing discovered them — `mxcli init` did not even create
// `.claude/skills/`, the one directory Claude Code scans. These tests pin the
// three properties that fix depends on.

var frontmatter = regexp.MustCompile(`(?s)\A---\n(.*?)\n---\n`)

// Every shipped skill is a `<name>/SKILL.md` carrying `name` and `description`.
// A skill without a description is no more discoverable than the flat file it
// replaced: the description IS the index now.
func TestEmbeddedSkillsCarryAgentSkillsFrontmatter(t *testing.T) {
	names := embeddedSkillNames(t)
	if len(names) < 50 {
		t.Fatalf("only %d skills embedded; the sync or the embed directive is wrong", len(names))
	}

	for _, n := range names {
		body, err := skillsFS.ReadFile("skills/" + n + "/SKILL.md")
		if err != nil {
			t.Errorf("%s: %v", n, err)
			continue
		}
		m := frontmatter.FindSubmatch(body)
		if m == nil {
			t.Errorf("%s/SKILL.md has no YAML frontmatter", n)
			continue
		}
		fm := string(m[1])

		var gotName, gotDesc string
		for _, line := range strings.Split(fm, "\n") {
			if v, ok := strings.CutPrefix(line, "name: "); ok {
				gotName = strings.TrimSpace(v)
			}
			if v, ok := strings.CutPrefix(line, "description: "); ok {
				gotDesc = strings.Trim(strings.TrimSpace(v), `"`)
			}
		}
		if gotName != n {
			t.Errorf("%s: frontmatter name is %q; it must match the directory or the skill is addressed by two names", n, gotName)
		}
		switch {
		case gotDesc == "":
			t.Errorf("%s: no description — nothing can decide when to invoke it", n)
		case len(gotDesc) < 40:
			t.Errorf("%s: description is %d chars; too short to say when to use it: %q", n, len(gotDesc), gotDesc)
		case len(gotDesc) > 600:
			t.Errorf("%s: description is %d chars; skill listings are truncated well before that", n, len(gotDesc))
		}
	}
}

// The point of the migration: a Claude project gets `.claude/skills/<name>/SKILL.md`,
// which is the only path Claude Code scans. Before this, `mxcli init` created
// `.claude/commands/`, `.claude/lint-rules/` and no skills directory at all.
func TestSyncWritesTheDirectoryClaudeCodeScans(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := syncAIContextSkills(dir); err != nil {
		t.Fatal(err)
	}

	names := embeddedSkillNames(t)
	for _, base := range []string{".ai-context", ".claude"} {
		p := filepath.Join(dir, base, "skills", names[0], "SKILL.md")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s not written: %v", p, err)
		}
	}

	// Control: without `.claude/`, the Claude copy is not created — the project
	// was not set up for it, and mxcli should not invent the directory.
	bare := t.TempDir()
	if _, err := syncAIContextSkills(bare); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bare, ".claude", "skills")); err == nil {
		t.Error(".claude/skills/ was created in a project that has no .claude/")
	}
}

// Upgrading a project written by an older mxcli must retire the flat files it
// used to own — otherwise 67 orphans sit beside the new tree, and an agent
// reading the directory cannot tell which copy is current. A file mxcli never
// wrote is left alone: the docs invite users to add their own skills here.
func TestSyncRetiresLegacyFlatSkillsButKeepsUserFiles(t *testing.T) {
	dir := t.TempDir()
	skillsDir := filepath.Join(dir, ".ai-context", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	names := embeddedSkillNames(t)
	legacy := filepath.Join(skillsDir, names[0]+".md")
	if err := os.WriteFile(legacy, []byte("# shipped by an older mxcli\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(skillsDir, "our-house-conventions.md")
	if err := os.WriteFile(mine, []byte("# ours\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := syncAIContextSkills(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Errorf("%s survived the upgrade; it now contradicts %s/SKILL.md", filepath.Base(legacy), names[0])
	}
	if !containsString(res.Removed, names[0]+".md") {
		t.Errorf("the retirement was not reported: %v", res.Removed)
	}
	if _, err := os.Stat(mine); err != nil {
		t.Error("a user's own skill file was deleted; only names mxcli ships may be retired")
	}
}

func containsString(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
