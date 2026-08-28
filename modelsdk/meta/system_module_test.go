// SPDX-License-Identifier: Apache-2.0

package meta

import (
	"sort"
	"strings"
	"testing"
)

// upstream #972: a GRANT on any specialization of System.Image wrote a member
// access for System.Thumbnail_Image, and mxbuild rejected the rule with
//
//	[CE1613] "The selected association 'System.Thumbnail_Image' no longer exists."
//	         at Access rule of entity '<Module>.CaseImage'
//
// The association is real — in the RUNTIME's metamodel. It came in with the
// block marked "extracted from MDP (Phase 3, 2026-04-24)", which describes the
// tables the Mendix runtime keeps rather than the System module Studio Pro
// shows. `Thumbnail_Image` is owned by BOTH ends, so System.Image counted as an
// owner through the child side and every specialization inherited a member for
// a name the modeler does not have.
//
// The damage was invisible from inside mxcli: `describe entity` renders the rule
// without the broken member, and the legacy engine — whose System list never had
// the MDP block — writes a clean rule from the same script. That contrast is the
// control, and it is what pins the defect to this list rather than to the GRANT
// path that reads it.

// runtimeOnlyByMeasurement is the list as MEASURED, not as reasoned about.
//
// Method: one microflow per candidate, each retrieving from System.<Name>, then
// `mx check` on mxbuild 11.13.0. Every name below came back
// CE1613 "The selected entity 'System.<Name>' no longer exists"; the controls in
// the same run — Image, FileDocument, Session — came back clean, so the probe
// discriminates rather than failing everything.
//
// All eleven are the MDP block in its entirety. If a future harvest adds a row
// there, this test fails until someone decides which side of the line it is on.
var runtimeOnlyByMeasurement = []string{
	"AutoCommitEntry",
	"BackgroundJob",
	"ChangeHash",
	"OfflineCreatedGuids",
	"OfflineSynchronizationHistory",
	"PrivateFileDocument",
	"Thumbnail",
	"UnreferencedFile",
	"WorkflowActivity",
	"WorkflowActivityUserTaskOutcome",
	"WorkflowVersion",
}

func TestSystemEntities_RuntimeOnlyMatchesTheMeasuredList(t *testing.T) {
	var flagged []string
	for _, e := range SystemEntities {
		if e.RuntimeOnly {
			flagged = append(flagged, e.Name)
		}
	}
	sort.Strings(flagged)

	if strings.Join(flagged, ",") != strings.Join(runtimeOnlyByMeasurement, ",") {
		t.Errorf("RuntimeOnly entities\n got %v\nwant %v\n(the want list is measured against mxbuild — see the comment above it)",
			flagged, runtimeOnlyByMeasurement)
	}
}

// The modeler view is what every write path resolves names against, so a
// runtime-only entity must not appear in it.
func TestModelerSystemEntities_ExcludesRuntimeOnly(t *testing.T) {
	got := map[string]bool{}
	for _, e := range ModelerSystemEntities() {
		got[e.Name] = true
	}

	for _, name := range runtimeOnlyByMeasurement {
		if got[name] {
			t.Errorf("System.%s is runtime-only and must not be in the modeler view — mxbuild reports CE1613 for it", name)
		}
	}
	// Controls: the entities the same measurement run showed ARE in the modeler.
	// Without these the test would pass against a function that returns nothing.
	for _, name := range []string{"Image", "FileDocument", "Session", "User", "WorkflowUserTask"} {
		if !got[name] {
			t.Errorf("System.%s is a real modeler entity and must be kept", name)
		}
	}
}

// An association is runtime-only exactly when one of its ends is — derived
// rather than flagged by hand, so the two lists cannot drift apart.
func TestModelerSystemAssociations_ExcludesAssociationsOnRuntimeOnlyEntities(t *testing.T) {
	got := map[string]bool{}
	for _, a := range ModelerSystemAssociations() {
		got[a.Name] = true
	}

	if got["Thumbnail_Image"] {
		t.Error("System.Thumbnail_Image is the reported defect (#972): its parent System.Thumbnail does not exist in the modeler")
	}
	// Every MDP-block association hangs off a runtime-only entity, so none of
	// them survives. Spot-check the ones reachable by inheritance or by name.
	for _, name := range []string{"BackgroundJob_Session", "ChangeHash_Session", "WorkflowActivity_Actor", "UnreferencedFile_XASInstance"} {
		if got[name] {
			t.Errorf("System.%s has a runtime-only end and must not be in the modeler view", name)
		}
	}
	// Controls.
	for _, name := range []string{"Session_User", "UserRoles", "Workflow_WorkflowDefinition"} {
		if !got[name] {
			t.Errorf("System.%s is a real modeler association and must be kept", name)
		}
	}
}

// The invariant that makes the derivation trustworthy: nothing in the modeler
// view points at an entity the modeler view does not have. A dangling end is a
// by-name reference Mendix resolves to null.
func TestModelerSystemAssociations_HaveBothEndsInTheModelerView(t *testing.T) {
	entities := map[string]bool{}
	for _, e := range ModelerSystemEntities() {
		entities[e.Name] = true
	}
	for _, a := range ModelerSystemAssociations() {
		if !entities[a.Parent] {
			t.Errorf("association %s has parent System.%s, which is not in the modeler view", a.Name, a.Parent)
		}
		if !entities[a.Child] {
			t.Errorf("association %s has child System.%s, which is not in the modeler view", a.Name, a.Child)
		}
	}
}

// A generalization must resolve too: an entity whose parent is runtime-only
// could not be built, and none of the modeler entities has one.
func TestModelerSystemEntities_HaveResolvableGeneralizations(t *testing.T) {
	entities := map[string]bool{}
	for _, e := range ModelerSystemEntities() {
		entities[e.Name] = true
	}
	for _, e := range ModelerSystemEntities() {
		if e.Generalization == "" {
			continue
		}
		name := strings.TrimPrefix(e.Generalization, "System.")
		if !entities[name] {
			t.Errorf("System.%s extends %s, which is not in the modeler view", e.Name, e.Generalization)
		}
	}
}

// The full lists keep the runtime rows: they were harvested deliberately and a
// storage backend speaking to the runtime metamodel needs them. Filtering is a
// view, not a deletion.
func TestSystemEntities_StillCarryTheRuntimeRows(t *testing.T) {
	all := map[string]bool{}
	for _, e := range SystemEntities {
		all[e.Name] = true
	}
	for _, name := range runtimeOnlyByMeasurement {
		if !all[name] {
			t.Errorf("System.%s was deleted rather than flagged; the runtime metamodel still has it", name)
		}
	}
	if len(ModelerSystemEntities()) >= len(SystemEntities) {
		t.Error("the modeler view is not narrower than the full list — the filter is doing nothing")
	}
	if len(ModelerSystemAssociations()) >= len(SystemAssociations) {
		t.Error("the modeler association view is not narrower than the full list — the filter is doing nothing")
	}
}
