// SPDX-License-Identifier: Apache-2.0

package model

import "strings"

// Mendix stores a mapping member's location as a pipe-separated JsonPath —
// "(Object)|fields|Title" — where each segment is one JSON member. MDL spells
// the same thing with "/" (`Title = fields/Title`), because "|" reads badly and
// "/" cannot collide with the association form on the other side of the "=".
//
// The INLINE REST mapping had no conversion between the two: it appended the
// member text verbatim, so a multi-segment reference was stored as ONE member
// whose name contains a slash ("(Object)|fields/Title"). Every gate passed —
// mxcli check, exec, mx check, describe — and at runtime the value was simply
// empty, because no JSON member is called "fields/Title". Confirmed against
// four Studio Pro-authored inline response mappings in the demo apps, which all
// store full pipe paths (e.g. "(Object)|results|bindings|(Object)|caseId|value").
//
// These helpers are shared by the two REST serializers (sdk/mpr and
// mdl/backend/modelsdk) so the engines cannot drift on it.

// InlineMappingPath appends an MDL member reference to a parent JsonPath,
// converting "/" segments to Mendix's "|" separators.
func InlineMappingPath(parentPath, member string) string {
	return parentPath + "|" + strings.ReplaceAll(member, "/", "|")
}

// InlineMappingExposedName is the display name stored for a member reference:
// its last segment, so a single-segment member keeps exactly the name it always
// had and only a multi-segment one changes.
//
// Studio Pro derives ExposedName from the JSON structure at authoring time and
// uniquifies it against its siblings ("caseId|value" becomes "CaseId_Value",
// but "graphData|value" in the same document becomes plain "Value"), so the
// algorithm is not reproducible from the path alone — and it does not need to
// be. ExposedName is a label; JsonPath is what binds.
func InlineMappingExposedName(member string) string {
	if i := strings.LastIndex(member, "/"); i >= 0 {
		return member[i+1:]
	}
	return member
}
