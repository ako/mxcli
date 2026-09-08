// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"os"
	"strings"
	"testing"
)

// SaveShard re-renders a shard from the entries it parsed, so anything in the
// file that is not an entry was silently discarded on the next promote, drop,
// resolve or rename. YAML frontmatter is the case that matters: it is where
// every markdown tool in the ecosystem keeps its per-file metadata — Foam and
// Obsidian tags, Jekyll/Hugo front matter, a docs site's nav weight — and a
// store that eats it cannot be kept in one of them.
//
// Silently is the operative word. The file stays valid, the entries are all
// there, and the loss shows up whenever someone next looks. That is the failure
// mode ADR-0005's guard-don't-drop rule exists to prevent, and this writer was
// on the wrong side of it.
func TestFrontmatterSurvivesAPromote(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "planning uses a snapshot", "@Planning.Snapshot"))
	addFrontmatter(t, s, "Planning", "---\ntags: [planning, architecture]\nweight: 3\n---\n")

	promote(t, s, mustEntry(t, "rollups run nightly", "@Planning.ACT_Rollup"))

	got := readShard(t, s, "Planning")
	if !strings.HasPrefix(got, "---\ntags: [planning, architecture]\nweight: 3\n---\n") {
		t.Fatalf("frontmatter did not survive the promote:\n%s", got)
	}
	// It has to stay frontmatter — first thing in the file, before the title —
	// or it is inert text that every tool reads as body content.
	if strings.Index(got, "---") > strings.Index(got, "# Planning") {
		t.Error("frontmatter is no longer at the top of the file")
	}
	for _, want := range []string{"planning uses a snapshot", "rollups run nightly"} {
		if !strings.Contains(got, want) {
			t.Errorf("entry %q was lost while preserving frontmatter", want)
		}
	}
}

// Every write path rewrites the whole file, so each has to be covered. Promote
// is the obvious one; these are the three that are easy to forget.
func TestFrontmatterSurvivesEveryWritePath(t *testing.T) {
	t.Run("drop", func(t *testing.T) {
		s := newTestStore(t)
		keep := mustEntry(t, "planning uses a snapshot", "@Planning.Snapshot")
		gone := mustEntry(t, "rollups run nightly", "@Planning.ACT_Rollup")
		promote(t, s, keep)
		promote(t, s, gone)
		addFrontmatter(t, s, "Planning", "---\ntags: [planning]\n---\n")

		if _, _, err := s.Drop(gone.ID); err != nil {
			t.Fatal(err)
		}
		assertFrontmatter(t, s, "Planning", "tags: [planning]")
	})

	t.Run("resolve", func(t *testing.T) {
		s := newTestStore(t)
		q, err := NewQuestion("should planning own the totals", []string{"@Planning.Snapshot"}, "", day)
		if err != nil {
			t.Fatal(err)
		}
		promote(t, s, q)
		addFrontmatter(t, s, "Planning", "---\ntags: [planning]\n---\n")

		answered, err := q.Resolve("yes, Finance reads them", day)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Replace("Planning", answered); err != nil {
			t.Fatal(err)
		}
		assertFrontmatter(t, s, "Planning", "tags: [planning]")
	})

	t.Run("rename", func(t *testing.T) {
		s := newTestStore(t)
		promote(t, s, mustEntry(t, "planning uses a snapshot", "@Planning.Snapshot"))
		addFrontmatter(t, s, "Planning", "---\ntags: [planning]\n---\n")

		if _, err := s.RenameAnchors("Planning.Snapshot", "Planning.Baseline"); err != nil {
			t.Fatal(err)
		}
		assertFrontmatter(t, s, "Planning", "tags: [planning]")
	})
}

// A module rename writes to a NEW path and deletes the old one, so there is no
// existing file at the destination to read the frontmatter back off. It has to
// be carried across the move explicitly — the one place the choke point alone
// does not cover.
func TestFrontmatterMovesWithARenamedModuleShard(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "planning uses a snapshot", "@Planning.Snapshot"))
	addFrontmatter(t, s, "Planning", "---\ntags: [planning]\nweight: 3\n---\n")

	if _, err := s.RenameAnchors("Planning", "Forecasting"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(s.ShardPath("Planning")); !os.IsNotExist(err) {
		t.Error("the old shard survived the module rename")
	}
	assertFrontmatter(t, s, "Forecasting", "tags: [planning]")
	assertFrontmatter(t, s, "Forecasting", "weight: 3")
}

