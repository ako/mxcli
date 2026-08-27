// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// alignJavaVersion lowers a freshly created project's Java version ONLY when the
// environment cannot build the one it asks for, and reports whether it did.
//
// Mendix 11.14's `mx create-project` writes JavaVersion = 25, matching Studio
// Pro's support for it. mxcli now resolves the JDK from the project rather than
// from a constant, so a machine with a JDK 25 builds and runs such a project as
// generated — measured: mxbuild reports BUILD SUCCEEDED with no deprecation
// warning, and the runtime boots on the JDK 25.
//
// So the question this step answers is no longer "is 25 supported" (it is) but
// "can THIS machine build it". A devcontainer that ships only a JDK 21 cannot,
// and a generated project that fails its first build is a bad starting point —
// so the version is lowered to something present, with the reason stated.
//
// The order matters both ways round:
//
//   - Lowering when a JDK 25 IS present would downgrade the project against the
//     platform's direction and earn Mendix's "Java versions below 25 are
//     deprecated for deployment" warning for nothing. That is what the first
//     version of this function did, before the runtime launcher became
//     version-aware, and it is why the check is on the environment rather than
//     on a constant.
//   - Not lowering when no JDK 25 is present leaves `mxcli new` producing a
//     project whose first build fails with `error: release version 25 not
//     supported`.
func alignJavaVersion(projectPath string, out io.Writer) (bool, error) {
	major, ok := docker.ProjectJavaMajor(projectPath)
	if !ok {
		// An unreadable or unparseable version is not something to overwrite:
		// guessing would replace a setting the user can see with one they did not
		// choose, and the build reports the real problem better than we can.
		return false, nil
	}
	if _, err := docker.ResolveJDK(major); err == nil {
		return false, nil // this machine can build what the project asks for
	}

	fallback := docker.DefaultJavaMajor
	if major == fallback {
		return false, nil // nothing to fall back to; let the build say so
	}
	if _, err := docker.ResolveJDK(fallback); err != nil {
		// Neither version is installed. Lowering would swap one missing JDK for
		// another and hide that the environment has no JDK at all.
		return false, nil
	}

	script := fmt.Sprintf("alter settings MODEL JavaVersion = '%d';\n", fallback)
	if err := runMDL(projectPath, script, io.Discard); err != nil {
		return false, fmt.Errorf("lowering JavaVersion from %d to %d: %w", major, fallback, err)
	}
	fmt.Fprintf(out, "  Java %d → %d: no JDK %d on this machine, and the project would not build.\n",
		major, fallback, major)
	fmt.Fprintf(out, "  Install a JDK %d and set JavaVersion back to %d to build for it:\n", major, major)
	fmt.Fprintf(out, "    mxcli -p <project>.mpr -c \"alter settings MODEL JavaVersion = '%d'\"\n", major)
	return true, nil
}
