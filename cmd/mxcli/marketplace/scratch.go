// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
)

// InstalledModule reports what the project records about a marketplace module:
// the version it was installed from, and the project's own Mendix version.
//
// The recorded AppStoreVersion is what makes "have I changed this?" answerable
// at all — it names the published package the installed copy started life as.
// A module with no recorded version was not installed from the marketplace (or
// was imported by hand), and cannot be compared against anything.
func InstalledModule(mprPath, moduleName string) (appStoreVersion, mendixVersion string, err error) {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return "", "", fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer reader.Close()

	mendixVersion, _ = reader.GetMendixVersion()

	mods, err := reader.ListModules()
	if err != nil {
		return "", "", fmt.Errorf("list modules: %w", err)
	}
	for _, m := range mods {
		if !strings.EqualFold(m.Name, moduleName) {
			continue
		}
		if m.AppStoreVersion == "" {
			return "", mendixVersion, fmt.Errorf(
				"module %q records no marketplace version, so there is no published package to compare it against",
				m.Name)
		}
		return m.AppStoreVersion, mendixVersion, nil
	}
	return "", mendixVersion, fmt.Errorf("module %q not found in %s", moduleName, filepath.Base(mprPath))
}

// PackageProject builds a throwaway project containing nothing but the module
// from mpkPath, and returns the path to its .mpr.
//
// The project is created at mendixVersion — the consuming project's version, not
// the latest — so the imported module goes through the same conversion the
// installed copy went through. Comparing against a package converted to a
// different Mendix version would report the platform's own migrations as user
// edits.
//
// workDir must already exist and is written to freely; callers own its lifetime
// (t.TempDir in tests, an os.MkdirTemp the command removes in production).
// **Keep it short**: mx create-project extracts its template with .NET path
// handling and fails with PathTooLongException under a deep directory, so a
// nested scratch path breaks it where /tmp/xxxx works.
//
// newBackend is used to remove a module the template already ships (see below).
func PackageProject(ctx context.Context, mpkPath, mendixVersion, workDir string, newBackend func() backend.FullBackend) (string, error) {
	mxPath, err := docker.ResolveMxForVersion("", mendixVersion)
	if err != nil {
		return "", fmt.Errorf("locate mx for Mendix %s: %w\n"+
			"hint: run 'mxcli setup mxbuild --version %s'", mendixVersion, err, mendixVersion)
	}

	const appName = "PackageRef"
	create := exec.CommandContext(ctx, mxPath, "create-project", "--app-name", appName)
	create.Dir = workDir
	docker.PrepareMxCommand(create)
	if out, err := create.CombinedOutput(); err != nil {
		return "", fmt.Errorf("mx create-project failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}

	mprPath, err := findScratchMpr(workDir, appName)
	if err != nil {
		return "", err
	}

	// Verify the reference project really is at the requested version.
	//
	// This is not paranoia: ResolveMxForVersion falls back to any cached mxbuild
	// when the requested version is missing — asking for 11.6.6 on a machine
	// holding 11.12.1 returns 11.12.1 without complaint. mx create-project stamps
	// the project with the version of the binary that ran it, so the reference
	// would silently be built one or more Mendix versions away from the project
	// being compared, and every platform migration between them would surface as
	// a user edit. A tool whose false positives are indistinguishable from real
	// findings is worse than no tool, so this refuses instead of warning.
	if err := verifyProjectVersion(mprPath, mendixVersion); err != nil {
		return "", err
	}

	// A "blank" Mendix project is not empty: the template ships Administration,
	// Atlas_Core, Atlas_Web_Content and friends already installed. module-import
	// refuses a name that already exists ("Module 'X' already exist in the app",
	// exit 47) — and Administration is exactly the module the field report cares
	// about. So remove the template's copy first, then import the published one.
	//
	// mxcli's own DROP MODULE is used rather than a file edit because it also
	// unpicks the references the template set up (it reports e.g. "Removed
	// Administration.User from 1 user role(s)"); leaving those dangling would
	// give module-import an inconsistent model to write into.
	if name, err := moduleNameInPackage(mpkPath); err == nil && name != "" {
		if err := dropModuleIfPresent(mprPath, name, newBackend); err != nil {
			return "", err
		}
	}

	imp := exec.CommandContext(ctx, mxPath, "module-import", mpkPath, mprPath)
	docker.PrepareMxCommand(imp)
	if out, err := imp.CombinedOutput(); err != nil {
		return "", fmt.Errorf("mx module-import failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return mprPath, nil
}

// findScratchMpr locates the project mx just created. The name follows
// --app-name, but that is not contractual, so fall back to whatever .mpr exists.
func findScratchMpr(workDir, appName string) (string, error) {
	named := filepath.Join(workDir, appName+".mpr")
	if _, err := os.Stat(named); err == nil {
		return named, nil
	}
	matches, _ := filepath.Glob(filepath.Join(workDir, "*.mpr"))
	if len(matches) > 0 {
		return matches[0], nil
	}
	// mx may nest the project one level down under the app name.
	matches, _ = filepath.Glob(filepath.Join(workDir, "*", "*.mpr"))
	if len(matches) > 0 {
		return matches[0], nil
	}
	return "", fmt.Errorf("mx create-project produced no .mpr under %s", workDir)
}

// verifyProjectVersion checks that a freshly created project is stamped with the
// version we asked for, and returns an actionable error when it is not.
//
// The stamped version is read from the project rather than inferred from the mx
// binary's path, because the path is a cache-layout detail while the stamp is
// what the comparison actually depends on.
func verifyProjectVersion(mprPath, want string) error {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return fmt.Errorf("open reference project: %w", err)
	}
	got, _ := reader.GetMendixVersion()
	_ = reader.Close()

	if got == want {
		return nil
	}
	return fmt.Errorf(
		"reference project was built at Mendix %s but the project under comparison is %s.\n"+
			"Comparing across versions reports Mendix's own conversions as local edits, so this is refused.\n"+
			"hint: run 'mxcli setup mxbuild --version %s' to fetch the matching toolchain",
		orUnknown(got), want, want)
}

func orUnknown(v string) string {
	if v == "" {
		return "an unknown version"
	}
	return v
}

// moduleNameInPackage reads the module name a .mpk declares. package.xml is the
// manifest mx itself reads; note it carries no version — the AppStoreVersion a
// project ends up with after `mx module-import` is the module's *internal*
// version (set by its author), not the marketplace release number. Importing
// Administration 4.3.2 stamps 2.0.1. That stamp never reaches DESCRIBE output,
// so it does not affect the comparison, but it does mean the reference project's
// recorded version is not evidence of which package was imported.
func moduleNameInPackage(mpkPath string) (string, error) {
	zr, err := zip.OpenReader(mpkPath)
	if err != nil {
		return "", fmt.Errorf("open package %s: %w", filepath.Base(mpkPath), err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != "package.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()

		var manifest struct {
			Module struct {
				Name string `xml:"name,attr"`
			} `xml:"modelerProject>module"`
		}
		if err := xml.NewDecoder(rc).Decode(&manifest); err != nil {
			return "", fmt.Errorf("parse package.xml: %w", err)
		}
		return manifest.Module.Name, nil
	}
	return "", fmt.Errorf("no package.xml in %s", filepath.Base(mpkPath))
}

// dropModuleIfPresent removes a module from the reference project when the
// template already provides it. A module that is not there is not an error.
func dropModuleIfPresent(mprPath, moduleName string, newBackend func() backend.FullBackend) error {
	var sink bytes.Buffer
	ex := executor.New(&sink)
	defer ex.Close()
	if newBackend != nil {
		ex.SetBackendFactory(newBackend)
	}
	if err := ex.Execute(&ast.ConnectStmt{Path: mprPath}); err != nil {
		return fmt.Errorf("connect reference project: %w", err)
	}

	mods, err := ex.Backend().ListModules()
	if err != nil {
		return fmt.Errorf("list reference modules: %w", err)
	}
	for _, m := range mods {
		if strings.EqualFold(m.Name, moduleName) {
			if err := ex.Execute(&ast.DropModuleStmt{Name: m.Name}); err != nil {
				return fmt.Errorf("remove the template's %s before importing the package: %w", m.Name, err)
			}
			return nil
		}
	}
	return nil
}
