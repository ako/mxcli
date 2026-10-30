// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
	mversion "github.com/mendixlabs/mxcli/modelsdk/version"
)

// ako/mxcli#1774: the version data held property floors only, so a $Type
// newer than the project went through every write, and the project then did
// not load (`TypeCacheUnknownTypeException: The type cache does not contain a
// type with qualified name …`). It shipped four times: #1754, #1209, #1163
// (Workflows$EndOfParallelSplitPathActivity, 10.21) and #1771
// (Workflows$SingleUserTaskActivity, 10.12). Each type's own introduction
// version is now in mendixmodelsdk_type_versions.json, and every unit write
// refuses an element whose type the project does not have yet.

// waitActivity is a wait activity as the workflow encoder writes one.
func waitActivity(typeName, name string) bson.D {
	return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: typeName},
		{Key: "Annotation", Value: nil}, {Key: "BoundaryEvents", Value: bson.A{int32(2)}},
		{Key: "Caption", Value: name}, {Key: "Name", Value: name}}
}

// workflowHolding is a workflow whose main flow holds the given activities.
func workflowHolding(acts ...bson.D) bson.D {
	list := bson.A{int32(3)}
	for _, a := range acts {
		list = append(list, a)
	}
	return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Flow", Value: bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$Flow"},
			{Key: "Activities", Value: list}}},
		{Key: "Name", Value: "WF"}, {Key: "Title", Value: "WF"}}
}

// parallelSplitHolding is a parallel split whose one path holds act, nested
// two flows down from the workflow.
func parallelSplitHolding(act bson.D) bson.D {
	path := bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
		{Key: "Flow", Value: bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$Flow"},
			{Key: "Activities", Value: bson.A{int32(3), act}}}}}
	return bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$ParallelSplitActivity"},
		{Key: "Annotation", Value: nil}, {Key: "Caption", Value: "Parallel split"}, {Key: "Name", Value: "split"},
		{Key: "Outcomes", Value: bson.A{int32(3), path}}}
}

func label(v *version.ProjectVersion) string {
	return mversion.Version{Major: v.MajorVersion, Minor: v.MinorVersion, Patch: v.PatchVersion}.String()
}

// Workflows$WaitForTimerActivity is a 10.6.0 type and
// Workflows$WaitForNotificationActivity a 10.0.0 one (mendixmodelsdk 4.117.0,
// each type's versionInfo). Neither has an older shape, so below its version
// the write is refused, naming the type, its version and the project's.
func TestNormalize_RefusesATypeNewerThanTheProject(t *testing.T) {
	for _, tt := range []struct {
		v      *version.ProjectVersion
		typ    string
		refuse string // "" = accepted
	}{
		{pv(10, 5, 0), "Workflows$WaitForTimerActivity", "Workflows$WaitForTimerActivity (Mendix 10.6.0+)"},
		{pv(10, 6, 0), "Workflows$WaitForTimerActivity", ""},
		{pv(9, 24, 30), "Workflows$WaitForNotificationActivity", "Workflows$WaitForNotificationActivity (Mendix 10.0.0+)"},
		{pv(10, 0, 0), "Workflows$WaitForNotificationActivity", ""},
		{pv(11, 13, 0), "Workflows$WaitForTimerActivity", ""}, // control
	} {
		_, err := normalizeShape(t, tt.v, workflowHolding(waitActivity(tt.typ, "wait")))
		switch {
		case tt.refuse == "" && err != nil:
			t.Errorf("%s: %s is refused: %v", label(tt.v), tt.typ, err)
		case tt.refuse != "" && err == nil:
			t.Errorf("%s: %s was accepted; want it refused", label(tt.v), tt.typ)
		case tt.refuse != "":
			for _, want := range []string{"this Mendix " + label(tt.v) + " project cannot store", tt.refuse} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("%s: refusal %q does not say %q", label(tt.v), err, want)
				}
			}
		}
	}
}

