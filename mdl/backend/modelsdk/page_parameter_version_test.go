// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// Forms$PageParameter carries two properties Mendix introduced in 11.5.0:
// IsRequired and DefaultValue (mendixmodelsdk 4.115.0, PageParameter.versionInfo).
// The element itself is 9.4.0.
//
// MDL has no syntax for either — every page parameter is required and has no
// default — so below 11.5 they are not information, they are two keys the
// project's metamodel does not declare. mxbuild tolerates unknown properties;
// Studio Pro resolves every stored property against the type's property list and
// throws System.InvalidOperationException at MprProperty.cs, so a green build is
// not a safety net here (see CLAUDE.md, "Overlay Writes: Never Invent a Key").
//
// This matters because mendixlabs/mxcli#1121 lifted the (wrong) 11.0 version gate
// that had been hiding it: without this guard, opening page parameters up to 10.x
// would trade an honest refusal for a document Studio Pro cannot open.
func TestPageParameterOmits11_5KeysBelow11_5(t *testing.T) {
	has := func(t *testing.T, pv *types.ProjectVersion, key string) bool {
		t.Helper()
		g := pageParameterToGen(&pages.PageParameter{
			Name:       "Item",
			EntityName: "MyModule.Item",
			IsRequired: true,
		}, pv)
		b, err := (&codec.Encoder{}).Encode(g)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		_, err = bson.Raw(b).LookupErr(key)
		return err == nil
	}

	v := func(major, minor, patch int) *types.ProjectVersion {
		return &types.ProjectVersion{MajorVersion: major, MinorVersion: minor, PatchVersion: patch}
	}

	tests := []struct {
		name string
		pv   *types.ProjectVersion
		want bool // want IsRequired + DefaultValue emitted
	}{
		{"10.24.25 (the project in #1121)", v(10, 24, 25), false},
		{"9.4.0 (floor for the element itself)", v(9, 4, 0), false},
		// 11.0–11.4 passed the old 11.0 gate and got both keys written, so the
		// bug was live on released 11.x too. This row is the regression control.
		{"11.4.0", v(11, 4, 0), false},
		{"11.5.0 (floor for both properties)", v(11, 5, 0), true},
		{"11.13.0", v(11, 13, 0), true},
		// A backend that cannot report a version must not be guessed at. The
		// conservative choice is to omit: an absent optional property is filled
		// in on load, an unknown one is unopenable.
		{"unknown version", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"IsRequired", "DefaultValue"} {
				if got := has(t, tt.pv, key); got != tt.want {
					t.Errorf("%s emitted = %v, want %v", key, got, tt.want)
				}
			}
		})
	}
}

// Whatever the version, the parameter's own identity must survive — a guard that
// drops Name or ParameterType would pass the test above and write a broken page
// (CE5601/CE5606).
func TestPageParameterKeepsNameAndTypeAtEveryVersion(t *testing.T) {
	for _, pv := range []*types.ProjectVersion{
		{MajorVersion: 10, MinorVersion: 24, PatchVersion: 25},
		{MajorVersion: 11, MinorVersion: 13, PatchVersion: 0},
		nil,
	} {
		g := pageParameterToGen(&pages.PageParameter{
			Name:       "Item",
			EntityName: "MyModule.Item",
			IsRequired: true,
		}, pv)
		b, err := (&codec.Encoder{}).Encode(g)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		raw := bson.Raw(b)
		if name, err := raw.LookupErr("Name"); err != nil || name.StringValue() != "Item" {
			t.Errorf("pv=%v: Name = %v (err %v), want \"Item\"", pv, name, err)
		}
		pt, err := raw.LookupErr("ParameterType")
		if err != nil {
			t.Fatalf("pv=%v: ParameterType missing", pv)
		}
		ent, err := pt.Document().LookupErr("Entity")
		if err != nil || ent.StringValue() != "MyModule.Item" {
			t.Errorf("pv=%v: ParameterType.Entity = %v (err %v), want \"MyModule.Item\"", pv, ent, err)
		}
	}
}
