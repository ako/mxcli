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

// baseSlug builds the preferred subdomain slug from a registration:
// [prefix-]project[-branch]. The optional prefix namespaces the hostname
// (organization, solution, team, env — whatever the client passes). The main /
// master branch is dropped, so a project's primary preview is the clean
// [prefix-]project; other branches append the branch. Solution is a grouping
// dimension in the overview, not part of the hostname (unless passed as prefix).
func baseSlug(prefix, project, branch string) string {
	var parts []string
	if pre := slugify(prefix); pre != "" {
		parts = append(parts, pre)
	}
	if p := slugify(project); p != "" {
		parts = append(parts, p)
	}
	if b := slugify(branch); b != "" && b != "main" && b != "master" {
		parts = append(parts, b)
	}
	if len(parts) == 0 {
		return "app"
	}
	return truncateLabel(strings.Join(parts, "-"))
}

// truncateLabel bounds a slug to a single DNS label, trimming any dash the cut
// leaves at the end.
func truncateLabel(s string) string {
	if len(s) <= maxLabelLen {
		return s
	}
	return strings.TrimRight(s[:maxLabelLen], "-")
}
