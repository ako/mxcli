// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	"github.com/mendixlabs/mxcli/modelsdk/mpr/version"
	mversion "github.com/mendixlabs/mxcli/modelsdk/version"
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

// The vendored generator gave two more properties the row of their type's next
// property and left that one out, as it did AggregateFunction: mendixmodelsdk
// 4.117.0 has CustomWidgets$WidgetReturnType `type: {}` and `assignableTo:
// {introduced: "9.20.0"}`, and Forms$GroupBox (pages.GroupBox) `caption: {}`
// and `headerMode: {introduced: "8.10.0"}`. The table had type 9.20.0 and
// caption 8.10.0, and no row for assignableTo or headerMode.
// Workflows$CallMicroflowTask.boundaryEvents is 10.14.0 in both, and stays.
func TestPropertyDeclared_RowsTheGeneratorMisattributed(t *testing.T) {
	for _, tt := range []struct {
		typ, key string
		v        *version.ProjectVersion
		want     bool
	}{
		// Studio Pro 9.1.1 and 9.4.0 store Type on every return type; 9.24.3
		// adds AssignableTo.
		{"CustomWidgets$WidgetReturnType", "Type", pv(9, 0, 0), true},
		{"CustomWidgets$WidgetReturnType", "Type", pv(9, 18, 0), true},
		{"CustomWidgets$WidgetReturnType", "AssignableTo", pv(9, 19, 0), false},
		{"CustomWidgets$WidgetReturnType", "AssignableTo", pv(9, 20, 0), true},
		// The caption is stored as CaptionTemplate, which the table never
		// keyed: the wrong row applied to nothing. HeaderMode is 8.10, below
		// every version mxcli writes.
		{"Forms$GroupBox", "CaptionTemplate", pv(8, 0, 0), true},
		{"Forms$GroupBox", "HeaderMode", pv(8, 9, 0), false},
		{"Forms$GroupBox", "HeaderMode", pv(8, 10, 0), true},
		{"Forms$GroupBox", "HeaderMode", pv(9, 0, 0), true},
		// Unchanged: right in the table already.
		{"Workflows$CallMicroflowTask", "BoundaryEvents", pv(10, 13, 0), false},
		{"Workflows$CallMicroflowTask", "BoundaryEvents", pv(10, 14, 0), true},
	} {
		v := mversion.Version{Major: tt.v.MajorVersion, Minor: tt.v.MinorVersion, Patch: tt.v.PatchVersion}
		if got := canon.PropertyDeclared(tt.typ, tt.key, v); got != tt.want {
			t.Errorf("%s.%s declared on %s = %v, want %v", tt.typ, tt.key, v, got, tt.want)
		}
	}
}

// pageWithReturnType is a page holding one pluggable widget whose Type has a
// return type as the template carries it: Type set, AssignableTo empty.
func pageWithReturnType() bson.D {
	rt := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "CustomWidgets$WidgetReturnType"},
		{Key: "AssignableTo", Value: ""}, {Key: "EntityProperty", Value: ""}, {Key: "IsList", Value: false}, {Key: "Type", Value: "String"}}
	vt := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "CustomWidgets$WidgetValueType"}, {Key: "ReturnType", Value: rt}}
	widget := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "CustomWidgets$CustomWidget"}, {Key: "Name", Value: "w"},
		{Key: "Type", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "CustomWidgets$WidgetObjectType"},
				{Key: "PropertyTypes", Value: bson.A{int32(2), bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "CustomWidgets$WidgetPropertyType"},
					{Key: "ValueType", Value: vt}}}}}}}}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Forms$Page"}, {Key: "Name", Value: "P"},
		{Key: "Widgets", Value: bson.A{int32(2), widget}}}
}

// Below 9.20 a return type's Type was refused ("this Mendix 9.18.0 project
// cannot store CustomWidgets$WidgetReturnType.Type (Mendix 9.20.0+)") and its
// AssignableTo passed. Now Type is kept and the empty AssignableTo dropped.
func TestNormalize_WidgetReturnTypeAcrossMendix9_20(t *testing.T) {
	for _, tt := range []struct {
		v    *version.ProjectVersion
		want string
	}{
		{pv(9, 18, 0), "$ID,$Type,EntityProperty,IsList,Type"},
		{pv(9, 20, 0), "$ID,$Type,AssignableTo,EntityProperty,IsList,Type"},
		{pv(11, 13, 0), "$ID,$Type,AssignableTo,EntityProperty,IsList,Type"}, // control
	} {
		d, err := normalizeShape(t, tt.v, pageWithReturnType())
		if err != nil {
			t.Errorf("%d.%d: a widget with a return type is refused: %v", tt.v.MajorVersion, tt.v.MinorVersion, err)
			continue
		}
		if got := strings.Join(keysOf(find(d, "CustomWidgets$WidgetReturnType")), ","); got != tt.want {
			t.Errorf("%d.%d: return type keys %s, want %s", tt.v.MajorVersion, tt.v.MinorVersion, got, tt.want)
		}
	}
}

