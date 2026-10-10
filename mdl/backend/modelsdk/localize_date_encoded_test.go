// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// encodedLocalizeDate encodes an attribute type the way a domain-model write
// does and returns the stored LocalizeDate, and whether the key is present at all.
func encodedLocalizeDate(t *testing.T, at domainmodel.AttributeType) (value, present bool) {
	t.Helper()
	contents, err := (&codec.Encoder{}).Encode(attributeTypeToGen(at))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(contents, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, e := range doc {
		if e.Key == "LocalizeDate" {
			b, ok := e.Value.(bool)
			if !ok {
				t.Fatalf("LocalizeDate is %T, want bool", e.Value)
			}
			return b, true
		}
	}
	return false, false
}

// `not localized` (#1373) reaches the stored document as LocalizeDate: false.
func TestEncodedDateTime_NotLocalizedIsFalse(t *testing.T) {
	v, ok := encodedLocalizeDate(t, &domainmodel.DateTimeAttributeType{LocalizeDate: false})
	if !ok || v {
		t.Errorf("LocalizeDate encoded = %v (present %v), want false", v, ok)
	}
	// Control: the default encodes as true, so the assertion above can fail.
	if v, ok := encodedLocalizeDate(t, &domainmodel.DateTimeAttributeType{LocalizeDate: true}); !ok || !v {
		t.Errorf("control: LocalizeDate encoded = %v (present %v), want true", v, ok)
	}
}

// The date-only DateAttributeType was written as a DateTimeAttributeType with
// no LocalizeDate key at all. An omitted key is not what Studio Pro writes, and
// Mendix's merge/diff tooling trips on it (mendixlabs/mxcli#1373). It is written
// explicitly now, as true: what an absent key has always been read as
// (storedLocalizeDate), so the stored meaning does not change.
func TestEncodedDateAttributeType_WritesLocalizeDate(t *testing.T) {
	v, ok := encodedLocalizeDate(t, &domainmodel.DateAttributeType{})
	if !ok {
		t.Fatal("DateAttributeType encoded without a LocalizeDate key")
	}
	if !v {
		t.Errorf("LocalizeDate = false, want true (what the absent key was read as)")
	}
}
