// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// folderFixture builds a catalog with one user module whose documents are laid
// out by the caller: `rootPages`/`rootFlows` land in the module root, and
// `folderedFlows` land in a folder.
//
// Rows go in directly rather than through the MPR reader — the subject is the
// `objects` projection and the rule on top of it, not the parser.
func folderFixture(t *testing.T, module string, rootPages, rootFlows, folderedFlows int) *catalog.Catalog {
	t.Helper()

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()

	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec %s: %v", q, err)
		}
	}

	exec(`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		"mod-"+module, module, "default", "s1")

	for i := 0; i < rootPages; i++ {
		name := fmt.Sprintf("Page_%d", i)
		exec(`INSERT INTO pages_data (Id, Name, QualifiedName, ModuleName, Folder, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?)`,
			fmt.Sprintf("pg-%s-%d", module, i), name, module+"."+name, module, "", "default", "s1")
	}
	addFlow := func(i int, folder string) {
		name := fmt.Sprintf("ACT_Flow_%d", i)
		exec(`INSERT INTO microflows_data
			(Id, Name, QualifiedName, ModuleName, Folder, MicroflowType, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("mf-%s-%d", module, i), name, module+"."+name, module, folder,
			"MICROFLOW", "default", "s1")
	}
	for i := 0; i < rootFlows; i++ {
		addFlow(i, "")
	}
	for i := 0; i < folderedFlows; i++ {
		addFlow(1000+i, "Ordering")
	}

	return cat
}

func runFolderRule(t *testing.T, cat *catalog.Catalog, options map[string]any) []linter.Violation {
	t.Helper()
	// The rule that ships, loaded from disk. A copy inlined here would prove the
	// builtin works and say nothing about whether CONV018 uses it.
	rule, err := linter.LoadStarlarkRule("../../.claude/lint-rules/conv018_module_folder_organization.star")
	if err != nil {
		t.Fatalf("loading the shipped CONV018: %v", err)
	}
	if options != nil {
		rule.Configure(options)
	}
	return rule.Check(linter.NewLintContext(cat, &minimalReader{}))
}

func messages(vs []linter.Violation) string {
	var out []string
	for _, v := range vs {
		out = append(out, v.Message)
	}
	return strings.Join(out, "\n")
}

// The reported project: everything in the root of a single module, no folders.
func TestCONV018_FlagsAFlatModule(t *testing.T) {
	vs := runFolderRule(t, folderFixture(t, "Sales", 15, 10, 0), nil)

	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1:\n%s", len(vs), messages(vs))
	}
	if vs[0].RuleID != "CONV018" {
		t.Errorf("RuleID = %q, want CONV018", vs[0].RuleID)
	}
	got := vs[0].Message
	// The count must be the real total across kinds, not one kind's worth: the
	// whole point is that microflows and pages pile up together.
	for _, want := range []string{"Sales", "all 25 of its documents", "15 pages", "10 microflows"} {
		if !strings.Contains(got, want) {
			t.Errorf("message missing %q:\n%s", want, got)
		}
	}
}

// The exemption that keeps this from being noise. A module that has started to
// organise itself is left alone however much is still at its root — the team has
// made a choice about where things go.
func TestCONV018_SilentOnceAnyFolderExists(t *testing.T) {
	vs := runFolderRule(t, folderFixture(t, "Sales", 15, 10, 1), nil)
	if len(vs) != 0 {
		t.Errorf("a module with a folder was flagged:\n%s", messages(vs))
	}
}

// A small module is not disorganised, it is small.
func TestCONV018_SilentBelowTheThreshold(t *testing.T) {
	vs := runFolderRule(t, folderFixture(t, "Sales", 10, 10, 0), nil)
	if len(vs) != 0 {
		t.Errorf("a 20-document module was flagged at the default threshold of 20:\n%s", messages(vs))
	}
}

// The threshold is the knob a team reaches for first, so it has to actually be
// wired to the option and not only to the module-level default.
func TestCONV018_ThresholdIsConfigurable(t *testing.T) {
	cat := folderFixture(t, "Sales", 15, 10, 0)

	if vs := runFolderRule(t, cat, map[string]any{"max_root_documents": 40}); len(vs) != 0 {
		t.Errorf("raising the threshold to 40 did not silence a 25-document module:\n%s", messages(vs))
	}
	// Control: the same catalog at a threshold below 25 still reports, so the
	// silence above is the option taking effect and not the fixture going empty.
	if vs := runFolderRule(t, cat, map[string]any{"max_root_documents": 5}); len(vs) != 1 {
		t.Errorf("got %d violations at a threshold of 5, want 1:\n%s", len(vs), messages(vs))
	}
}

