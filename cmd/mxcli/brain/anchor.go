// SPDX-License-Identifier: Apache-2.0

// anchor.go - references from a brain entry into the model.
//
// An anchor is what makes an entry checkable and routable. It is written the
// way a Mendix developer already names things — `@Sales.Order` — and its module
// prefix decides which shard the entry lives in, so routing is a string split
// rather than an index someone has to maintain (PROPOSAL_project_brain.md A8).
package brain

import (
	"fmt"
	"regexp"
	"strings"
)

// ProjectShard is the shard for entries that carry no anchor: facts about the
// project as a whole. It is the only file loaded unconditionally, which is why
// it carries the tightest cap.
const ProjectShard = "project"

// identifier is the Mendix name shape — a leading letter or underscore, then
// letters, digits and underscores. Anything else is rejected at parse time
// rather than becoming an anchor that can never resolve.
var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Anchor is a parsed reference into the model at one of three granularities:
// a module (`@Sales`), a document (`@Sales.Order`), or a member
// (`@Sales.Order.Status`).
type Anchor struct {
	Module  string
	Element string // empty for a module anchor
	Member  string // empty unless the anchor is member-scoped
}

// ParseAnchor reads one anchor. The leading '@' is optional so that callers can
// pass either the written form or a bare qualified name.
func ParseAnchor(s string) (Anchor, error) {
	raw := strings.TrimSpace(s)
	raw = strings.TrimPrefix(raw, "@")
	if raw == "" {
		return Anchor{}, fmt.Errorf("empty anchor")
	}
	parts := strings.Split(raw, ".")
	if len(parts) > 3 {
		return Anchor{}, fmt.Errorf("anchor %q has %d parts; anchors go at most three deep (@Module.Entity.Attribute)", s, len(parts))
	}
	for _, p := range parts {
		if !identifier.MatchString(p) {
			return Anchor{}, fmt.Errorf("anchor %q: %q is not a Mendix identifier", s, p)
		}
	}
	a := Anchor{Module: parts[0]}
	if len(parts) > 1 {
		a.Element = parts[1]
	}
	if len(parts) > 2 {
		a.Member = parts[2]
	}
	return a, nil
}

// ParseAnchors reads a list, reporting the first failure rather than silently
// dropping an anchor that would then never be checked.
func ParseAnchors(ss []string) ([]Anchor, error) {
	out := make([]Anchor, 0, len(ss))
	for _, s := range ss {
		a, err := ParseAnchor(s)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// String renders the anchor as it is written in a shard.
func (a Anchor) String() string {
	sb := "@" + a.Module
	if a.Element != "" {
		sb += "." + a.Element
	}
	if a.Member != "" {
		sb += "." + a.Member
	}
	return sb
}

// QualifiedName is the name to look up in the catalog. For a member anchor that
// is the *owning document's* qualified name — the member itself is resolved
// separately, because attributes live outside the objects view (A2).
func (a Anchor) QualifiedName() string {
	if a.Element == "" {
		return a.Module
	}
	return a.Module + "." + a.Element
}

// IsMember reports whether resolution needs the second, member-level query.
func (a Anchor) IsMember() bool { return a.Member != "" }

// ShardFor derives an entry's destination: the module of its first anchor, or
// the project shard when it has none. Deriving rather than asking is what makes
// the routing checkable — see MisfiledIn.
func ShardFor(anchors []Anchor) string {
	if len(anchors) == 0 {
		return ProjectShard
	}
	return anchors[0].Module
}
