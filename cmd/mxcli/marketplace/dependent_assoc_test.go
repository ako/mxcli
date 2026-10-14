// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// A local module that links to a marketplace module's entity owns that link: a
// cross-module association is stored in the FROM entity's module. It is still the
// update's casualty — DROP MODULE deletes the marketplace entity, and DeleteEntity
// cascades into every other domain model and removes the cross-module
// associations that reach the entity by name. `--save-edits` enumerated only the
// replaced module, so the one piece of local work the update was certain to
// destroy was the one it never saved (mendixlabs/mxcli#1502).
const crossModuleAssocMDL = `
create persistent entity MyFirstModule.Config ( Note: String(100) );
create association MyFirstModule.Config_Account from MyFirstModule.Config to Administration.Account;
`

const crossModuleAssoc = "MyFirstModule.Config_Account"

// describeAssoc returns the DESCRIBE output for an association, and whether it exists.
func describeAssoc(t *testing.T, mprPath, qualified string) (string, bool) {
	t.Helper()
	var sink bytes.Buffer
	exec := executor.New(&sink)
	exec.SetBackendFactory(testBackend)
	defer exec.Close()
	if err := exec.Execute(&ast.ConnectStmt{Path: mprPath}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	qn, err := splitQualified(qualified)
	if err != nil {
		t.Fatal(err)
	}
	if err := exec.Execute(&ast.DescribeStmt{ObjectType: ast.DescribeAssociation, Name: qn}); err != nil {
		return err.Error(), false
	}
	return sink.String(), true
}

// storedCrossAssoc returns the stored element of a cross-module association, or
// nil when no domain model holds one by that name.
func storedCrossAssoc(t *testing.T, mprPath, name string) (unitID string, elem bson.D) {
	t.Helper()
	r, err := modelsdk.Open(mprPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Disconnect()
	units, err := r.ListRawUnitsByType("DomainModels$DomainModel")
	if err != nil {
		t.Fatalf("list domain models: %v", err)
	}
	for _, u := range units {
		var doc bson.D
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			t.Fatalf("decode %s: %v", u.ID, err)
		}
		for _, x := range crossAssociationsOf(doc) {
			if xd, ok := x.(bson.D); ok && xd.Map()["Name"] == name {
				return string(u.ID), xd
			}
		}
	}
	return "", nil
}

// giveStudioProGUID rewrites the association's GUID to a value distinct from
// its $ID. mxcli mints GUID == $ID, so on an mxcli-created element a write path
// that re-minted the GUID from the $ID would reproduce the stored value and the
// test could not tell (CLAUDE.md, "Before trusting any GUID test"). A Studio
// Pro-authored element has a GUID of its own; this makes the subject one.
func giveStudioProGUID(t *testing.T, mprPath, name string) []byte {
	t.Helper()
	unitID, _ := storedCrossAssoc(t, mprPath, name)
	if unitID == "" {
		t.Fatalf("no stored cross association %s", name)
	}
	r, err := modelsdk.Open(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := r.GetRawUnitBytes(model.ID(unitID))
	r.Disconnect()
	if err != nil {
		t.Fatal(err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	guid := []byte{0x15, 0x02, 0x15, 0x02, 0xaa, 0xbb, 0xcc, 0xdd, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	for _, x := range crossAssociationsOf(doc) {
		xd, ok := x.(bson.D)
		if !ok || xd.Map()["Name"] != name {
			continue
		}
		for i, e := range xd {
			if e.Key == "GUID" {
				xd[i].Value = primitive.Binary{Subtype: e.Value.(primitive.Binary).Subtype, Data: guid}
			}
		}
	}
	enc, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	w, err := modelsdk.OpenForWriting(mprPath)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Disconnect()
	if err := w.UpdateRawUnitOwningStorageGUIDs(unitID, enc); err != nil {
		t.Fatalf("set GUID: %v", err)
	}
	return guid
}

func binaryOf(t *testing.T, d bson.D, key string) []byte {
	t.Helper()
	b, ok := d.Map()[key].(primitive.Binary)
	if !ok {
		t.Fatalf("%s is not binary in %v", key, d)
	}
	return b.Data
}

// The update must not cost a local module its association into the updated
// one. DROP MODULE removes it (DeleteEntity's cross-module cascade), and
// replaying it from --save-edits mints a new GUID — the database's identity for
// the association — so the update carries the stored element across instead,
// $ID and GUID unchanged.
func TestPerformUpdate_KeepsCrossModuleAssociationsIntoTheModule(t *testing.T) {
	target := copyFixture(t)
	execMDL(t, target, crossModuleAssocMDL)
	guid := giveStudioProGUID(t, target, "Config_Account")
	unitBefore, before := storedCrossAssoc(t, target, "Config_Account")
	if before == nil || !bytes.Equal(binaryOf(t, before, "GUID"), guid) {
		t.Fatalf("control: the association does not carry the distinct GUID: %v", before)
	}
	if bytes.Equal(binaryOf(t, before, "GUID"), binaryOf(t, before, "$ID")) {
		t.Fatal("control: GUID == $ID, so a re-mint would go undetected")
	}

	res, err := PerformUpdate(target, copyFixture(t), emptyMpk(t), "Administration",
		"4.3.2", "4.3.3", "00000000-0000-0000-0000-000000000000", testBackend)
	if err != nil {
		t.Fatalf("PerformUpdate: %v", err)
	}

	unitAfter, after := storedCrossAssoc(t, target, "Config_Account")
	if after == nil {
		t.Fatalf("the update removed %s — mendixlabs/mxcli#1502's follow-up", crossModuleAssoc)
	}
	if unitAfter != unitBefore {
		t.Errorf("association moved from unit %s to %s", unitBefore, unitAfter)
	}
	for _, key := range []string{"GUID", "$ID", "ParentPointer"} {
		if !bytes.Equal(binaryOf(t, before, key), binaryOf(t, after, key)) {
			t.Errorf("%s changed across the update: %x -> %x", key, binaryOf(t, before, key), binaryOf(t, after, key))
		}
	}
	if got := after.Map()["Child"]; got != "Administration.Account" {
		t.Errorf("Child = %v; want Administration.Account", got)
	}
	if out, ok := describeAssoc(t, target, crossModuleAssoc); !ok {
		t.Errorf("DESCRIBE cannot read the kept association: %s", out)
	}
	if len(res.DependentsKept) != 1 || res.DependentsKept[0] != crossModuleAssoc || len(res.DependentsLost) != 0 {
		t.Errorf("kept %v, lost %v; want kept [%s]", res.DependentsKept, res.DependentsLost, crossModuleAssoc)
	}
}

// When the new version no longer has the entity the association points at,
// there is nothing to point it at: putting it back would leave a by-name
// reference to a missing entity (CE1613). It stays removed and is reported.
func TestPerformUpdate_ReportsACrossModuleAssociationWhoseTargetIsGone(t *testing.T) {
	target := copyFixture(t)
	execMDL(t, target, crossModuleAssocMDL)

	ref := copyFixture(t)
	execMDL(t, ref, "rename entity Administration.Account to AccountV2;")

	res, err := PerformUpdate(target, ref, emptyMpk(t), "Administration",
		"4.3.2", "4.3.3", "00000000-0000-0000-0000-000000000000", testBackend)
	if err != nil {
		t.Fatalf("PerformUpdate: %v", err)
	}
	if _, after := storedCrossAssoc(t, target, "Config_Account"); after != nil {
		t.Errorf("association restored although Administration.Account is gone: %v", after)
	}
	if len(res.DependentsLost) != 1 || !strings.HasPrefix(res.DependentsLost[0], crossModuleAssoc) {
		t.Errorf("lost = %v; want %s reported", res.DependentsLost, crossModuleAssoc)
	}
	if len(res.DependentsKept) != 0 {
		t.Errorf("kept = %v; want none", res.DependentsKept)
	}
}

// The reported symptom: --save-edits saves nothing for the association, so after
// the update it is gone with no file to replay.
func TestSaveEdits_SavesCrossModuleAssociationsIntoTheModule(t *testing.T) {
	target := copyFixture(t)
	execMDL(t, target, crossModuleAssocMDL)

	installed, err := SnapshotModule(target, "Administration", testBackend)
	if err != nil {
		t.Fatalf("snapshot installed: %v", err)
	}
	published, err := SnapshotModule(copyFixture(t), "Administration", testBackend)
	if err != nil {
		t.Fatalf("snapshot package: %v", err)
	}
	drift := Compare(installed, published)

	if got := drift.DependentNames(); len(got) != 1 || got[0] != "ASSOCIATION "+crossModuleAssoc {
		t.Fatalf("dependents = %v; want exactly ASSOCIATION %s", got, crossModuleAssoc)
	}
	// The module itself is untouched: the association lives in MyFirstModule.
	// It must not be passed off as an edit to Administration.
	if drift.LocallyModified() {
		t.Errorf("Administration is unedited; the association belongs to MyFirstModule")
	}

	// `marketplace diff` reports them too, without counting them as edits.
	if res := NewDiffResult("Administration", "", "", drift); len(res.Dependents) != 1 || res.LocallyModified {
		t.Errorf("diff result: dependents %v, locallyModified %v", res.Dependents, res.LocallyModified)
	}

	dir := t.TempDir()
	written, unsaved, err := SaveEdits(dir, drift)
	if err != nil {
		t.Fatalf("SaveEdits: %v", err)
	}
	if len(unsaved) != 0 {
		t.Errorf("unsaved = %v", unsaved)
	}
	want := filepath.Join(dir, "association-MyFirstModule_Config_Account.mdl")
	if len(written) != 1 || written[0] != want {
		t.Fatalf("written = %v; want [%s] — this is mendixlabs/mxcli#1502", written, want)
	}
	body, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "association MyFirstModule.Config_Account") {
		t.Fatalf("saved file does not recreate the association:\n%s", body)
	}

	// The file is only a rescue if it replays. The update now keeps the
	// association, so take it away by hand and replay onto that.
	execMDL(t, target, "drop association MyFirstModule.Config_Account;")
	if _, ok := describeAssoc(t, target, crossModuleAssoc); ok {
		t.Fatal("control: the association was not dropped")
	}
	execMDL(t, target, string(body))
	out, ok := describeAssoc(t, target, crossModuleAssoc)
	if !ok {
		t.Fatalf("replaying %s did not restore the association: %s", want, out)
	}
	if !strings.Contains(out, "Administration.Account") {
		t.Errorf("restored association does not reach Administration.Account:\n%s", out)
	}
}
