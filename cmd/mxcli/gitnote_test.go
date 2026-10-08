// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	mpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Unit ids used by the fixtures. The root's container is itself, as in a real .mpr.
const (
	tRoot   = "e3cc035b-6ecc-4c98-8d6f-bb72709291b8"
	tMod    = "11111111-0000-0000-0000-000000000001"
	tFolder = "11111111-0000-0000-0000-000000000002"
	tPage   = "11111111-0000-0000-0000-000000000003"
	tMf     = "11111111-0000-0000-0000-000000000004"
	tDM     = "11111111-0000-0000-0000-000000000005"
)

func unitDoc(t *testing.T, typ, name string, extra ...bson.E) []byte {
	t.Helper()
	d := bson.D{{Key: "$ID", Value: mpr.IDToBsonBinary("aaaaaaaa-0000-0000-0000-000000000000")}, {Key: "$Type", Value: typ}}
	if name != "" {
		d = append(d, bson.E{Key: "Name", Value: name})
	}
	d = append(d, extra...)
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type tUnit struct {
	container string
	raw       []byte
}

func snap(version string, units map[string]tUnit) *noteSnapshot {
	s := &noteSnapshot{Version: version, Units: map[string]noteUnit{}}
	for id, u := range units {
		sum := sha256.Sum256(u.raw)
		s.Units[id] = noteUnit{Container: u.container, Hash: base64.StdEncoding.EncodeToString(sum[:])}
	}
	s.Read = func(id string) ([]byte, error) {
		if u, ok := units[id]; ok {
			return u.raw, nil
		}
		return nil, os.ErrNotExist
	}
	return s
}

// baseUnits is a project with one module holding a folder, a page in the
// folder, a microflow and a domain model.
func baseUnits(t *testing.T) map[string]tUnit {
	return map[string]tUnit{
		tRoot:   {tRoot, unitDoc(t, "Projects$Project", "")},
		tMod:    {tRoot, unitDoc(t, "Projects$ModuleImpl", "Shop")},
		tFolder: {tMod, unitDoc(t, "Projects$Folder", "Pages")},
		tPage:   {tFolder, unitDoc(t, "Forms$Page", "Home", bson.E{Key: "Title", Value: "Home"})},
		tMf:     {tMod, unitDoc(t, "Microflows$Microflow", "ACT_Save")},
		tDM:     {tMod, unitDoc(t, "DomainModels$DomainModel", "")},
	}
}

func clone(m map[string]tUnit) map[string]tUnit {
	out := map[string]tUnit{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func statuses(changes []mxModelChange) map[string]string {
	out := map[string]string{}
	for _, c := range changes {
		out[c.UnitID] = c.Status
	}
	return out
}

func TestModelChanges_NoChangeListsNothing(t *testing.T) {
	u := baseUnits(t)
	if got := computeModelChanges(snap("11.13.0", u), snap("11.13.0", clone(u)), nil); len(got) != 0 {
		t.Fatalf("want no changes, got %+v", got)
	}
}

func TestModelChanges_AddedModifiedDeleted(t *testing.T) {
	parent := baseUnits(t)
	child := clone(parent)
	child[tPage] = tUnit{tFolder, unitDoc(t, "Forms$Page", "Home", bson.E{Key: "Title", Value: "Welcome"})}
	newMf := "11111111-0000-0000-0000-000000000009"
	child[newMf] = tUnit{tMod, unitDoc(t, "Microflows$Microflow", "ACT_New")}
	delete(child, tMf)

	got := computeModelChanges(snap("11.13.0", parent), snap("11.13.0", child), nil)
	want := map[string]string{tPage: "Modified", newMf: "Added", tMf: "Deleted"}
	if s := statuses(got); len(s) != len(want) || s[tPage] != want[tPage] || s[newMf] != want[newMf] || s[tMf] != want[tMf] {
		t.Fatalf("want %v, got %+v", want, got)
	}
	for _, c := range got {
		if c.Module != "Shop" {
			t.Errorf("%s: Module = %q, want Shop", c.UnitName, c.Module)
		}
		if c.UnitID == tPage && (c.UnitType != "Forms$Page" || c.UnitName != "Home") {
			t.Errorf("page reported as %+v", c)
		}
	}
}

// A re-serialisation that only mints new element $IDs is not a change: that is
// what mxcli's create-or-replace does to an unchanged document.
func TestModelChanges_IDOnlyChurnIsNotListed(t *testing.T) {
	parent := baseUnits(t)
	child := clone(parent)
	d := bson.D{{Key: "$ID", Value: mpr.IDToBsonBinary("bbbbbbbb-0000-0000-0000-000000000000")}, {Key: "$Type", Value: "Microflows$Microflow"}, {Key: "Name", Value: "ACT_Save"}}
	raw, _ := bson.Marshal(d)
	child[tMf] = tUnit{tMod, raw}
	if got := computeModelChanges(snap("11.13.0", parent), snap("11.13.0", child), nil); len(got) != 0 {
		t.Fatalf("an $ID-only difference must not be listed, got %+v", got)
	}
}

func TestModelChanges_MovedToAnotherFolder(t *testing.T) {
	parent := baseUnits(t)
	child := clone(parent)
	child[tMf] = tUnit{tFolder, parent[tMf].raw}
	got := computeModelChanges(snap("11.13.0", parent), snap("11.13.0", child), nil)
	if s := statuses(got); len(s) != 1 || s[tMf] != "Moved" {
		t.Fatalf("want ACT_Save Moved, got %+v", got)
	}
}

// Measured on a Studio Pro commit that deleted and re-added a module: only the
// deleted module unit is listed, not the units inside it.
func TestModelChanges_DeletedModuleListsOnlyTheModule(t *testing.T) {
	parent := baseUnits(t)
	child := map[string]tUnit{tRoot: parent[tRoot]}
	got := computeModelChanges(snap("11.13.0", parent), snap("11.13.0", child), nil)
	if len(got) != 1 || got[0].UnitID != tMod || got[0].Status != "Deleted" ||
		got[0].UnitType != "Projects$ModuleImpl" || got[0].UnitName != "Shop" || got[0].Module != "Shop" {
		t.Fatalf("want only the module Deleted, got %+v", got)
	}
}

// A unit whose file was missing from the parent while the parent's index
// already held its hash shows no hash change; the file diff catches it.
func TestModelChanges_TouchedFileWithUnchangedHash(t *testing.T) {
	parent := baseUnits(t)
	ps := snap("11.13.0", parent)
	ps.Read = func(id string) ([]byte, error) {
		if id == tPage {
			return nil, os.ErrNotExist
		}
		return parent[id].raw, nil
	}
	got := computeModelChanges(ps, snap("11.13.0", clone(parent)), map[string]bool{tPage: true})
	if s := statuses(got); len(s) != 1 || s[tPage] != "Modified" {
		t.Fatalf("want Home Modified, got %+v", got)
	}
}

// Measured on four Studio Pro upgrade commits (10.17→10.21 … 11.5→11.13):
// the project root is listed on every version change, and properties a new
// version adds are not a change.
func TestModelChanges_UpgradeListsRootAndIgnoresNewProperties(t *testing.T) {
	parent := baseUnits(t)
	child := clone(parent)
	child[tPage] = tUnit{tFolder, unitDoc(t, "Forms$Page", "Home",
		bson.E{Key: "Title", Value: "Home"}, bson.E{Key: "MarkAsUsed", Value: false})}

	got := computeModelChanges(snap("11.5.0", parent), snap("11.13.0", child), nil)
	if s := statuses(got); len(s) != 1 || s[tRoot] != "Modified" {
		t.Fatalf("want only the project root Modified, got %+v", got)
	}

	// Control: the same new property within one version is a change.
	got = computeModelChanges(snap("11.13.0", parent), snap("11.13.0", child), nil)
	if s := statuses(got); len(s) != 1 || s[tPage] != "Modified" {
		t.Fatalf("control: want Home Modified within one version, got %+v", got)
	}
}

// A real note Studio Pro 11.13 wrote (header of 6779a52, one change of it).
const realStudioProNote = `{"BranchName":"jts-redesign","ModelerVersion":"11.13.0","ModelChanges":[{"Status":"Modified","UnitID":"03b0dc68-ffc8-4c22-bd85-244e58bd15f8","UnitType":"Forms$Page","UnitName":"Opstappers_Select_Web","Module":"JTSBootLogboek"}],"RelatedStories":[],"SolutionVersion":"","MPRFormatVersion":"Version2","HasModelerVersion":true}`

func TestNote_MarshalsByteIdenticalToStudioPro(t *testing.T) {
	var n mxNote
	if err := json.Unmarshal([]byte(realStudioProNote), &n); err != nil {
		t.Fatal(err)
	}
	if got := n.marshal(); got != realStudioProNote {
		t.Fatalf("marshal differs from Studio Pro's note:\n got %s\nwant %s", got, realStudioProNote)
	}
	if got := (mxNote{}).marshal(); !strings.Contains(got, `"ModelChanges":[]`) || !strings.Contains(got, `"RelatedStories":[]`) {
		t.Fatalf("empty lists must be [] not null: %s", got)
	}
}

func TestNote_PlaceholderDetection(t *testing.T) {
	cases := map[string]bool{
		"":                true,
		"\n":              true,
		realStudioProNote: false,
		"not json":        false,
		`{"BranchName":"(unknown)","ModelerVersion":"(unknown)","ModelChanges":[],"RelatedStories":[],"SolutionVersion":"","MPRFormatVersion":"","HasModelerVersion":false}`: true,
		`{"ModelChanges":[],"RelatedStories":[],"isWebModelerCommit":false,"BranchName":"","ModelerVersion":"9.19.0.55544"}`:                                                 false,
	}
	for text, want := range cases {
		if got := isPlaceholderNote(text); got != want {
			t.Errorf("isPlaceholderNote(%q) = %v, want %v", text, got, want)
		}
	}
	if noteBranchName("main") != "" || noteBranchName("jts-redesign") != "jts-redesign" {
		t.Error("BranchName: want \"\" on main and the name elsewhere")
	}
}

// --- end to end against a real git repository -------------------------------

// writeProject writes an MPR v2 project (.mpr index + mprcontents) to dir.
func writeProject(t *testing.T, dir, version string, units map[string]tUnit) {
	t.Helper()
	mprPath := filepath.Join(dir, "App.mpr")
	_ = os.Remove(mprPath)
	_ = os.RemoveAll(filepath.Join(dir, "mprcontents"))
	db, err := sql.Open("sqlite", mprPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE _MetaData (_ProductVersion TEXT)`,
		`CREATE TABLE Unit (UnitID BLOB PRIMARY KEY, ContainerID BLOB, ContainmentName TEXT, TreeConflict INTEGER, ContentsHash TEXT, ContentsConflicts TEXT)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO _MetaData VALUES (?)`, version); err != nil {
		t.Fatal(err)
	}
	for id, u := range units {
		sum := sha256.Sum256(u.raw)
		if _, err := db.Exec(`INSERT INTO Unit VALUES (?, ?, '', 0, ?, '')`,
			mpr.IDToBsonBinary(id).Data, mpr.IDToBsonBinary(u.container).Data, base64.StdEncoding.EncodeToString(sum[:])); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "mprcontents", id[:2], id[2:4], id+".mxunit")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, u.raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func gitOutT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func runNote(t *testing.T, mprPath string, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := gitNoteCmd
	cmd.SetOut(&out)
	for _, f := range []string{"write", "force", "branch"} {
		_ = cmd.Flags().Set(f, cmd.Flags().Lookup(f).DefValue)
		cmd.Flags().Lookup(f).Changed = false
	}
	_ = cmd.Flags().Set("project", mprPath)
	var pos []string
	for _, a := range args {
		if strings.HasPrefix(a, "--") {
			kv := strings.SplitN(strings.TrimPrefix(a, "--"), "=", 2)
			v := "true"
			if len(kv) == 2 {
				v = kv[1]
			}
			if err := cmd.Flags().Set(kv[0], v); err != nil {
				t.Fatal(err)
			}
			continue
		}
		pos = append(pos, a)
	}
	if err := runGitNote(cmd, pos); err != nil {
		t.Fatalf("git note %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

func TestGitNote_EndToEnd(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	gitIn(t, dir, "init", "-q", "-b", "feature")
	mprPath := filepath.Join(dir, "App.mpr")

	units := baseUnits(t)
	writeProject(t, dir, "11.13.0", units)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "base")
	// Studio Pro's own note on the base commit must survive.
	gitIn(t, dir, "notes", "--ref="+mxMetadataRef, "add", "-m", realStudioProNote, "HEAD")

	units[tPage] = tUnit{tFolder, unitDoc(t, "Forms$Page", "Home", bson.E{Key: "Title", Value: "Welcome"})}
	writeProject(t, dir, "11.13.0", units)
	gitIn(t, dir, "commit", "-q", "-am", "retitle home")
	// What Studio Pro 11.13 back-fills after a fetch.
	gitIn(t, dir, "notes", "--ref="+mxMetadataRef, "add", "-m",
		`{"BranchName":"(unknown)","ModelerVersion":"(unknown)","ModelChanges":[],"RelatedStories":[],"SolutionVersion":"","MPRFormatVersion":"","HasModelerVersion":false}`, "HEAD")

	out := runNote(t, mprPath, "HEAD~1", "HEAD", "--write")
	if !strings.Contains(out, "kept: already has a Studio Pro note") {
		t.Errorf("the base commit's real note must be kept:\n%s", out)
	}
	if got := strings.TrimSpace(gitOutT(t, dir, "notes", "--ref="+mxMetadataRef, "show", "HEAD~1")); got != realStudioProNote {
		t.Fatalf("real note was changed: %s", got)
	}
	var n mxNote
	if err := json.Unmarshal([]byte(gitOutT(t, dir, "notes", "--ref="+mxMetadataRef, "show", "HEAD")), &n); err != nil {
		t.Fatal(err)
	}
	if n.BranchName != "feature" || n.ModelerVersion != "11.13.0" || n.MPRFormatVersion != "Version2" || !n.HasModelerVersion {
		t.Errorf("header: %+v", n)
	}
	if len(n.ModelChanges) != 1 || n.ModelChanges[0] != (mxModelChange{"Modified", tPage, "Forms$Page", "Home", "Shop"}) {
		t.Errorf("changes: %+v", n.ModelChanges)
	}
	if !strings.Contains(out, "git push origin feature refs/notes/mx_metadata") {
		t.Errorf("must tell how to push the notes ref:\n%s", out)
	}

	// --force replaces a real note.
	runNote(t, mprPath, "HEAD~1", "--write", "--force")
	if got := strings.TrimSpace(gitOutT(t, dir, "notes", "--ref="+mxMetadataRef, "show", "HEAD~1")); got == realStudioProNote {
		t.Fatal("--force did not replace the note")
	}
}
