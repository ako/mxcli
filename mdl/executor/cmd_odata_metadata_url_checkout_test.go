// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1499: a relative MetadataUrl is stored as the absolute file:// URL
// of whichever checkout ran the script. Re-executing the same unchanged script
// from a checkout at another path rewrote every client to its own path — 5 of
// the 20 writes a full re-exec of the reported project produced, on a run that
// should have written nothing:
//
//	-  MetadataUrl: 'file:///workspaces/mxcli-formula1/Formula1Frontend/contracts/f1-metadata.xml',
//	+  MetadataUrl: 'file:///home/user/mxcli-formula1/Formula1Frontend/contracts/f1-metadata.xml',
//
// The stored URL keeps its absolute file:// form (mx check is indifferent to
// the value, so nothing here can show Studio Pro accepts a relative one), and a
// statement whose relative path names the same project file leaves it alone.

// checkoutProject lays out <root>/<name>/app.mpr with contracts/f1.xml holding
// contract, and returns the project directory.
func checkoutProject(t *testing.T, root, name, contract string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "contracts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.mpr"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "contracts", "f1.xml"), []byte(contract), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func fileURL(path string) string { return "file://" + filepath.ToSlash(path) }

// modifyFromCheckout builds the fixture in project dir `here`, with the client
// stored as another run left it (storedURL), runs the statement, and returns
// the service before the statement and the one handed to the backend.
func modifyFromCheckout(t *testing.T, here, storedURL, stmtURL string) (before, after model.ConsumedODataService, f *odataModifyFixture) {
	t.Helper()
	f = newODataModifyFixture(t, here, filepath.Join(here, "contracts", "f1.xml"))
	f.svc.MetadataUrl = storedURL
	before = *f.svc
	f.modify(t, stmtURL)
	return before, **f.updated, f
}

// The reported case: the stored path is from a checkout on another machine,
// where this project sat somewhere that does not exist here.
func TestCreateOrModifyODataClient_KeepsMetadataUrlFromAnotherMachine(t *testing.T) {
	root := t.TempDir()
	here := checkoutProject(t, root, "home-user", edmxWith("Session"))
	foreign := fileURL(filepath.Join(root, "workspaces", "Formula1Frontend", "contracts", "f1.xml"))

	before, after, _ := modifyFromCheckout(t, here, foreign, "./contracts/f1.xml")

	if after.MetadataUrl != foreign {
		t.Errorf("an unchanged statement rewrote the client:\n-  MetadataUrl: %q\n+  MetadataUrl: %q", foreign, after.MetadataUrl)
	}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("the client handed to the backend differs from the stored one, so the write is not elided:\nstored %+v\nnow    %+v", before, after)
	}
}

// A second checkout on the same machine (a git worktree): the stored path
// exists, in another directory holding the same project.
func TestCreateOrModifyODataClient_KeepsMetadataUrlFromAnotherWorktree(t *testing.T) {
	root := t.TempDir()
	first := checkoutProject(t, root, "main", edmxWith("Session"))
	here := checkoutProject(t, root, "worktree", edmxWith("Session"))
	stored := fileURL(filepath.Join(first, "contracts", "f1.xml"))

	before, after, _ := modifyFromCheckout(t, here, stored, "./contracts/f1.xml")

	if !reflect.DeepEqual(after, before) {
		t.Errorf("re-exec from a worktree rewrote the client:\n-  MetadataUrl: %q\n+  MetadataUrl: %q", before.MetadataUrl, after.MetadataUrl)
	}
}

