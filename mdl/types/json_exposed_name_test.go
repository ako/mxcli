// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

// An ExposedName Mendix reserves makes the whole project unbuildable:
//
//	CE9524 "JSON element '(Object)/owner' has an invalid custom name 'Owner'.
//	        Custom name should start with a letter or underscore followed by
//	        either letters, digits or underscores."
//
// The message quotes a character pattern that 'Owner' satisfies perfectly, so it
// names the wrong rule. The real one — measured against mxbuild 11.13, one
// structure per name — is a case-insensitive reserved set. `owner` is an
// ordinary member in real REST payloads and nothing before the build warns.
//
// Studio Pro's avoidance is an underscore prefix keeping the key's ORIGINAL
// case: OpenAI_API.JSON_OpenAI_Response stores `_id` and `_object`. (ako/mxcli#300)

func TestResolveExposedNameReservedNames(t *testing.T) {
	b := &snippetBuilder{}

	for _, tc := range []struct{ key, want, why string }{
		// Rejected by mxbuild — the correctness half.
		{"owner", "_owner", "system attribute name"},
		{"changedDate", "_changedDate", "system attribute name, camelCase preserved"},
		{"createdDate", "_createdDate", "system attribute name"},
		{"changedBy", "_changedBy", "system attribute name"},
		{"currentUser", "_currentUser", "Mendix reserved"},
		{"guid", "_guid", "Mendix reserved"},
		{"class", "_class", "Java keyword"},
		{"new", "_new", "Java keyword"},
		{"break", "_break", "Java keyword"},
		{"finally", "_finally", "Java keyword"},
		{"strictfp", "_strictfp", "Java keyword"},
		{"int", "_int", "Java keyword"},
		{"true", "_true", "Java literal"},

		// Accepted by mxbuild, avoided by Studio Pro — the fidelity half.
		{"id", "_id", "Studio Pro stores _id"},
		{"object", "_object", "Studio Pro stores _object"},
		{"type", "_type", "Studio Pro avoids it"},

		// Untouched. `Type` and `Id` being reserved for ENTITY members does not
		// make every entity-reserved name reserved here.
		{"name", "Name", "ordinary"},
		{"value", "Value", "ordinary"},
		{"status", "Status", "ordinary"},
		{"index", "Index", "ordinary — a Java keyword lookalike that is not one"},
		{"finish_reason", "Finish_reason", "only the initial is capitalised"},
	} {
		if got := b.resolveExposedName(tc.key); got != tc.want {
			t.Errorf("resolveExposedName(%q) = %q, want %q (%s)", tc.key, got, tc.want, tc.why)
		}
	}
}

// The reserved check must be case-insensitive: Mendix's own is. A member spelled
// `Class` derives `Class`, which is not literally a Java keyword, and is still
// rejected — so matching on the derived spelling alone would miss it.
func TestResolveExposedNameIsCaseInsensitive(t *testing.T) {
	b := &snippetBuilder{}
	for _, key := range []string{"Class", "CLASS", "Owner", "OWNER", "Int"} {
		if got := b.resolveExposedName(key); got != "_"+key {
			t.Errorf("resolveExposedName(%q) = %q, want %q", key, got, "_"+key)
		}
	}
}

// A custom name supplied by the caller wins over the reserved handling — it is
// what the stored document already carries, and rewriting it would churn every
// structure read back from a project.
func TestResolveExposedNameCustomWins(t *testing.T) {
	b := &snippetBuilder{customNameMap: map[string]string{"owner": "Owner"}}
	if got := b.resolveExposedName("owner"); got != "Owner" {
		t.Errorf("resolveExposedName(owner) = %q, want the custom name Owner", got)
	}
}
