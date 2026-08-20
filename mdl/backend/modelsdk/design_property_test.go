// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// TestAppearanceDesignProperties verifies the codec emits flat and compound
// design properties (#668). Each is a Forms$DesignPropertyValue wrapper (Key +
// typed Value); compound nests a Forms$CompoundDesignPropertyValue whose
// Properties are themselves wrappers.
func TestAppearanceDesignProperties(t *testing.T) {
	dps := []pages.DesignPropertyValue{
		{Key: "Column gap", ValueType: "option", Option: "Medium"},
		{Key: "Cards style", ValueType: "toggle"},
		{Key: "Spacing", ValueType: "compound", Compound: []pages.DesignPropertyValue{
			{Key: "margin-top", ValueType: "option", Option: "Large"},
		}},
	}

	out, err := (&codec.Encoder{}).Encode(newAppearance("", "", "", dps))
	if err != nil {
		t.Fatalf("encode appearance: %v", err)
	}

	// $Type strings and string values are stored as UTF-8 in the BSON, so a
	// substring check confirms the structure was serialized.
	for _, want := range []string{
		"Forms$DesignPropertyValue", "Column gap",
		"Forms$OptionDesignPropertyValue", "Medium",
		"Cards style", "Forms$ToggleDesignPropertyValue",
		"Spacing", "Forms$CompoundDesignPropertyValue", "margin-top", "Large",
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("encoded appearance missing %q\nBSON: %x", want, out)
		}
	}
}

// TestAppearanceDynamicClasses locks in the DynamicClasses fix: the codec serializes the
// Forms$Appearance.DynamicClasses expression when the widget carries one
// (previously hardcoded to ""), so the runtime class list is not dropped.
func TestAppearanceDynamicClasses(t *testing.T) {
	expr := "if $currentObject/Name = 'Astute' then 'ss-chip--astute' else ''"
	out, err := (&codec.Encoder{}).Encode(newAppearance("ss-chip", "", expr, nil))
	if err != nil {
		t.Fatalf("encode appearance: %v", err)
	}
	if !bytes.Contains(out, []byte(expr)) {
		t.Errorf("encoded appearance missing DynamicClasses expression %q\nBSON: %x", expr, out)
	}
}

// Studio Pro writes a DesignProperties list on every Forms$Appearance, empty ones
// included (measured: 16 of 16 pages in a blank 11.13 app, and the list `mx
// update-widgets` writes back into an mxcli-authored page). The codec omits an
// empty, never-appended PartList, so mxcli's widgets carried no DesignProperties
// key at all — which is also why ALTER STYLING's design-property write was a
// silent no-op: bsonnav.DSet cannot add a key that is not there. (upstream #931)
func TestAppearanceAlwaysEmitsDesignProperties(t *testing.T) {
	out, err := (&codec.Encoder{}).Encode(newAppearance("card", "", "", nil))
	if err != nil {
		t.Fatalf("encode appearance: %v", err)
	}

	var doc bson.D
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	val, ok := lookupKey(doc, "DesignProperties")
	if !ok {
		t.Fatalf("no DesignProperties key on an appearance with none set: %v", doc)
	}
	arr, ok := val.(bson.A)
	if !ok || len(arr) != 1 || arr[0] != int32(3) {
		t.Errorf("DesignProperties = %v, want the empty typed-array marker [3]", val)
	}
}