// Marketplace and System modules hold documents the user did not write and
// cannot reorganise — an update replaces the module. Reporting them would bury
// the finding that is actionable.
func TestCONV018_ExcludesPlatformModules(t *testing.T) {
	cat := folderFixture(t, "Sales", 15, 10, 1) // organised: the user module stays quiet
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, Source, ProjectId, SnapshotId) VALUES (?,?,?,?,?)`,
		"mod-mp", "CommunityCommons", "Marketplace", "default", "s1"); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	for i := 0; i < 30; i++ {
		name := fmt.Sprintf("MP_Flow_%d", i)
		if _, err := db.Exec(`INSERT INTO microflows_data
			(Id, Name, QualifiedName, ModuleName, Folder, MicroflowType, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("mf-mp-%d", i), name, "CommunityCommons."+name, "CommunityCommons", "",
			"MICROFLOW", "default", "s1"); err != nil {
			t.Fatalf("insert microflow: %v", err)
		}
	}

	if vs := runFolderRule(t, cat, nil); len(vs) != 0 {
		t.Errorf("a Marketplace module was flagged:\n%s", messages(vs))
	}

	// Control: the identical 30 flat microflows in a module the user owns DO
	// report. Without this the test passes against a rule that never fires.
	if _, err := db.Exec(`UPDATE modules_data SET Source = '' WHERE Id = ?`, "mod-mp"); err != nil {
		t.Fatalf("update module source: %v", err)
	}
	vs := runFolderRule(t, cat, nil)
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "CommunityCommons") {
		t.Fatalf("control failed: the same module without the Marketplace marker was not reported; got %d:\n%s",
			len(vs), messages(vs))
	}
}

// An association has no folder and cannot be given one — it belongs to the
// domain model. Counting rows the `objects` view hardcodes to an empty folder
// would report a module as unorganised on the strength of elements nobody can
// move. The fixture's user module is under the threshold on real documents, so
// any violation here is entirely made of associations.
func TestCONV018_IgnoresElementsThatCannotBeFoldered(t *testing.T) {
	cat := folderFixture(t, "Sales", 5, 5, 0)
	db := cat.CatalogDB()

	for i := 0; i < 40; i++ {
		name := fmt.Sprintf("Sales.A_%d", i)
		if _, err := db.Exec(`INSERT INTO associations_data
			(Id, Name, QualifiedName, ModuleName, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?)`,
			fmt.Sprintf("as-%d", i), name, name, "Sales", "default", "s1"); err != nil {
			t.Fatalf("insert association: %v", err)
		}
	}

	if vs := runFolderRule(t, cat, nil); len(vs) != 0 {
		t.Errorf("associations were counted as loose documents:\n%s", messages(vs))
	}

	// Control: 40 rows of a kind that CAN be foldered, in the same module, do
	// take it over the threshold — so the silence above is the kind filter and
	// not the fixture failing to reach the rule.
	for i := 0; i < 40; i++ {
		name := fmt.Sprintf("Extra_%d", i)
		if _, err := db.Exec(`INSERT INTO pages_data
			(Id, Name, QualifiedName, ModuleName, Folder, ProjectId, SnapshotId)
			VALUES (?,?,?,?,?,?,?)`,
			fmt.Sprintf("pg-x-%d", i), name, "Sales."+name, "Sales", "", "default", "s1"); err != nil {
			t.Fatalf("insert page: %v", err)
		}
	}
	if vs := runFolderRule(t, cat, nil); len(vs) != 1 {
		t.Fatalf("control failed: 40 loose pages were not reported; got %d:\n%s", len(vs), messages(vs))
	}
}

// Documents() is the projection the rule stands on: it must carry the folder and
// must reach microflows, which documentable_elements deliberately omits.
func TestDocuments_CarryFolderAndCoverFlows(t *testing.T) {
	ctx := linter.NewLintContext(folderFixture(t, "Sales", 2, 2, 1), &minimalReader{})

	byName := map[string]linter.Document{}
	for d := range ctx.Documents() {
		byName[d.QualifiedName] = d
	}

	root, ok := byName["Sales.ACT_Flow_0"]
	if !ok {
		t.Fatalf("microflows are missing from Documents(): %v", byName)
	}
	if root.Kind != "MICROFLOW" || root.Folder != "" {
		t.Errorf("root microflow = %+v, want kind MICROFLOW and an empty folder", root)
	}

	foldered, ok := byName["Sales.ACT_Flow_1000"]
	if !ok {
		t.Fatal("the foldered microflow is missing from Documents()")
	}
	if foldered.Folder != "Ordering" {
		t.Errorf("folder = %q, want %q — the folder is what the rule reads", foldered.Folder, "Ordering")
	}

	// A module is a container, not something inside one.
	for qn, d := range byName {
		if d.Kind == "MODULE" {
			t.Errorf("Documents() yielded a MODULE row: %s", qn)
		}
	}
}
