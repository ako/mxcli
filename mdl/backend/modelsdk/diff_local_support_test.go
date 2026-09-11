// SPDX-License-Identifier: Apache-2.0

// The two FullBackend methods `mxcli diff-local` needs from the engine, and
// which the modelsdk engine did not have (mendixlabs/mxcli#1080). Both return
// no error, so both failed silently rather than loudly:
//
//   - ContentsDir() returned "" — read by diff-local as "not a v2 project",
//     reported as "mprcontents directory not found" on a project whose
//     mprcontents/ was populated.
//   - ParseMicroflowFromRaw() returned nil — which the caller DOES guard, by
//     substituting a "-- parse failed --" stub. The stub is a constant, so both
//     sides of the diff got the same text and the microflow's diff came out
//     EMPTY while the summary still counted the unit as modified.
package modelsdkbackend

import (
	"os"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// TestContentsDirResolvesForV2 pins the defect itself: a v2 project must report
// a real, existing directory.
func TestContentsDirResolvesForV2(t *testing.T) {
	b := New()
	if dir := b.ContentsDir(); dir != "" {
		t.Errorf("ContentsDir() on an unconnected backend = %q, want empty", dir)
	}
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	if b.Version() != 2 {
		t.Fatalf("fixture is MPR v%d; this test needs a v2 project", b.Version())
	}
	dir := b.ContentsDir()
	if dir == "" {
		t.Fatal("ContentsDir() = \"\" for a v2 project — diff-local reports " +
			"\"mprcontents directory not found\" on exactly this")
	}
	if !strings.HasSuffix(dir, "mprcontents") {
		t.Errorf("ContentsDir() = %q, want a path ending in mprcontents", dir)
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Errorf("ContentsDir() = %q, which is not a directory (err %v)", dir, err)
	}
}

// TestParseMicroflowFromRawDecodesRealUnit feeds the method what diff-local
// feeds it: a unit's raw BSON, unmarshalled to a map, with no unit to look up
// (the real caller's bytes come from `git show`, so they are not in storage at
// all). Asserting on the activities is the point — a microflow parsed down to
// its name only would still diff to nothing.
func TestParseMicroflowFromRawDecodesRealUnit(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	refs, err := b.reader.ListUnitsByType("Microflows$Microflow")
	if err != nil {
		t.Fatalf("ListUnitsByType: %v", err)
	}
	var picked map[string]any
	var pickedName string
	for _, ref := range refs {
		var raw map[string]any
		if err := bson.Unmarshal(ref.Contents, &raw); err != nil {
			continue
		}
		if name, _ := raw["Name"].(string); name == "NewAccount" {
			picked, pickedName = raw, name
			break
		}
	}
	if picked == nil {
		t.Skip("fixture has no Administration.NewAccount microflow")
	}

	mf := b.ParseMicroflowFromRaw(picked, "some-unit-id", "some-container")
	if mf == nil {
		t.Fatal("ParseMicroflowFromRaw returned nil — diff-local renders both " +
			"sides as the same '-- parse failed --' stub and shows no diff at all")
	}
	if mf.Name != pickedName {
		t.Errorf("Name = %q, want %q", mf.Name, pickedName)
	}
	if mf.ObjectCollection == nil || len(mf.ObjectCollection.Objects) == 0 {
		t.Error("microflow decoded with no flow objects — a body-less microflow " +
			"diffs to nothing, which is the same silent miss wearing a different hat")
	}
}
