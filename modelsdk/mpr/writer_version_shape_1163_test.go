// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
)

// ako/mxcli#1163: what the 9.24 doctype run found mxcli writing that Mendix 9
// stores differently, or not at all. Each case is the document as mxcli builds
// it, normalized for the project's version the way every unit write is.

// aggregateMicroflow is a microflow holding one Aggregate list activity as the
// writer builds it: Count/Sum/… carry no fold; Reduce, All and Any carry
// ReduceInitialValueExpression and ReduceReturnDataType.
func aggregateMicroflow(function string) bson.D {
	action := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Microflows$AggregateAction"},
		{Key: "AggregateFunction", Value: function},
		{Key: "AggregateVariableName", Value: "Items"},
		{Key: "Attribute", Value: "M.Item.Price"},
		{Key: "ErrorHandlingType", Value: "Rollback"}}
	switch function {
	case "Reduce", "All", "Any":
		initial := ""
		if function == "Reduce" {
			initial = "0"
		}
		action = append(action,
			bson.E{Key: "ReduceInitialValueExpression", Value: initial},
			bson.E{Key: "ReduceReturnDataType", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "DataTypes$BooleanType"}}})
	}
	action = append(action, bson.E{Key: "VariableName", Value: "Total"})
	activity := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Microflows$ActionActivity"}, {Key: "Action", Value: action}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Microflows$Microflow"}, {Key: "Name", Value: "Agg"},
		{Key: "ObjectCollection", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3), activity}}}}}
}

// Microflows$AggregateAction.AggregateFunction is declared on every version
// mxcli writes. The generated version data floored it at 10.0.0, so every
// count/sum/average/minimum/maximum on 9.x was refused ("cannot store
// Microflows$AggregateAction.AggregateFunction (Mendix 10.0.0+)"). Studio Pro
// 9.24.0 stores it (Workflow Commons 2.6.0: 10 of 10 aggregates, `Count`), `mx
// convert` 9.24.30 keeps it, and mendixmodelsdk gives it no version at all; the
// 10.0.0 floor belongs to ReduceReturnDataType, the fold Reduce added.
func TestNormalize_AggregateFunctionIsDeclaredOnMendix9(t *testing.T) {
	for _, v := range []*version.ProjectVersion{pv(9, 0, 5), pv(9, 24, 30), pv(10, 0, 0), pv(11, 13, 0)} {
		d, err := normalizeShape(t, v, aggregateMicroflow("Sum"))
		if err != nil {
			t.Errorf("%d.%d: a sum aggregate is refused: %v", v.MajorVersion, v.MinorVersion, err)
			continue
		}
		if f, _ := get(find(d, "Microflows$AggregateAction"), "AggregateFunction"); f != "Sum" {
			t.Errorf("%d.%d: AggregateFunction = %v, want Sum", v.MajorVersion, v.MinorVersion, f)
		}
	}
}

// The fold is what Mendix 10.0 added (mendixmodelsdk: ReduceReturnDataType
// introduced 10.0.0). `all` carries an empty initial value, so the fold's
// return type is the one key that says it: on 9.24 it is refused, naming it,
// rather than written into a document Studio Pro 9 cannot resolve. Before, the
// generated data had no row for it and it passed through.
func TestNormalize_ReduceFoldIsRefusedBelowMendix10(t *testing.T) {
	for _, fn := range []string{"Reduce", "All"} {
		if _, err := normalizeShape(t, pv(9, 24, 30), aggregateMicroflow(fn)); err == nil ||
			!strings.Contains(err.Error(), "Microflows$AggregateAction.ReduceReturnDataType (Mendix 10.0.0+)") {
			t.Errorf("%s on 9.24 was not refused naming its fold's return type: %v", fn, err)
		}
		d, err := normalizeShape(t, pv(10, 0, 0), aggregateMicroflow(fn))
		if err != nil {
			t.Fatalf("%s on 10.0 is refused: %v", fn, err)
		}
		if _, ok := get(find(d, "Microflows$AggregateAction"), "ReduceReturnDataType"); !ok {
			t.Errorf("%s on 10.0 lost ReduceReturnDataType", fn)
		}
	}
}

