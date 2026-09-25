// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// Studio Pro lets a type parameter take any name, including a primitive's. The
// catalog wrote a type-parameter reference as its bare name, so an action whose
// type parameter is called `String` and returns it read back as ReturnType
// 'String' — the same value as an action returning the primitive String — and
// the column's values for type parameters were whatever the modeler had named
// them ('TypeParameter', 'TypeParEntity', 'FileTypeDocument', …) with nothing to
// say they were type parameters at all (mendixlabs/mxcli#1183).
func TestJavaActionTypeParameterNamedAfterPrimitive(t *testing.T) {
	const modID = model.ID("mod-util")

	tpDef := &javaactions.TypeParameterDef{BaseElement: model.BaseElement{ID: "tp-string"}, Name: "String"}
	generic := &javaactions.JavaAction{
		BaseElement:    model.BaseElement{ID: "ja-generic"},
		ContainerID:    modID,
		Name:           "ReturnsTypeParam",
		TypeParameters: []*javaactions.TypeParameterDef{tpDef},
		ReturnType:     &javaactions.TypeParameter{TypeParameterID: "tp-string", TypeParameter: "String"},
		Parameters: []*javaactions.JavaActionParameter{
			{
				BaseElement:   model.BaseElement{ID: "p-selector"},
				Name:          "EntityType",
				ParameterType: &javaactions.EntityTypeParameterType{TypeParameterID: "tp-string", TypeParameterName: "String"},
			},
			{
				BaseElement:   model.BaseElement{ID: "p-object"},
				Name:          "Input",
				ParameterType: &javaactions.TypeParameter{TypeParameterID: "tp-string", TypeParameter: "String"},
			},
		},
	}
	primitive := &javaactions.JavaAction{
		BaseElement: model.BaseElement{ID: "ja-primitive"},
		ContainerID: modID,
		Name:        "ReturnsString",
		ReturnType:  &javaactions.StringType{},
		Parameters: []*javaactions.JavaActionParameter{
			{
				BaseElement:   model.BaseElement{ID: "p-text"},
				Name:          "Text",
				ParameterType: &javaactions.StringType{},
			},
		},
	}

	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b := &Builder{
		catalog: cat,
		reader: &mock.MockBackend{
			ListJavaActionsFullFunc: func() ([]*javaactions.JavaAction, error) {
				return []*javaactions.JavaAction{generic, primitive}, nil
			},
		},
		snapshot: &Snapshot{ID: "snap"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{modID: true},
			moduleNames:     map[model.ID]string{modID: "Util"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx: tx,
	}
	if err := b.buildJavaActions(); err != nil {
		t.Fatalf("buildJavaActions: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	returns := queryStrings(t, cat, `SELECT QualifiedName, ReturnType FROM java_actions_data`)
	if got, want := returns["Util.ReturnsString"], "String"; got != want {
		t.Errorf("primitive return = %q, want %q (control)", got, want)
	}
	if got, want := returns["Util.ReturnsTypeParam"], "TypeParameter:String"; got != want {
		t.Errorf("type-parameter return = %q, want %q -- a type parameter named "+
			"after a primitive must not read back as that primitive", got, want)
	}

	params := queryStrings(t, cat, `SELECT Name, ParameterType FROM java_action_parameters_data`)
	want := map[string]string{
		"Text":       "String",
		"Input":      "TypeParameter:String",
		"EntityType": "EntityTypeParameter:String",
	}
	for name, w := range want {
		if params[name] != w {
			t.Errorf("parameter %s type = %q, want %q", name, params[name], w)
		}
	}
}

// queryStrings runs a two-column query and indexes the second column by the first.
func queryStrings(t *testing.T, cat *Catalog, q string) map[string]string {
	t.Helper()
	res, err := cat.Query(q)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	out := map[string]string{}
	for _, row := range res.Rows {
		k, _ := row[0].(string)
		v, _ := row[1].(string)
		out[k] = v
	}
	return out
}
