// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// runtimeMaxJavaVersion is the highest Java release mxcli can BUILD and RUN a
// project on. It is not a preference: `mxcli run --local`, `mxcli test --local`
// and the Docker build all resolve a JDK 21 explicitly (resolveJDK21) and refuse
// a JAVA_HOME that is anything else, and Mendix 11.14's own runtime bundles are
// still Java 21 class files (major version 65).
//
// Keep this in step with resolveJDK21. Raising one without the other is what
// this constant exists to make visible.
const runtimeMaxJavaVersion = 21

// alignJavaVersion lowers a freshly created project's JavaVersion to something
// mxcli can actually build and run, and reports what it did.
//
// Mendix 11.14's `mx create-project` writes JavaVersion = 25. mxcli launches the
// runtime on JDK 21 and validates it, so a blank 11.14 app cannot boot as
// generated: the first build fails with `error: release version 25 not
// supported`, and installing a JDK 25 to get past it only moves the failure to
// boot time (`UnsupportedClassVersionError … class file version 69.0`, because
// the runtime is still launched on 21). Compiling for 25 and running on 21
// cannot work either way (ako/mxcli-maintenance findings 1 and 3).
//
// This lowers ONLY when the project asks for more than mxcli can run, so a
// project already at or below the limit is untouched and the step disappears by
// itself the day the runtime launcher becomes version-aware.
//
// Mendix warns "Java versions below 25 are deprecated for deployment" at build
// time. That is a deprecation notice on a project that builds and runs, which
// beats a project that does neither.
func alignJavaVersion(projectPath string, out io.Writer) (bool, error) {
	current, err := readJavaVersion(projectPath)
	if err != nil {
		return false, err
	}
	n, ok := javaMajor(current)
	if !ok {
		// An unparseable or absent value is not something to overwrite: the model
		// may be using a spelling this build does not know, and guessing would
		// replace a setting the user can see with one they did not choose.
		return false, nil
	}
	if n <= runtimeMaxJavaVersion {
		return false, nil
	}
	script := fmt.Sprintf("alter settings MODEL JavaVersion = '%d';\n", runtimeMaxJavaVersion)
	if err := runMDL(projectPath, script, io.Discard); err != nil {
		return false, fmt.Errorf("lowering JavaVersion from %s to %d: %w", current, runtimeMaxJavaVersion, err)
	}
	fmt.Fprintf(out, "  Java %s → %d: mxcli builds and runs on JDK %d, and Mendix %s's runtime\n",
		current, runtimeMaxJavaVersion, runtimeMaxJavaVersion, "11.14+")
	fmt.Fprintf(out, "  bundles are still Java %d class files. Left at %s the project does not build.\n",
		runtimeMaxJavaVersion, current)
	return true, nil
}

// readJavaVersion reads the project's model JavaVersion. The stored key differs
// by Mendix version (JavaVersion "Java21" before 11.12, JavaMajorVersion "21"
// after), which the settings reader already normalises.
func readJavaVersion(projectPath string) (string, error) {
	b := newBackendFactory()()
	if err := b.Connect(projectPath); err != nil {
		return "", err
	}
	defer func() { _ = b.Disconnect() }()

	ps, err := b.GetProjectSettings()
	if err != nil {
		return "", err
	}
	if ps == nil || ps.Model == nil {
		return "", nil
	}
	return ps.Model.JavaVersion, nil
}

// javaMajor extracts the major release from either spelling Mendix has used:
// "21" and "Java21" both yield 21.
func javaMajor(v string) (int, bool) {
	s := strings.TrimSpace(v)
	s = strings.TrimPrefix(s, "Java")
	s = strings.TrimPrefix(s, "java")
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
