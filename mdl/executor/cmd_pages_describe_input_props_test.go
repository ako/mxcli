// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#550 — `describe page` → `exec` over a Studio Pro page silently drops
// four input-widget properties. Measured on ako/TestApp's
// Administration.ChangePasswordForm at Mendix 11.14.0, with `mx check` at 0
// errors on both sides:
//
//	…/Widgets/[1]/IsPasswordBox          True                          → False
//	…/Widgets/[1]/Validation/Expression  length(toString($value)) > 0  → ''
//	…/ReadOnlyStyle                      Text                          → Control
//	/PopupCloseAction                    cancelButton1                 → ''
//
// The first is the one that matters: a password field round-trips into a
// plaintext text box, and CLAUDE.md makes describe → rename → exec the copy
// operation, so copying a login or change-password page loses it silently.
//
// Each has a different cause, established before writing any of this:
//
//	IsPasswordBox     model + writer carry it; nothing parses it, nothing emits it
//	Validation        widgetValidationToGen() writes a DEFAULT EMPTY validation
//	                  over whatever was stored, on five widget types
//	ReadOnlyStyle     wired for checkbox only; a dataview's is accepted by
//	                  MDL-WIDGET07 (the list is a union across widget types) and
//	                  then dropped
//	PopupCloseAction  pageToGen writes "" unconditionally
package executor

import (
	"strings"
	"testing"
)

// describeOneWidget renders a single widget through the real emitter and returns
// the MDL line, so these assertions cover what a user would replay.
func describeOneWidget(t *testing.T, w rawWidget) string {
	t.Helper()
	ctx, buf := newMockCtx(t)
	outputWidgetMDLV3(ctx, w, 0)
	return strings.TrimSpace(buf.String())
}

func TestDescribeTextBox_EmitsPassword(t *testing.T) {
	got := describeOneWidget(t, rawWidget{
		Type: "Forms$TextBox", Name: "tbSecret", Content: "Secret", IsPassword: true,
	})
	if !strings.Contains(got, "Password: true") {
		t.Errorf("describe dropped the password flag — a password field round-trips\n"+
			"into a plaintext one. got:\n\t%s", got)
	}
}

// Control: an ordinary text box must not gain the clause. Emitting a default is
// the "invents" shape from docs-wiki/bug-patterns/describe-round-trip-gaps.md —
// it puts something in the user's script that they did not write.
func TestDescribeTextBox_OmitsPasswordWhenFalse(t *testing.T) {
	got := describeOneWidget(t, rawWidget{
		Type: "Forms$TextBox", Name: "tbPlain", Content: "Name",
	})
	if strings.Contains(strings.ToLower(got), "password") {
		t.Errorf("describe invented a password clause on a plain text box: %s", got)
	}
}

func TestDescribeTextBox_EmitsValidation(t *testing.T) {
	got := describeOneWidget(t, rawWidget{
		Type: "Forms$TextBox", Name: "tbSecret", Content: "Secret",
		ValidationExpression: "length(toString($value)) > 0",
		ValidationMessage:    "Required",
	})
	if !strings.Contains(got, "length(toString($value)) > 0") {
		t.Errorf("describe dropped the validation expression. got:\n\t%s", got)
	}
	if !strings.Contains(got, "Required") {
		t.Errorf("describe dropped the validation message. got:\n\t%s", got)
	}
	// The expression must be QUOTED, not bracketed. `[...]` is the
	// XPath-constraint spelling and parses as an array, so the builder would see
	// []any and GetStringProp would yield "" — emitter green, round trip broken.
	if strings.Contains(got, "Validation: [") {
		t.Errorf("validation emitted in the bracket form, which does not parse back "+
			"as a string. got:\n\t%s", got)
	}
}

// An expression carrying a quote has to survive the round trip, which means the
// emitter must double it the way every other MDL string does.
func TestDescribeTextBox_ValidationQuotesAreEscaped(t *testing.T) {
	got := describeOneWidget(t, rawWidget{
		Type: "Forms$TextBox", Name: "tbSecret", Content: "Secret",
		ValidationExpression: "$value != 'x'",
	})
	if !strings.Contains(got, "''x''") {
		t.Errorf("an embedded quote was not doubled, so the emitted MDL will not "+
			"re-parse. got:\n\t%s", got)
	}
}

// Control: no validation stored means no clause emitted.
func TestDescribeTextBox_OmitsEmptyValidation(t *testing.T) {
	got := describeOneWidget(t, rawWidget{Type: "Forms$TextBox", Name: "tbPlain", Content: "Name"})
	if strings.Contains(strings.ToLower(got), "validation") {
		t.Errorf("describe invented a validation clause: %s", got)
	}
}

// A DataView's read-only style is a different property from a CheckBox's, and
// only the CheckBox was ever wired — in the builder, the parser and the emitter.
func TestDescribeDataView_EmitsReadOnlyStyle(t *testing.T) {
	got := describeOneWidget(t, rawWidget{
		Type: "Forms$DataView", Name: "dvMain", ReadOnlyStyle: "Text",
	})
	if !strings.Contains(got, "ReadOnlyStyle: Text") {
		t.Errorf("describe dropped a DataView's ReadOnlyStyle. got:\n\t%s", got)
	}
}

// Control: Inherit is Studio Pro's default, so emitting it would invent a clause.
func TestDescribeDataView_OmitsInheritReadOnlyStyle(t *testing.T) {
	for _, style := range []string{"", "Inherit"} {
		got := describeOneWidget(t, rawWidget{
			Type: "Forms$DataView", Name: "dvMain", ReadOnlyStyle: style,
		})
		if strings.Contains(got, "ReadOnlyStyle") {
			t.Errorf("describe emitted ReadOnlyStyle for %q: %s", style, got)
		}
	}
}
