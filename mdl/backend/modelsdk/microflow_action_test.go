// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// TestMicroflowActionToGen_LogAndVariableActions guards against the CE0008
// ("No action defined") / CE0109 ("Undefined variable") gap: log/declare/set
// actions must convert to a non-nil gen element with the correct storage $Type.
// An unhandled action returns nil → the ActionActivity is written without an
// Action → Studio Pro rejects the microflow.
func TestMicroflowActionToGen_LogAndVariableActions(t *testing.T) {
	cases := []struct {
		name   string
		action microflows.MicroflowAction
		typ    string
	}{
		{"log", &microflows.LogMessageAction{
			LogLevel:        "Info",
			MessageTemplate: &model.Text{Translations: map[string]string{"en_US": "hi"}},
		}, "Microflows$LogMessageAction"},
		{"declare", &microflows.CreateVariableAction{
			VariableName: "X", InitialValue: "0", DataType: &microflows.DecimalType{},
		}, "Microflows$CreateVariableAction"},
		{"set", &microflows.ChangeVariableAction{
			VariableName: "X", Value: "5",
		}, "Microflows$ChangeVariableAction"},
		// List / aggregate / cast / validation actions — storage $Type differs
		// from the Go type name for several of these (verified vs the legacy
		// serializer's case): AggregateListAction → Microflows$AggregateAction,
		// ListOperationAction → Microflows$ListOperationsAction.
		{"cast", &microflows.CastAction{OutputVariable: "O"}, "Microflows$CastAction"},
		{"aggregate", &microflows.AggregateListAction{
			InputVariable: "L", OutputVariable: "S", Function: "Sum", AttributeQualifiedName: "M.E.A",
		}, "Microflows$AggregateAction"},
		{"createlist", &microflows.CreateListAction{
			EntityQualifiedName: "M.E", OutputVariable: "L",
		}, "Microflows$CreateListAction"},
		{"changelist", &microflows.ChangeListAction{
			ChangeVariable: "L", Type: "Add", Value: "$x",
		}, "Microflows$ChangeListAction"},
		{"validationfeedback", &microflows.ValidationFeedbackAction{
			ObjectVariable: "P", AttributeName: "M.E.Code",
			Template: &model.Text{Translations: map[string]string{"en_US": "req"}},
		}, "Microflows$ValidationFeedbackAction"},
		{"listop", &microflows.ListOperationAction{
			OutputVariable: "Out", Operation: &microflows.HeadOperation{ListVariable: "L"},
		}, "Microflows$ListOperationsAction"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := microflowActionToGen(c.action)
			if g == nil {
				t.Fatalf("%s: microflowActionToGen returned nil (would emit an actionless activity → CE0008)", c.name)
			}
			if g.TypeName() != c.typ {
				t.Errorf("%s: $Type = %q, want %q", c.name, g.TypeName(), c.typ)
			}
		})
	}
}

// TestMicroflowActionToGen_ClientAndRestActions guards the same CE0008/CE0109 gap
// for the page/message/REST actions: each must convert to a non-nil gen element with
// the correct storage $Type. The $Type differs from the Go type name for the page
// actions (verified vs the legacy serializer): ShowPageAction → Microflows$ShowFormAction,
// ClosePageAction → Microflows$CloseFormAction.
func TestMicroflowActionToGen_ClientAndRestActions(t *testing.T) {
	cases := []struct {
		name   string
		action microflows.MicroflowAction
		typ    string
	}{
		{"showpage", &microflows.ShowPageAction{
			PageName: "M.Home",
			PageParameterMappings: []*microflows.PageParameterMapping{
				{Parameter: "M.Home.Obj", Argument: "$Obj"},
			},
		}, "Microflows$ShowFormAction"},
		{"closepage", &microflows.ClosePageAction{NumberOfPages: 1}, "Microflows$CloseFormAction"},
		{"showhomepage", &microflows.ShowHomePageAction{}, "Microflows$ShowHomePageAction"},
		{"showmessage", &microflows.ShowMessageAction{
			Type: "Information", Blocking: true,
			Template:           &model.Text{Translations: map[string]string{"en_US": "done {1}"}},
			TemplateParameters: []string{"$x"},
		}, "Microflows$ShowMessageAction"},
		{"restcall", &microflows.RestCallAction{
			OutputVariable: "Resp",
			HttpConfiguration: &microflows.HttpConfiguration{
				HttpMethod:       microflows.HttpMethodGet,
				LocationTemplate: "https://api/{1}",
				LocationParams:   []string{"$id"},
				CustomHeaders: []*microflows.HttpHeader{
					{Name: "Accept", Value: "application/json"},
				},
			},
			ResultHandling: &microflows.ResultHandlingString{VariableName: "Resp"},
		}, "Microflows$RestCallAction"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := microflowActionToGen(c.action)
			if g == nil {
				t.Fatalf("%s: microflowActionToGen returned nil (would emit an actionless activity → CE0008)", c.name)
			}
			if g.TypeName() != c.typ {
				t.Errorf("%s: $Type = %q, want %q", c.name, g.TypeName(), c.typ)
			}
			// The directly-built REST/FormSettings subtrees must also encode without
			// error (guards a missing list marker / mistyped property panic).
			if _, err := (&codec.Encoder{}).Encode(g); err != nil {
				t.Errorf("%s: encode: %v", c.name, err)
			}
		})
	}
}

