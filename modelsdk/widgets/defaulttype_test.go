// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// A widget property may declare `defaultType` beside `defaultValue` — the KIND of
// the default, e.g. an action defaulting to a nanoflow call, or a datasource
// defaulting to an association. It is part of the widget DEFINITION, so storing the
// wrong one is CE0463, the same class as the stale `onChange` of #716.
//
// File Uploader 2.5.0 is the widget that has them: six properties, and it is the
// widget upstream #956 is about. Measured on 11.13.0 — mxcli wrote `DefaultType:
// "None"` on all six, `mx check` reported CE0463 on every page carrying the widget,
// and `mx update-widgets` (Studio Pro's own normalizer) rewrote exactly those six
// values and nothing else: a 2,650-path document diff whose only differences were
// these.
func TestValueTypeCarriesDefaultTypeFromTheWidgetXML(t *testing.T) {
	cases := []struct {
		name, xmlType, defaultType, want string
	}{
		{"action defaulting to a nanoflow", "action", "CallNanoflow", "CallNanoflow"},
		{"datasource defaulting to an association", "datasource", "Association", "Association"},
		{"action defaulting to a microflow", "action", "CallMicroflow", "CallMicroflow"},
		{"no defaultType declared stays None", "action", "", "None"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := mpk.PropertyDef{
				Key:         "createFileAction",
				Type:        c.xmlType,
				DefaultType: c.defaultType,
			}
			vt := createDefaultValueType("vt-1", "Action", p)
			if got := vt["DefaultType"]; got != c.want {
				t.Errorf("DefaultType = %v, want %v — a widget Type that disagrees with "+
					"the package definition is CE0463, and mx check is the only thing "+
					"that says so", got, c.want)
			}
		})
	}
}
