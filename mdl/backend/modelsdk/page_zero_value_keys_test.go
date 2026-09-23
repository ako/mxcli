// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#541 (dropped-key half) — a describe → exec round trip of a Studio Pro
// page drops eight keys whose stored value is the type's zero value:
//
//	.../DataSource/SourceVariable/{LocalVariable,SnippetParameter,SubKey,Widget} = ''
//	.../DataSource/SourceVariable/UseAllPages                                    = False
//	.../AttributeRef/EntityRef                                                   = null  (×3)
//
// Studio Pro writes them; mxcli sets only the one field that carries a value, so
// the rest are never marked dirty and the encoder omits them. Measured across
// the 67 pages of ako/TestApp at 11.14.0:
//
//	DomainModels$AttributeRef with an EntityRef key: 338 of 338
//	                          (313 explicitly null, 25 an IndirectEntityRef)
//
// This is the codec's TypeDefaults registry's job — the same mechanism already
// used for an association's null Source and a visibility setting's empty-string
// Attribute — rather than a set-every-field edit at each of the three
// PageVariable construction sites.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// encodeToD runs an element through the real encoder and returns the document,
// so these assertions cover what actually reaches storage rather than what the
// gen object holds.
func encodeToD(t *testing.T, el element.Element) bson.D {
	t.Helper()
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return d
}

func hasKey(d bson.D, key string) bool {
	for _, e := range d {
		if e.Key == key {
			return true
		}
	}
	return false
}

// A plain attribute binding must still carry the EntityRef key, as null.
func TestAttributeRefToGen_EmitsNullEntityRef(t *testing.T) {
	el := attributeRefToGen("Rules.RuleAction.ActionType")
	if el == nil {
		t.Fatal("attributeRefToGen returned nil for a qualified attribute")
	}
	d := encodeToD(t, el)
	if !hasKey(d, "EntityRef") {
		t.Errorf("EntityRef key absent; Studio Pro writes it on 338 of 338 AttributeRefs")
	}
	if v := bsonnav.DGet(d, "EntityRef"); v != nil {
		t.Errorf("EntityRef = %v, want null for a plain (non-navigated) binding", v)
	}
}

// Control: a navigated binding must keep its real EntityRef, not be flattened
// to null by the default. Without this, "always write null" passes the test
// above and silently undoes ako/mxcli#529.
func TestAttributeRefWithSteps_KeepsItsEntityRef(t *testing.T) {
	el := attributeRefWithStepsToGen("Rules.BusinessRule.Name", []pages.AttributeRefStep{
		{Association: "Rules.RuleAction_BusinessRule", DestinationEntity: "Rules.BusinessRule"},
	})
	d := encodeToD(t, el)
	if v := bsonnav.DGet(d, "EntityRef"); v == nil {
		t.Error("EntityRef was flattened to null — the association hops were lost")
	}
}

// Every PageVariable key Studio Pro writes must be present, whichever one of the
// four name fields actually carries the value.
func TestPageVariable_EmitsEveryStoredKey(t *testing.T) {
	want := []string{"LocalVariable", "PageParameter", "SnippetParameter", "SubKey", "UseAllPages", "Widget"}
	for _, kind := range []string{"", "local", "snippet"} {
		name := kind
		if name == "" {
			name = "page"
		}
		t.Run(name, func(t *testing.T) {
			d := encodeToD(t, sourceVariableToGen("RuleAction", kind))
			for _, k := range want {
				if !hasKey(d, k) {
					t.Errorf("key %q absent; Studio Pro writes all six on Forms$PageVariable", k)
				}
			}
			// Control: the field the caller DID set keeps its value.
			set := map[string]string{"": "PageParameter", "local": "LocalVariable", "snippet": "SnippetParameter"}[kind]
			if got := bsonnav.DGetString(d, set); got != "RuleAction" {
				t.Errorf("%s = %q, want RuleAction — the authored value was overwritten by the default", set, got)
			}
		})
	}
}