// TestListOperationToGen_StorageNames guards the list-operation sub-element
// storage names and (critically) BSON keys: the gen ListOperation types use the
// wrong keys (ListVariableName/SecondListOrObjectVariableName), so these are
// built directly with the verified legacy keys ListName / SecondListOrObjectName
// under the wrapper's "NewOperation" / "ResultVariableName".
func TestListOperationToGen_StorageNames(t *testing.T) {
	cases := []struct {
		name string
		op   microflows.ListOperation
		typ  string
	}{
		{"head", &microflows.HeadOperation{ListVariable: "L"}, "Microflows$Head"},
		{"tail", &microflows.TailOperation{ListVariable: "L"}, "Microflows$Tail"},
		{"find", &microflows.FindOperation{ListVariable: "L", Expression: "x"}, "Microflows$FindByExpression"},
		{"filter", &microflows.FilterOperation{ListVariable: "L", Expression: "x"}, "Microflows$FilterByExpression"},
		{"sort", &microflows.SortOperation{ListVariable: "L"}, "Microflows$Sort"},
		{"union", &microflows.UnionOperation{ListVariable1: "A", ListVariable2: "B"}, "Microflows$Union"},
		{"intersect", &microflows.IntersectOperation{ListVariable1: "A", ListVariable2: "B"}, "Microflows$Intersect"},
		{"subtract", &microflows.SubtractOperation{ListVariable1: "A", ListVariable2: "B"}, "Microflows$Subtract"},
		{"contains", &microflows.ContainsOperation{ListVariable: "L", ObjectVariable: "O"}, "Microflows$Contains"},
		{"equals", &microflows.EqualsOperation{ListVariable1: "A", ListVariable2: "B"}, "Microflows$Equals"},
		{"findby", &microflows.FindByAttributeOperation{ListVariable: "L", Attribute: "M.E.A"}, "Microflows$Find"},
		{"filterby", &microflows.FilterByAttributeOperation{ListVariable: "L", Attribute: "M.E.A"}, "Microflows$Filter"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := listOperationToGen(c.op)
			if e == nil {
				t.Fatalf("%s: listOperationToGen returned nil", c.name)
			}
			if e.TypeName() != c.typ {
				t.Errorf("%s: $Type = %q, want %q", c.name, e.TypeName(), c.typ)
			}
			// The BSON key must be ListName, never the gen ListVariableName.
			var hasListName, hasWrong bool
			for _, p := range e.Properties() {
				switch p.Name() {
				case "ListName":
					hasListName = true
				case "ListVariableName", "SecondListOrObjectVariableName":
					hasWrong = true
				}
			}
			if !hasListName {
				var names []string
				for _, p := range e.Properties() {
					names = append(names, p.Name())
				}
				t.Errorf("%s: missing ListName property (got %v)", c.name, names)
			}
			if hasWrong {
				t.Errorf("%s: emits gen storage key instead of verified key", c.name)
			}
		})
	}
}