// pid is a fresh PersistentId: a non-zero GUID, as the writer mints one (a zero
// GUID is an empty value, which the floor would drop rather than refuse).
func pid() bson.Binary {
	pidSeq++
	return bson.Binary{Subtype: 0, Data: []byte{pidSeq, 0x11, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}}
}

var pidSeq byte

// workflowWithPersistentIDs is a workflow as mxcli builds it: a fresh
// PersistentId on the workflow, on every activity and on every outcome.
func workflowWithPersistentIDs() bson.D {
	outcome := func(value bool) bson.D {
		return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$BooleanConditionOutcome"},
			{Key: "Flow", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Flow"}, {Key: "Activities", Value: bson.A{int32(3)}}}},
			{Key: "PersistentId", Value: pid()}, {Key: "Value", Value: value}}
	}
	split := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$ExclusiveSplitActivity"},
		{Key: "Caption", Value: "Active?"}, {Key: "Expression", Value: "$WorkflowContext/Active"}, {Key: "Name", Value: "split"},
		{Key: "Outcomes", Value: bson.A{int32(3), outcome(true), outcome(false)}},
		{Key: "PersistentId", Value: pid()}}
	call := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$CallMicroflowTask"},
		{Key: "BoundaryEvents", Value: bson.A{int32(2)}}, {Key: "Caption", Value: "Validate"},
		{Key: "Microflow", Value: "M.Validate"}, {Key: "Name", Value: "call"}, {Key: "PersistentId", Value: pid()}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Flow", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Flow"},
			{Key: "Activities", Value: bson.A{int32(3), call, split}}}},
		{Key: "Name", Value: "WF"},
		{Key: "PersistentId", Value: pid()},
		{Key: "Title", Value: "WF"}}
}

// persistentIDHolders returns the $Type of every element in v carrying a
// PersistentId.
func persistentIDHolders(v any) []string {
	var out []string
	switch x := v.(type) {
	case bson.D:
		typ, _ := get(x, "$Type")
		for _, e := range x {
			if e.Key == "PersistentId" {
				out = append(out, typ.(string))
			}
			out = append(out, persistentIDHolders(e.Value)...)
		}
	case bson.A:
		for _, el := range x {
			out = append(out, persistentIDHolders(el)...)
		}
	}
	return out
}

// Workflows exist from 9.0, PersistentId from 10.21.0 — on the workflow and on
// WorkflowActivity, Outcome and BoundaryEvent (mendixmodelsdk). Below that a
// workflow was refused ("cannot store Workflows$Workflow.PersistentId (Mendix
// 10.21.0+)"), and the activities' and outcomes' were never seen by the floor,
// which does not look up the abstract supertypes that declare them: `mx
// convert` 9.24.30 strips all of them. A PersistentId is minted by the writer,
// never stated by a script, so below 10.21 it is not written.
func TestNormalize_WorkflowPersistentIDsBelowMendix10_21(t *testing.T) {
	for _, tt := range []struct {
		v    *version.ProjectVersion
		want int
	}{
		{pv(9, 24, 30), 0},
		{pv(10, 20, 0), 0},
		{pv(10, 21, 0), 5},
		{pv(10, 24, 24), 5},
		{pv(11, 13, 0), 5}, // control
	} {
		d, err := normalizeShape(t, tt.v, workflowWithPersistentIDs())
		if err != nil {
			t.Errorf("%d.%d: a workflow is refused: %v", tt.v.MajorVersion, tt.v.MinorVersion, err)
			continue
		}
		if got := persistentIDHolders(d); len(got) != tt.want {
			t.Errorf("%d.%d: PersistentId on %d elements %v, want %d", tt.v.MajorVersion, tt.v.MinorVersion, len(got), got, tt.want)
		}
	}
}

