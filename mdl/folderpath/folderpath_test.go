// SPDX-License-Identifier: Apache-2.0

package folderpath

import (
	"reflect"
	"testing"
)

func TestJoinSplitRoundTrip(t *testing.T) {
	for _, names := range [][]string{
		{"Private"},
		{"Private", "Apis"},
		// mendixlabs/mxcli#1367: one folder whose name holds a slash.
		{"Private - String en/de-cryption", "Apis"},
		{`A\B`},
		{`ends\`, "next"},
		{`a\/b`},
		{"/", "//"},
	} {
		path := Join(names)
		if got := Split(path); !reflect.DeepEqual(got, names) {
			t.Errorf("Split(Join(%q)) = %q via %q", names, got, path)
		}
	}
}

func TestSplitKeepsUnescapedPathsAsBefore(t *testing.T) {
	for in, want := range map[string][]string{
		"":          nil,
		"A/B":       {"A", "B"},
		"/A//B/":    {"A", "B"},
		`A\B`:       {`A\B`}, // a backslash before an ordinary character is literal
		`C:\temp/x`: {`C:\temp`, "x"},
	} {
		if got := Split(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Split(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinEscapesOnlyWhatItMust(t *testing.T) {
	if got := Join([]string{"Private - String en/de-cryption", "Apis"}); got != `Private - String en\/de-cryption/Apis` {
		t.Errorf("got %q", got)
	}
	if got := Join([]string{"Private", "Apis"}); got != "Private/Apis" {
		t.Errorf("a path without slashes changed: %q", got)
	}
}
