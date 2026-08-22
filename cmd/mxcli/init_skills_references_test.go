// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io/fs"
	"path"
	"strings"
	"testing"
)

// A supporting file is inert unless SKILL.md links it: Claude loads the body, and
// only follows a link it can see. The Agent Skills docs are explicit — "Reference
// supporting files from SKILL.md so Claude knows what each file contains and when
// to load it" — so an unlinked reference file is dead weight that ships with every
// project and is never read.
//
// This is the one property the split of the six largest skills depends on. It
// cannot be checked by reading a single file, which is why it gets a test rather
// than a convention.
func TestEverySupportingFileIsLinkedFromItsSkill(t *testing.T) {
	names := embeddedSkillNames(t)

	for _, skill := range names {
		body, err := skillsFS.ReadFile("skills/" + skill + "/SKILL.md")
		if err != nil {
			t.Errorf("%s: %v", skill, err)
			continue
		}
		text := string(body)

		var supporting []string
		err = fs.WalkDir(skillsFS, "skills/"+skill, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || path.Base(p) == "SKILL.md" {
				return err
			}
			if strings.HasSuffix(p, ".md") {
				rel := strings.TrimPrefix(p, "skills/"+skill+"/")
				supporting = append(supporting, rel)
			}
			return nil
		})
		if err != nil {
			t.Errorf("%s: walking: %v", skill, err)
			continue
		}

		for _, rel := range supporting {
			if !strings.Contains(text, "("+rel+")") {
				t.Errorf("%s/SKILL.md does not link %s — nothing will ever open it", skill, rel)
			}
		}
	}
}

// Splitting is only worth doing if the body actually got smaller. These six were
// 811–1906 lines, well past the documented "keep SKILL.md under 500 lines", and
// they are the ones agents load most often. The bound is deliberately loose: a
// body that is all essential beats one that hit a number by dropping something,
// and the heading-preservation check lives in the split itself.
func TestLargeSkillsWereSplit(t *testing.T) {
	const bound = 700

	for _, skill := range embeddedSkillNames(t) {
		body, err := skillsFS.ReadFile("skills/" + skill + "/SKILL.md")
		if err != nil {
			t.Errorf("%s: %v", skill, err)
			continue
		}
		if n := strings.Count(string(body), "\n"); n > bound {
			t.Errorf("%s/SKILL.md is %d lines (> %d) and has no supporting files to move detail into; "+
				"a body this long is loaded whole every time the skill is used", skill, n, bound)
		}
	}
}
