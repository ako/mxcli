// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// upstream #975: a list widget navigating a REVERSE association typed its rows
// as the wrong end when the page's context object was a specialization.
//
//	entity Sub extends Base;  association Note_Base from Note to Base;
//	page param $Sub: Sub
//	  listview (DataSource: $currentObject/M.Note_Base) { textbox (Attribute: Content) }
//
//	[CE1613] "The selected attribute 'M.Base.Content' no longer exists."
//	         at Text box [Content]
//
// The destination is the end OPPOSITE the context, matched by qualified name.
// `M.Sub` equals neither `M.Note` nor `M.Base`, both ends resolve so neither
// empty-end fallback applies, and control reached the last line:
//
//	// No context or mismatch — default to the child (TO) side, which
//	// matches the common FROM=context pattern.
//	return childEntity
//
// A guess that is silent when it is wrong. Measured control: the identical page
// whose context parameter is `Base` itself types the rows as `Note` and checks
// clean, so the specialization is the trigger — not reverse navigation as such.

// specializationDMs is Base ← Sub, Note, and `Note_Base from Note to Base`,
// plus a self-association to pin the degenerate case.
func specializationDMs() *pageBuilder {
	const modID = model.ID("mod-975")
	baseID := model.ID("e-base")
	subID := model.ID("e-sub")
	noteID := model.ID("e-note")

	return &pageBuilder{
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "M"}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: modID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: baseID}, Name: "Base"},
					{BaseElement: model.BaseElement{ID: subID}, Name: "Sub", GeneralizationRef: "M.Base"},
					{BaseElement: model.BaseElement{ID: noteID}, Name: "Note"},
				},
				Associations: []*domainmodel.Association{
					{Name: "Note_Base", ParentID: noteID, ChildID: baseID},
					{Name: "Note_Note", ParentID: noteID, ChildID: noteID},
				},
			}},
		},
	}
}

func TestResolveAssociationDestination_ContextIsASpecialization(t *testing.T) {
	pb := specializationDMs()

	// Reverse navigation from the TO end: the rows are the FROM entity.
	if got, want := pb.resolveAssociationDestination("M.Note_Base", "M.Sub"), "M.Note"; got != want {
		t.Errorf("from a specialization of the TO end, destination = %q, want %q — "+
			"mxbuild rejects an attribute bound on the wrong end with CE1613", got, want)
	}
}

// The controls. Each passed before the fix, and each would keep passing if the
// walk simply returned the parent for everything — which is why the forward case
// is here too.
func TestResolveAssociationDestination_ExactEndsStillWin(t *testing.T) {
	pb := specializationDMs()

	cases := []struct{ name, assoc, context, want string }{
		{"reverse from the TO end itself", "M.Note_Base", "M.Base", "M.Note"},
		{"forward from the FROM end", "M.Note_Base", "M.Note", "M.Base"},
		{"self-association", "M.Note_Note", "M.Note", "M.Note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pb.resolveAssociationDestination(tc.assoc, tc.context); got != tc.want {
				t.Errorf("destination = %q, want %q", got, tc.want)
			}
		})
	}
}

// A context related to neither end still falls back rather than erroring, and an
// unknown association still resolves to nothing.
func TestResolveAssociationDestination_UnrelatedContextAndUnknownAssociation(t *testing.T) {
	pb := specializationDMs()

	if got, want := pb.resolveAssociationDestination("M.Note_Base", "M.Unrelated"), "M.Base"; got != want {
		t.Errorf("an unrelated context keeps the documented fallback: got %q, want %q", got, want)
	}
	if got := pb.resolveAssociationDestination("M.NoSuchAssociation", "M.Sub"); got != "" {
		t.Errorf("an unknown association must resolve to nothing, got %q", got)
	}
}

// The walk itself, including the cycle guard a corrupt model could need.
func TestPageBuilderIsSpecializationOf(t *testing.T) {
	pb := specializationDMs()

	if !pb.isSpecializationOf("M.Sub", "M.Base") {
		t.Error("Sub extends Base")
	}
	if !pb.isSpecializationOf("M.Base", "M.Base") {
		t.Error("an entity is trivially itself")
	}
	if pb.isSpecializationOf("M.Base", "M.Sub") {
		t.Error("inheritance is not symmetric")
	}
	if pb.isSpecializationOf("M.Note", "M.Base") {
		t.Error("Note does not extend Base")
	}
	if pb.isSpecializationOf("M.Sub", "") || pb.isSpecializationOf("", "M.Base") {
		t.Error("an empty name is not a relationship")
	}

	cyclic := specializationDMs()
	for _, e := range cyclic.execCache.domainModels[0].Entities {
		switch e.Name {
		case "Base":
			e.GeneralizationRef = "M.Sub"
		case "Sub":
			e.GeneralizationRef = "M.Base"
		}
	}
	if cyclic.isSpecializationOf("M.Sub", "M.Note") {
		t.Error("a cycle must terminate as 'not a specialization', not loop")
	}
}
