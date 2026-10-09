// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// Re-running `create or modify navigation Responsive …` printed "Navigation
// profile 'Responsive' updated." on every run, while canon.Reconcile elided the
// unit write and no file changed. The statement reports through reportWrite like
// security and settings do (ako/mxcli#890), so the second run says Unchanged.
func TestCreateOrModifyNavigation_ReportsWhatHappened(t *testing.T) {
	for _, tc := range []struct {
		name     string
		written  int
		want     string
		wantKept bool
	}{
		{"rewrite that landed (control)", 1, "Navigation profile 'Responsive' updated.", true},
		{"elided rewrite", 0, "Unchanged navigation profile 'Responsive'", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cb := &countingBackend{}
			cb.MockBackend = &mock.MockBackend{
				IsConnectedFunc: func() bool { return true },
				GetNavigationFunc: func() (*types.NavigationDocument, error) {
					return &types.NavigationDocument{Profiles: []*types.NavigationProfile{{
						Name: "Responsive", Kind: "Responsive",
						MenuItems: []*types.NavMenuItem{{
							Caption: "Reports", ActionType: "Forms$UnknownFutureClientAction",
							StoredAction: []byte("stored"), ActionDoc: map[string]any{"$Type": "Forms$UnknownFutureClientAction"},
						}},
					}}}, nil
				},
				UpdateNavigationProfileFunc: func(model.ID, string, types.NavigationProfileSpec) error {
					cb.offer(1, tc.written)
					return nil
				},
			}
			ctx, out := newMockCtx(t)
			ctx.Backend = cb
			prog := parseMDL(t, "create or modify navigation Responsive {\n  menu item 'Reports'\n};")
			assertNoError(t, execAlterNavigation(ctx, prog.Statements[0].(*ast.AlterNavigationStmt)))
			got := out.String()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("output %q, want it to contain %q", got, tc.want)
			}
			if tc.written == 0 && strings.Contains(got, "updated") {
				t.Errorf("an elided rewrite must not report a write, got %q", got)
			}
			// The kept-action note describes what a rewrite carried; a run that
			// rewrote nothing carried nothing.
			if kept := strings.Contains(got, "kept the stored action"); kept != tc.wantKept {
				t.Errorf("kept-action note printed=%v, want %v; output %q", kept, tc.wantKept, got)
			}
		})
	}
}