// TestMicroflowActionToGen_ExecuteDatabaseQuery guards 05: EXECUTE DATABASE QUERY
// must convert to a DatabaseConnector$ExecuteDatabaseQueryAction (else CE0008).
func TestMicroflowActionToGen_ExecuteDatabaseQuery(t *testing.T) {
	g := microflowActionToGen(&microflows.ExecuteDatabaseQueryAction{
		Query:              "DbTest.Conn.GetAll",
		OutputVariableName: "Rows",
		ParameterMappings:  []*microflows.DatabaseQueryParameterMapping{{ParameterName: "p", Value: "$x"}},
	})
	if g == nil {
		t.Fatal("nil action (CE0008 regression)")
	}
	if g.TypeName() != "DatabaseConnector$ExecuteDatabaseQueryAction" {
		t.Errorf("$Type = %q, want DatabaseConnector$ExecuteDatabaseQueryAction", g.TypeName())
	}
}

// TestMicroflowActionToGen_JavaScriptActionCall guards finding #11: a
// `call javascript action` (e.g. NanoflowCommons.OpenURL) must convert to a
// Microflows$JavaScriptActionCallAction. Before the fix the modelsdk engine had
// no case for it, so it returned nil → the ActionActivity was written without an
// Action (`-- Empty action`), failing the build with CE0008 "No action defined."
// and CE0109 on the now-missing output variable. This affects both microflows
// and nanoflows (nanoflows reuse microflowActionToGen).
func TestMicroflowActionToGen_JavaScriptActionCall(t *testing.T) {
	g := microflowActionToGen(&microflows.JavaScriptActionCallAction{
		JavaScriptAction:   "NanoflowCommons.OpenURL",
		OutputVariableName: "Opened",
		UseReturnVariable:  true,
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{Parameter: "NanoflowCommons.OpenURL.url", Value: &microflows.ExpressionBasedCodeActionParameterValue{Expression: "$Active/Link"}},
		},
	})
	if g == nil {
		t.Fatal("nil action (CE0008/CE0109 regression — JS action call dropped on write)")
	}
	if g.TypeName() != "Microflows$JavaScriptActionCallAction" {
		t.Errorf("$Type = %q, want Microflows$JavaScriptActionCallAction", g.TypeName())
	}
	// The whole subtree must encode cleanly (guards a missing list marker / mistyped property).
	if _, err := (&codec.Encoder{}).Encode(g); err != nil {
		t.Errorf("encode: %v", err)
	}
	// JS parameter mappings use the "ParameterValue" storage key, not the
	// Java-style "Value" — the read side and legacy serializer both key off it.
	var mappings []element.Element
	for _, p := range g.Properties() {
		if p.Name() == "ParameterMappings" {
			if lst, ok := p.(element.ChildListProperty); ok {
				mappings = lst.ChildElements()
			}
		}
	}
	if len(mappings) != 1 {
		t.Fatalf("ParameterMappings = %d, want 1", len(mappings))
	}
	var hasParameterValue, hasWrongValueKey bool
	for _, p := range mappings[0].Properties() {
		switch p.Name() {
		case "ParameterValue":
			hasParameterValue = true
		case "Value":
			hasWrongValueKey = true
		}
	}
	if !hasParameterValue {
		t.Error("mapping missing ParameterValue key (JS uses ParameterValue, not Value)")
	}
	if hasWrongValueKey {
		t.Error("mapping emits the Java-style Value key instead of ParameterValue")
	}
}

