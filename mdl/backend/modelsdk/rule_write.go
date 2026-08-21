// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func init() {
	// Studio Pro writes Flows on every rule, as the bare typed-array marker when
	// the rule has none. Without this a rule whose body produced no sequence
	// flows omitted the key entirely — one key short of the reference document,
	// and `mx check` passes either way.
	codec.RegisterTypeDefaults("Microflows$Rule", codec.TypeDefaults{
		MandatoryLists: []string{"Flows"},
	})
}

// CreateRule inserts a new Microflows$Rule document unit. A rule shares the
// microflow flow model (parameters + object collection + sequence flows), so it
// reuses the microflow object/flow converters; only the top-level field set
// differs, and it differs by omission — see ruleToGen.
func (b *Backend) CreateRule(rule *microflows.Rule) error {
	if rule == nil {
		return fmt.Errorf("CreateRule: nil rule")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateRule: not connected for writing")
	}
	if rule.ID == "" {
		rule.ID = model.ID(mmpr.GenerateID())
	}
	g := ruleToGen(rule, b.majorVersion())
	g.SetID(element.ID(rule.ID))
	assignRuleIDs(g)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateRule: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(rule.ID), string(rule.ContainerID), "Documents", "Microflows$Rule", contents); err != nil {
		return fmt.Errorf("CreateRule: insert: %w", err)
	}
	return nil
}

// UpdateRule rebuilds a rule document (the CREATE OR REPLACE path).
func (b *Backend) UpdateRule(rule *microflows.Rule) error {
	if rule == nil {
		return fmt.Errorf("UpdateRule: nil rule")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateRule: not connected for writing")
	}
	g := ruleToGen(rule, b.majorVersion())
	g.SetID(element.ID(rule.ID))
	assignRuleIDs(g)
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("UpdateRule: encode: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(rule.ID), contents); err != nil {
		return fmt.Errorf("UpdateRule: update: %w", err)
	}
	return nil
}

// DeleteRule removes the rule unit.
func (b *Backend) DeleteRule(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteRule: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// ruleToGen builds a gen Rule from the model. The field set is the ten
// properties a Studio Pro-authored rule stores, measured against ako/TestApp
// (Mendix 11.13.0). Three omissions are deliberate and load-bearing:
//
//   - No AllowedModuleRoles. A rule is not independently callable, so it has no
//     module-role security; writing an empty list would invent a property the
//     document does not have.
//   - No ReturnType. gen declares it beside MicroflowReturnType, but Studio Pro
//     does not write it and generated/metamodel 11.6.0 does not list it — a
//     pre-7 legacy sibling with nothing to carry through.
//   - No concurrency group, Url, StableId or action-info slots: those are the
//     nine properties that make a microflow a microflow.
func ruleToGen(rule *microflows.Rule, major int) *genMf.Rule {
	out := genMf.NewRule()
	out.SetName(rule.Name)
	out.SetDocumentation(rule.Documentation)
	// Both Studio Pro reference rules store ExportLevel "Hidden", and both
	// engines already hardcode it for microflows. Omitting it was the one key
	// the first authored rule was missing against the reference document.
	out.SetExportLevel("Hidden")
	out.SetExcluded(rule.Excluded)
	out.SetMarkAsUsed(rule.MarkAsUsed)
	out.SetApplyEntityAccess(rule.ApplyEntityAccess)
	out.SetReturnVariableName(rule.ReturnVariableName)
	if rule.ReturnType != nil {
		out.SetMicroflowReturnType(microflowDataTypeToGen(rule.ReturnType))
	}

	oc := genMf.NewMicroflowObjectCollection()
	for i, p := range rule.Parameters {
		oc.AddObjects(microflowParameterToGen(p, i, major))
	}
	if rule.ObjectCollection != nil {
		for _, obj := range rule.ObjectCollection.Objects {
			if g := microflowObjectToGen(obj); g != nil {
				oc.AddObjects(g)
			}
		}
	}
	out.SetObjectCollection(oc)

	if rule.ObjectCollection != nil {
		for _, f := range rule.ObjectCollection.Flows {
			out.AddFlows(sequenceFlowToGen(f, major))
		}
	}
	return out
}

// assignRuleIDs assigns fresh IDs to the rule's return type, object collection
// and sequence flows — the nanoflow counterpart.
func assignRuleIDs(r *genMf.Rule) {
	if rt := r.MicroflowReturnType(); rt != nil {
		assignID(rt)
	}
	if oc, ok := r.ObjectCollection().(*genMf.MicroflowObjectCollection); ok {
		assignObjectCollectionIDs(oc)
	}
	for _, el := range r.FlowsItems() {
		assignID(el)
		if sf, ok := el.(*genMf.SequenceFlow); ok {
			for _, cv := range sf.CaseValuesItems() {
				assignID(cv)
			}
			assignID(sf.Line())
		}
	}
}
