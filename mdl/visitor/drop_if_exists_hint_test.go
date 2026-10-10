// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

const dropIfExistsHint = "`if exists` goes before the name"

// TestDropIfExistsAfterNameHint covers ledger FINDINGS #166: the SQL order
// `drop microflow M.F if exists;` is a syntax error, and under `mdl 1;` the
// parser's recovery swallowed `if exists;` so a second error blamed a missing
// `;` that is there. The fix is one sentence: name the order, and do not claim
// a missing terminator on a line that already has a syntax error.
func TestDropIfExistsAfterNameHint(t *testing.T) {
	for _, src := range []string{
		"drop microflow M.F if exists;",
		"mdl 1;\ndrop microflow M.F if exists;",
		"mdl 1;\ndrop module role M.Admin if exists;",
	} {
		_, errs := Build(src)
		if len(errs) == 0 {
			t.Fatalf("%q: expected a syntax error", src)
		}
		joined := errsText(errs)
		if !strings.Contains(joined, dropIfExistsHint) {
			t.Errorf("%q: no if-exists hint in:\n%s", src, joined)
		}
		if strings.Contains(joined, "no terminating `;`") {
			t.Errorf("%q: blames a missing `;` that is present:\n%s", src, joined)
		}
	}
}

// The control: the right order parses, and a statement that really lacks its
// `;` under mdl 1 is still refused for it.
func TestDropIfExistsControls(t *testing.T) {
	if _, errs := Build("mdl 1;\ndrop microflow if exists M.F;"); len(errs) != 0 {
		t.Errorf("canonical order refused: %v", errs)
	}
	_, errs := Build("mdl 1;\ndrop microflow if exists M.F\nlist modules;")
	if !strings.Contains(errsText(errs), "no terminating `;`") {
		t.Errorf("a genuinely missing `;` is no longer reported: %v", errs)
	}
}
