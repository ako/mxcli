// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1373: `close page n`. The grammar had no count, so a Studio
// Pro flow that closes two pages described as `close page;` (the reader read
// the deleted int key, 0) and a describe → exec made it close one. Once the
// reader read the real NumberOfPagesToClose, describe printed `close page 2;`,
// which did not parse.
func closePageCount(t *testing.T, script string) int {
	t.Helper()
	for _, st := range buildOK(t, script) {
		mf, ok := st.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		for _, b := range mf.Body {
			if c, ok := b.(*ast.ClosePageStmt); ok {
				return c.NumberOfPages
			}
		}
	}
	t.Fatalf("no close page statement in %q", script)
	return 0
}

func TestClosePageCount(t *testing.T) {
	if got := closePageCount(t, "create microflow M.MF () begin close page 2; end;"); got != 2 {
		t.Errorf("close page 2: NumberOfPages = %d, want 2", got)
	}
	// Control: the bare form is one page.
	if got := closePageCount(t, "create microflow M.MF () begin close page; end;"); got != 1 {
		t.Errorf("close page: NumberOfPages = %d, want 1", got)
	}
}

func TestClosePageCountMustBePositiveWhole(t *testing.T) {
	wantRejected(t, "create microflow M.MF () begin close page 0; end;", "at least 1")
	wantRejected(t, "create microflow M.MF () begin close page 1.5; end;", "at least 1")
}
