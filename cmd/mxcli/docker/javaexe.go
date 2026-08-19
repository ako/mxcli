// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"path/filepath"
	"runtime"
)

// JavaExePath is <javaHome>/bin/java, with the .exe Windows needs.
//
// Detection got this right (isJDK21 appends it) and every consumer got it
// wrong: the path handed to mxbuild as --java-exe-path, and the one
// exec.Command runs to boot the runtime, were both built as a bare
// "…\bin\java". So on Windows a correctly-detected JDK was passed on in a form
// that need not resolve — which reads, from the outside, as "mxcli does not
// detect Java".
//
// One helper rather than five call sites, because the next one added would have
// made the same mistake.
func JavaExePath(javaHome string) string {
	exe := "java"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	return filepath.Join(javaHome, "bin", exe)
}

// javaExeName is the java binary's file name for a given OS, so the Windows
// form is assertable from a Linux runner (nothing in CI executes Windows code).
func javaExeName(goos string) string {
	if goos == "windows" {
		return "java.exe"
	}
	return "java"
}
