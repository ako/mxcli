// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"sort"
	"strings"
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The widgets that used to send a user to MXCLI_ENGINE=legacy, pinned to the
// shape Mendix actually stores.
//
// studioProStaticImageKeys is measured, not derived: the three
// Forms$StaticImageViewer widgets ako/TestApp inherits from FeedbackModule are
// Studio Pro's own, and all three carry exactly these keys. The others come from
// generated/metamodel, which CLAUDE.md names the arbiter — for each type it is
// every property the metamodel declares without `omitempty`, plus the optional
// ones mxcli sets.
//
// The same expectations are asserted against the legacy writer in
// sdk/mpr/writer_widgets_display_test.go, so the two engines cannot drift apart
// silently. They already had: legacy omitted AlternativeText here and wrote a
// FallbackValue key that Forms$ClientTemplate does not have.
var (
	studioProStaticImageKeys = []string{
		"$ID", "$Type", "AlternativeText", "Appearance", "ClickAction",
		"ConditionalVisibilitySettings", "Height", "HeightUnit", "Image", "Name",
		"NativeAccessibilitySettings", "Responsive", "TabIndex", "Width", "WidthUnit",
	}
	dynamicImageKeys = []string{
		"$ID", "$Type", "AlternativeText", "Appearance", "ClickAction",
		"ConditionalVisibilitySettings", "DataSource", "DefaultImage", "Height",
		"HeightUnit", "Name", "NativeAccessibilitySettings", "OnClickEnlarge",
		"Responsive", "ShowAsThumbnail", "TabIndex", "Width", "WidthUnit",
	}
	dropDownKeys = []string{
		"$ID", "$Type", "Appearance", "AriaRequired", "AttributeRef",
		"ConditionalEditabilitySettings", "ConditionalVisibilitySettings", "Editable",
		"EmptyOptionCaption", "LabelTemplate", "Name", "NativeAccessibilitySettings",
		"OnChangeAction", "OnEnterAction", "OnLeaveAction", "ReadOnlyStyle",
		"ScreenReaderLabel", "SourceVariable", "TabIndex", "Validation",
	}
)

