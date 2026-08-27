// SPDX-License-Identifier: Apache-2.0

// Version-aware JDK selection. Mendix Studio Pro 11.14 supports Java 25 and its
// blank app asks for it, so a project's Settings > Model > JavaVersion is now a
// real variable rather than always 21. mxcli resolved a JDK 21 unconditionally
// and refused any other JAVA_HOME, which made an 11.14 project impossible to
// build (`error: release version 25 not supported`) or to boot when built
// elsewhere (`UnsupportedClassVersionError … class file version 69.0`).
package docker

import (
	"strings"
	"testing"
)

func TestParseJavaMajor(t *testing.T) {
	tests := []struct {
		in   string
		want int
		ok   bool
	}{
		// Both spellings Mendix has used for the setting.
		{"21", 21, true},
		{"25", 25, true},
		{"Java21", 21, true},
		{"Java25", 25, true},
		{" 25 ", 25, true},

		// Not understood: the caller must fall back rather than guess. Guessing a
		// JDK from an unreadable model trades a clear failure for a confusing one.
		{"", 0, false},
		{"Java", 0, false},
		{"tip", 0, false},
		{"25-ea", 0, false},
		{"-3", 0, false},
	}
	for _, tc := range tests {
		got, ok := ParseJavaMajor(tc.in)
		if ok != tc.ok {
			t.Errorf("ParseJavaMajor(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("ParseJavaMajor(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestJavaMajorRegexDoesNotMatchALongerNumber(t *testing.T) {
	// The trap in matching `java -version` text: a bare "21" also occurs inside
	// "210". The major must be followed by a dot, whitespace or the closing
	// quote. Without this a future JDK 210 would satisfy a request for 21 — and,
	// more immediately, "2" would satisfy "25" if the pattern were ever loosened.
	re := javaMajorRegex(21)
	for _, out := range []string{
		`openjdk version "21.0.4" 2024-07-16`,
		`openjdk version "21" 2023-09-19`,
	} {
		if !re.MatchString(out) {
			t.Errorf("JDK 21 pattern should match %q", out)
		}
	}
	for _, out := range []string{
		`openjdk version "210.0.1" 2099-01-01`,
		`openjdk version "25" 2025-09-16`,
		`openjdk version "17.0.9" 2023-10-17`,
	} {
		if re.MatchString(out) {
			t.Errorf("JDK 21 pattern must not match %q", out)
		}
	}
	// And the new case has to work in its own right.
	if !javaMajorRegex(25).MatchString(`openjdk version "25" 2025-09-16`) {
		t.Error("JDK 25 pattern should match a 25 version string")
	}
}

func TestJdkSearchPathsTrackTheRequestedMajor(t *testing.T) {
	// The search paths are globs with the version baked in, so they have to move
	// with the request or a JDK 25 is never found even when installed.
	// windowsProgramDirs reads the environment, so the Windows list is empty
	// without it — the same setup the existing search-path test does.
	t.Setenv("PROGRAMFILES", `C:\Program Files`)
	t.Setenv("LOCALAPPDATA", `C:\Users\dev\AppData\Local`)
	for _, goos := range []string{"linux", "darwin", "windows"} {
		for _, major := range []int{21, 25} {
			paths := jdkSearchPathsFor(goos, major)
			if len(paths) == 0 {
				t.Fatalf("%s/%d: no search paths", goos, major)
			}
			want := map[int]string{21: "21", 25: "25"}[major]
			other := map[int]string{21: "25", 25: "21"}[major]
			for _, p := range paths {
				if !strings.Contains(p, want) {
					t.Errorf("%s/%d: %q does not pin the requested major", goos, major, p)
				}
				if strings.Contains(p, other) {
					t.Errorf("%s/%d: %q still mentions %s", goos, major, p, other)
				}
			}
		}
	}
}

func TestJavaMajorOrDefault(t *testing.T) {
	// Zero means "the caller did not say", which must behave exactly as mxcli did
	// before this change — otherwise every construction site that forgets the
	// field silently changes behaviour.
	if got := javaMajorOrDefault(0); got != DefaultJavaMajor {
		t.Errorf("javaMajorOrDefault(0) = %d, want %d", got, DefaultJavaMajor)
	}
	if got := javaMajorOrDefault(-1); got != DefaultJavaMajor {
		t.Errorf("javaMajorOrDefault(-1) = %d, want %d", got, DefaultJavaMajor)
	}
	if got := javaMajorOrDefault(25); got != 25 {
		t.Errorf("javaMajorOrDefault(25) = %d, want 25", got)
	}
}

func TestProjectJavaMajorFallsBackOnAnUnreadableProject(t *testing.T) {
	got, fromProject := ProjectJavaMajor("/nonexistent/project.mpr")
	if got != DefaultJavaMajor || fromProject {
		t.Errorf("ProjectJavaMajor(missing) = (%d, %v), want (%d, false)", got, fromProject, DefaultJavaMajor)
	}
}
