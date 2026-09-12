// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#813: `dataview dv (…, showFooter: true)` parsed, passed `check`,
// and was silently discarded. ShowFooter was only ever set implicitly, by the
// presence of a `footer { … }` block, so there was no way to show an empty footer and
// no way to declare footer widgets that start hidden.
//
// Two traps sat behind it, both of which produce a silent false rather than an error:
// the property key arrives with the author's casing, and GetBoolProp is
// case-SENSITIVE (unlike GetStringProp) and accepts only a real bool.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

func dataViewWith(props map[string]any, children ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: "dataview", Name: "dv", Properties: props, Children: children}
}

func footerBlock() *ast.WidgetV3 {
	return &ast.WidgetV3{Type: "footer", Name: "f", Children: []*ast.WidgetV3{
		{Type: "dynamictext", Name: "t", Properties: map[string]any{"Content": "x"}},
	}}
}

func TestBuildDataView_ShowFooter(t *testing.T) {
	tests := []struct {
		name     string
		widget   *ast.WidgetV3
		want     bool
		wantErr  bool
		wantFoot int
	}{
		{"absent and no footer block", dataViewWith(map[string]any{}), false, false, 0},
		{"footer block implies true", dataViewWith(map[string]any{}, footerBlock()), true, false, 1},

		// The reported case. Also covers the casing trap: the author writes
		// `showFooter:` and the lookup must match it.
		{"explicit lowercase true", dataViewWith(map[string]any{"showFooter": true}), true, false, 0},
		{"explicit canonical true", dataViewWith(map[string]any{"ShowFooter": true}), true, false, 0},

		// The value may arrive as a string depending on how it was written; a bare
		// GetBoolProp would read that as false rather than complaining.
		{"string true", dataViewWith(map[string]any{"showFooter": "true"}), true, false, 0},
		{"string false", dataViewWith(map[string]any{"showFooter": "false"}), false, false, 0},

		// Explicit wins over the footer block in both directions.
		{"explicit false with footer block", dataViewWith(map[string]any{"showFooter": false}, footerBlock()), false, false, 1},
		{"explicit true without footer block", dataViewWith(map[string]any{"showFooter": true}), true, false, 0},

		// A nonsense value must be refused, not silently treated as false — that is
		// the failure mode this whole fix is about.
		{"invalid value is refused", dataViewWith(map[string]any{"showFooter": "maybe"}), false, true, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pb := &pageBuilder{paramEntityNames: map[string]string{}, widgetScope: map[string]model.ID{}}
			dv, err := pb.buildDataViewV3(tc.widget)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error for an invalid ShowFooter value, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildDataViewV3: %v", err)
			}
			if dv.ShowFooter != tc.want {
				t.Errorf("ShowFooter = %v, want %v (#813)", dv.ShowFooter, tc.want)
			}
			if len(dv.FooterWidgets) != tc.wantFoot {
				t.Errorf("FooterWidgets = %d, want %d — hiding a footer must not discard its widgets",
					len(dv.FooterWidgets), tc.wantFoot)
			}
		})
	}
}

// TestBuildDataView_FormOrientation covers the parse half of #762; the write half
// (that it reaches BSON as LabelWidth) is pinned by ResolvedLabelWidth in sdk/pages
// and by the modelsdk writer test.
func TestBuildDataView_FormOrientation(t *testing.T) {
	tests := []struct {
		name    string
		props   map[string]any
		wantLW  int
		wantErr bool
	}{
		{"vertical", map[string]any{"FormOrientation": "Vertical"}, 0, false},
		{"horizontal", map[string]any{"FormOrientation": "Horizontal"}, 3, false},
		{"lowercase accepted", map[string]any{"FormOrientation": "vertical"}, 0, false},
		{"unset defaults to horizontal", map[string]any{}, 3, false},
		{"explicit LabelWidth wins", map[string]any{"FormOrientation": "Vertical", "LabelWidth": 4}, 4, false},
		{"invalid orientation refused", map[string]any{"FormOrientation": "Sideways"}, 0, true},
		{"out-of-range LabelWidth refused", map[string]any{"LabelWidth": 13}, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pb := &pageBuilder{paramEntityNames: map[string]string{}, widgetScope: map[string]model.ID{}}
			dv, err := pb.buildDataViewV3(dataViewWith(tc.props))
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildDataViewV3: %v", err)
			}
			if got := dv.ResolvedLabelWidth(); got != tc.wantLW {
				t.Errorf("ResolvedLabelWidth() = %d, want %d (#762)", got, tc.wantLW)
			}
		})
	}
}
