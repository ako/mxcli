// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestAnnotateForwardRef_SkipsOwnName covers a misfire found while adding
// MDL054: a statement's OWN name is "defined in the script but not yet
// created" at the moment it fails, so any validation error whose message names
// its own subject picked up the reorder hint — advising the author to move a
// statement before itself.
//
// A genuine forward reference (a name some LATER statement defines) must still
// be annotated.
func TestAnnotateForwardRef_SkipsOwnName(t *testing.T) {
	script := `create non-persistent entity Test.NpEntity ( "Name" : String(100) not null );
create microflow Test.Later () returns Boolean begin return true; end;`
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	allDefined := newScriptContext()
	for _, s := range prog.Statements {
		allDefined.collectSingle(s)
	}
	created := newScriptContext() // nothing created yet

	t.Run("own name is not a forward reference", func(t *testing.T) {
		err := errors.New("attribute 'Name' declares `not null` on non-persistent entity Test.NpEntity")
		got := annotateForwardRef(err, prog.Statements[0], created, allDefined)
		if strings.Contains(got.Error(), "defined later in this script") {
			t.Errorf("statement was annotated as referring forward to itself:\n%s", got.Error())
		}
	})

	t.Run("a genuine forward reference is still annotated", func(t *testing.T) {
		err := errors.New("microflow not found: Test.Later")
		got := annotateForwardRef(err, prog.Statements[0], created, allDefined)
		if !strings.Contains(got.Error(), "defined later in this script") {
			t.Errorf("expected a reorder hint for Test.Later, got:\n%s", got.Error())
		}
	})
}
