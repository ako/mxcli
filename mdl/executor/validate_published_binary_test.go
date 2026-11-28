// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A published OData service that exposes a Binary attribute does not build: MxBuild
// 11.16.1 reports CE5013 "Binary attribute 'Photo' cannot be exposed through an OData
// entity.", for the entity's own attribute and for one it inherits. check and exec
// accepted both. Every other attribute type (String, HashedString, Integer, Long,
// Decimal, Boolean, DateTime, AutoNumber, Enumeration) exposes clean.
func publishedBinaryScript(expose string) string {
	return `mdl 1;
create persistent entity Api.Base (Data: binary, Title: string(50));
create persistent entity Api.Child extends Api.Base (Code: string(36) unique, Photo: binary, Name: string(50));
create published odata service Api.Svc (path: 'odata/api/', version: '1.0.0', ODataVersion: OData4, namespace: 'My.Api', ServiceName: 'Api')
{
  publish entity Api.Child as 'Children' (ReadMode: source, InsertMode: not_supported, UpdateMode: not_supported, DeleteMode: not_supported)
  expose (` + expose + `);
};
`
}

func TestValidatePublishedBinary(t *testing.T) {
	cases := []struct {
		name, expose string
		want         []string // attributes reported
	}{
		{"no binary exposed (control)", "Code (KEY), Name, Title", nil},
		{"the entity's own binary attribute", "Code (KEY), Photo", []string{"Photo"}},
		{"an inherited binary attribute", "Code (KEY), Data", []string{"Data"}},
		{"both", "Code (KEY), Photo, Data", []string{"Photo", "Data"}},
		{"the name in another case", "Code (KEY), photo", []string{"photo"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ValidatePublishedBinary(mustBuild(t, publishedBinaryScript(c.expose)), nil)
			if len(got) != len(c.want) {
				t.Fatalf("want %d violations, got %v: %+v", len(c.want), rulesOf(got), got)
			}
			for i, attr := range c.want {
				v := got[i]
				if v.RuleID != publishedBinaryRule || !strings.Contains(v.Message, "CE5013") || !strings.Contains(v.Message, attr) {
					t.Errorf("violation %d: %+v", i, v)
				}
			}
		})
	}
}

// The entity lives in the project, not the script.
func TestValidatePublishedBinary_ProjectEntity(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-pj"}, Name: "Pj"}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-pj"}, ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{
			{BaseElement: model.BaseElement{ID: "e1"}, Name: "Doc", Attributes: []*domainmodel.Attribute{
				{Name: "Code", Type: &domainmodel.StringAttributeType{Length: 36}},
				{Name: "Blob", Type: &domainmodel.BinaryAttributeType{}},
			}},
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Pj" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	open := func() backend.FullBackend { return mb }
	src := func(expose string) string {
		return `mdl 1;
create published odata service Pj.Svc (path: 'odata/pj/', version: '1.0.0', ODataVersion: OData4, namespace: 'Pj', ServiceName: 'Pj')
{
  publish entity Pj.Doc as 'Docs' (ReadMode: source, InsertMode: not_supported, UpdateMode: not_supported, DeleteMode: not_supported)
  expose (` + expose + `);
};
`
	}
	if got := ValidatePublishedBinary(mustBuild(t, src("Code (KEY)")), open); len(got) != 0 {
		t.Errorf("control: want none, got %+v", got)
	}
	if got := ValidatePublishedBinary(mustBuild(t, src("Code (KEY), Blob")), open); len(got) != 1 {
		t.Errorf("a project entity's binary attribute: want one, got %+v", got)
	}
	// Unknown entity, no project: not judged.
	if got := ValidatePublishedBinary(mustBuild(t, src("Code (KEY), Blob")), nil); len(got) != 0 {
		t.Errorf("no project: want none, got %+v", got)
	}
}
