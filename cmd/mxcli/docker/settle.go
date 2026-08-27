// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

// settle.go runs one deploy build against a freshly created project so that the
// sources MxBuild generates are already in their post-build shape.
//
// mxcli-todo findings #7: the template ships `javascriptsource/*/actions/*.js`
// (and the matching Java stubs) in a slightly older shape, and MxBuild rewrites
// every one of them on the first build — banner, `import { Big } from "big.js"`,
// `async function X()` -> `export async function X()`. They are *tracked* files,
// so a fresh clone goes dirty on the first build (48 of them in a blank Mendix
// 11.12 app) and stays dirty until someone commits build output they did not
// write. `mx check` does not do this; only a build does. Doing that build while
// the project is still being created means the first commit already holds the
// settled form.

// SettleGeneratedSources runs `mxbuild --target=deploy` once against
// projectPath. It is best-effort by contract: every failure is returned for the
// caller to report as a warning, never as a reason to fail project creation.
//
// mxPath is the `mx` binary already resolved by the caller — mxbuild lives
// beside it — and version is used to find a cached download when it does not.
func SettleGeneratedSources(projectPath, mxPath, version string, w io.Writer) error {
	mxbuildPath := resolveMxBuildForSettle(mxPath, version)
	if mxbuildPath == "" {
		return fmt.Errorf("mxbuild not found next to %s or in the cache for %s", mxPath, version)
	}
	javaMajor, _ := ProjectJavaMajor(projectPath)
	javaHome, err := resolveJDK(javaMajor)
	if err != nil {
		return fmt.Errorf("no JDK %d available: %w", javaMajor, err)
	}

	cmd := exec.Command(mxbuildPath,
		"--target=deploy",
		fmt.Sprintf("--java-home=%s", javaHome),
		fmt.Sprintf("--java-exe-path=%s", JavaExePath(javaHome)),
		projectPath,
	)
	cmd.Dir = filepath.Dir(projectPath)
	PrepareMxCommand(cmd) // FreeType LD_PRELOAD workaround

	// MxBuild is very chatty and this build is incidental to what the user asked
	// for, so its output is only worth showing when it fails.
	out := &syncBuffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mxbuild failed: %w\n%s", err, lastLines(out.String(), 20))
	}
	fmt.Fprintln(w, "  Generated sources are in their post-build form.")
	return nil
}

// resolveMxBuildForSettle finds mxbuild beside the resolved mx binary (the
// Studio Pro / CDN layout puts them in the same directory), falling back to the
// version's download cache.
func resolveMxBuildForSettle(mxPath, version string) string {
	if mxPath != "" {
		if found := findMxBuildInDir(filepath.Dir(mxPath)); found != "" {
			return found
		}
	}
	return CachedMxBuildPath(version)
}

// lastLines returns the final n lines of s — enough to show why a build failed
// without pasting the whole log into a creation summary.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
