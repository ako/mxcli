//go:build integration

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/internal/marketplace"
	"github.com/spf13/cobra"
)

// TestInstallFile_ModuleEndToEnd installs a real module package from disk into a
// blank project and checks what an online install promises: the module is in
// the model, the project's storage format is untouched, the package's bundled
// files are on disk, and — new for --file — the module carries no marketplace
// stamp, because it has no marketplace identity.
//
// It needs a package and an mxbuild:
//
//	MXCLI_TEST_MPK=/path/to/Module.mpk MXCLI_TEST_MPK_MENDIX=11.14.0 \
//	  go test -tags integration ./cmd/mxcli -run InstallFile_ModuleEndToEnd -count=1 -v
//
// Without them it skips, so the suite stays hermetic by default. The package
// must NOT be one the blank template already ships (Administration, Atlas_*,
// DataWidgets, MyFirstModule, …): that exercises the "already installed" branch
// instead, which the second half of this test covers on purpose.
func TestInstallFile_ModuleEndToEnd(t *testing.T) {
	mpk := os.Getenv("MXCLI_TEST_MPK")
	version := os.Getenv("MXCLI_TEST_MPK_MENDIX")
	if mpk == "" || version == "" {
		t.Skip("set MXCLI_TEST_MPK and MXCLI_TEST_MPK_MENDIX to run this")
	}
	if _, err := os.Stat(mpk); err != nil {
		t.Skipf("MXCLI_TEST_MPK does not exist: %v", err)
	}
	mxPath, err := docker.ResolveMxForVersion("", version)
	if err != nil {
		t.Skipf("mxbuild %s is not cached; run 'mxcli setup mxbuild --version %s'", version, version)
	}
	moduleName, err := moduleNameFromMpk(mpk)
	if err != nil {
		t.Fatalf("read the package's module name: %v", err)
	}

	// A blank project at the requested version. Short temp path on purpose: mx
	// create-project fails under a deep directory (see scratch.go).
	work, err := os.MkdirTemp("", "mxif")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(work) })
	create := exec.CommandContext(context.Background(), mxPath, "create-project", "--app-name", "InstallFileTarget")
	create.Dir = work
	docker.PrepareMxCommand(create)
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("mx create-project: %v\n%s", err, out)
	}
	var mprPath string
	_ = filepath.WalkDir(work, func(p string, d os.DirEntry, _ error) error {
		if !d.IsDir() && strings.HasSuffix(p, ".mpr") && mprPath == "" {
			mprPath = p
		}
		return nil
	})
	if mprPath == "" {
		t.Fatal("no .mpr in the blank project")
	}
	// ResolveMxForVersion falls back to any cached mxbuild; refuse to test
	// against a project that is not at the version we asked for.
	if got := mendixVersionOf(mprPath); got != version {
		t.Skipf("blank project is Mendix %s, wanted %s (cached mxbuild fallback)", got, version)
	}
	wasV2 := isMPRv2(mprPath)

	// No marketplace client may be constructed. HOME is deliberately NOT
	// redirected here (unlike the unit tests): the transplant resolves mx from
	// ~/.mxcli/mxbuild, and hiding that behind a temp HOME makes the install
	// fail with "mx not found" for a reason that has nothing to do with --file.
	origFactory := marketplaceClientFactory
	marketplaceClientFactory = func(_ context.Context, _ *cobra.Command) (*marketplace.Client, error) {
		t.Fatal("--file must not construct a marketplace client")
		return nil, nil
	}
	t.Cleanup(func() { marketplaceClientFactory = origFactory })

	run := func() (string, error) {
		resetMarketplaceFlags()
		var out bytes.Buffer
		rootCmd.SetOut(&out)
		rootCmd.SetErr(&out)
		rootCmd.SetArgs([]string{"marketplace", "install", "--file", mpk, "-p", mprPath})
		// Two statements on purpose: return operands are evaluated left to
		// right, so `return out.String(), rootCmd.Execute…()` reads the buffer
		// before the command has written to it.
		err := rootCmd.ExecuteContext(context.Background())
		return out.String(), err
	}

	out, err := run()
	if err != nil {
		t.Fatalf("install --file: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Installed module") {
		t.Fatalf("expected an install report, got:\n%s", out)
	}

	// 1. The module is in the model, and carries no marketplace stamp.
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		t.Fatalf("open project after install: %v", err)
	}
	found, appStoreVersion := findModule(reader, moduleName)
	_ = reader.Disconnect()
	if !found {
		t.Fatalf("module %q is not in the project after install\n%s", moduleName, out)
	}
	if appStoreVersion != "" {
		t.Errorf("a package from disk must not be stamped with a marketplace version, got %q", appStoreVersion)
	}

	// 2. Storage format preserved — the reason the transplant writer exists.
	if wasV2 && !isMPRv2(mprPath) {
		t.Errorf("project was MPR v2 before the install and is not afterwards")
	}

	// 3. Bundled files landed. Pick any non-manifest entry from the package.
	zr, err := zip.OpenReader(mpk)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	checked := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || f.Name == "package.xml" || f.Name == "project.mpr" || f.Name == "manifest.json" {
			continue
		}
		if _, serr := os.Stat(filepath.Join(filepath.Dir(mprPath), filepath.FromSlash(f.Name))); serr != nil {
			t.Errorf("bundled file %s not installed: %v", f.Name, serr)
		}
		checked++
	}
	t.Logf("module %q installed from %s: %d bundled file(s) verified on disk", moduleName, filepath.Base(mpk), checked)

	// 4. A second install is reported, not repeated.
	out, err = run()
	if err != nil {
		t.Fatalf("second install --file: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already installed") {
		t.Errorf("second install should report the module as already installed, got:\n%s", out)
	}
}