// The check walks the whole unit: a wait activity on a parallel split path,
// two flows below the workflow, is refused as surely as one in the main flow.
func TestNormalize_RefusesANestedTypeNewerThanTheProject(t *testing.T) {
	wf := workflowHolding(parallelSplitHolding(waitActivity("Workflows$WaitForTimerActivity", "nested")))
	_, err := normalizeShape(t, pv(10, 5, 0), wf)
	if err == nil || !strings.Contains(err.Error(), "Workflows$WaitForTimerActivity (Mendix 10.6.0+)") {
		t.Errorf("a nested 10.6 type on 10.5 was not refused naming it: %v", err)
	}
}

// The check runs after LowerToProjectShape, so what has an older shape is
// lowered and not refused: the parallel split end marker (10.21) on 9.24. With
// the lowering stubbed out, the same unit is refused — the check is what
// stands behind every lowering, and would have caught #1163 with none.
func TestNormalize_TypeFloorRunsAfterTheLowering(t *testing.T) {
	if _, err := normalizeShape(t, pv(9, 24, 30), workflowWithParallelSplit()); err != nil {
		t.Fatalf("a lowered parallel split is refused on 9.24: %v", err)
	}

	saved := lowerToProjectShape
	lowerToProjectShape = func(contents []byte, _ *mversion.Version) ([]byte, error) { return contents, nil }
	defer func() { lowerToProjectShape = saved }()

	// The marker without its PersistentId (10.21, a property the floor refuses
	// on its own), so what is left to refuse it is its type.
	marker := bson.D{{Key: "$ID", Value: eid()}, {Key: "$Type", Value: "Workflows$EndOfParallelSplitPathActivity"},
		{Key: "Annotation", Value: nil}, {Key: "Caption", Value: "End of parallel split path"},
		{Key: "Name", Value: "EndOfParallelSplitPath"}}
	_, err := normalizeShape(t, pv(9, 24, 30), workflowHolding(parallelSplitHolding(marker)))
	if err == nil || !strings.Contains(err.Error(), "Workflows$EndOfParallelSplitPathActivity (Mendix 10.21.0+)") {
		t.Errorf("with the lowering stubbed, the 10.21 end marker on 9.24 was not refused: %v", err)
	}
}

// The refusal is a refusal of the write: the stored unit is left as it was,
// byte for byte, and nothing of the statement lands.
func TestWriter_TypeNewerThanTheProjectIsNotWritten(t *testing.T) {
	stored, err := bson.Marshal(workflowHolding())
	if err != nil {
		t.Fatal(err)
	}
	unitID := "8a6f3c1e-1774-4d2b-9a5e-0c3b2f1e7d64"
	w, path := newV2WriterForCommitTest(t, unitID, stored)
	w.reader.projectVersion = pv(10, 5, 0)

	next, err := bson.Marshal(workflowHolding(waitActivity("Workflows$WaitForTimerActivity", "wait")))
	if err != nil {
		t.Fatal(err)
	}
	err = w.UpdateRawUnit(unitID, next)
	if err == nil || !strings.Contains(err.Error(), "Workflows$WaitForTimerActivity (Mendix 10.6.0+)") {
		t.Fatalf("UpdateRawUnit of a 10.6 type on 10.5 = %v, want it refused", err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(after, stored) {
		t.Error("the refused write changed the stored unit")
	}

	// A new unit is refused the same way, and leaves no file behind.
	newID := "8a6f3c1e-1774-4d2b-9a5e-0c3b2f1e7d65"
	err = w.InsertUnit(newID, unitID, "Documents", "Workflows$Workflow", next)
	if err == nil || !strings.Contains(err.Error(), "Workflows$WaitForTimerActivity (Mendix 10.6.0+)") {
		t.Fatalf("InsertUnit of a 10.6 type on 10.5 = %v, want it refused", err)
	}
	swapped := blobToUUIDSwapped(uuidToBlob(newID))
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(path))),
		swapped[0:2], swapped[2:4], swapped+".mxunit")); !os.IsNotExist(statErr) {
		t.Errorf("the refused insert left a unit file (stat: %v)", statErr)
	}
}
