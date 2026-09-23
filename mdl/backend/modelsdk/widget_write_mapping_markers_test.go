// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#550, the typed-array markers. A describe → exec round trip of
// ako/TestApp's Administration.ChangePasswordForm changed two:
//
//	…/Action/MicroflowSettings/ParameterMappings/[0]   2 → 3
//	…/Action/MicroflowSettings/OutputMappings/[0]      3 → (key dropped)
//
// Measured across all 67 pages of that app at Mendix 11.14.0:
//
//	ParameterMappings   marker 2 on 220 of 220 lists, empty or populated, in
//	                    every parent type — MicroflowSettings, FormSettings,
//	                    SnippetCall, CallNanoflowClientAction, NanoflowSource
//	OutputMappings      present on 91 of 91 MicroflowSettings, marker 3, always
//	                    empty
//
// mxcli registered ParameterMappings through MandatoryLists, which emits the
// encoder's default 3, and never emitted OutputMappings at all.
package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// listMarker returns the typed-array marker of a list key, or -1 when the key is
// absent — the two failures here are "wrong marker" and "no key at all".
func listMarker(d bsonv1.D, key string) int32 {
	arr, ok := bsonnav.DGet(d, key).(bsonv1.A)
	if !ok || len(arr) == 0 {
		return -1
	}
	m, _ := arr[0].(int32)
	return m
}

func TestWriteMicroflowSettings_MappingListMarkers(t *testing.T) {
	d := encodeWidget(t, &pages.ActionButton{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "b"}, Name: "btnOk"},
		Action: &pages.MicroflowClientAction{
			BaseElement: model.BaseElement{ID: "a"}, MicroflowName: "M.MF",
		},
	})
	settings := bsonnav.DGetDoc(bsonnav.DGetDoc(d, "Action"), "MicroflowSettings")
	if settings == nil {
		t.Fatal("no Forms$MicroflowSettings written")
	}
	if got := listMarker(settings, "ParameterMappings"); got != 2 {
		t.Errorf("ParameterMappings marker = %d, want 2 (220 of 220 stored lists)", got)
	}
	if got := listMarker(settings, "OutputMappings"); got != 3 {
		t.Errorf("OutputMappings marker = %d, want 3 — -1 means the key was not "+
			"emitted at all, and Studio Pro writes it on 91 of 91", got)
	}
}

// The same list under a different parent. Registering the marker by child type
// alone would not cover an EMPTY list, which has no child to key on — 59 of the
// 91 MicroflowSettings lists and all 21 of these are empty.
func TestWriteCallNanoflow_ParameterMappingsMarker(t *testing.T) {
	d := encodeWidget(t, &pages.ActionButton{
		BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{ID: "b"}, Name: "btnGo"},
		Action: &pages.NanoflowClientAction{
			BaseElement: model.BaseElement{ID: "a"}, NanoflowName: "M.NF",
		},
	})
	action := bsonnav.DGetDoc(d, "Action")
	if action == nil {
		t.Fatal("no client action written")
	}
	if got := listMarker(action, "ParameterMappings"); got != 2 {
		t.Errorf("ParameterMappings marker = %d, want 2", got)
	}
}
