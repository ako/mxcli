// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// getFormSettings extracts FormSettings from a serialized Forms$FormAction document.
func getFormSettings(t *testing.T, doc bson.D) bson.D {
	t.Helper()
	for _, e := range doc {
		if e.Key == "FormSettings" {
			fs, ok := e.Value.(bson.D)
			if !ok {
				t.Fatalf("FormSettings is not bson.D, got %T", e.Value)
			}
			return fs
		}
	}
	t.Fatal("FormSettings not found")
	return nil
}

// getParamMappings extracts ParameterMappings from a FormSettings document.
func getParamMappings(t *testing.T, formSettings bson.D) primitive.A {
	t.Helper()
	for _, e := range formSettings {
		if e.Key == "ParameterMappings" {
			arr, ok := e.Value.(primitive.A)
			if !ok {
				t.Fatalf("ParameterMappings is not primitive.A, got %T", e.Value)
			}
			return arr
		}
	}
	t.Fatal("ParameterMappings not found")
	return nil
}

// TestPageClientAction_ParameterMappings_TypeIndicator verifies that
// Forms$FormAction always serializes ParameterMappings as [2] (type indicator
// only, no inline mapping objects), matching Studio Pro's native format.
//
// Studio Pro infers $currentObject from the enclosing widget context at runtime
// rather than reading explicit Forms$PageParameterMapping objects from BSON.
// Using int32(len) as the array's first element produces an invalid type
// indicator that Studio Pro cannot read, causing CE0115 (issue #296).
func TestPageClientAction_ParameterMappings_TypeIndicator(t *testing.T) {
	action := &pages.PageClientAction{
		BaseElement: model.BaseElement{ID: "action-id"},
		PageName:    "AuditTrail.Log_View",
		ParameterMappings: []*pages.PageClientParameterMapping{
			{
				BaseElement:   model.BaseElement{ID: "mapping-id"},
				ParameterName: "Log",
				Variable:      "$currentObject",
			},
		},
	}

	doc := serializeClientAction(action)
	if doc == nil {
		t.Fatal("serializeClientAction returned nil")
	}

	formSettings := getFormSettings(t, doc)
	mappings := getParamMappings(t, formSettings)

	// Must be exactly [int32(2)] — type indicator only, no inline objects.
	// Studio Pro's reader skips the type indicator (2 or 3) and reads the rest
	// as items; any other first-element value is treated as invalid.
	if len(mappings) != 1 {
		t.Fatalf("ParameterMappings: want exactly 1 element (type indicator), got %d", len(mappings))
	}
	indicator, ok := mappings[0].(int32)
	if !ok {
		t.Fatalf("ParameterMappings[0] is not int32, got %T", mappings[0])
	}
	if indicator != 2 {
		t.Errorf("ParameterMappings type indicator = %d, want 2", indicator)
	}
}

// TestPageClientAction_NoParams_TypeIndicator verifies that a PageClientAction
// without parameter mappings still serializes ParameterMappings as [2].
func TestPageClientAction_NoParams_TypeIndicator(t *testing.T) {
	action := &pages.PageClientAction{
		BaseElement: model.BaseElement{ID: "action-id"},
		PageName:    "Sales.Customer_Overview",
	}

	doc := serializeClientAction(action)
	if doc == nil {
		t.Fatal("serializeClientAction returned nil")
	}

	var bsonType string
	for _, e := range doc {
		if e.Key == "$Type" {
			bsonType, _ = e.Value.(string)
		}
	}
	if bsonType != "Forms$FormAction" {
		t.Errorf("$Type = %q, want %q", bsonType, "Forms$FormAction")
	}

	formSettings := getFormSettings(t, doc)
	mappings := getParamMappings(t, formSettings)
	if len(mappings) != 1 {
		t.Fatalf("ParameterMappings: want [2], got %v", mappings)
	}
}

// TestPageClientAction_RequiredFields verifies that Forms$FormAction includes
// all fields required by Studio Pro: NumberOfPagesToClose2, PagesForSpecializations,
// and FormSettings.TitleOverride.
func TestPageClientAction_RequiredFields(t *testing.T) {
	action := &pages.PageClientAction{
		BaseElement: model.BaseElement{ID: "action-id"},
		PageName:    "Sales.Order_Detail",
		ParameterMappings: []*pages.PageClientParameterMapping{
			{ParameterName: "Order", Variable: "$Order"},
			{ParameterName: "Customer", Variable: "$Customer"},
		},
	}

	doc := serializeClientAction(action)

	fields := map[string]bool{}
	for _, e := range doc {
		fields[e.Key] = true
	}
	for _, required := range []string{"NumberOfPagesToClose2", "PagesForSpecializations"} {
		if !fields[required] {
			t.Errorf("Forms$FormAction missing required field %q", required)
		}
	}

	formSettings := getFormSettings(t, doc)
	fsFields := map[string]bool{}
	for _, e := range formSettings {
		fsFields[e.Key] = true
	}
	if !fsFields["TitleOverride"] {
		t.Errorf("FormSettings missing required field %q", "TitleOverride")
	}

	// TitleOverride must be null. A button opening a page has no MDL syntax for
	// overriding the opened page's title, so the page always keeps its own.
	//
	// This assertion was previously inverted, on the reasoning that Studio Pro rejects
	// null embedded objects ("same class of bug as issue #295"). #295 was about
	// Forms$PageVariable; the conclusion was generalised to TitleOverride without being
	// observed. An empty Microflows$TextTemplate is not "no override" — it overrides
	// with the empty string, so every popup opened by an mxcli-authored button showed a
	// blank caption and only the close button (#812).
	found := false
	for _, e := range formSettings {
		if e.Key != "TitleOverride" {
			continue
		}
		found = true
		if e.Value != nil {
			t.Fatalf("TitleOverride = %#v, want nil (#812)", e.Value)
		}
	}
	if !found {
		t.Error("TitleOverride key missing entirely; Studio Pro writes it as an explicit null")
	}
}

