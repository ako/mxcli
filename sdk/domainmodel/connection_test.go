// SPDX-License-Identifier: Apache-2.0

package domainmodel

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// upstream #872. The anchors are stored as the string "x;y" and were never read,
// so every association write reset them to mxcli's own defaults. Round-tripping
// them safely turns on two properties of the format, both established against
// mxbuild 11.13.0 by hand-patching a project and running `mx check`:
//
//   - Both components must be INTEGERS. "0.5;50" is rejected at load with
//     StorageLoadException ("One or more invalid values were detected while
//     loading the project"), so a value that does not parse as two ints is not
//     a value we should be writing back.
//   - There is NO range validation. "0;500" and "-20;50" both load with 0
//     errors, so a value outside 0..100 must round-trip untouched rather than
//     being clamped to something "sensible".
func TestConnectionPointRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		stored   string
		want     *model.Point
		reFormat string // what FormatConnectionPoint must produce; "" = the fallback
	}{
		{"the value mxcli writes", "0;50", &model.Point{X: 0, Y: 50}, "0;50"},
		{"the value Studio Pro writes in a blank 11.13 app", "0;54", &model.Point{X: 0, Y: 54}, "0;54"},
		{"a hand-dragged anchor", "50;100", &model.Point{X: 50, Y: 100}, "50;100"},
		// The zero point is a REAL anchor (top-left), which is why the field is a
		// pointer — treating it as "unset" would silently rewrite it to the default.
		{"the zero point is a value, not an absence", "0;0", &model.Point{X: 0, Y: 0}, "0;0"},
		// mxbuild accepts both of these, so neither may be normalised away.
		{"out of 0..100 range", "0;500", &model.Point{X: 0, Y: 500}, "0;500"},
		{"negative", "-20;50", &model.Point{X: -20, Y: 50}, "-20;50"},

		// Unreadable ⇒ nil ⇒ the writer's default. Anything Mendix itself would
		// refuse to load is not worth preserving.
		{"absent", "", nil, ""},
		{"non-integer (rejected by Mendix's own loader)", "0.5;50", nil, ""},
		{"non-numeric", "abc;50", nil, ""},
		{"missing separator", "050", nil, ""},
	}

	const fallback = "0;50"
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseConnectionPoint(tc.stored)
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("ParseConnectionPoint(%q) = %+v, want nil so the default applies", tc.stored, got)
			case tc.want != nil && got == nil:
				t.Fatalf("ParseConnectionPoint(%q) = nil, want %+v — a stored anchor was dropped", tc.stored, tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("ParseConnectionPoint(%q) = %+v, want %+v", tc.stored, *got, *tc.want)
			}

			want := tc.reFormat
			if want == "" {
				want = fallback
			}
			if out := FormatConnectionPoint(got, fallback); out != want {
				t.Errorf("FormatConnectionPoint = %q, want %q — the stored anchor must go back verbatim", out, want)
			}
		})
	}
}
