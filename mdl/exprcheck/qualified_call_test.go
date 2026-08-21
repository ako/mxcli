// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

// A qualified call left its `(` on the stream, and Parse reported the leftover as
// "Unexpected token after expression … glued keywords such as 'emptyor'" with an
// empty location. It fired on `mxcli check -p` for the VALID decision form as
// much as the invalid ones, so it carried no signal and pointed the author at a
// typo that was not there (upstream #939).
func TestQualifiedCallParsesWithoutTrailingTokenHint(t *testing.T) {
	expr, hs := (&parserImpl{}).Parse("Sample.Rule_IsActive(IsActive = $IsActive)", Context{})
	for _, h := range hs {
		if h.Problem != "" && h.Severity == hints.SeverityError {
			t.Errorf("unexpected error hint: %s", h.Problem)
		}
	}
	call, ok := expr.(*CallExpr)
	if !ok {
		t.Fatalf("parsed %T, want *CallExpr", expr)
	}
	if call.Name != "Sample.Rule_IsActive" {
		t.Errorf("Name = %q, want Sample.Rule_IsActive", call.Name)
	}
	if !call.Qualified {
		t.Error("Qualified = false; the unknown-function check relies on it to skip this call")
	}
	if len(call.Args) != 1 {
		t.Errorf("Args = %d, want 1", len(call.Args))
	}
}

// A bare qualified name (no parentheses) is an enumeration or entity reference
// and must still parse as one — the call handling must not swallow it.
func TestQualifiedNameWithoutCallIsStillAQName(t *testing.T) {
	expr, _ := (&parserImpl{}).Parse("Sample.Status.Open", Context{})
	if _, ok := expr.(*QNameExpr); !ok {
		t.Fatalf("parsed %T, want *QNameExpr", expr)
	}
}

// UnknownFunctionCalls feeds MDL044 ("not a Mendix expression function", with a
// did-you-mean). A qualified call is never a built-in, so a suggestion would be
// nonsense and the diagnostic would also fire on the legal decision form —
// MDL066 reports it instead, with the working spelling.
func TestUnknownFunctionCallsSkipsQualifiedCalls(t *testing.T) {
	if refs := UnknownFunctionCalls("Sample.Rule_IsActive(IsActive = $IsActive)"); len(refs) != 0 {
		t.Errorf("qualified call reported as an unknown function: %+v", refs)
	}
	// Control: an unqualified unknown is still reported.
	refs := UnknownFunctionCalls("randomInt(1)")
	if len(refs) != 1 || refs[0].Name != "randomInt" {
		t.Errorf("unqualified unknown function not reported: %+v", refs)
	}
}
