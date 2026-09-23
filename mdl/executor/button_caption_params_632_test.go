// SPDX-License-Identifier: Apache-2.0

// An action button's caption parameters (ako/mxcli#632).
//
// Reported: "`captionparams` on an action button is accepted by `mxcli check`
// but not written; a container with `onclick` and a dynamic text does the job."
//
// Two silent drops, both measured on a real 11.12.1 app:
//
//   - The button carried its own copy of the parameter resolver, which read a
//     bare attribute (`[{1} = Title]`) as the string literal 'Title'. The same
//     text on a dynamictext binds the attribute. `check --references` passed, so
//     the button rendered the word "Title".
//   - DESCRIBE printed the button's parameters as `ContentParams:`, which the
//     button builder never read. Re-executing a description passed `check` and
//     wrote the caption with every parameter gone.
package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

func buttonCaptionPB() *pageBuilder {
	return &pageBuilder{
		paramEntityNames: map[string]string{"o": "Rp.Order"},
		widgetScope:      map[string]model.ID{},
		entityContext:    "Rp.Order",
	}
}

// A bare attribute name binds the attribute in the dataview's context, exactly
// as it does on a dynamictext — not a literal holding the attribute's name.
func TestButtonCaptionParams_BareAttributeBindsAttribute(t *testing.T) {
	btn, err := buttonCaptionPB().buildButtonV3(&ast.WidgetV3{
		Type: "actionbutton", Name: "b",
		Properties: map[string]any{
			"Caption":       "Bare {1}",
			"CaptionParams": []ast.ParamAssignmentV3{{Index: 1, Value: "Title"}},
			"Action":        "SAVE_CHANGES",
		},
	})
	if err != nil {
		t.Fatalf("buildButtonV3: %v", err)
	}
	params := btn.CaptionTemplate.Parameters
	if len(params) != 1 {
		t.Fatalf("caption has %d parameter(s), want 1", len(params))
	}
	p := params[0]
	if p.Expression != "" {
		t.Errorf("bare attribute written as expression %q — the button shows the attribute's NAME, not its value", p.Expression)
	}
	if p.AttributeRef != "Rp.Order.Title" {
		t.Errorf("AttributeRef = %q, want Rp.Order.Title", p.AttributeRef)
	}
}

// A quoted literal stays a literal.
func TestButtonCaptionParams_QuotedLiteralStaysLiteral(t *testing.T) {
	btn, err := buttonCaptionPB().buildButtonV3(&ast.WidgetV3{
		Type: "actionbutton", Name: "b",
		Properties: map[string]any{
			"Caption":       "Lit {1}",
			"CaptionParams": []ast.ParamAssignmentV3{{Index: 1, Value: "'Hello'"}},
		},
	})
	if err != nil {
		t.Fatalf("buildButtonV3: %v", err)
	}
	p := btn.CaptionTemplate.Parameters[0]
	if p.Expression != "'Hello'" || p.AttributeRef != "" {
		t.Errorf("param = {Expression:%q AttributeRef:%q}, want the literal 'Hello'", p.Expression, p.AttributeRef)
	}
}

// DESCRIBE's historical spelling for a button, `ContentParams:`, is honoured so
// that descriptions written before this fix still re-execute with their
// parameters.
func TestButtonCaptionParams_ContentParamsAliasIsRead(t *testing.T) {
	btn, err := buttonCaptionPB().buildButtonV3(&ast.WidgetV3{
		Type: "actionbutton", Name: "b",
		Properties: map[string]any{
			"Caption":       "Dollar {1}",
			"ContentParams": []ast.ParamAssignmentV3{{Index: 1, Value: "Title"}},
		},
	})
	if err != nil {
		t.Fatalf("buildButtonV3: %v", err)
	}
	if n := len(btn.CaptionTemplate.Parameters); n != 1 {
		t.Fatalf("caption has %d parameter(s) — `ContentParams:` on a button was dropped, leaving {1} unbound", n)
	}
}

// DESCRIBE names the property the language documents for a button.
func TestDescribeButtonEmitsCaptionParams(t *testing.T) {
	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, rawWidget{
		Type:       "Forms$ActionButton",
		Name:       "b",
		Caption:    "Bare {1}",
		Parameters: []string{"Title"},
		Action:     "save_changes",
	}, 0)
	out := buf.String()
	if !strings.Contains(out, "CaptionParams: [{1} = Title]") {
		t.Errorf("button parameters not described as CaptionParams:\n%s", out)
	}
	if strings.Contains(out, "ContentParams") {
		t.Errorf("button described with ContentParams (the dynamictext name):\n%s", out)
	}
}

// A caption placeholder with no parameter is CE0720 at build time. It is what an
// old description re-executed to, and `check` said nothing: the orphan check
// covered dynamictext only.
func TestButtonCaptionOrphanPlaceholderIsFlagged(t *testing.T) {
	for _, kw := range []string{"actionbutton", "linkbutton"} {
		orphan := &ast.WidgetV3{Type: kw, Name: "b", Properties: map[string]any{"Caption": "Save {1}"}}
		if v := validateDynamicTextPlaceholders(orphan, "page X"); v == nil {
			t.Errorf("%s: caption {1} with no parameter passed — mx check reports CE0720", kw)
		}
		for _, key := range []string{"CaptionParams", "ContentParams"} {
			bound := &ast.WidgetV3{Type: kw, Name: "b", Properties: map[string]any{
				"Caption": "Save {1}",
				key:       []ast.ParamAssignmentV3{{Index: 1, Value: "'x'"}},
			}}
			if v := validateDynamicTextPlaceholders(bound, "page X"); v != nil {
				t.Errorf("%s with %s: false positive: %s", kw, key, v.Message)
			}
		}
	}
}
