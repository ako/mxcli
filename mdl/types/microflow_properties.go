// SPDX-License-Identifier: Apache-2.0

package types

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Microflow document properties MDL can author but Mendix constrains. Each rule
// here is enforced by BOTH `mxcli check` and the writer, from this one function
// — the layout-placeholder lesson (two copies in two currencies is how a
// resolver drifts), and the reason these live in mdl/types rather than in either
// caller.

// ExportLevelAPI and ExportLevelHidden are the two members of
// MicroflowsExportLevel. Validated against generated/metamodel, not passed
// through from user text.
const (
	ExportLevelAPI    = "API"
	ExportLevelHidden = "Hidden"
)

// urlPlaceholder matches Mendix's {Name} parameter placeholders in a deep link.
var urlPlaceholder = regexp.MustCompile(`\{([^{}]*)\}`)

// URLPathParameters returns the parameter names a deep-link URL interpolates,
// in order of appearance, without duplicates.
//
// A segment binds its parameter by the LEADING identifier, because Mendix allows
// an attribute path inside it: `{Customer/Name}` binds the Customer parameter by
// one of its attributes. Matching the whole segment would read that as a
// parameter named "Customer/Name" and flag a correct URL — the same rule
// urlBindsParameter applies on the page side (CE5601).
func URLPathParameters(url string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range urlPlaceholder.FindAllStringSubmatch(url, -1) {
		name := strings.TrimSpace(m[1])
		if idx := strings.IndexAny(name, "/"); idx >= 0 {
			name = strings.TrimSpace(name[:idx])
		}
		if name == "" || seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		out = append(out, name)
	}
	return out
}

// CheckMicroflowURL validates a deep link against the microflow's parameters.
//
// Two rules, both measured against mxbuild 11.6.6 rather than inferred:
//
//   - A {Name} placeholder must name a parameter of this microflow.
//   - A parameter used in the PATH may not ALSO be a search parameter. That
//     overlap is CE5612 ("cannot be used as a URL parameter if it is already a
//     URL search parameter"); the two sets are disjoint. This was found by
//     seeding a real project and running mx check — the first version of the
//     fixture used one parameter for both and described a document Mendix
//     refuses to build.
//
// paramNames is every parameter the microflow declares; searchParams is what the
// URL SEARCH PARAMETERS clause named. Both are compared case-insensitively,
// matching how MDL resolves parameter references elsewhere.
func CheckMicroflowURL(url string, searchParams []string, paramNames []string) []string {
	if url == "" && len(searchParams) == 0 {
		return nil
	}
	declared := map[string]string{} // lower -> declared spelling
	for _, p := range paramNames {
		declared[strings.ToLower(p)] = p
	}

	var problems []string
	pathParams := URLPathParameters(url)
	inPath := map[string]bool{}
	for _, name := range pathParams {
		inPath[strings.ToLower(name)] = true
		if _, ok := declared[strings.ToLower(name)]; !ok {
			problems = append(problems, fmt.Sprintf(
				"URL placeholder {%s} does not name a parameter of this microflow%s",
				name, declaredHint(paramNames)))
		}
	}

	var overlap []string
	for _, sp := range searchParams {
		if _, ok := declared[strings.ToLower(sp)]; !ok {
			problems = append(problems, fmt.Sprintf(
				"URL search parameter $%s is not a parameter of this microflow%s",
				sp, declaredHint(paramNames)))
			continue
		}
		if inPath[strings.ToLower(sp)] {
			overlap = append(overlap, sp)
		}
	}
	if len(overlap) > 0 {
		sort.Strings(overlap)
		problems = append(problems, fmt.Sprintf(
			"parameter(s) %s appear in the URL path AND in URL SEARCH PARAMETERS — "+
				"Mendix rejects that as CE5612. A parameter is either part of the path "+
				"or a query argument, never both: drop it from one of the two",
			"$"+strings.Join(overlap, ", $")))
	}
	return problems
}

// CheckMicroflowConcurrency reports the CE4899 omission: Mendix requires an
// error message or an error microflow when concurrent execution is disallowed.
//
// It is a check rather than a grammar rule so the message can say which CE
// number it prevents, instead of a parse error reading "expecting ERROR".
func CheckMicroflowConcurrency(disallow, hasMessage bool, errorMicroflow string) string {
	if !disallow || hasMessage || errorMicroflow != "" {
		return ""
	}
	return "DISALLOW CONCURRENT EXECUTION needs an error handler — Mendix reports " +
		"CE4899 without one. Add `ERROR MESSAGE 'text'` or `ERROR MICROFLOW Module.Name`"
}

// CheckExportLevel validates an export level against the enum's two members.
func CheckExportLevel(level string) string {
	switch level {
	case "", ExportLevelAPI, ExportLevelHidden:
		return ""
	}
	return fmt.Sprintf("export level %q is not a member of MicroflowsExportLevel — use %s or %s",
		level, ExportLevelAPI, ExportLevelHidden)
}

// declaredHint lists the parameters that ARE declared.
//
// It does not special-case a case-only mismatch: resolution is already
// case-insensitive, matching how MDL resolves parameter references everywhere
// else, so `{key}` against a `$Key` parameter resolves and never reaches here.
// A "did you mean" branch for that case was written and deleted — unreachable
// code a reader would have trusted.
func declaredHint(params []string) string {
	if len(params) == 0 {
		return " (this microflow has no parameters)"
	}
	return fmt.Sprintf(" (declared: $%s)", strings.Join(params, ", $"))
}
