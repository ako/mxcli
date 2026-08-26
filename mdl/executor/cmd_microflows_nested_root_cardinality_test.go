// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// A mapping rooted BELOW an array yields one object per item, so the import
// activity must bind a LIST. mappingRootIsList answered from the JSON
// STRUCTURE's root element, which for `root choices/message` is the document
// root — an Object — so it reported a single object and mxbuild rejected the
// activity with CE0243 "The mapping used to return a value of type 'RtMap.Msg',
// but now returns a value of type 'List of RtMap.Msg'".
//
// The mapping document itself is correct and cannot be used to decide this:
// Studio Pro's own OpenAI_API.IM_OpenAI stores MaxOccurs 1 on exactly this root
// (verified against the fixture), so list-ness lives only in the root's PATH.
//
// Found by building an app and running mx check on it — every unit test in this
// package passed while this was broken (ako/mxcli#267 follow-up).

func TestMappingRootPathCrossesArray(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
		why  string
	}{
		{"(Object)", false, "the structure's own root, one object"},
		{"", false, "no path at all — an XML-schema or message-definition mapping"},
		{"(Object)|data", false, "a nested root that does NOT pass an array"},
		{"(Object)|meta|pagination", false, "two levels down, still no array"},
		{"(Array)|(Object)", true, "an array-rooted structure (#248)"},
		{"(Object)|choices|(Object)|message", true, "rooted below an array (#267)"},
		{"(Object)|tags|(Wrapper)", true, "below an array of primitives (#268)"},
	} {
		if got := mappingRootPathCrossesArray(tc.path); got != tc.want {
			t.Errorf("mappingRootPathCrossesArray(%q) = %v, want %v — %s",
				tc.path, got, tc.want, tc.why)
		}
	}
}

// TestMappingRootIsList_NestedRootThroughArray pins the caller: the structure
// says Object, the mapping root's path says otherwise, and the path wins.
func TestMappingRootIsList_NestedRootThroughArray(t *testing.T) {
	fb := importRangeBuilder(false) // structure root is an Object

	nested := &model.ImportMapping{
		JsonStructure: "M.Payload",
		Elements: []*model.ImportMappingElement{{
			// Exactly what `root choices/message` stores, MaxOccurs and all.
			JsonPath: "(Object)|choices|(Object)|message", MaxOccurs: 1,
		}},
	}
	if !fb.mappingRootIsList(nested) {
		t.Error("a mapping rooted below an array reported a single object — " +
			"mxbuild rejects the activity with CE0243")
	}

	// Control: the same structure, a root that does not cross an array. This
	// must stay a single object, or the fix has simply inverted the bug.
	plain := &model.ImportMapping{
		JsonStructure: "M.Payload",
		Elements: []*model.ImportMappingElement{{
			JsonPath: "(Object)|data", MaxOccurs: 1,
		}},
	}
	if fb.mappingRootIsList(plain) {
		t.Error("a nested root that does not cross an array reported a list")
	}
}