// CapTrackV2 FINDINGS §10 — `ACTIONBUTTON … (Action: SIGN_OUT)` was refused by
// the default modelsdk engine with "client action *pages.SignOutClientAction
// not yet supported … rerun with MXCLI_ENGINE=legacy".
//
// That advice was the more dangerous of the two paths. The legacy writer had no
// case for the action either, so it fell through to the default below and wrote
// Forms$NoAction: the button rendered, said "Sign out", and did nothing, with
// `mxcli check`, `exec` and `mx check` all clean. Measured on Mendix 11.13 —
// `describe page` came back `actionbutton btnOut (Caption: 'Sign out')`, no
// action at all, and the stored BSON held Forms$NoAction.
//
// The shape is pinned against a Studio Pro-authored sign-out button
// (ako/TestApp), which is provably Studio Pro's rather than mxcli's: until this
// change NEITHER engine could emit the type.
func TestSignOutClientAction_IsNotSilentlyDroppedToNoAction(t *testing.T) {
	doc := serializeClientAction(&pages.SignOutClientAction{
		BaseElement: model.BaseElement{ID: "action-id"},
	})
	if doc == nil {
		t.Fatal("serializeClientAction returned nil")
	}

	got := map[string]any{}
	for _, e := range doc {
		got[e.Key] = e.Value
	}

	if got["$Type"] == "Forms$NoAction" {
		t.Fatal("SIGN_OUT was written as Forms$NoAction — the button renders and does nothing, " +
			"which check, exec and mx check all report as fine")
	}
	if got["$Type"] != "Forms$SignOutClientAction" {
		t.Errorf("$Type = %v, want Forms$SignOutClientAction", got["$Type"])
	}
	if got["DisabledDuringExecution"] != true {
		t.Errorf("DisabledDuringExecution = %v, want true (the Studio Pro reference's only property)",
			got["DisabledDuringExecution"])
	}
	// The reference carries exactly these three keys and no more. An extra
	// property is what Studio Pro refuses to open even when mxbuild accepts it.
	if len(doc) != 3 {
		t.Errorf("the action has %d keys, want 3 ($ID, $Type, DisabledDuringExecution): %v", len(doc), doc)
	}
}

// CONTROL: the quiet default still exists and still yields Forms$NoAction, so
// the tests here prove something about the actions they name rather than about
// the fallback having been removed.
//
// ShowHomePage is the stand-in: no MDL statement builds one, so it is a
// semantic type nothing writes. (An earlier draft used LinkClientAction, which
// stopped being a valid control the moment OPEN_LINK was implemented — a
// control has to name something still genuinely unhandled.)
func TestUnhandledClientActionStillFallsBackToNoAction(t *testing.T) {
	doc := serializeClientAction(&pages.ShowHomePageClientAction{
		BaseElement: model.BaseElement{ID: "home-id"},
	})
	var typeName string
	for _, e := range doc {
		if e.Key == "$Type" {
			typeName, _ = e.Value.(string)
		}
	}
	if typeName != "Forms$NoAction" {
		t.Errorf("$Type = %q; this control pins the fallback SIGN_OUT and OPEN_LINK used to hit, "+
			"so the tests above cannot pass for the wrong reason", typeName)
	}
}

// OPEN_LINK on the legacy engine, which fell to that same NoAction default.
// Pinned against the 31 Studio Pro references: five keys, and the address a
// nested Forms$StaticOrDynamicString whose AttributeRef is null for the static
// form MDL authors.
func TestOpenLinkClientAction_IsNotSilentlyDroppedToNoAction(t *testing.T) {
	doc := serializeClientAction(&pages.LinkClientAction{
		BaseElement: model.BaseElement{ID: "link-id"},
		LinkType:    pages.LinkTypeWeb,
		Address:     "https://example.com",
	})
	got := map[string]any{}
	for _, e := range doc {
		got[e.Key] = e.Value
	}
	if got["$Type"] != "Forms$OpenLinkClientAction" {
		t.Fatalf("$Type = %v, want Forms$OpenLinkClientAction (NOT Forms$LinkClientAction, "+
			"which is the SDK name and not what Mendix stores)", got["$Type"])
	}
	if got["LinkType"] != "Web" {
		t.Errorf("LinkType = %v, want Web", got["LinkType"])
	}
	if len(doc) != 5 {
		t.Errorf("the action has %d keys, want 5: %v", len(doc), doc)
	}
	addr, ok := got["Address"].(bson.D)
	if !ok {
		t.Fatalf("Address is %T, want a nested document", got["Address"])
	}
	a := map[string]any{}
	for _, e := range addr {
		a[e.Key] = e.Value
	}
	if a["$Type"] != "Forms$StaticOrDynamicString" || a["IsDynamic"] != false ||
		a["Value"] != "https://example.com" {
		t.Errorf("Address = %v", addr)
	}
	if _, present := a["AttributeRef"]; !present {
		t.Error("AttributeRef is absent; all 31 references carry it as null")
	}
}
