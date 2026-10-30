// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// ako/mxcli#1618 (mendixlabs/mxcli#1634): a code action's body is not in the
// model. `create java action … as $$ … $$` stores the action in a unit and
// writes the body to javasource/<module>/actions/<Name>.java, and `exec` named
// only the model element — so a workflow that commits the model (.mpr and
// mprcontents/) left the body behind, and a clone got an action with no logic.
// The backend now records every file it changes beside the model, at the point
// it touches disk, so `exec` can name each one.

var _ backend.SourceFileReporter = (*Backend)(nil)

func newSourceFileBackend(t *testing.T) (*Backend, string) {
	t.Helper()
	root := t.TempDir()
	return &Backend{path: filepath.Join(root, "App.mpr")}, root
}

func takeChanges(t *testing.T, b *Backend) []backend.SourceFileChange {
	t.Helper()
	return b.TakeSourceFileChanges()
}

func TestWriteJavaSourceFileRecordsTheFile(t *testing.T) {
	b, _ := newSourceFileBackend(t)
	if err := b.WriteJavaSourceFile("Probe", "JA_Hello", `return "hi";`, nil, nil, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	want := []backend.SourceFileChange{{Path: "javasource/probe/actions/JA_Hello.java", What: "Java action body"}}
	if got := takeChanges(t, b); !reflect.DeepEqual(got, want) {
		t.Errorf("changes = %+v, want %+v", got, want)
	}
	if got := takeChanges(t, b); len(got) != 0 {
		t.Errorf("a second take returned %+v; taking must forget what it returned", got)
	}

	// An identical rewrite does not touch the file, so there is nothing to
	// commit and nothing is recorded.
	if err := b.WriteJavaSourceFile("Probe", "JA_Hello", `return "hi";`, nil, nil, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if got := takeChanges(t, b); len(got) != 0 {
		t.Errorf("identical rewrite recorded %+v, want nothing", got)
	}

	// Control: a changed body is recorded again.
	if err := b.WriteJavaSourceFile("Probe", "JA_Hello", `return "bye";`, nil, nil, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if got := takeChanges(t, b); !reflect.DeepEqual(got, want) {
		t.Errorf("changed body: changes = %+v, want %+v", got, want)
	}
}

func TestWriteJavaScriptSourceFileRecordsTheFile(t *testing.T) {
	b, _ := newSourceFileBackend(t)
	if err := b.WriteJavaScriptSourceFile("Probe", "JS_Hello", `return "hi";`, nil, nil); err != nil {
		t.Fatal(err)
	}
	want := []backend.SourceFileChange{{Path: "javascriptsource/probe/actions/JS_Hello.js", What: "JavaScript action body"}}
	if got := takeChanges(t, b); !reflect.DeepEqual(got, want) {
		t.Errorf("changes = %+v, want %+v", got, want)
	}
	if err := b.WriteJavaScriptSourceFile("Probe", "JS_Hello", `return "hi";`, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := takeChanges(t, b); len(got) != 0 {
		t.Errorf("identical rewrite recorded %+v, want nothing", got)
	}
}

// A rename moves the body, and a drop deletes it: both are changes the commit
// has to carry, or a clone keeps the old file and lacks the new one.
func TestRenameAndDeleteOfASourceFileAreRecorded(t *testing.T) {
	b, _ := newSourceFileBackend(t)
	if err := b.WriteJavaSourceFile("Probe", "JA_Old", `return "hi";`, nil, nil, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := b.WriteJavaScriptSourceFile("Probe", "JS_Old", `return "hi";`, nil, nil); err != nil {
		t.Fatal(err)
	}
	takeChanges(t, b)

	if err := b.RenameJavaSourceFile("Probe", "JA_Old", "JA_New"); err != nil {
		t.Fatal(err)
	}
	if err := b.RenameJavaScriptSourceFile("Probe", "JS_Old", "JS_New"); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteJavaSourceFile("Probe", "JA_New"); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteJavaScriptSourceFile("Probe", "JS_New"); err != nil {
		t.Fatal(err)
	}
	want := []backend.SourceFileChange{
		{Path: "javasource/probe/actions/JA_New.java", From: "javasource/probe/actions/JA_Old.java", What: "Java action body"},
		{Path: "javascriptsource/probe/actions/JS_New.js", From: "javascriptsource/probe/actions/JS_Old.js", What: "JavaScript action body"},
		{Path: "javasource/probe/actions/JA_New.java", Removed: true, What: "Java action body"},
		{Path: "javascriptsource/probe/actions/JS_New.js", Removed: true, What: "JavaScript action body"},
	}
	if got := takeChanges(t, b); !reflect.DeepEqual(got, want) {
		t.Errorf("changes =\n%+v\nwant\n%+v", got, want)
	}

	// A file that is not there is not renamed or removed, and nothing is recorded.
	if err := b.RenameJavaSourceFile("Probe", "JA_Missing", "JA_Other"); err != nil {
		t.Fatal(err)
	}
	if err := b.RenameJavaScriptSourceFile("Probe", "JS_Missing", "JS_Other"); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteJavaSourceFile("Probe", "JA_Missing"); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteJavaScriptSourceFile("Probe", "JS_Missing"); err != nil {
		t.Fatal(err)
	}
	if got := takeChanges(t, b); len(got) != 0 {
		t.Errorf("missing files recorded %+v, want nothing", got)
	}
}

// DROP MODULE removes the module's source folders beside the model.
func TestRemovingAModulesSourceFoldersIsRecorded(t *testing.T) {
	b, root := newSourceFileBackend(t)
	for _, dir := range []string{"themesource/probe/web", "javasource/probe/actions"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b.removeModuleSourceDirs("Probe")
	want := []backend.SourceFileChange{
		{Path: "themesource/probe/", Removed: true, What: "module theme source"},
		{Path: "javasource/probe/", Removed: true, What: "module Java source"},
	}
	if got := takeChanges(t, b); !reflect.DeepEqual(got, want) {
		t.Errorf("changes = %+v, want %+v", got, want)
	}
	for _, dir := range []string{"themesource/probe", "javasource/probe"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); !os.IsNotExist(err) {
			t.Errorf("%s still exists (stat err %v)", dir, err)
		}
	}
}
