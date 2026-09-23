// SPDX-License-Identifier: Apache-2.0

package codec

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/modelsdk/property"
	"go.mongodb.org/mongo-driver/bson"
)

// omitProbe is a minimal element with one primitive property, plus a registered
// mandatory list — the two ways a key can reach the document.
type omitProbe struct {
	element.Base
	name *property.Primitive[string]
}

func newOmitProbe(typeName string) *omitProbe {
	o := &omitProbe{}
	o.SetTypeName(typeName)
	o.name = property.NewPrimitive[string]("Name", property.DecodeString)
	o.name.Bind(&o.Base, 0)
	o.SetProperties([]element.Property{o.name})
	o.name.Set("probe")
	o.MarkDirty(63)
	return o
}

func TestOmitKeysSuppressesPropertyAndMandatoryList(t *testing.T) {
	const typeName = "Probe$OmitKeys"
	RegisterTypeDefaults(typeName, TypeDefaults{MandatoryLists: []string{"Variables"}})

	encode := func(e *Encoder) bson.Raw {
		t.Helper()
		b, err := e.Encode(newOmitProbe(typeName))
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		return bson.Raw(b)
	}
	has := func(raw bson.Raw, key string) bool {
		_, err := raw.LookupErr(key)
		return err == nil
	}

	// The zero Encoder must behave exactly as before this field existed.
	base := encode(&Encoder{})
	if !has(base, "Name") || !has(base, "Variables") {
		t.Fatalf("zero Encoder dropped something: %v", base)
	}

	// A plain property.
	got := encode(&Encoder{OmitKeys: map[string]map[string]bool{typeName: {"Name": true}}})
	if has(got, "Name") {
		t.Error("Name emitted despite OmitKeys")
	}
	if !has(got, "Variables") {
		t.Error("Variables dropped by an OmitKeys that did not name it")
	}

	// A key that only exists because of the Studio Pro defaults registry.
	got = encode(&Encoder{OmitKeys: map[string]map[string]bool{typeName: {"Variables": true}}})
	if has(got, "Variables") {
		t.Error("mandatory list emitted despite OmitKeys")
	}
	if !has(got, "Name") {
		t.Error("Name dropped by an OmitKeys that did not name it")
	}

	// Keyed by $Type: another type's entry must not leak across.
	got = encode(&Encoder{OmitKeys: map[string]map[string]bool{"Other$Type": {"Name": true}}})
	if !has(got, "Name") {
		t.Error("another $Type's OmitKeys suppressed this one's key")
	}

	// $ID and $Type are structural and are never subject to suppression.
	got = encode(&Encoder{OmitKeys: map[string]map[string]bool{typeName: {"$Type": true, "$ID": true}}})
	if !has(got, "$Type") || !has(got, "$ID") {
		t.Error("OmitKeys removed a structural key")
	}
}
