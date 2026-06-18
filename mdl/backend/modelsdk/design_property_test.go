// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"bytes"
	"testing"

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

	out, err := (&codec.Encoder{}).Encode(newAppearance("", "", dps))
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