// settingsWithConstantValue is the project settings unit as `alter settings
// constant … in configuration` writes a new override: the value nested in a
// Settings$SharedValue (or a Settings$PrivateValue, which holds none). flat is
// the empty flat Value the overlay leaves beside a nested one, "-" for none.
func settingsWithConstantValue(valueType, value, flat string) bson.D {
	inner := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: valueType}}
	if valueType == "Settings$SharedValue" {
		inner = append(inner, bson.E{Key: "Value", Value: value})
	}
	cv := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Settings$ConstantValue"},
		{Key: "ConstantId", Value: "M.ServerUrl"}, {Key: "SharedOrPrivateValue", Value: inner}}
	if flat != "-" {
		cv = append(cv, bson.E{Key: "Value", Value: flat})
	}
	config := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Settings$ServerConfiguration"},
		{Key: "ConstantValues", Value: bson.A{int32(3), cv}}, {Key: "Name", Value: "Default"}}
	part := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Settings$ConfigurationSettings"},
		{Key: "Configurations", Value: bson.A{int32(3), config}}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Settings$ProjectSettings"},
		{Key: "Settings", Value: bson.A{int32(2), part}}}
}

// A configuration's constant override is a flat Value string below 10.9 and a
// Settings$SharedValue under SharedOrPrivateValue from it (mendixmodelsdk:
// Value deleted, SharedOrPrivateValue introduced 10.9.0). `mx convert` 9.24.30
// turns the nested form into `{$ID, $Type, ConstantId, Value: ""}` — it does
// not know the nested value, so the override silently became empty, and mxcli
// refused it ("cannot store Settings$ConstantValue.SharedOrPrivateValue
// (Mendix 10.9.0+)").
func TestNormalize_ConstantValueIsFlatBelowMendix10_9(t *testing.T) {
	for _, tt := range []struct {
		v    *version.ProjectVersion
		flat bool
	}{
		{pv(9, 24, 30), true},
		{pv(10, 8, 0), true},
		{pv(10, 9, 0), false},
		{pv(11, 13, 0), false}, // control
	} {
		for _, flat := range []string{"-", ""} {
			d, err := normalizeShape(t, tt.v, settingsWithConstantValue("Settings$SharedValue", "kafka:9092", flat))
			if err != nil {
				t.Errorf("%d.%d: a constant override is refused: %v", tt.v.MajorVersion, tt.v.MinorVersion, err)
				continue
			}
			cv := find(d, "Settings$ConstantValue")
			if !tt.flat {
				if _, ok := get(cv, "SharedOrPrivateValue"); !ok {
					t.Errorf("%d.%d: SharedOrPrivateValue was lowered: %v", tt.v.MajorVersion, tt.v.MinorVersion, keysOf(cv))
				}
				continue
			}
			if got := strings.Join(keysOf(cv), ","); got != "$ID,$Type,ConstantId,Value" {
				t.Errorf("%d.%d: constant value keys = %s, want $ID,$Type,ConstantId,Value", tt.v.MajorVersion, tt.v.MinorVersion, got)
			}
			if v, _ := get(cv, "Value"); v != "kafka:9092" {
				t.Errorf("%d.%d: Value = %v, want kafka:9092", tt.v.MajorVersion, tt.v.MinorVersion, v)
			}
		}
	}
}

// A private value lives on the developer's workstation, which a flat Value
// cannot say: refused, naming the constant, not written as an empty override.
func TestNormalize_RefusesAPrivateConstantValueBelowMendix10_9(t *testing.T) {
	_, err := normalizeShape(t, pv(9, 24, 30), settingsWithConstantValue("Settings$PrivateValue", "", "-"))
	if err == nil || !strings.Contains(err.Error(), "M.ServerUrl") {
		t.Errorf("a private constant value on 9.24 was not refused naming the constant: %v", err)
	}
}