// encodeElement is encodeWidget's sibling for a sub-element built directly,
// without going through the widget dispatch.
func encodeElement(t *testing.T, el element.Element) bsonv1.D {
	t.Helper()
	out, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var doc bsonv1.D
	if err := bsonv1.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

func docKeys(doc bsonv1.D) []string {
	out := make([]string, 0, len(doc))
	for _, e := range doc {
		out = append(out, e.Key)
	}
	sort.Strings(out)
	return out
}

func assertKeys(t *testing.T, doc bsonv1.D, want []string) {
	t.Helper()
	got := docKeys(doc)
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if strings.Join(got, ",") != strings.Join(sorted, ",") {
		t.Errorf("keys\n got  %v\n want %v", got, sorted)
	}
}

func TestStaticImageMatchesStudioProShape(t *testing.T) {
	img := &pages.StaticImage{Responsive: true}
	img.Name = "i1"
	doc := encodeWidget(t, img)

	if got := docGet(doc, "$Type"); got != "Forms$StaticImageViewer" {
		t.Fatalf("$Type = %v", got)
	}
	assertKeys(t, doc, studioProStaticImageKeys)
	// An unset by-name reference is "", never null. Measured across ako/TestApp:
	// 0 nulls against 4,400+ empty strings over 40 (type, property) pairs.
	if got := docGet(doc, "Image"); got != "" {
		t.Errorf("Image = %#v, want the empty string", got)
	}
	assertEmptyClientTemplate(t, doc, "AlternativeText")
}

func TestDynamicImageMatchesMetamodelShape(t *testing.T) {
	img := &pages.DynamicImage{Responsive: true}
	img.Name = "i2"
	doc := encodeWidget(t, img)

	if got := docGet(doc, "$Type"); got != "Forms$ImageViewer" {
		t.Fatalf("$Type = %v", got)
	}
	assertKeys(t, doc, dynamicImageKeys)
	if got := docGet(doc, "DefaultImage"); got != "" {
		t.Errorf("DefaultImage = %#v, want the empty string", got)
	}
	assertEmptyClientTemplate(t, doc, "AlternativeText")

	src, ok := docGet(doc, "DataSource").(bsonv1.D)
	if !ok {
		t.Fatalf("DataSource = %T, want a Forms$ImageViewerSource", docGet(doc, "DataSource"))
	}
	if got := docGet(src, "$Type"); got != "Forms$ImageViewerSource" {
		t.Errorf("DataSource.$Type = %v", got)
	}
}

func TestDropDownMatchesMetamodelShape(t *testing.T) {
	dd := &pages.DropDown{}
	dd.Name = "d1"
	doc := encodeWidget(t, dd)

	if got := docGet(doc, "$Type"); got != "Forms$DropDown" {
		t.Fatalf("$Type = %v", got)
	}
	assertKeys(t, doc, dropDownKeys)
	// EmptyOptionCaption is the blank option's caption — a holder, not a null.
	cap, ok := docGet(doc, "EmptyOptionCaption").(bsonv1.D)
	if !ok {
		t.Fatalf("EmptyOptionCaption = %T, want a Texts$Text", docGet(doc, "EmptyOptionCaption"))
	}
	if got := docGet(cap, "$Type"); got != "Texts$Text" {
		t.Errorf("EmptyOptionCaption.$Type = %v", got)
	}
}

// assertEmptyClientTemplate pins the AlternativeText holder to the three Studio
// Pro references: Fallback and Template are empty Texts$Text, Parameters is an
// empty list under marker 2, and there is NO FallbackValue — that key does not
// exist on Forms$ClientTemplate (generated/metamodel: Fallback / Parameters /
// Template), and an invented key is what Studio Pro reports as "Sequence
// contains no matching element" while mxbuild builds it at 0 errors.
func assertEmptyClientTemplate(t *testing.T, parent bsonv1.D, key string) {
	t.Helper()
	ct, ok := docGet(parent, key).(bsonv1.D)
	if !ok {
		t.Fatalf("%s = %T, want a Forms$ClientTemplate", key, docGet(parent, key))
	}
	if got := docGet(ct, "$Type"); got != "Forms$ClientTemplate" {
		t.Errorf("%s.$Type = %v", key, got)
	}
	if docGet(ct, "FallbackValue") != nil {
		t.Errorf("%s carries a FallbackValue; Forms$ClientTemplate has no such property", key)
	}
	for _, sub := range []string{"Fallback", "Template"} {
		txt, ok := docGet(ct, sub).(bsonv1.D)
		if !ok {
			t.Errorf("%s.%s = %T, want a Texts$Text", key, sub, docGet(ct, sub))
			continue
		}
		if got := docGet(txt, "$Type"); got != "Texts$Text" {
			t.Errorf("%s.%s.$Type = %v", key, sub, got)
		}
		// Marker 3 and no translations. Measured: every one of the 3,257
		// Texts$Text lists in ako/TestApp uses marker 3, empty or not — legacy
		// writes 2 for a non-empty one, which is the other way this drifted.
		items, ok := docGet(txt, "Items").(bsonv1.A)
		if !ok || len(items) != 1 || items[0] != int32(3) {
			t.Errorf("%s.%s.Items = %#v, want [3]", key, sub, docGet(txt, "Items"))
		}
	}
	params, ok := docGet(ct, "Parameters").(bsonv1.A)
	if !ok || len(params) != 1 || params[0] != int32(2) {
		t.Errorf("%s.Parameters = %#v, want [2]", key, docGet(ct, "Parameters"))
	}
}

// TestNanoflowSourceNestsSettings — gen binds the nanoflow name directly on the
// source; Studio Pro nests it in a Forms$NanoflowSettings child. Writing gen's
// shape would put the name in a key Studio Pro does not read there.
func TestNanoflowSourceNestsSettings(t *testing.T) {
	el := nanoflowSourceToGen(&pages.NanoflowSource{Nanoflow: "MyModule.NF_GetItems"})
	doc := encodeElement(t, el)

	if got := docGet(doc, "$Type"); got != "Forms$NanoflowSource" {
		t.Fatalf("$Type = %v", got)
	}
	if docGet(doc, "Nanoflow") != nil {
		t.Error("Nanoflow bound directly on the source; it belongs in NanoflowSettings")
	}
	settings, ok := docGet(doc, "NanoflowSettings").(bsonv1.D)
	if !ok {
		t.Fatalf("NanoflowSettings = %T", docGet(doc, "NanoflowSettings"))
	}
	if got := docGet(settings, "Nanoflow"); got != "MyModule.NF_GetItems" {
		t.Errorf("NanoflowSettings.Nanoflow = %v", got)
	}
}
