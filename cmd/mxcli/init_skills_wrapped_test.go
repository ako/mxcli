// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// skillNameSet is embeddedSkillNames() as a set, for membership checks.
func skillNameSet(t *testing.T) map[string]bool {
	t.Helper()
	set := map[string]bool{}
	for _, n := range embeddedSkillNames(t) {
		set[n] = true
	}
	return set
}

// TestWriteWrappedSkills_OneDirectoryPerSkill pins the failure that shipped when
// skills moved to <name>/SKILL.md: the name was read off the file, so every
// skill resolved to "SKILL" and they all wrote to the same directory.
func TestWriteWrappedSkills_OneDirectoryPerSkill(t *testing.T) {
	dest := t.TempDir()
	want := skillNameSet(t)

	n, err := writeWrappedSkills(dest, wrapSkillContent)
	if err != nil {
		t.Fatalf("writeWrappedSkills: %v", err)
	}
	if n != len(want) {
		t.Errorf("wrote %d skills, embedded carries %d", n, len(want))
	}

	// The collapse symptom, named directly: a folder called SKILL means every
	// skill landed on the same path and all but one was overwritten.
	if _, err := os.Stat(filepath.Join(dest, "SKILL")); err == nil {
		t.Error("a directory named SKILL exists — skills collapsed onto one path (name taken from the file, not the folder)")
	}

	for name := range want {
		root := filepath.Join(dest, name, "SKILL.md")
		if _, err := os.Stat(root); err != nil {
			t.Errorf("skill %q has no SKILL.md at %s: %v", name, root, err)
		}
	}
}

// TestWriteWrappedSkills_ResourcesAreNotSkills covers the second half of the
// same defect: a skill's supporting .md files were each promoted to a top-level
// skill, so .opencode/skills/ filled up with fragments like "pitfalls" and
// "writing-java" that no skill index should list.
func TestWriteWrappedSkills_ResourcesAreNotSkills(t *testing.T) {
	dest := t.TempDir()
	want := skillNameSet(t)

	if _, err := writeWrappedSkills(dest, wrapSkillContent); err != nil {
		t.Fatalf("writeWrappedSkills: %v", err)
	}

	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatalf("reading dest: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("unexpected loose file %q at the skills root", e.Name())
			continue
		}
		if !want[e.Name()] {
			t.Errorf("%q is not an embedded skill — a supporting file was promoted to a skill", e.Name())
		}
	}
}

// TestWriteWrappedSkills_ResourceFilesSurviveVerbatim is the guard for shipping
// a non-markdown asset (narrate.js) inside a skill folder: the resource must
// arrive byte-identical and must NOT be wrapped in skill frontmatter.
func TestWriteWrappedSkills_ResourceFilesSurviveVerbatim(t *testing.T) {
	dest := t.TempDir()
	if _, err := writeWrappedSkills(dest, wrapSkillContent); err != nil {
		t.Fatalf("writeWrappedSkills: %v", err)
	}

	checked := 0
	err := fs.WalkDir(skillsFS, "skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel("skills", filepath.FromSlash(p))
		if relErr != nil || filepath.Dir(rel) == "." || filepath.Base(rel) == "SKILL.md" {
			return relErr
		}
		want, readErr := skillsFS.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		got, readErr := os.ReadFile(filepath.Join(dest, rel))
		if readErr != nil {
			t.Errorf("resource %s missing from output: %v", rel, readErr)
			return nil
		}
		if string(got) != string(want) {
			t.Errorf("resource %s was altered in transit (len %d -> %d)", rel, len(want), len(got))
		}
		if strings.HasPrefix(string(got), "---\nname:") {
			t.Errorf("resource %s was wrapped in skill frontmatter — it is not a skill", rel)
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walking skills: %v", err)
	}
	if checked == 0 {
		t.Skip("no skill carries a supporting file yet — nothing to prove")
	}
}