// flowMapping is one argument of a page's flow call as mxcli builds it
// (bindParameterMappingValue): a page-variable reference under Variable with an
// empty Expression, anything else as Expression text with Variable null.
func flowMapping(mappingType, param, expression, pageParam string) bson.D {
	var variable any
	if pageParam != "" {
		variable = bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$PageVariable"},
			{Key: "PageParameter", Value: pageParam}, {Key: "SnippetParameter", Value: ""},
			{Key: "UseAllPages", Value: false}, {Key: "Widget", Value: ""}}
	}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: mappingType},
		{Key: "Expression", Value: expression}, {Key: "Parameter", Value: param}, {Key: "Variable", Value: variable}}
}

// pageWithFlowCalls is a page with a microflow button and a nanoflow button,
// each passing the given mappings.
func pageWithFlowCalls(microflowMappings, nanoflowMappings []bson.D) bson.D {
	list := func(ms []bson.D) bson.A {
		a := bson.A{int32(2)}
		for _, m := range ms {
			a = append(a, m)
		}
		return a
	}
	mfButton := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$ActionButton"}, {Key: "Name", Value: "btnMf"},
		{Key: "Action", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$MicroflowAction"},
			{Key: "MicroflowSettings", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Microflow", Value: "M.ACT"}, {Key: "ParameterMappings", Value: list(microflowMappings)}}}}}}
	nfButton := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$ActionButton"}, {Key: "Name", Value: "btnNf"},
		{Key: "Action", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$CallNanoflowClientAction"},
			{Key: "Nanoflow", Value: "M.NF"}, {Key: "ParameterMappings", Value: list(nanoflowMappings)}}}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$Page"}, {Key: "Name", Value: "P"},
		{Key: "Widgets", Value: bson.A{int32(2), mfButton, nfButton}}}
}

// mappingsOf returns the mappings stored under the first $Type holder in d.
func mappingsOf(t *testing.T, d bson.D, holder string) []bson.D {
	t.Helper()
	list, _ := get(find(d, holder), "ParameterMappings")
	var out []bson.D
	for _, el := range listItems(list) {
		out = append(out, el.(bson.D))
	}
	return out
}

func listItems(v any) bson.A {
	a, _ := v.(bson.A)
	if len(a) > 0 {
		if _, marker := a[0].(int32); marker {
			return a[1:]
		}
	}
	return a
}