// Control: a stored path that only shares the tail, in a directory that exists
// here and is not a checkout of this project, is a different file. The
// statement moves the client onto the project's copy.
func TestCreateOrModifyODataClient_RewritesMetadataUrlNamingAnotherFile(t *testing.T) {
	root := t.TempDir()
	here := checkoutProject(t, root, "app", edmxWith("Session"))
	shared := filepath.Join(root, "shared")
	if err := os.MkdirAll(filepath.Join(shared, "contracts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "contracts", "f1.xml"), []byte(edmxWith("Session")), 0o600); err != nil {
		t.Fatal(err)
	}
	stored := fileURL(filepath.Join(shared, "contracts", "f1.xml"))

	_, after, _ := modifyFromCheckout(t, here, stored, "./contracts/f1.xml")

	if want := fileURL(filepath.Join(here, "contracts", "f1.xml")); after.MetadataUrl != want {
		t.Errorf("MetadataUrl = %q, want the project's own file %q", after.MetadataUrl, want)
	}
}

// Control: a statement naming a different project file is a change, wherever
// the stored path came from.
func TestCreateOrModifyODataClient_RewritesMetadataUrlForAnotherPath(t *testing.T) {
	root := t.TempDir()
	here := checkoutProject(t, root, "home-user", edmxWith("Session"))
	if err := os.WriteFile(filepath.Join(here, "contracts", "f2.xml"), []byte(edmxWith("Session")), 0o600); err != nil {
		t.Fatal(err)
	}
	foreign := fileURL(filepath.Join(root, "workspaces", "app", "contracts", "f1.xml"))

	_, after, _ := modifyFromCheckout(t, here, foreign, "./contracts/f2.xml")

	if want := fileURL(filepath.Join(here, "contracts", "f2.xml")); after.MetadataUrl != want {
		t.Errorf("MetadataUrl = %q, want %q", after.MetadataUrl, want)
	}
}

// Keeping another checkout's path must not cost the contract refresh: the
// statement's file is the one in this checkout, and it is what gets read.
func TestCreateOrModifyODataClient_RefreshesFromThisCheckoutsFile(t *testing.T) {
	root := t.TempDir()
	here := checkoutProject(t, root, "home-user", edmxWith("Session"))
	f := newODataModifyFixture(t, here, filepath.Join(here, "contracts", "f1.xml"))
	foreign := fileURL(filepath.Join(root, "workspaces", "home-user", "contracts", "f1.xml"))
	f.svc.MetadataUrl = foreign

	if err := os.WriteFile(filepath.Join(here, "contracts", "f1.xml"), []byte(edmxWith("Session", "Trace")), 0o600); err != nil {
		t.Fatal(err)
	}
	f.modify(t, "./contracts/f1.xml")

	got := *f.updated
	if !strings.Contains(got.Metadata, `Name="Trace"`) {
		t.Errorf("the contract was not refreshed from this checkout's file; output:\n%s", f.out.String())
	}
	if got.MetadataUrl != foreign {
		t.Errorf("MetadataUrl = %q, want the stored %q kept", got.MetadataUrl, foreign)
	}
}

// describe prints a stored URL inside the project directory as the
// project-relative path a script uses, so a generated script carries no
// machine path; executing it leaves the client unchanged.
func TestDescribeODataClient_PrintsProjectRelativeMetadataUrl(t *testing.T) {
	root := t.TempDir()
	here := checkoutProject(t, root, "app", edmxWith("Session"))
	f := newODataModifyFixture(t, here, filepath.Join(here, "contracts", "f1.xml"))
	before := *f.svc

	var out bytes.Buffer
	f.ctx.Output = &out
	if err := outputConsumedODataServiceMDL(f.ctx, f.svc, "MyModule", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "MetadataUrl: './contracts/f1.xml'") {
		t.Fatalf("describe did not print the project-relative path:\n%s", out.String())
	}
	if strings.Contains(out.String(), root) {
		t.Fatalf("describe printed this machine's path:\n%s", out.String())
	}

	var stmt *ast.CreateODataClientStmt
	for _, s := range parseMDL(t, out.String()).Statements {
		if c, ok := s.(*ast.CreateODataClientStmt); ok {
			stmt = c
		}
	}
	if stmt == nil {
		t.Fatalf("no create odata client in:\n%s", out.String())
	}
	f.ctx.Output = f.out
	f.modify(t, stmt.MetadataUrl)
	if after := **f.updated; after.MetadataUrl != before.MetadataUrl || after.Metadata != before.Metadata {
		t.Errorf("exec(describe) changed the client: MetadataUrl %q -> %q", before.MetadataUrl, after.MetadataUrl)
	}
}

// Control: a stored URL outside the project directory has no project-relative
// form, and describe prints it as stored.
func TestDescribeODataClient_PrintsMetadataUrlOutsideProjectAsStored(t *testing.T) {
	root := t.TempDir()
	here := checkoutProject(t, root, "app", edmxWith("Session"))
	f := newODataModifyFixture(t, here, filepath.Join(here, "contracts", "f1.xml"))
	outside := fileURL(filepath.Join(root, "elsewhere", "f1.xml"))
	f.svc.MetadataUrl = outside

	var out bytes.Buffer
	f.ctx.Output = &out
	if err := outputConsumedODataServiceMDL(f.ctx, f.svc, "MyModule", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "MetadataUrl: '"+outside+"'") {
		t.Fatalf("describe did not print the stored URL %q:\n%s", outside, out.String())
	}
}
