// SPDX-License-Identifier: Apache-2.0

// check.go - does the store still describe the project?
//
// Two independent questions, and conflating them is the mistake this file
// exists to avoid:
//
//   - Does each anchor still resolve? Three states, and only one is a failure.
//     An anchor to a document type the catalog does not index resolves as
//     *missing* through the catalog alone, which is a false staleness signal —
//     so a second, type-agnostic lookup separates "gone" from "not indexed".
//   - Is the entry in the right shard? A separate axis, not a fourth state: an
//     anchor can resolve perfectly and the entry still sit in the wrong file.
package brain

import "sort"

// AnchorState is the outcome of resolving one anchor.
type AnchorState int

const (
	// Resolved: the anchor names something the catalog knows.
	Resolved AnchorState = iota
	// NotFound: the anchor names nothing in the project. The only failure.
	NotFound
	// NotIndexable: the target exists but is of a type the catalog's objects
	// view does not cover. Reported, never failed — treating it as missing
	// would make `check` demand edits to entries that are perfectly current.
	NotIndexable
)

func (s AnchorState) String() string {
	switch s {
	case Resolved:
		return "resolved"
	case NotFound:
		return "not found"
	default:
		return "not indexable"
	}
}

// Resolution is what a Resolver reports about one anchor.
type Resolution struct {
	State AnchorState
	// Module is the module the target actually lives in, which is what the
	// misfiling check compares against. Empty unless State is Resolved.
	Module string
	// Kind is the target's type, for the report ("entity", "microflow").
	Kind string
}

// Resolver answers anchors against a project. It is an interface so the check
// logic is testable without a .mpr — the states that matter are the awkward
// ones, and a fixture project cannot readily produce a NotIndexable.
type Resolver interface {
	Resolve(Anchor) (Resolution, error)
}

// AnchorFinding is one anchor's outcome.
type AnchorFinding struct {
	Shard   string
	EntryID string
	Title   string
	Anchor  string
	State   AnchorState
	Kind    string
}

// MisfiledFinding is an entry sitting in a shard none of its anchors belong to.
type MisfiledFinding struct {
	Shard   string
	EntryID string
	Title   string
	Belongs string // the shard it should be in, from its first resolved anchor
}

// SliceProgress is a slice's requirements counted against the model. Every
// figure is derived from resolving anchors, so nothing here is self-reported
// and no one has to maintain a status column that will go stale.
type SliceProgress struct {
	Slice string
	// Built is requirements whose anchors all resolve — the thing exists.
	Built int
	// Planned is requirements with at least one anchor that does not resolve
	// yet. Not a failure: that is what a requirement is until it is built.
	Planned int
	// Unanchored is requirements with no anchor at all. They cannot be
	// measured, and are counted apart rather than silently called planned.
	Unanchored int
}

// Total is every requirement in the slice.
func (p SliceProgress) Total() int { return p.Built + p.Planned + p.Unanchored }

// Report is what `brain check` prints and exits on.
type Report struct {
	Shards    []string
	Entries   int
	Anchors   int
	ResolvedN int
	Findings  []AnchorFinding // NotFound and NotIndexable only
	Misfiled  []MisfiledFinding
	Malformed []string // entry blocks whose metadata line could not be read
	Slices    []SliceProgress
}

// Failed reports whether the check should exit non-zero.
//
// Only two things fail: an anchor that names nothing at all, and an entry in
// the wrong shard. A NotIndexable anchor is information, not a defect — see the
// constant's comment.
func (r Report) Failed() bool {
	if len(r.Misfiled) > 0 || len(r.Malformed) > 0 {
		return true
	}
	for _, f := range r.Findings {
		if f.State == NotFound {
			return true
		}
	}
	return false
}

// Check validates the given shards. Passing a subset is how `--changed` avoids
// paying for shards a diff did not touch.
func Check(s *Store, r Resolver, shards []string) (Report, error) {
	rep := Report{Shards: shards}
	for _, shard := range shards {
		entries, malformed, err := s.LoadShard(shard)
		if err != nil {
			return rep, err
		}
		for _, m := range malformed {
			rep.Malformed = append(rep.Malformed, shard+": "+m)
		}
		if IsPlanShard(shard) {
			progress, err := checkSlice(r, shard, entries)
			if err != nil {
				return rep, err
			}
			rep.Entries += len(entries)
			rep.Anchors += progress.anchors
			rep.ResolvedN += progress.resolved
			rep.Slices = append(rep.Slices, progress.SliceProgress)
			continue
		}
		for _, e := range entries {
			rep.Entries++
			var resolvedModules []string
			for _, a := range e.ParsedAnchors() {
				rep.Anchors++
				res, err := r.Resolve(a)
				if err != nil {
					return rep, err
				}
				if res.State == Resolved {
					rep.ResolvedN++
					resolvedModules = append(resolvedModules, res.Module)
					continue
				}
				rep.Findings = append(rep.Findings, AnchorFinding{
					Shard: shard, EntryID: e.ID, Title: e.Title,
					Anchor: a.String(), State: res.State, Kind: res.Kind,
				})
			}
			if MisfiledIn(shard, resolvedModules) {
				rep.Misfiled = append(rep.Misfiled, MisfiledFinding{
					Shard: shard, EntryID: e.ID, Title: e.Title,
					Belongs: belongsIn(resolvedModules),
				})
			}
		}
	}
	return rep, nil
}

type sliceCounts struct {
	SliceProgress
	anchors, resolved int
}

// checkSlice counts a slice's requirements against the model. It records no
// findings and no misfiling, and that is the point rather than an omission:
//
//   - A requirement's anchor points FORWARD. Not resolving means not built,
//     which is the normal state of a requirement and must never fail a check —
//     measured: filed as an ordinary entry, one unbuilt requirement took
//     `brain check` to exit 1.
//   - A slice spans modules by design ("approvals" touches Sales and Finance),
//     so the misfiling rule that keeps decisions honest does not apply.
func checkSlice(r Resolver, shard string, entries []Entry) (sliceCounts, error) {
	out := sliceCounts{SliceProgress: SliceProgress{Slice: SliceOf(shard)}}
	for _, e := range entries {
		anchors := e.ParsedAnchors()
		if len(anchors) == 0 {
			out.Unanchored++
			continue
		}
		built := true
		for _, a := range anchors {
			out.anchors++
			res, err := r.Resolve(a)
			if err != nil {
				return out, err
			}
			// NotIndexable counts as built: the thing is there, the catalog
			// simply does not index its type. Calling it planned would report
			// finished work as outstanding.
			if res.State == NotFound {
				built = false
				continue
			}
			out.resolved++
		}
		if built {
			out.Built++
		} else {
			out.Planned++
		}
	}
	return out, nil
}

// belongsIn names the shard an entry should have gone to. With no resolved
// anchor at all there is nothing to suggest, and the empty string says so
// rather than guessing.
func belongsIn(resolved []string) string {
	if len(resolved) == 0 {
		return ""
	}
	uniq := map[string]bool{}
	for _, m := range resolved {
		uniq[m] = true
	}
	keys := make([]string, 0, len(uniq))
	for k := range uniq {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic output; maps iterate randomly
	return keys[0]
}