// TestActionFromGen_WorkflowActions guards FINDINGS #54: the modelsdk read path
// had no case for the workflow microflow actions (SET TASK OUTCOME / OPEN USER
// TASK / NOTIFY WORKFLOW), so DESCRIBE rendered them as "-- Empty action" and a
// describe→drop→exec round-trip silently dropped them. Each must survive a
// write→encode→decode→read round-trip (the real describe path) with its fields
// intact.
func TestActionFromGen_WorkflowActions(t *testing.T) {
	// readBack encodes the write-path gen element to BSON and decodes it through
	// the codec registry — the concrete *genMf.* type the read path sees — then
	// runs actionFromGen, exactly as DESCRIBE does.
	readBack := func(t *testing.T, action microflows.MicroflowAction) microflows.MicroflowAction {
		t.Helper()
		raw, err := (&codec.Encoder{}).Encode(microflowActionToGen(action))
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(raw))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return actionFromGen(el)
	}

	setOutcome := &microflows.SetTaskOutcomeAction{OutcomeValue: "Approve", WorkflowTaskVariable: "UserTask"}
	setOutcome.ID = "id-set"
	if got, ok := readBack(t, setOutcome).(*microflows.SetTaskOutcomeAction); !ok {
		t.Fatalf("SetTaskOutcome read back as %T (Empty-action regression)", readBack(t, setOutcome))
	} else if got.OutcomeValue != "Approve" || got.WorkflowTaskVariable != "UserTask" {
		t.Errorf("SetTaskOutcome fields lost: %+v", got)
	}

	openTask := &microflows.OpenUserTaskAction{UserTaskVariable: "UserTask"}
	openTask.ID = "id-open"
	if got, ok := readBack(t, openTask).(*microflows.OpenUserTaskAction); !ok {
		t.Fatalf("OpenUserTask read back as %T", readBack(t, openTask))
	} else if got.UserTaskVariable != "UserTask" {
		t.Errorf("OpenUserTask field lost: %+v", got)
	}

	notify := &microflows.NotifyWorkflowAction{WorkflowVariable: "Wf"}
	notify.ID = "id-notify"
	if got, ok := readBack(t, notify).(*microflows.NotifyWorkflowAction); !ok {
		t.Fatalf("NotifyWorkflow read back as %T (Empty-action regression)", readBack(t, notify))
	} else if got.WorkflowVariable != "Wf" {
		t.Errorf("NotifyWorkflow WorkflowVariable lost: %+v", got)
	}
}

// TestMicroflowActionToGen_JavaScriptActionEntityTypeParameter pins the BSON the
// write path produces for a JavaScript-action entity-type parameter
// (mendixlabs/mxcli#1137). The reported symptom was an `mxcli bson dump`
// showing
//
//	"$Type": "Microflows$BasicCodeActionParameterValue",
//	"Argument": "CustomModule.BufferDefinition"
//
// where Studio Pro stores $Type Microflows$EntityTypeCodeActionParameterValue
// with the entity under the "Entity" key. The wrong shape leaves the Studio Pro
// entity picker empty and fails mxbuild with CE0115 (measured on 11.6.6).
// The builder chooses the value (mdl/executor); this asserts the chosen value
// survives to BSON under the right $Type and key rather than being flattened on
// the way out.
func TestMicroflowActionToGen_JavaScriptActionEntityTypeParameter(t *testing.T) {
	g := microflowActionToGen(&microflows.JavaScriptActionCallAction{
		JavaScriptAction: "NanoflowCommons.RefreshEntity",
		ParameterMappings: []*microflows.JavaScriptActionParameterMapping{
			{
				Parameter: "NanoflowCommons.RefreshEntity.EntityToRefresh",
				Value:     &microflows.EntityTypeCodeActionParameterValue{Entity: "CustomModule.BufferDefinition"},
			},
		},
	})
	if g == nil {
		t.Fatal("nil action")
	}
	encoded, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	mappings, err := bson.Raw(encoded).LookupErr("ParameterMappings")
	if err != nil {
		t.Fatalf("no ParameterMappings in encoded action: %v", err)
	}
	vals, err := mappings.Array().Values()
	if err != nil {
		t.Fatalf("ParameterMappings array: %v", err)
	}
	// Element 0 is Mendix's typed-array marker (an int), not a mapping.
	var docs []bson.Raw
	for _, v := range vals {
		if doc, ok := v.DocumentOK(); ok {
			docs = append(docs, doc)
		}
	}
	if len(docs) != 1 {
		t.Fatalf("ParameterMappings documents = %d, want 1", len(docs))
	}
	value, err := docs[0].LookupErr("ParameterValue")
	if err != nil {
		t.Fatalf("mapping has no ParameterValue: %v", err)
	}
	valueDoc := value.Document()

	if got := valueDoc.Lookup("$Type").StringValue(); got != "Microflows$EntityTypeCodeActionParameterValue" {
		t.Fatalf("$Type = %q, want Microflows$EntityTypeCodeActionParameterValue", got)
	}
	if got := valueDoc.Lookup("Entity").StringValue(); got != "CustomModule.BufferDefinition" {
		t.Errorf("Entity = %q, want CustomModule.BufferDefinition", got)
	}
	if _, err := valueDoc.LookupErr("Argument"); err == nil {
		t.Error("value carries an Argument key; that is the BasicCodeActionParameterValue shape")
	}
}