// Control: a shard nobody has added frontmatter to must not grow an empty
// block. Without this the fix could "pass" by emitting `---\n---` everywhere,
// which is a diff on every existing project and reads as metadata that is not
// there.
func TestShardWithoutFrontmatterStaysWithout(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "planning uses a snapshot", "@Planning.Snapshot"))
	promote(t, s, mustEntry(t, "rollups run nightly", "@Planning.ACT_Rollup"))

	got := readShard(t, s, "Planning")
	if !strings.HasPrefix(got, "# Planning") {
		t.Errorf("a shard with no frontmatter no longer starts with its title:\n%s", got)
	}
	if strings.Contains(got, "---") {
		t.Errorf("an empty frontmatter block was invented:\n%s", got)
	}
}

// An unterminated `---` is not frontmatter, and treating it as such would
// swallow the whole document — every entry in the file would be carried
// forward as opaque preserved text and then written back twice. Refusing to
// recognise it leaves the old behaviour for that one file, which is the safe
// direction.
func TestUnterminatedFrontmatterIsNotTreatedAsFrontmatter(t *testing.T) {
	for _, tc := range []struct {
		name, content string
	}{
		{"no closing fence", "---\ntags: [planning]\n\n# Planning\n\n## a decision\n"},
		{"fence not at the start", "# Planning\n\n---\ntags: [x]\n---\n"},
		{"horizontal rule", "***\n\n# Planning\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if fm := extractFrontmatter(tc.content); fm != "" {
				t.Errorf("treated %q as frontmatter: %q", tc.content, fm)
			}
		})
	}
}

func TestFrontmatterIsExtractedWhenWellFormed(t *testing.T) {
	got := extractFrontmatter("---\ntags: [planning]\n---\n\n# Planning\n")
	if got != "---\ntags: [planning]\n---\n" {
		t.Errorf("extractFrontmatter = %q", got)
	}
}

// The cap is what a session pays to load the shard, and frontmatter is part of
// what it loads. `brain show` counts lines off the file on disk, so if the
// promote-time check ignored frontmatter the two would disagree — and the
// disagreement would appear exactly when a shard is near its limit.
func TestFrontmatterCountsTowardTheCap(t *testing.T) {
	s := newTestStore(t)
	promote(t, s, mustEntry(t, "planning uses a snapshot", "@Planning.Snapshot"))

	before := usageFor(t, s, "Planning")
	addFrontmatter(t, s, "Planning", "---\ntags: [planning]\nweight: 3\n---\n")
	promote(t, s, mustEntry(t, "rollups run nightly", "@Planning.ACT_Rollup"))
	after := usageFor(t, s, "Planning")

	if after.Lines <= before.Lines+2 {
		t.Errorf("shard was %d lines, now %d; the four frontmatter lines are not being counted",
			before.Lines, after.Lines)
	}
}

func addFrontmatter(t *testing.T, s *Store, shard, fm string) {
	t.Helper()
	path := s.ShardPath(shard)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append([]byte(fm+"\n"), body...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readShard(t *testing.T, s *Store, shard string) string {
	t.Helper()
	b, err := os.ReadFile(s.ShardPath(shard))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertFrontmatter(t *testing.T, s *Store, shard, want string) {
	t.Helper()
	got := readShard(t, s, shard)
	if !strings.HasPrefix(got, "---\n") || !strings.Contains(got, want) {
		t.Errorf("%s lost its frontmatter (%q):\n%s", shard, want, got)
	}
}

func usageFor(t *testing.T, s *Store, shard string) Usage {
	t.Helper()
	usage, err := s.Usage()
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range usage {
		if u.Shard == shard {
			return u
		}
	}
	t.Fatalf("no usage for shard %s", shard)
	return Usage{}
}
