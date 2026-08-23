// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// init_skills_sync.go keeps a project's .ai-context/skills/ in step with the
// mxcli binary that serves it.
//
// The skills are embedded in the binary and written once by `mxcli init`.
// Upgrading the binary therefore did nothing to them: a project initialised on
// Monday still served Monday's guidance from Tuesday's mxcli, with no warning —
// confirmed with a binary rebuilt at 12:05 beside skills stamped the previous
// day (mxcli-formula1 §16). Stale guidance is worse than missing guidance,
// because an agent reads it with the same confidence either way, and the whole
// point of shipping skills in the binary is that the two versions agree.
//
// These files are generated, never user-edited (the sources live in the mxcli
// repo), so refreshing is a copy, not a merge. The SessionStart bootstrap script
// runs it on every session — the moment before an agent would read them.

// skillSyncResult reports what a refresh did.
type skillSyncResult struct {
	Total   int      // skills the binary carries
	Changed []string // names whose on-disk content differed (added or updated)
	Removed []string // legacy flat files retired by the directory layout
}

// Stale reports whether anything on disk disagreed with the binary.
func (r skillSyncResult) Stale() bool { return len(r.Changed) > 0 || len(r.Removed) > 0 }

// skillDests returns every directory a project's skills are mirrored into.
//
// `.ai-context/skills/` is the vendor-neutral home and is always written.
// `.claude/skills/` is written too when the project is set up for Claude Code,
// because that is the only path it scans for skills — one level deep, `<name>/
// SKILL.md`. Before this existed a project carried 68 skills that Claude Code
// could not see at all, and routing depended entirely on a hand-maintained
// table in CLAUDE.md (issue #906).
//
// Presence of `.claude/` is the signal rather than a flag, so the SessionStart
// refresh — which has no memory of which tools were chosen at init — keeps both
// copies current without being told.
func skillDests(projectDir string) []string {
	dests := []string{filepath.Join(projectDir, ".ai-context", "skills")}
	if st, err := os.Stat(filepath.Join(projectDir, ".claude")); err == nil && st.IsDir() {
		dests = append(dests, filepath.Join(projectDir, ".claude", "skills"))
	}
	return dests
}

// syncAIContextSkills rewrites a project's skill directories from the binary's
// embedded copies, reporting what differed. Writing only the files that changed
// keeps mtimes meaningful, so "when did this guidance last move" stays
// answerable from the filesystem.
func syncAIContextSkills(projectDir string) (skillSyncResult, error) {
	var res skillSyncResult
	seen := map[string]bool{}
	for i, dest := range skillDests(projectDir) {
		one, err := syncSkillTree(dest)
		if err != nil {
			return res, err
		}
		if i == 0 {
			res.Total = one.Total
		}
		for _, n := range one.Changed {
			if !seen["c:"+n] {
				seen["c:"+n] = true
				res.Changed = append(res.Changed, n)
			}
		}
		for _, n := range one.Removed {
			if !seen["r:"+n] {
				seen["r:"+n] = true
				res.Removed = append(res.Removed, n)
			}
		}
	}
	sort.Strings(res.Changed)
	sort.Strings(res.Removed)
	return res, nil
}

// syncSkillTree mirrors the embedded skills into one destination directory.
func syncSkillTree(skillsDir string) (skillSyncResult, error) {
	var res skillSyncResult

	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return res, fmt.Errorf("creating %s: %w", skillsDir, err)
	}

	live := map[string]bool{}
	err := fs.WalkDir(skillsFS, "skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel("skills", filepath.FromSlash(p))
		if relErr != nil || rel == "." {
			return relErr
		}
		target := filepath.Join(skillsDir, rel)
		if d.IsDir() {
			live[rel] = true
			return os.MkdirAll(target, 0o755)
		}
		want, err := skillsFS.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading embedded skill %s: %w", p, err)
		}
		live[rel] = true
		if filepath.Base(rel) == "SKILL.md" {
			res.Total++
		}
		if have, readErr := os.ReadFile(target); readErr == nil && bytes.Equal(have, want) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, want, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
		res.Changed = append(res.Changed, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return res, err
	}

	res.Removed = retireFlatSkills(skillsDir, live)
	sort.Strings(res.Changed)
	sort.Strings(res.Removed)
	return res, nil
}

// retireFlatSkills deletes the pre-#906 flat `<name>.md` files this binary used
// to write, once `<name>/SKILL.md` has replaced them.
//
// The sync has never deleted anything, which was right while the layout was
// stable and is exactly wrong across a layout change: without this, every
// project upgraded from an older mxcli keeps 67 orphaned files that no longer
// match the shipped guidance, sitting beside the ones that do. An agent reading
// the directory cannot tell which is current.
//
// Only names the binary itself used to own are removed, and only when the
// replacement directory is present, so a user's own skill file in the same
// directory — which the docs explicitly invite — is never touched.
func retireFlatSkills(skillsDir string, live map[string]bool) []string {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var removed []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		if !live[filepath.Join(name, "SKILL.md")] {
			continue // not a skill this binary ships — leave it alone
		}
		if err := os.Remove(filepath.Join(skillsDir, e.Name())); err == nil {
			removed = append(removed, e.Name())
		}
	}
	return removed
}

// reportSkillSync prints a one-line summary, and nothing at all when the project
// was already current — this runs on every session start, so silence is the
// common case and the only acceptable one.
func reportSkillSync(w io.Writer, res skillSyncResult) {
	if !res.Stale() {
		return
	}
	if len(res.Changed) > 0 {
		fmt.Fprintf(w, "Refreshed %d file(s) across %d skills to match this mxcli: %s\n",
			len(res.Changed), res.Total, abridge(res.Changed))
	}
	if len(res.Removed) > 0 {
		fmt.Fprintf(w, "Retired %d file(s) superseded by the <name>/SKILL.md layout: %s\n",
			len(res.Removed), abridge(res.Removed))
	}
}

// abridge renders a file list without turning a first-run or post-upgrade
// refresh into a wall of 68 names. The count above it is the number that
// matters; the names are there to answer "which one moved" on the ordinary
// one-or-two-file refresh.
func abridge(names []string) string {
	const max = 6
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:max], ", "), len(names)-max)
}
