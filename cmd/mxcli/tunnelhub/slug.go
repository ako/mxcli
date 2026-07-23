// SPDX-License-Identifier: Apache-2.0

// Package tunnelhub implements the multi-tenant mxcli tunnel-hub: a static
// ingress relay that fronts many locally-running Mendix apps (across projects,
// solutions, branches, and worktrees) at per-preview subdomains over a single
// 443 connection, with a registration API and an admin overview.
package tunnelhub

import (
	"regexp"
	"strings"
)

// maxLabelLen is the DNS label limit; a subdomain slug must fit in one label.
const maxLabelLen = 63

var nonLabel = regexp.MustCompile(`[^a-z0-9-]+`)

// slugify reduces s to a DNS-label-safe fragment: lowercase, only [a-z0-9-],
// runs collapsed, no leading/trailing dashes.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer("/", "-", "_", "-", ".", "-", " ", "-").Replace(s)
	s = nonLabel.ReplaceAllString(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

// baseSlug builds the preferred subdomain slug from a registration. The main /
// master branch collapses to just the project (so a project's primary preview is
// project.mxcli.org); other branches append the branch. Solution is a grouping
// dimension in the overview, not part of the hostname.
func baseSlug(project, branch string) string {
	p := slugify(project)
	b := slugify(branch)
	switch {
	case p == "" && b == "":
		return "app"
	case p == "":
		return truncateLabel(b)
	case b == "" || b == "main" || b == "master":
		return truncateLabel(p)
	default:
		return truncateLabel(p + "-" + b)
	}
}

// truncateLabel bounds a slug to a single DNS label, trimming any dash the cut
// leaves at the end.
func truncateLabel(s string) string {
	if len(s) <= maxLabelLen {
		return s
	}
	return strings.TrimRight(s[:maxLabelLen], "-")
}
