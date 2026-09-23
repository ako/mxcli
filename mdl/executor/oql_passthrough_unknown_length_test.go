// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#585 — MDL031's pass-through length error prescribed the very
// declaration mxbuild rejects.
//
// `formatDataTypeForMDL` substitutes a hardcoded `String(200)` when the inferred
// length is 0, so a source attribute whose length mxcli does not know produced:
//
//	declared as String(200) but pass-through column 'u.Name' inherits length 0
//	from source attribute System.User.Name … Fix: change to 'X: String(200)'
//
// — refusing String(200) and prescribing String(200) in one sentence. Measured on
// mxbuild 11.14.0, `System.User.Name` is String(100): the refusal was right and
// the advice was actively wrong, which is why the reporter rewrote the column as
// `cast(u.Name as string)` instead.
//
// The hardcoded 200 is CORRECT for a derived string column, which is String(200)
// by rule whatever its source — so the fix is confined to the pass-through
// branch, where the number is the SOURCE's and mxcli does not have it.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestPassthroughLength_UnknownSourceLengthIsNotJudged(t *testing.T) {
	// Measured on mxbuild 11.14.0, System.User.Name is String(100):
	//
	//	String(200)  refused by check   CE6770 from mxbuild
	//	String(100)  refused by check   0 errors from mxbuild   ← the correct one
	//	String       passed  by check   CE6770 from mxbuild
	//
	// So judging against a length of 0 blocked the right answer and waved through
	// the wrong one. Neither declaration may be refused until mxcli knows the
	// source length (#584).
	for _, declaredLen := range []int{100, 200} {
		if passthroughStringLengthMismatch(
			ast.DataType{Kind: ast.TypeString, Length: declaredLen},
			ast.DataType{Kind: ast.TypeString, Length: 0},
			"Name") {
			t.Errorf("String(%d) over a source of unknown length was refused — the rule cannot tell "+
				"a correct declaration from a wrong one here, and refusing blocks the one that builds",
				declaredLen)
		}
	}

	// The control: a KNOWN source length is the case the rule exists for, and it
	// must still fire, with advice that names the exact declaration.
	t.Run("a known source length is still judged", func(t *testing.T) {
		declared := ast.DataType{Kind: ast.TypeString, Length: 200}
		inferred := ast.DataType{Kind: ast.TypeString, Length: 100}
		if !passthroughStringLengthMismatch(declared, inferred, "Name") {
			t.Fatal("String(200) over a String(100) source was not refused — this is CE6770 at build time")
		}
		msg := passthroughLengthError("UserName", declared, inferred, "u.Name", "System.User", "Name")
		for _, want := range []string{"inherits length 100", "change to 'UserName: String(100)'", "CE6770"} {
			if !strings.Contains(msg, want) {
				t.Errorf("message does not mention %q: %s", want, msg)
			}
		}
	})

	// And an equal known length is not a mismatch at all.
	if passthroughStringLengthMismatch(
		ast.DataType{Kind: ast.TypeString, Length: 100},
		ast.DataType{Kind: ast.TypeString, Length: 100},
		"Name") {
		t.Error("a matching declaration was refused")
	}
}

// A derived string column is String(200) by rule whatever its source, so the
// default in formatDataTypeForMDL must stay for the type-mismatch suggestions.
func TestFormatDataTypeForMDL_KeepsTheDerivedStringDefault(t *testing.T) {
	if got := formatDataTypeForMDL(ast.DataType{Kind: ast.TypeString, Length: 0}); got != "String(200)" {
		t.Errorf("derived string suggestion = %q, want String(200) — MDL031's rule for a derived column", got)
	}
	if got := formatDataTypeForMDL(ast.DataType{Kind: ast.TypeString, Length: 100}); got != "String(100)" {
		t.Errorf("suggestion with a known length = %q, want String(100)", got)
	}
}
