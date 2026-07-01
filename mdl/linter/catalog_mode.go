// SPDX-License-Identifier: Apache-2.0

package linter

// CatalogMode is the catalog build depth a lint run needs. Higher values are
// supersets: Communities implies Full implies Fast.
type CatalogMode int

const (
	// CatalogFast is metadata only (the default REFRESH CATALOG).
	CatalogFast CatalogMode = iota
	// CatalogFull adds the `refs` cross-reference table (REFRESH CATALOG FULL) —
	// needed by rules using the refs_to / refs_from builtins.
	CatalogFull
	// CatalogCommunities adds the graph_* analysis tables (REFRESH CATALOG
	// COMMUNITIES, which implies full) — needed by rules using cycles /
	// module_dependencies / community_of / layer_of / centrality / god_nodes /
	// integration_surface.
	CatalogCommunities
)

// String returns the MDL refresh mode name (for diagnostics).
func (m CatalogMode) String() string {
	switch m {
	case CatalogFull:
		return "full"
	case CatalogCommunities:
		return "communities"
	default:
		return "fast"
	}
}

// CatalogRequirer is an OPTIONAL interface a Rule may implement to declare that
// it needs more than a fast catalog. Rules that do not implement it default to
// CatalogFast. It is checked via type assertion, so adding it to a rule (or not)
// is never a breaking change to the base Rule interface.
type CatalogRequirer interface {
	RequiredCatalogMode() CatalogMode
}

// RequiredCatalogMode returns the highest catalog mode any of the given rules
// declares it needs (CatalogFast when none declare a requirement). The lint entry
// points call this before building the catalog and build the computed mode, so a
// rule that needs the refs or graph_* tables gets them automatically instead of
// silently returning empty results.
func RequiredCatalogMode(rules []Rule) CatalogMode {
	mode := CatalogFast
	for _, r := range rules {
		req, ok := r.(CatalogRequirer)
		if !ok {
			continue
		}
		if m := req.RequiredCatalogMode(); m > mode {
			mode = m
		}
	}
	return mode
}
