// SPDX-License-Identifier: Apache-2.0

// entry.go - the unit the brain stores.
//
// An entry is deliberately small: a title, an optional body, and the anchors
// that make it routable and checkable. Anything derivable from the model is not
// an entry — mxcli answers that with a query, and a store that transcribes it
// is a store that will disagree with the project.
package brain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Kind is what an entry records, and it decides what a failed anchor MEANS.
//
// This is the whole reason requirements are not simply more decisions. A
// decision's anchor points backward at something that exists, so an anchor that
// no longer resolves means the decision is stale. A requirement's anchor points
// forward at something intended, so an anchor that does not resolve means "not
// built yet" — the normal state of a requirement, and measured to fail the
// check outright when requirements were tried as ordinary entries.
type Kind string

const (
	KindDecision    Kind = "decision"
	KindRequirement Kind = "requirement"
)

// Entry is one recorded piece of project knowledge.
type Entry struct {
	ID string `json:"id"`
	// Kind is empty for a decision, so an entry written before requirements
	// existed still reads correctly. Use EntryKind rather than this field.
	Kind Kind `json:"kind,omitempty"`
	// Slice is the deliverable a requirement belongs to. Empty for a decision.
	Slice   string   `json:"slice,omitempty"`
	Title   string   `json:"title"`
	Body    string   `json:"body,omitempty"`
	Anchors []string `json:"anchors,omitempty"`
	Date    string   `json:"date"`
}

// EntryKind normalises the zero value: an entry with no kind is a decision.
func (e Entry) EntryKind() Kind {
	if e.Kind == KindRequirement {
		return KindRequirement
	}
	return KindDecision
}

// NewEntry builds an entry from captured text. The first line is the title and
// the remainder is the body, so `brain capture` can take one argument and still
// produce something that renders as a heading plus prose.
func NewEntry(text string, anchors []string, now time.Time) (Entry, error) {
	title, body := splitTitle(text)
	if title == "" {
		return Entry{}, fmt.Errorf("an entry needs at least a title line")
	}
	if _, err := ParseAnchors(anchors); err != nil {
		return Entry{}, err
	}
	e := Entry{
		Title:   title,
		Body:    body,
		Anchors: anchors,
		Date:    now.Format("2006-01-02"),
	}
	e.ID = e.computeID()
	return e, nil
}

// computeID derives the id from the content rather than from a random source.
// That is what makes the duplicate check of A5 free: capturing the same fact
// twice produces the same id, so the queue can refuse it without comparing
// prose. Date is deliberately excluded — the same fact captured on two days is
// the same fact.
func (e Entry) computeID() string {
	h := sha256.New()
	h.Write([]byte(strings.ToLower(strings.Join(strings.Fields(e.Title), " "))))
	if e.Slice != "" {
		h.Write([]byte("\x01" + e.Slice))
	}
	for _, a := range e.Anchors {
		h.Write([]byte("\x00" + a))
	}
	return hex.EncodeToString(h.Sum(nil))[:6]
}

// ParsedAnchors returns the entry's anchors, ignoring any that do not parse.
// Callers that need to report a bad anchor use ParseAnchors directly; this is
// for the paths where a malformed anchor must not stop the rest of the work.
func (e Entry) ParsedAnchors() []Anchor {
	out := make([]Anchor, 0, len(e.Anchors))
	for _, s := range e.Anchors {
		if a, err := ParseAnchor(s); err == nil {
			out = append(out, a)
		}
	}
	return out
}

// NewRequirement builds a requirement: a piece of intended scope belonging to a
// deliverable slice. Its anchors are forward references — they may name things
// that do not exist yet, and usually do.
func NewRequirement(text string, anchors []string, slice string, now time.Time) (Entry, error) {
	e, err := NewEntry(text, anchors, now)
	if err != nil {
		return Entry{}, err
	}
	if !sliceName.MatchString(slice) {
		return Entry{}, fmt.Errorf("slice %q: use letters, digits, '-' and '_' (a leading number orders it: 01-accounts)", slice)
	}
	e.Kind, e.Slice = KindRequirement, slice
	// The id folds in the slice, so the same sentence can legitimately appear
	// as a requirement of two slices without the second being refused as a
	// duplicate.
	e.ID = e.computeID()
	return e, nil
}

// sliceName is deliberately permissive about ordering: a slice is sorted by
// name, so a numeric prefix is how a roadmap gets its order, and that is the
// user's choice rather than a field mxcli maintains.
var sliceName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// Shard is where this entry belongs. A requirement goes to its slice; a
// decision to the module of its first anchor.
func (e Entry) Shard() string {
	if e.EntryKind() == KindRequirement {
		return PlanShard(e.Slice)
	}
	return ShardFor(e.ParsedAnchors())
}

// MisfiledIn reports whether the entry sits in the wrong shard, given which of
// its anchors resolved and in which module.
//
// This is a second axis, not a fourth anchor state: an anchor can resolve
// perfectly and the entry still be in the wrong file. The rule is deliberately
// relaxed — *at least one* anchor must belong to the shard, and anchors into
// other modules are fine. A fact like "Sales.Order is committed by
// Finance.ACT_Post" is genuinely two-module, and forcing it into project.md
// would grow the one file that must stay small.
func MisfiledIn(shard string, resolvedModules []string) bool {
	if shard == ProjectShard {
		return false // the catch-all is never misfiled
	}
	if len(resolvedModules) == 0 {
		// Nothing resolved, so there is no evidence about where the entry
		// belongs — and an entry whose only anchor is NotIndexable would
		// otherwise be reported as misfiled, reintroducing through this axis
		// exactly the false staleness that A1's third state exists to prevent.
		// An anchor that names nothing is already a failure on its own axis.
		return false
	}
	for _, m := range resolvedModules {
		if m == shard {
			return false
		}
	}
	return true
}

func splitTitle(text string) (title, body string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", ""
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+1:])
	}
	return text, ""
}