// boundaryEvent is a timer boundary event as mxcli writes one (10.20's type).
func boundaryEvent() bson.D {
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$InterruptingTimerBoundaryEvent"},
		{Key: "Caption", Value: "Late"}, {Key: "FirstExecutionTime", Value: "addDays([%CurrentDateTime%], 1)"},
		{Key: "Flow", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Flow"}, {Key: "Activities", Value: bson.A{int32(3)}}}}}
}

// workflowWithCallBoundaryEvents is a workflow holding one microflow call with
// the given BoundaryEvents list.
func workflowWithCallBoundaryEvents(events bson.A) bson.D {
	call := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$CallMicroflowTask"},
		{Key: "BoundaryEvents", Value: events}, {Key: "Caption", Value: "Validate"},
		{Key: "Microflow", Value: "M.Validate"}, {Key: "Name", Value: "call"}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Flow", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Flow"},
			{Key: "Activities", Value: bson.A{int32(3), call}}}},
		{Key: "Name", Value: "WF"}, {Key: "Title", Value: "WF"}}
}

// Workflows$CallMicroflowTask.BoundaryEvents is 10.14.0 (mendixmodelsdk, and
// the table). The writer gives every microflow call an empty list, so below
// 10.14 the empty list is dropped, not refused: Studio Pro 9.24.3 stores no
// BoundaryEvents on a call (Multilevel Dynamic Workflow 1.0.0), and with the
// row's floor removed mxcli writes `BoundaryEvents: [2]` on 10.12.19, which
// `mx convert` 10.12.19 strips. A boundary event the script states is
// refused there, naming the key.
func TestNormalize_CallMicroflowBoundaryEventsBelowMendix10_14(t *testing.T) {
	for _, tt := range []struct {
		v    *version.ProjectVersion
		kept bool
	}{
		{pv(9, 24, 30), false},
		{pv(10, 13, 0), false},
		{pv(10, 14, 0), true},
		{pv(11, 13, 0), true}, // control
	} {
		d, err := normalizeShape(t, tt.v, workflowWithCallBoundaryEvents(bson.A{int32(2)}))
		if err != nil {
			t.Errorf("%d.%d: a microflow call is refused: %v", tt.v.MajorVersion, tt.v.MinorVersion, err)
			continue
		}
		if _, ok := get(find(d, "Workflows$CallMicroflowTask"), "BoundaryEvents"); ok != tt.kept {
			t.Errorf("%d.%d: BoundaryEvents kept %v, want %v", tt.v.MajorVersion, tt.v.MinorVersion, ok, tt.kept)
		}
	}
	_, err := normalizeShape(t, pv(10, 13, 0), workflowWithCallBoundaryEvents(bson.A{int32(2), boundaryEvent()}))
	if err == nil || !strings.Contains(err.Error(), "Workflows$CallMicroflowTask.BoundaryEvents (Mendix 10.14.0+)") {
		t.Errorf("a boundary event on a microflow call on 10.13 was not refused naming the key: %v", err)
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

// endOfPath is the marker mxcli ends a parallel split path with
// (workflows.EndParallelSplitPath).
func endOfPath() bson.D {
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$EndOfParallelSplitPathActivity"},
		{Key: "Annotation", Value: nil}, {Key: "Caption", Value: "End of parallel split path"},
		{Key: "Name", Value: "EndOfParallelSplitPath"}, {Key: "PersistentId", Value: pid()}}
}

// workflowWithParallelSplit is a workflow as mxcli builds it: a parallel split
// whose first path calls a microflow and whose second is empty, each ending in
// the end-of-path marker, followed by another microflow call.
func workflowWithParallelSplit() bson.D {
	call := func(name string) bson.D {
		return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$CallMicroflowTask"},
			{Key: "BoundaryEvents", Value: bson.A{int32(2)}}, {Key: "Caption", Value: name},
			{Key: "Microflow", Value: "M." + name}, {Key: "Name", Value: name}, {Key: "PersistentId", Value: pid()}}
	}
	path := func(acts ...bson.D) bson.D {
		list := bson.A{int32(3)}
		for _, a := range acts {
			list = append(list, a)
		}
		return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$ParallelSplitOutcome"},
			{Key: "Flow", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Flow"}, {Key: "Activities", Value: list}}},
			{Key: "PersistentId", Value: pid()}}
	}
	split := bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$ParallelSplitActivity"},
		{Key: "Annotation", Value: nil}, {Key: "Caption", Value: "Parallel split"}, {Key: "Name", Value: "split"},
		{Key: "Outcomes", Value: bson.A{int32(3), path(call("Left"), endOfPath()), path(endOfPath())}},
		{Key: "PersistentId", Value: pid()}}
	return bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "Flow", Value: bson.D{{Key: "$ID", Value: bid()}, {Key: "$Type", Value: "Workflows$Flow"},
			{Key: "Activities", Value: bson.A{int32(3), split, call("After")}}}},
		{Key: "Name", Value: "WF"},
		{Key: "PersistentId", Value: pid()},
		{Key: "Title", Value: "WF"}}
}

