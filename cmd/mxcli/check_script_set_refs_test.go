// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1355: `check -p` over a script set reported a combobox
// datasource naming an entity an earlier file creates as "entity not found",
// although exec, run file by file in that order, succeeds. Every file was
// resolved against the project as it stands, so nothing an earlier file
// creates — entity, association, module — was visible to the files after it.
// The module is a stored one, as in the report, so what fails without the fix
// is the combobox's datasource and the page parameter, not the module.
//
// Controls: the same set in the wrong order still fails (the later file has
// not run yet), a datasource entity no file creates is still reported, and
// the project itself is left as it was.
func TestCheck_ScriptSetResolvesWhatEarlierFilesCreate(t *testing.T) {
	src := filepath.Join("..", "..", "testdata", "pedapp")
	if _, err := os.Stat(filepath.Join(src, "PedApp.mpr")); err != nil {
		t.Skipf("PedApp fixture not found: %v", err)
	}
	dir := t.TempDir()
	if err := copyTree(src, dir); err != nil {
		t.Fatal(err)
	}
	mpr := filepath.Join(dir, "PedApp.mpr")
	_ = checkCmd.InheritedFlags()
	_ = rootCmd.PersistentFlags().Set("project", mpr)
	defer func() {
		_ = rootCmd.PersistentFlags().Set("project", "")
		rootCmd.PersistentFlags().Lookup("project").Changed = false
	}()
	if f := checkCmd.Flags().Lookup("format"); f != nil {
		_ = f.Value.Set(f.DefValue)
	}
	before := treeDigest(t, dir)

	scripts := t.TempDir()
	domain := writeScript(t, scripts, "10-domain.mdl", `create persistent entity MyFirstModule.ShopCustomer ( Name: String(100) );
create persistent entity MyFirstModule.ShopOrder ( Number: String(50) );
create association MyFirstModule.ShopOrder_ShopCustomer from MyFirstModule.ShopOrder to MyFirstModule.ShopCustomer;
`)
	page := func(name, entity string) string {
		return writeScript(t, scripts, name, `create or modify page MyFirstModule.ShopOrderEdit
( Title: 'Order', Layout: Atlas_Core.Atlas_Default, Params: ( $Order: MyFirstModule.ShopOrder ) )
{
  layoutgrid lg { row { column (desktopwidth: autofill) {
    dataview dv (datasource: $Order) {
      combobox cmbCustomer (
        label: 'Customer',
        Association: MyFirstModule.ShopOrder_ShopCustomer,
        datasource: database `+entity+`,
        CaptionAttribute: Name
      )
    }
  } } }
};
`)
	}
	good := page("20-pages.mdl", "MyFirstModule.ShopCustomer")
	bad := page("20-bad.mdl", "MyFirstModule.ShopNoSuchCustomer")

	var code int
	out := captureStd(t, func() { code = runCheckFiles(checkCmd, []string{domain, good}) })
	if code != 0 || strings.Contains(out, "not found") || strings.Contains(out, "does not exist") {
		t.Errorf("a set exec runs cleanly failed its check (exit %d):\n%s", code, out)
	}

	out = captureStd(t, func() { code = runCheckFiles(checkCmd, []string{good, domain}) })
	if code == 0 || !strings.Contains(out, "entity MyFirstModule.ShopOrder does not exist") {
		t.Errorf("control, wrong order: the page ran before its entities and passed (exit %d):\n%s", code, out)
	}

	out = captureStd(t, func() { code = runCheckFiles(checkCmd, []string{domain, bad}) })
	if code == 0 || !strings.Contains(out, "entity not found: MyFirstModule.ShopNoSuchCustomer") {
		t.Errorf("control, missing datasource entity: not reported (exit %d):\n%s", code, out)
	}

	if n := countDiff(before, treeDigest(t, dir)); n > 0 {
		t.Errorf("check wrote to the project: %d file(s) changed", n)
	}
}

// treeDigest maps each file under dir (bar mxcli's own .mxcli cache) to its
// contents.
func treeDigest(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".mxcli" {
				return filepath.SkipDir
			}
			return nil
		}
		b, err := os.ReadFile(p)
		out[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func countDiff(a, b map[string]string) int {
	n := 0
	for k, v := range a {
		if b[k] != v {
			n++
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			n++
		}
	}
	return n
}
