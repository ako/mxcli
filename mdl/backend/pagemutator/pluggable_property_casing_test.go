// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// makePluggableWidget builds a CustomWidget carrying one pluggable property
// whose template key is propKey, with an initial primitive value.
func makePluggableWidget(name, propKey, initial string) bson.D {
	typeID := primitive.Binary{Subtype: 0x04, Data: []byte{
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00,
	}}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: name},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					bson.D{
						{Key: "$ID", Value: typeID},
						{Key: "PropertyKey", Value: propKey},
					},
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "TypePointer", Value: typeID},
					{Key: "Value", Value: bson.D{
						{Key: "PrimitiveValue", Value: initial},
					}},
				},
			}},
		}},
	}
}

func pluggablePrimitive(t *testing.T, rawData bson.D, widgetName string) string {
	t.Helper()
	result := findBsonWidget(rawData, widgetName)
	if result == nil {
		t.Fatalf("widget %q not found after mutation", widgetName)
	}
	obj := bsonnav.DGetDoc(result.widget, "Object")
	props := bsonnav.DGetArrayElements(bsonnav.DGet(obj, "Properties"))
	if len(props) == 0 {
		t.Fatalf("widget %q has no pluggable properties", widgetName)
	}
	propDoc, ok := props[0].(bson.D)
	if !ok {
		t.Fatalf("widget %q property 0 is %T, want bson.D", widgetName, props[0])
	}
	return bsonnav.DGetString(bsonnav.DGetDoc(propDoc, "Value"), "PrimitiveValue")
}

// TestSetPluggableProperty_MatchesTemplateKeyRegardlessOfCase is the regression
// test for mendixlabs/mxcli#1069.
//
// A DataGrid 2's paging property is keyed `pageSize` in the widget template.
// `CREATE PAGE` resolves the author's spelling case-insensitively
// (lookupProperty in the widget engine, and WidgetV3.GetStringProp before it),
// so `PageSize: 20` on a CREATE is accepted and lands. `ALTER PAGE … SET` went
// through a separate resolver that compared the template key BYTE-FOR-BYTE, so
// the same spelling on the same widget failed with
//
//	pluggable property "PageSize" not found
//
// Measured end-to-end on a real 11.13.0 project before the fix: CREATE with
// `PageSize: 20` wrote pageSize=20; `set pageSize = 10` altered it; `set
// PageSize = 10` errored. DESCRIBE PAGE emits the capitalised `PageSize:`, so
// the tool's own round-trip output was a script it then refused to execute.
//
// The four spellings below are the four an author actually writes: the one
// DESCRIBE prints, the one the template stores, and the two flat cases.
func TestSetPluggableProperty_MatchesTemplateKeyRegardlessOfCase(t *testing.T) {
	for _, spelling := range []string{"PageSize", "pageSize", "pagesize", "PAGESIZE"} {
		t.Run(spelling, func(t *testing.T) {
			rawData := makeRawPage(makePluggableWidget("dgProducts", "pageSize", "20"))
			m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

			if err := m.SetWidgetProperty("dgProducts", spelling, 10); err != nil {
				t.Fatalf("set %s: %v", spelling, err)
			}
			if got := pluggablePrimitive(t, rawData, "dgProducts"); got != "10" {
				t.Errorf("pageSize = %q, want %q", got, "10")
			}
		})
	}
}

// TestSetPluggableProperty_UnknownPropertyStillErrors is the control for the
// test above: relaxing the comparison to case-insensitive must not turn a
// genuinely unknown property into a silent no-op. A typo has to keep failing —
// that is the only signal the author gets, since `mxcli check --references`
// does not resolve pluggable property names.
func TestSetPluggableProperty_UnknownPropertyStillErrors(t *testing.T) {
	rawData := makeRawPage(makePluggableWidget("dgProducts", "pageSize", "20"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	err := m.SetWidgetProperty("dgProducts", "PagSize", 10)
	if err == nil {
		t.Fatal("expected an error for an unknown pluggable property, got nil")
	}
	if !strings.Contains(err.Error(), "PagSize") {
		t.Errorf("error should name the property the author wrote, got: %v", err)
	}
	if got := pluggablePrimitive(t, rawData, "dgProducts"); got != "20" {
		t.Errorf("a failed set must not change the stored value: got %q, want %q", got, "20")
	}
}

// TestPluggablePropertyKeysAreUniqueIgnoringCase is the argument that matching
// case-insensitively is unambiguous rather than merely convenient.
//
// setPluggableWidgetPropertyMut searches ONE object type's PropertyTypes at a
// time. Case-insensitive matching is safe exactly when no such list holds two
// keys differing only in case. Measured across every shipped widget template
// and definition: 96 property scopes, 1208 keys, 0 collisions. This test keeps
// that true as templates are added — a colliding pair would make the resolver
// pick whichever came first in the BSON, which is the guess-instead-of-refuse
// hazard the mutator-addressing pattern warns about.
func TestPluggablePropertyKeysAreUniqueIgnoringCase(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "..", "modelsdk", "widgets"),
		filepath.Join("..", "..", "..", "sdk", "widgets"),
	}

	scopes, keys := 0, 0
	var collect func(t *testing.T, file string, node any)
	collect = func(t *testing.T, file string, node any) {
		switch v := node.(type) {
		case map[string]any:
			if pts, ok := v["PropertyTypes"].([]any); ok {
				seen := map[string]string{}
				local := 0
				for _, pt := range pts {
					ptMap, ok := pt.(map[string]any)
					if !ok {
						continue
					}
					key, ok := ptMap["PropertyKey"].(string)
					if !ok || key == "" {
						continue
					}
					local++
					if prev, dup := seen[strings.ToLower(key)]; dup {
						t.Errorf("%s: property keys %q and %q differ only in case — "+
							"case-insensitive resolution would have to guess between them",
							file, prev, key)
					}
					seen[strings.ToLower(key)] = key
				}
				if local > 0 {
					scopes++
					keys += local
				}
			}
			for _, child := range v {
				collect(t, file, child)
			}
		case []any:
			for _, child := range v {
				collect(t, file, child)
			}
		}
	}

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var doc any
			if err := json.Unmarshal(data, &doc); err != nil {
				return nil // not a widget document; the loaders skip these too
			}
			collect(t, path, doc)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	// A positive control: an empty walk would pass vacuously.
	if scopes == 0 || keys == 0 {
		t.Fatalf("found no property scopes to check (scopes=%d keys=%d) — "+
			"the widget templates moved and this test is now vacuous", scopes, keys)
	}
	t.Logf("checked %d property scopes, %d keys", scopes, keys)
}
