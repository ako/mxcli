// SPDX-License-Identifier: Apache-2.0

// rename.go - keeping anchors valid when the model moves under them.
//
// `mxcli rename` updates every cross-reference in the model. The brain's
// anchors are references to the same elements and were not among them, so a
// refactor silently invalidated them.
//
// `brain check` catches only half of that, and the missing half is inherent to
// the design rather than a gap in the check. A decision's anchor points BACKWARD
// at something that exists, so one that stops resolving is reported NOT FOUND. A
// requirement's points FORWARD at something intended, so one that stops
// resolving counts as PLANNED — which is exactly what a forward anchor failing
// is supposed to mean. There is no way to tell "never built" from "was built,
// then renamed" after the fact. The observed symptom was a plan's progress
// moving from 65/65 to 63/65 with nothing else to see.
//
// That ambiguity is the argument for fixing it at the rename, where both names
// are still known, rather than at the check, where neither is.
package brain

import (
	"fmt"
	"os"
	"strings"
)

// RenameAnchors rewrites every anchor naming old so it names new, and returns
// how many it changed. Both names are qualified: "Sales.Order", or "Sales" for
// a module.
//
// An entry's id is NOT re-derived. The id is content-derived and its content
// includes its anchors, so re-deriving is the obvious move and is wrong: the id
// is a handle (`brain promote <id>`, `brain resolve <id>`, prose that cites
// one), and invalidating every reference TO an entry in order to fix that
// entry's references to the model trades one dangling pointer for several.
func (s *Store) RenameAnchors(old, new string) (int, error) {
	shards, err := s.ListShards()
	if err != nil {
		return 0, err
	}

	// A module rename moves the module's own shard. Without that, every entry
	// in modules/Old.md anchors into New the moment the rewrite lands and
	// `brain check` reports the lot as misfiled — one false signal traded for
	// another. Element renames never move anything: the module is unchanged.
	moveShard := ""
	if !strings.Contains(old, ".") {
		moveShard = old
	}

	var total int
	for _, shard := range shards {
		entries, malformed, err := s.LoadShard(shard)
		if err != nil {
			return total, err
		}
		if len(malformed) > 0 {
			// Refuse rather than rewrite around it: SaveShard re-renders the
			// whole file from the entries it parsed, so writing back a shard
			// with an unreadable block would delete that block.
			return total, fmt.Errorf("%s has %d entry block(s) that cannot be parsed; "+
				"fix them before renaming, or the rewrite would drop them: %s",
				shardFileName(shard), len(malformed), strings.Join(malformed, "; "))
		}

		var changed int
		for i := range entries {
			for j, a := range entries[i].Anchors {
				if rewritten, ok := rewriteAnchor(a, old, new); ok {
					entries[i].Anchors[j] = rewritten
					changed++
				}
			}
		}
		if changed == 0 {
			continue
		}
		total += changed

		if shard == moveShard {
			// Write the new shard first, then drop the old one, so an
			// interruption leaves a duplicate rather than nothing.
			if err := s.SaveShard(new, entries); err != nil {
				return total, err
			}
			if err := os.Remove(s.ShardPath(shard)); err != nil && !os.IsNotExist(err) {
				return total, err
			}
			continue
		}
		if err := s.SaveShard(shard, entries); err != nil {
			return total, err
		}
	}
	return total, nil
}

// RenameQueueAnchors does the same to the staged queue. Captures that have not
// been promoted yet are the ones most likely to name something just renamed —
// nobody has looked at them, so nobody has noticed.
func RenameQueueAnchors(q *Queue, old, new string) (int, error) {
	release, err := acquireQueueLock(q.Path)
	if err != nil {
		return 0, err
	}
	defer release()

	entries, err := q.Load()
	if err != nil {
		return 0, err
	}
	var changed int
	for i := range entries {
		for j, a := range entries[i].Anchors {
			if rewritten, ok := rewriteAnchor(a, old, new); ok {
				entries[i].Anchors[j] = rewritten
				changed++
			}
		}
	}
	if changed == 0 {
		return 0, nil
	}
	return changed, q.write(entries)
}

// rewriteAnchor replaces the old qualified name in one anchor, matching only at
// a name boundary.
//
// The boundary is the whole difficulty. Anchors are dotted names, so a plain
// prefix replace rewrites everything whose name merely STARTS with the renamed
// one: renaming Sales.Order would turn @Sales.OrderLine into
// @Sales.PurchaseOrderLine, which names nothing — and which `brain check` then
// reports as a stale decision, so the repair invents the very problem it exists
// to prevent. A match must be followed by end-of-anchor or a dot.
func rewriteAnchor(anchor, old, new string) (string, bool) {
	at, name := "", anchor
	if strings.HasPrefix(anchor, "@") {
		at, name = "@", anchor[1:]
	}
	if name == old {
		return at + new, true
	}
	if rest, ok := strings.CutPrefix(name, old+"."); ok {
		return at + new + "." + rest, true
	}
	return anchor, false
}

// CountAnchorsNaming reports how many anchors a rename would rewrite, without
// touching anything. It backs `mxcli rename --dry-run`, which must preview the
// brain's share of the change as well as the model's — a preview that silently
// omitted it would understate what the real run does.
func CountAnchorsNaming(s *Store, old string) (int, error) {
	shards, err := s.ListShards()
	if err != nil {
		return 0, err
	}
	var n int
	for _, shard := range shards {
		entries, _, err := s.LoadShard(shard)
		if err != nil {
			return 0, err
		}
		for _, e := range entries {
			for _, a := range e.Anchors {
				if _, ok := rewriteAnchor(a, old, old); ok {
					n++
				}
			}
		}
	}
	return n, nil
}
