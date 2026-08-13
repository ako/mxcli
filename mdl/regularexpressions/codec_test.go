// SPDX-License-Identifier: Apache-2.0

package regularexpressions

import (
	"encoding/hex"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// Five complete, unedited regular-expression documents lifted out of Mendix's
// own marketplace modules — the only Studio Pro-authored references available
// without Studio Pro. Between them they cover documented and undocumented,
// patterns containing quotes, backslashes and lookbehinds.
//
//	emailConnHex  Mendix Email Connector 6.4.2  RegEx_CallbackOperationPath
//	identifierHex Community Commons 11.5.1      Identifier
//	emailAddrHex  Community Commons 11.5.1      EmailAddressRegex
//	guidOrEmptyHex Community Commons 11.5.1     GUIDOrEmpty
//	guidHex       Community Commons 11.5.1      GUIDRegex
const (
	emailConnHex   = "c600000005244944001000000000e0f7973d0e93bd4987caa7c1c6107a840224547970650025000000526567756c617245787072657373696f6e7324526567756c617245787072657373696f6e0002446f63756d656e746174696f6e000100000000084578636c756465640000024578706f72744c6576656c000700000048696464656e000245787072657373696f6e000a0000002e2a283f3c212f292400024e616d65001c00000052656745785f43616c6c6261636b4f7065726174696f6e506174680000"
	identifierHex  = "1b01000005244944001000000000c683e3d0ab2ac14fb1bfde29caf23c690224547970650025000000526567756c617245787072657373696f6e7324526567756c617245787072657373696f6e0002446f63756d656e746174696f6e0057000000537570706f72747320616c7068616e756d65726963206368617261637465727320616e6420756e64657273636f72652c206973206e6f7420616c6c6f77656420746f20737461727420776974682061206e756d62657200084578636c756465640000024578706f72744c6576656c000700000048696464656e000245787072657373696f6e001a0000005e5b612d7a412d5a5f5d2b5b612d7a412d5a302d395f5d2a2400024e616d65000b0000004964656e7469666965720000"
	emailAddrHex   = "180100000524494400100000000085c5d14182b4f445a5283969127c21b50224547970650025000000526567756c617245787072657373696f6e7324526567756c617245787072657373696f6e0002446f63756d656e746174696f6e0039000000412c206e6f7420746f6f2072657374726963746976652c20656d61696c206164647265737320726567756c61722065787072657373696f6e00084578636c756465640000024578706f72744c6576656c000700000048696464656e000245787072657373696f6e002e0000005c772b28282d7c5c2b7c5c2e295c772b292a405c772b285b5c2e2d5d3f5c772b292a285c2e5c777b322c7d292b00024e616d650012000000456d61696c4164647265737352656765780000"
	guidOrEmptyHex = "f3000000052449440010000000005356fa3203219d4f976ee19a5a941fdf0224547970650025000000526567756c617245787072657373696f6e7324526567756c617245787072657373696f6e0002446f63756d656e746174696f6e003600000053616d65206173204755494452656765782c20627574206163636570747320656d70747920737472696e672061732077656c6c2e2000084578636c756465640000024578706f72744c6576656c000700000048696464656e000245787072657373696f6e00120000005e5b612d7a412d5a302d395f2d5d2b7c2400024e616d65000c000000475549444f72456d7074790000"
	guidHex        = "f20000000524494400100000000003db1cf338bdf345a73a1adfca1208130224547970650025000000526567756c617245787072657373696f6e7324526567756c617245787072657373696f6e0002446f63756d656e746174696f6e0038000000537570706f72747320616c7068616e756d6572696320636861726163746572732c206461736820616e6420756e64657273636f72652e2000084578636c756465640000024578706f72744c6576656c000700000048696464656e000245787072657373696f6e00110000005e5b612d7a412d5a302d395f2d5d2b2400024e616d65000a0000004755494452656765780000"
)

var references = []struct {
	name, hexDoc, wantName, wantPattern string
}{
	{"EmailConnector", emailConnHex, "RegEx_CallbackOperationPath", `.*(?<!/)$`},
	{"Identifier", identifierHex, "Identifier", `^[a-zA-Z_]+[a-zA-Z0-9_]*$`},
	{"EmailAddress", emailAddrHex, "EmailAddressRegex", `\w+((-|\+|\.)\w+)*@\w+([\.-]?\w+)*(\.\w{2,})+`},
	{"GUIDOrEmpty", guidOrEmptyHex, "GUIDOrEmpty", `^[a-zA-Z0-9_-]+|$`},
	{"GUID", guidHex, "GUIDRegex", `^[a-zA-Z0-9_-]+$`},
}

func mustDecode(t *testing.T, h string) bson.M {
	t.Helper()
	raw, err := hex.DecodeString(h)
	if err != nil {
		t.Fatalf("hex: %v", err)
	}
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

// TestParseStudioProDocuments is the reason PatternKey exists: modelsdk/gen
// binds the pattern to "RegEx", but every real document stores "Expression". A
// reader keyed on gen's name returns an empty pattern for all five.
func TestParseStudioProDocuments(t *testing.T) {
	for _, tt := range references {
		t.Run(tt.name, func(t *testing.T) {
			doc := mustDecode(t, tt.hexDoc)
			if _, present := doc["RegEx"]; present {
				t.Error("this document has a RegEx key after all — the codec's premise needs rechecking")
			}
			re := Parse(doc, "id-1", "mod-1")
			if re.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", re.Name, tt.wantName)
			}
			if re.Expression != tt.wantPattern {
				t.Errorf("Expression = %q, want %q", re.Expression, tt.wantPattern)
			}
			if re.ExportLevel != "Hidden" {
				t.Errorf("ExportLevel = %q, want Hidden", re.ExportLevel)
			}
		})
	}
}

// TestSerializeMatchesStudioProDocuments is the shape pin: read each real
// document, write it back, and require the result to be identical apart from
// the regenerated $ID. Compares raw elements, so key ORDER, key SET and BSON
// TYPE are covered as well as values.
func TestSerializeMatchesStudioProDocuments(t *testing.T) {
	for _, tt := range references {
		t.Run(tt.name, func(t *testing.T) {
			originalBytes, err := hex.DecodeString(tt.hexDoc)
			if err != nil {
				t.Fatalf("hex: %v", err)
			}
			re := Parse(mustDecode(t, tt.hexDoc), "11111111-1111-1111-1111-111111111111", "mod-1")

			out, err := Serialize(re)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			assertSameExceptIDs(t, bson.Raw(originalBytes), bson.Raw(out))
		})
	}
}

func assertSameExceptIDs(t *testing.T, want, got bson.Raw) {
	t.Helper()
	wantEls, err := want.Elements()
	if err != nil {
		t.Fatalf("elements: %v", err)
	}
	gotEls, err := got.Elements()
	if err != nil {
		t.Fatalf("elements: %v", err)
	}
	if len(wantEls) != len(gotEls) {
		t.Fatalf("wrote %d properties, Studio Pro wrote %d\n  want %v\n  got  %v",
			len(gotEls), len(wantEls), keysOf(wantEls), keysOf(gotEls))
	}
	for i, we := range wantEls {
		ge := gotEls[i]
		if we.Key() != ge.Key() {
			t.Errorf("property %d is %q, want %q (order differs)", i, ge.Key(), we.Key())
			continue
		}
		wv, gv := we.Value(), ge.Value()
		if wv.Type != gv.Type {
			t.Errorf("%s: BSON type %s, want %s", we.Key(), gv.Type, wv.Type)
			continue
		}
		if we.Key() == "$ID" {
			continue
		}
		if !wv.Equal(gv) {
			t.Errorf("%s = %v, want %v", we.Key(), gv, wv)
		}
	}
}

func keysOf(els []bson.RawElement) []string {
	out := make([]string, len(els))
	for i, e := range els {
		out[i] = e.Key()
	}
	return out
}

func TestSerialize_Defaults(t *testing.T) {
	re := &model.RegularExpression{Name: "Plain", Expression: `^\d+$`}
	re.ID = "22222222-2222-2222-2222-222222222222"

	out, err := Serialize(re)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	var doc bson.M
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["ExportLevel"] != "Hidden" {
		t.Errorf("ExportLevel = %v, want Hidden", doc["ExportLevel"])
	}
	if doc[PatternKey] != `^\d+$` {
		t.Errorf("%s = %v", PatternKey, doc[PatternKey])
	}
	if _, present := doc["RegEx"]; present {
		t.Error("wrote a RegEx key — that is gen's name, not the stored one")
	}
}
