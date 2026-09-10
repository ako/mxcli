// SPDX-License-Identifier: Apache-2.0

// brief.go - the reading pack: exactly the shards a session needs, once.
//
// The store is sharded so a session can load project.md plus the modules it is
// touching instead of the whole thing, and the README says so. Nothing produced
// that pack, though: docs/brain/ is a directory, so a session either read all of
// it or guessed, and both are wrong in the same direction. Measured on a real
// 15-slice project, the whole store is 7,531 tokens and the correct pack for one
// slice is ~2,530.
//
// In one long session that is a rounding error — the store is read once and then
// cached. Under one sub-agent per slice it is a third of the context, because
// the pack is re-read per slice from a cold start, which is exactly the shape
// the sharding was designed for and the only one where it was not usable.
//
// Which module shards belong in the pack is DERIVED: they are the modules the
// slice's own requirements anchor into. Asking the caller which modules its
// slice touches would be asking it the thing it opened the brief to find out.
package brain

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// BriefShard is one shard's contents as they appear in a brief.
type BriefShard struct {
	Shard string `json:"shard"`
	Path  string `json:"path"`
	Body  string `json:"body"`
	Lines int    `json:"lines"`
}

// Brief is a reading pack: the shards a session needs, in the order it should
// read them, with the size it is paying.
type Brief struct {
	// Slice is the plan shard the brief was built for, empty for a
	// module-scoped brief.
	Slice  string       `json:"slice,omitempty"`
	Shards []BriefShard `json:"shards"`
	Lines  int          `json:"lines"`
	Bytes  int          `json:"bytes"`
	// StoreLines is the whole store, for comparison. The reason the brief
	// exists is the size of the alternative, so it reports both.
	StoreLines int `json:"store_lines"`
}

// Text renders the brief as one document, which is the form a session reads.
func (b Brief) Text() string {
	var sb strings.Builder
	for _, s := range b.Shards {
		sb.WriteString(s.Body)
		if !strings.HasSuffix(s.Body, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// Brief builds the pack for one slice: project.md, the shards of the modules
// that slice's requirements anchor into, and the slice's own plan shard.
//
// Ordering is deliberate and not alphabetical by accident: project first
// because it is the unconditional read, then the modules, then the plan. A
// session reads the decisions it must not contradict before the scope it is
// about to build.
func (s *Store) Brief(slice string) (Brief, error) {
	planShard := PlanShard(slice)
	entries, _, err := s.LoadShard(planShard)
	if err != nil {
		return Brief{}, err
	}

	seen := map[string]bool{}
	var modules []string
	for _, e := range entries {
		for _, a := range e.ParsedAnchors() {
			// Every anchor, not just the first. The first anchor decides where
			// an entry is FILED; a requirement spanning two modules is worked in
			// both, and a session that read only one of them is the case this
			// exists to prevent.
			if a.Module != "" && !seen[a.Module] {
				seen[a.Module] = true
				modules = append(modules, a.Module)
			}
		}
	}
	sort.Strings(modules)

	want := append([]string{ProjectShard}, modules...)
	if len(entries) > 0 {
		want = append(want, planShard)
	}
	b, err := s.briefOf(want)
	if err != nil {
		return Brief{}, err
	}
	b.Slice = slice
	return b, nil
}

// BriefForModules builds the pack for a session working named modules rather
// than a slice — maintenance rather than roadmap. No plan shard: nothing here
// says which slice the work belongs to, and guessing one would be inventing it.
func (s *Store) BriefForModules(modules []string) (Brief, error) {
	sorted := append([]string(nil), modules...)
	sort.Strings(sorted)
	return s.briefOf(append([]string{ProjectShard}, sorted...))
}

// briefOf reads the named shards, skipping those that do not exist. A missing
// shard is not an error: a module with no recorded decisions is the normal
// case, and refusing would make the brief unusable on exactly the projects
// that have only started recording.
func (s *Store) briefOf(shards []string) (Brief, error) {
	b := Brief{}
	for _, shard := range shards {
		path := s.ShardPath(shard)
		body, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Brief{}, err
		}
		text := string(body)
		b.Shards = append(b.Shards, BriefShard{
			Shard: shard,
			Path:  path,
			Body:  text,
			Lines: strings.Count(text, "\n"),
		})
	}
	b.Lines = strings.Count(b.Text(), "\n")
	b.Bytes = len(b.Text())

	total, err := s.Size()
	if err != nil {
		return Brief{}, err
	}
	b.StoreLines = total
	return b, nil
}

// Size is the whole store in lines — what a session pays for reading the
// directory instead of a brief. It is computed rather than recorded, like every
// other figure the store reports about itself.
func (s *Store) Size() (int, error) {
	shards, err := s.ListShards()
	if err != nil {
		return 0, err
	}
	var total int
	for _, shard := range shards {
		body, err := os.ReadFile(s.ShardPath(shard))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return 0, err
		}
		total += strings.Count(string(body), "\n")
	}
	return total, nil
}

// Summary is the one line a brief prints to stderr, so the saving is visible
// without polluting the pack itself on stdout.
func (b Brief) Summary() string {
	names := make([]string, 0, len(b.Shards))
	for _, s := range b.Shards {
		names = append(names, shardFileName(s.Shard))
	}
	return fmt.Sprintf("%d lines from %d shard(s) [%s]; whole store is %d",
		b.Lines, len(b.Shards), strings.Join(names, " "), b.StoreLines)
}

// shardFileName is the store-relative name, which is what a reader recognises.
func shardFileName(shard string) string {
	switch {
	case shard == ProjectShard:
		return "project.md"
	case IsPlanShard(shard):
		return "plan/" + SliceOf(shard) + ".md"
	default:
		return "modules/" + shard + ".md"
	}
}