// A flow argument on a page is a Variable (a Forms$PageVariable, required) below
// 10.10, and Expression text only from 10.10 (mendixmodelsdk). mxcli writes
// `$currentObject` as Expression with Variable null: the floor refused it on
// 9.24 ("cannot store Forms$MicroflowParameterMapping.Expression (Mendix
// 10.10.0+)"), and without the floor 9.24 does not load the page at all ("An
// error occurred when trying to set the 'Variable' property of a Microflow
// parameter mapping … Value cannot be null"). Studio Pro 9.24 stores no mapping
// for an argument that is the enclosing object — it infers it: Workflow
// Commons 2.6.0 (9.24.0), 75 microflow and 2 nanoflow calls taking parameters,
// none with a mapping; the 9.24.30 starter app's ShowPasswordForm(Account)
// button likewise. So below 10.10 that mapping is not written; a page
// parameter keeps its Variable and loses only the empty Expression.
func TestNormalize_FlowArgumentsBelowMendix10_10(t *testing.T) {
	build := func() bson.D {
		return pageWithFlowCalls(
			[]bson.D{flowMapping("Forms$MicroflowParameterMapping", "M.ACT.Item", "$currentObject", ""),
				flowMapping("Forms$MicroflowParameterMapping", "M.ACT.Order", "", "Order")},
			[]bson.D{flowMapping("Forms$NanoflowParameterMapping", "M.NF.Item", "$currentObject", "")})
	}
	for _, tt := range []struct {
		v        *version.ProjectVersion
		lowered  bool
		mfCount  int
		nfCount  int
		mfParams string
	}{
		{pv(9, 24, 30), true, 1, 0, "M.ACT.Order"},
		{pv(10, 9, 0), true, 1, 0, "M.ACT.Order"},
		{pv(10, 10, 0), false, 2, 1, "M.ACT.Item,M.ACT.Order"},
		{pv(11, 13, 0), false, 2, 1, "M.ACT.Item,M.ACT.Order"}, // control
	} {
		d, err := normalizeShape(t, tt.v, build())
		if err != nil {
			t.Errorf("%d.%d: the page is refused: %v", tt.v.MajorVersion, tt.v.MinorVersion, err)
			continue
		}
		mf := mappingsOf(t, d, "Forms$MicroflowSettings")
		nf := mappingsOf(t, d, "Forms$CallNanoflowClientAction")
		var params []string
		for _, m := range mf {
			p, _ := get(m, "Parameter")
			params = append(params, p.(string))
			if _, ok := get(m, "Expression"); ok == tt.lowered {
				t.Errorf("%d.%d: mapping %v keeps Expression %v", tt.v.MajorVersion, tt.v.MinorVersion, p, ok)
			}
			if v, _ := get(m, "Variable"); tt.lowered && v == nil {
				t.Errorf("%d.%d: mapping %v has no Variable, which 9.x cannot load", tt.v.MajorVersion, tt.v.MinorVersion, p)
			}
		}
		if len(mf) != tt.mfCount || len(nf) != tt.nfCount || strings.Join(params, ",") != tt.mfParams {
			t.Errorf("%d.%d: %d microflow mappings (%s) and %d nanoflow mappings, want %d (%s) and %d",
				tt.v.MajorVersion, tt.v.MinorVersion, len(mf), strings.Join(params, ","), len(nf), tt.mfCount, tt.mfParams, tt.nfCount)
		}
	}
}

// Any other expression — a literal, an attribute path — has no Variable form:
// refused below 10.10, naming the parameter, rather than dropped.
func TestNormalize_RefusesAnExpressionArgumentBelowMendix10_10(t *testing.T) {
	for _, m := range []bson.D{
		flowMapping("Forms$MicroflowParameterMapping", "M.ACT.Flag", "true", ""),
		flowMapping("Forms$NanoflowParameterMapping", "M.NF.Name", "$Order/Name", ""),
	} {
		_, err := normalizeShape(t, pv(9, 24, 30), pageWithFlowCalls([]bson.D{m}, nil))
		p, _ := get(m, "Parameter")
		if err == nil || !strings.Contains(err.Error(), p.(string)) || !strings.Contains(err.Error(), "10.10") {
			t.Errorf("an expression argument for %v on 9.24 was not refused naming it: %v", p, err)
		}
	}
}

// Studio Pro's property set for a workflow on 9.24 includes the two state-change
// event handlers Mendix removed in 11.0: `mx convert` 9.24.30 adds both as null
// to a workflow mxcli wrote, as 10.24.24 does — a property-name difference
// Mendix's merge engine refuses to compare (#1373).
func TestNormalize_WorkflowStateChangeEventsOnMendix9(t *testing.T) {
	for _, tt := range []struct {
		v    *version.ProjectVersion
		want bool
	}{
		{pv(9, 24, 30), true},
		{pv(10, 24, 24), true},
		{pv(11, 13, 0), false}, // removed in 11.0
	} {
		d, err := normalizeShape(t, tt.v, workflowWithPersistentIDs())
		if err != nil {
			t.Fatal(err)
		}
		wf := find(d, "Workflows$Workflow")
		for _, k := range []string{"WorkflowOnStateChangeEvent", "UsertaskOnStateChangeEvent"} {
			if v, ok := get(wf, k); ok != tt.want || v != nil {
				t.Errorf("%d.%d: %s present %v (%v), want present %v and null", tt.v.MajorVersion, tt.v.MinorVersion, k, ok, v, tt.want)
			}
		}
	}
}
