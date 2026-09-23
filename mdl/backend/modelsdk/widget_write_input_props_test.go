// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#550, the writer half: the semantic model now carries a text box's
// validation and a DataView's read-only style, so the writer has to emit them
// or the round trip still loses the value at the last layer.
//
// Two measurements from ako/TestApp's 67 pages at Mendix 11.14.0 shape this:
//
//	Forms$DataView.ReadOnlyStyle   Control ×47, Text ×9   (never Inherit)
//	Forms$WidgetValidation          Expression + Message, nothing else
//
// The first is why the unset default stays "Control" rather than becoming
// "Inherit" like every other input widget — assuming the common default would
// have been wrong here.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func TestWriteTextBox_Validation(t *testing.T) {
	d := encodeWidget(t, &pages.TextBox{
		BaseWidget:           pages.BaseWidget{BaseElement: model.BaseElement{ID: "tb"}, Name: "tbSecret"},
		ValidationExpression: "length(toString($value)) > 0",
		ValidationMessage:    "Required",
	})
	v := bsonnav.DGetDoc(d, "Validation")
	if v == nil {
		t.Fatal("no Forms$WidgetValidation written at all")
	}
	if got := bsonnav.DGetString(v, "Expression"); got != "length(toString($value)) > 0" {
		t.Errorf("Validation.Expression = %q, want the authored expression", got)
	}
	if bsonnav.DGetDoc(v, "Message") == nil {
		t.Error("Validation.Message is absent; Studio Pro stores a Texts$Text there")
	}
}

// Control: a widget with no authored validation keeps the empty element Studio
// Pro stores on every input widget — the fix must not start omitting it.
func TestWriteTextBox_NoValidationKeepsEmptyElement(t *testing.T) {
	d := encodeWidget(t, &pages.TextBox{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "tb"}, Name: "tbPlain"},
	})
	v := bsonnav.DGetDoc(d, "Validation")
	if v == nil {
		t.Fatal("the empty Forms$WidgetValidation was dropped")
	}
	if got := bsonnav.DGetString(v, "Expression"); got != "" {
		t.Errorf("Validation.Expression = %q, want empty", got)
	}
}

func TestWriteTextBox_IsPasswordBox(t *testing.T) {
	d := encodeWidget(t, &pages.TextBox{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "tb"}, Name: "tbSecret"},
		IsPassword: true,
	})
	if got := bsonnav.DGet(d, "IsPasswordBox"); got != true {
		t.Errorf("IsPasswordBox = %v, want true", got)
	}
}

func TestWriteDataView_ReadOnlyStyle(t *testing.T) {
	for _, tc := range []struct{ authored, want string }{
		// Unset keeps Control: measured 47 of 56 stored DataViews, and not one
		// carries Inherit, so the default the other input widgets use is wrong here.
		{"", "Control"},
		{"Text", "Text"},
		{"Control", "Control"},
		{"Inherit", "Inherit"},
	} {
		name := tc.authored
		if name == "" {
			name = "unset"
		}
		t.Run(name, func(t *testing.T) {
			d := encodeWidget(t, &pages.DataView{
				BaseWidget:    pages.BaseWidget{BaseElement: model.BaseElement{ID: "dv"}, Name: "dvMain"},
				ReadOnlyStyle: tc.authored,
			})
			if got := bsonnav.DGetString(d, "ReadOnlyStyle"); got != tc.want {
				t.Errorf("ReadOnlyStyle = %q, want %q", got, tc.want)
			}
		})
	}
}
