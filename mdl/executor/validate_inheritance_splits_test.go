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

// A `split type $V` must have a branch for $V's own entity, may only branch on that
// entity or a specialization of it, and may name each once. Measured on MxBuild
// 11.16.1, one project per case, Animal <- Dog <- Puppy and Animal <- Cat:
//
//	every type + (empty)                              clean
//	a subtype left out (Cat, or Puppy)                clean   (only the base is required)
//	the subject's entity left out                     CE0090 "The 'M.Animal' value should be configured for an outgoing flow."
//	a branch on an unrelated entity                   CE0774 "Value is not entity 'M.Animal' or a specialization."
//	a branch on the subject's generalization          CE0774  (subject $D: Dog, branch Animal)
//	a branch on a sibling (subject Dog, branch Cat)   CE0774
//	the same entity twice                             CE0771 "Value is already handled by another sequence flow."
//	no (empty) branch                                 clean   (the builder always adds it)
//
// check and exec accepted every rejected case. CE0089, the code this was started
// from ("The '(empty)' value should be configured"), cannot be produced by MDL: the
// builder emits the (empty) flow unconditionally.
const splitTypeEntities = `mdl 1;
create persistent entity Zoo.Animal (Name: string(50));
create persistent entity Zoo.Dog extends Zoo.Animal (Bark: string(50));
create persistent entity Zoo.Cat extends Zoo.Animal (Meow: string(50));
create persistent entity Zoo.Puppy extends Zoo.Dog (Small: boolean);
create persistent entity Zoo.Other (X: string(5));
`

func splitTypeFlow(subject string, branches ...string) string {
	var b strings.Builder
	b.WriteString(splitTypeEntities)
	b.WriteString("create microflow Zoo.MF ($A: Zoo." + subject + ") returns boolean begin\n  split type $A\n")
	for _, e := range branches {
		b.WriteString("    when Zoo." + e + " then\n      log info node 'n' 'x';\n")
	}
	b.WriteString("    when (empty) then\n      log info node 'n' 'e';\n  end split;\n  return true;\nend;\n")
	return b.String()
}

func TestValidateInheritanceSplits(t *testing.T) {
	cases := []struct {
		name, subject string
		branches      []string
		want          string // CE code, "" for clean
	}{
		{"every type (control)", "Animal", []string{"Dog", "Cat", "Puppy", "Animal"}, ""},
		{"a subtype left out", "Animal", []string{"Dog", "Animal"}, ""},
		{"a deeper subtype left out", "Animal", []string{"Dog", "Cat", "Animal"}, ""},
		{"the base left out", "Animal", []string{"Dog", "Cat", "Puppy"}, "CE0090"},
		{"an unrelated entity", "Animal", []string{"Dog", "Animal", "Other"}, "CE0774"},
		{"the subject's generalization", "Dog", []string{"Puppy", "Dog", "Animal"}, "CE0774"},
		{"a sibling", "Dog", []string{"Puppy", "Dog", "Cat"}, "CE0774"},
		{"the same entity twice", "Animal", []string{"Dog", "Dog", "Animal"}, "CE0771"},
		{"a leaf subject with its own branch (control)", "Puppy", []string{"Puppy"}, ""},
		{"a specialization subject without its own branch", "Dog", []string{"Puppy"}, "CE0090"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ValidateInheritanceSplits(mustBuild(t, splitTypeFlow(c.subject, c.branches...)), nil)
			if c.want == "" {
				if len(got) != 0 {
					t.Fatalf("want none, got %v: %+v", rulesOf(got), got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("want one violation, got %v: %+v", rulesOf(got), got)
			}
			if got[0].RuleID != inheritanceSplitRule || !strings.Contains(got[0].Message, c.want) {
				t.Errorf("rule or code wrong: %+v", got[0])
			}
			if got[0].Location.DocumentName != "MF" {
				t.Errorf("location: %+v", got[0].Location)
			}
		})
	}
}

// Nanoflows have the same split.
func TestValidateInheritanceSplits_Nanoflow(t *testing.T) {
	src := strings.Replace(splitTypeFlow("Animal", "Dog"), "create microflow", "create nanoflow", 1)
	if got := ValidateInheritanceSplits(mustBuild(t, src), nil); len(got) != 1 || !strings.Contains(got[0].Message, "CE0090") {
		t.Errorf("nanoflow without the base: want one CE0090, got %+v", got)
	}
}

// What is not judged: a subject whose entity is not known (a variable the flow
// does not declare), and a branch entity whose generalizations cannot be read.
func TestValidateInheritanceSplits_NotJudged(t *testing.T) {
	noSubject := strings.Replace(splitTypeFlow("Animal", "Dog"), "split type $A", "split type $Unknown", 1)
	if got := ValidateInheritanceSplits(mustBuild(t, noSubject), nil); len(got) != 0 {
		t.Errorf("an untyped subject: want none, got %+v", got)
	}
	// The entities are the project's, not the script's, and there is no project:
	// the base is still known from the parameter, so CE0090 is judged, but whether
	// Elsewhere is a specialization is not.
	src := `mdl 1;
create microflow Pj.MF ($A: Pj.Animal) returns boolean begin
  split type $A
    when Pj.Animal then
      log info node 'n' 'x';
    when Pj.Elsewhere then
      log info node 'n' 'y';
  end split;
  return true;
end;
`
	if got := ValidateInheritanceSplits(mustBuild(t, src), nil); len(got) != 0 {
		t.Errorf("unknown generalizations: want none, got %+v", got)
	}
}

// The hierarchy comes from the project when the entities are not the script's.
func TestValidateInheritanceSplits_ProjectHierarchy(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod-pj"}, Name: "Pj"}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: "dm-pj"}, ContainerID: mod.ID,
		Entities: []*domainmodel.Entity{
			{BaseElement: model.BaseElement{ID: "e1"}, Name: "Animal"},
			{BaseElement: model.BaseElement{ID: "e2"}, Name: "Dog", GeneralizationRef: "Pj.Animal"},
			{BaseElement: model.BaseElement{ID: "e3"}, Name: "Rock"},
		},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Pj" {
				return mod, nil
			}
			return nil, nil
		},
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			if id == mod.ID {
				return dm, nil
			}
			return nil, nil
		},
	}
	open := func() backend.FullBackend { return mb }
	flow := func(extra string) string {
		return `mdl 1;
create microflow Pj.MF ($A: Pj.Animal) returns boolean begin
  split type $A
    when Pj.Animal then
      log info node 'n' 'x';
    when Pj.` + extra + ` then
      log info node 'n' 'y';
  end split;
  return true;
end;
`
	}
	if got := ValidateInheritanceSplits(mustBuild(t, flow("Dog")), open); len(got) != 0 {
		t.Errorf("Dog specializes Animal in the project (control): want none, got %+v", got)
	}
	if got := ValidateInheritanceSplits(mustBuild(t, flow("Rock")), open); len(got) != 1 || !strings.Contains(got[0].Message, "CE0774") {
		t.Errorf("Rock is not a specialization: want one CE0774, got %+v", got)
	}
}
