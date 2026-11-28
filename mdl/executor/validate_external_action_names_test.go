// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// `call external action M.Client.Action` must name an action the consumed service's
// contract has, exactly. Measured on MxBuild 11.16.1 against a contract with one
// action, DoThing: `DoThing` clean; `NotThere` and `dothing` (the right name in
// another case) CE7251 "Service 'M.Client' does not contain action 'dothing'.", with
// CE0117 on the output variable. check and exec accepted both: the builder matches
// the name case-insensitively to type the result but writes it as typed.
const externalActionContract = `<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx Version="4.0" xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx">
  <edmx:DataServices>
    <Schema Namespace="App.Model" xmlns="http://docs.oasis-open.org/odata/ns/edm">
      <Action Name="DoThing"><Parameter Name="Input" Type="Edm.String"/><ReturnType Type="Edm.String"/></Action>
      <Action Name="Other"/>
      <EntityContainer Name="Container">
        <ActionImport Name="DoThing" Action="App.Model.DoThing"/>
        <ActionImport Name="Other" Action="App.Model.Other"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>`

func externalActionBackend() func() backend.FullBackend {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-pj"}, Name: "Pj"}
	svc := &model.ConsumedODataService{ContainerID: mod.ID, Name: "Client", Metadata: externalActionContract}
	mb := &mock.MockBackend{
		IsConnectedFunc:               func() bool { return true },
		ListModulesFunc:               func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) { return []*model.ConsumedODataService{svc}, nil },
	}
	return func() backend.FullBackend { return mb }
}

func externalActionFlow(action string) string {
	return `mdl 1;
create microflow Pj.MF () returns boolean begin
  $R = call external action Pj.Client.` + action + ` (Input = 'x');
  return true;
end;
`
}

func TestValidateExternalActionNames(t *testing.T) {
	open := externalActionBackend()
	cases := []struct {
		name, action string
		want         bool
	}{
		{"an action the contract has (control)", "DoThing", false},
		{"an action it does not have", "NotThere", true},
		{"the right name in another case", "dothing", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ValidateExternalActionNames(mustBuild(t, externalActionFlow(c.action)), open)
			if !c.want {
				if len(got) != 0 {
					t.Fatalf("want none, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want one, got %v: %+v", rulesOf(got), got)
			}
			v := got[0]
			if v.RuleID != externalActionNameRule || !strings.Contains(v.Message, "CE7251") ||
				!strings.Contains(v.Message, c.action) || !strings.Contains(v.Suggestion, "DoThing") {
				t.Errorf("violation: %+v", v)
			}
		})
	}
}

// Not judged: no project, a service the project does not have, a service the
// script creates or changes (its contract is not the stored one), a service with no
// stored contract.
func TestValidateExternalActionNames_NotJudged(t *testing.T) {
	if got := ValidateExternalActionNames(mustBuild(t, externalActionFlow("NotThere")), nil); len(got) != 0 {
		t.Errorf("no project: want none, got %+v", got)
	}
	open := externalActionBackend()
	other := strings.Replace(externalActionFlow("NotThere"), "Pj.Client.", "Pj.Elsewhere.", 1)
	if got := ValidateExternalActionNames(mustBuild(t, other), open); len(got) != 0 {
		t.Errorf("unknown service: want none, got %+v", got)
	}
	recreated := `mdl 1;
create or modify consumed odata service Pj.Client (ODataVersion: OData4, MetadataUrl: './x.xml');
` + strings.TrimPrefix(externalActionFlow("NotThere"), "mdl 1;\n")
	if got := ValidateExternalActionNames(mustBuild(t, recreated), open); len(got) != 0 {
		t.Errorf("a service the script rewrites: want none, got %+v", got)
	}
}

// A nanoflow cannot call an external action, so only microflows are walked; the
// call inside a branch is found.
func TestValidateExternalActionNames_Nested(t *testing.T) {
	src := `mdl 1;
create microflow Pj.MF ($B: boolean) returns boolean begin
  if $B then
    $R = call external action Pj.Client.NotThere (Input = 'x');
  end if;
  return true;
end;
`
	if got := ValidateExternalActionNames(mustBuild(t, src), externalActionBackend()); len(got) != 1 {
		t.Errorf("a call inside an if: want one, got %+v", got)
	}
}
