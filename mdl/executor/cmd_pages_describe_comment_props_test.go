// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A `--` comment in a widget's property list runs to the end of its line, so
// it swallows whatever the formatter puts after it there: the next property on
// the single-line form, or the `,` separator — and, on the last property, the
// separator's absence leaves the previous line's `,` dangling before `)`.
//
// Measured on FeedbackModule.PopupSuccess (Feedback v4.0.2, Mendix 11.13.0):
// DESCRIBE emitted
//
//	Action: -- open_link with a dynamic address (…) — MDL cannot author this; the button is left as-is,
//
// and exec of the output failed `extraneous input '}' expecting the start of a
// statement`. Five of 17 pages of that project failed to round-trip this way.
func TestFormatWidgetProps_CommentPropsStayParseable(t *testing.T) {
	const comment = "-- DataSource (Forms$Something) has no MDL spelling and is not reproduced here"
	tests := []struct {
		name  string
		props []string
	}{
		{"comment between properties", []string{"Caption: 'x'", comment, "ButtonStyle: Primary"}},
		{"comment last", []string{"Caption: 'x'", comment}},
		{"comment first", []string{comment, "Caption: 'x'"}},
		{"comment only", []string{comment}},
		// Short enough that the single-line form would otherwise be chosen.
		{"short comment", []string{"Caption: 'x'", "-- note"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			formatWidgetProps(&buf, "  ", "actionbutton b", tc.props, "\n")
			src := "create page M.P (Title: 'x', Layout: A.L) {\n" + buf.String() + "}\n"
			if _, errs := visitor.Build(src); len(errs) > 0 {
				t.Fatalf("formatted widget does not parse: %v\n%s", errs, src)
			}
			// The comment is still there for a reader.
			if !strings.Contains(buf.String(), strings.TrimPrefix(tc.props[indexOfComment(tc.props)], "-- ")) {
				t.Errorf("comment text lost:\n%s", buf.String())
			}
		})
	}
}

func indexOfComment(props []string) int {
	for i, p := range props {
		if strings.HasPrefix(p, "--") {
			return i
		}
	}
	return 0
}
