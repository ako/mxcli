// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance #1 and #3: a blank Mendix 11.14 app cannot build or boot
// as generated. `mx create-project` writes JavaVersion = 25; mxcli builds and
// launches the runtime on JDK 21 and validates it, so the first build fails with
// `error: release version 25 not supported`. Installing a JDK 25 only moves the
// failure to boot time (`UnsupportedClassVersionError … class file version 69.0`)
// because the runtime is still launched on 21.
package main

import "testing"

func TestJavaMajor(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		// Both spellings Mendix has used for this setting.
		{"21", 21, true},
		{"25", 25, true},
		{"Java21", 21, true},
		{"Java25", 25, true},
		{" 21 ", 21, true},

		// Anything not understood must NOT be treated as a version: overwriting a
		// setting we cannot read would replace a value the user can see with one
		// they did not choose.
		{"", 0, false},
		{"Java", 0, false},
		{"tip", 0, false},
		{"21-ea", 0, false},
		{"-3", 0, false},
	}
	for _, tc := range tests {
		got, ok := javaMajor(tc.in)
		if ok != tc.ok {
			t.Errorf("javaMajor(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("javaMajor(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestRuntimeMaxJavaVersionMatchesTheLauncher(t *testing.T) {
	// The constant only means something if it tracks the JDK the runtime is
	// actually launched on. resolveJDK21 hard-requires 21; if that ever becomes
	// version-aware this constant has to move with it, and this test is the
	// reminder — lowering a project to a version the launcher no longer uses
	// would be worse than not lowering it at all.
	if runtimeMaxJavaVersion != 21 {
		t.Errorf("runtimeMaxJavaVersion = %d, but the runtime launcher still resolves a JDK 21 "+
			"(docker.resolveJDK21). Update both together or neither.", runtimeMaxJavaVersion)
	}
}