// pathActivityTypes returns, per parallel split path, the $Type of each
// activity its flow holds.
func pathActivityTypes(t *testing.T, d bson.D) [][]string {
	t.Helper()
	outcomes, _ := get(find(d, "Workflows$ParallelSplitActivity"), "Outcomes")
	var out [][]string
	for _, o := range listItems(outcomes) {
		flow, _ := get(o.(bson.D), "Flow")
		acts, _ := get(flow.(bson.D), "Activities")
		if a, _ := acts.(bson.A); len(a) == 0 || a[0] != int32(3) {
			t.Errorf("a path's Activities is %v, want a list with marker 3", acts)
		}
		types := []string{}
		for _, a := range listItems(acts) {
			typ, _ := get(a.(bson.D), "$Type")
			types = append(types, typ.(string))
		}
		out = append(out, types)
	}
	return out
}

// Workflows$EndOfParallelSplitPathActivity is introduced in 10.21.0
// (mendixmodelsdk: the type's own versionInfo). mxcli ends every parallel split
// path with one, because from 10.21 the engine runs a path only up to it — and
// below 10.21 the type does not exist: mx check 9.24.30 does not load the
// project at all ("TypeCacheUnknownTypeException: The type cache does not
// contain a type with qualified name Workflows$EndOfParallelSplitPathActivity").
// The property floor holds property versions only, so it never saw the type.
// Studio Pro before 10.21 stores a path as its activities and nothing else:
// the Marketplace's "Example project workflow extension" 2.0.0 (Studio Pro
// 9.4.0) has a split whose two paths hold one user task each, no marker, and
// an empty flow stores `Activities: [3]`. So below 10.21 the marker is not
// written; from 10.21 it is, and must be.
func TestNormalize_ParallelSplitPathsBelowMendix10_21(t *testing.T) {
	for _, tt := range []struct {
		v    *version.ProjectVersion
		want string
	}{
		{pv(9, 24, 30), "Workflows$CallMicroflowTask|"},
		{pv(10, 20, 0), "Workflows$CallMicroflowTask|"},
		{pv(10, 21, 0), "Workflows$CallMicroflowTask,Workflows$EndOfParallelSplitPathActivity|Workflows$EndOfParallelSplitPathActivity"},
		{pv(11, 13, 0), "Workflows$CallMicroflowTask,Workflows$EndOfParallelSplitPathActivity|Workflows$EndOfParallelSplitPathActivity"}, // control
	} {
		d, err := normalizeShape(t, tt.v, workflowWithParallelSplit())
		if err != nil {
			t.Errorf("%d.%d: a parallel split is refused: %v", tt.v.MajorVersion, tt.v.MinorVersion, err)
			continue
		}
		var paths []string
		for _, p := range pathActivityTypes(t, d) {
			paths = append(paths, strings.Join(p, ","))
		}
		if got := strings.Join(paths, "|"); got != tt.want {
			t.Errorf("%d.%d: paths hold %s, want %s", tt.v.MajorVersion, tt.v.MinorVersion, got, tt.want)
		}
		// The rest of the workflow is untouched: the call after the split stays.
		acts, _ := get(find(d, "Workflows$Flow"), "Activities")
		if n := len(listItems(acts)); n != 2 {
			t.Errorf("%d.%d: the main flow holds %d activities, want 2", tt.v.MajorVersion, tt.v.MinorVersion, n)
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
