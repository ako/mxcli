// SPDX-License-Identifier: Apache-2.0

// Named action slots: addressing a pluggable widget's action property by its own
// key, for the slots that are not the click or change one.
//
// mxcli could author exactly two action slots — `onClick:` and `OnChange:`,
// matched by name after one Event/Action suffix is stripped. Every other
// action-typed property got no mapping, so the engine had nothing to write and
// `mxcli check` reported MDL-WIDGET06 ("recognized but not yet persisted"). File
// Uploader 2.5.0 extracts to six action slots and none of them were authorable:
// createFileAction, createImageAction, onUploadSuccess{File,Image},
// onUploadFailure{File,Image} — so the widget could be placed and not wired.
// Measured across one project's 42 widgets: 12 top-level slots had a surface and
// 4 did not, before counting the marketplace ones. (upstream #956)
//
// A named slot is a mapping with NO Source, addressed by its PropertyKey. That
// is the convention the object-list item mappings already use
// (`{"propertyKey": "action", "operation": "action"}` on a popupmenu item), so
// it introduces no new concept — and it leaves actionSourceForKey meaning
// exactly what it meant: the MDL alias for a key, or none.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// Every action-typed property gets a mapping. The click/change slots keep their
// Source (so `onClick:`/`OnChange:` still reach them); the rest are addressed by
// key.
func TestGenerateDefJSON_EveryActionSlotIsMapped(t *testing.T) {
	def := GenerateDefJSON(&mpk.WidgetDefinition{
		ID:   "com.mendix.widget.web.fileuploader.FileUploader",
		Name: "File uploader",
		Properties: []mpk.PropertyDef{
			{Key: "createFileAction", Type: "action"},
			{Key: "onUploadSuccessFile", Type: "action"},
			{Key: "onClickEvent", Type: "action"},
		},
	}, "FILEUPLOADER")

	got := map[string]string{}
	for _, m := range def.PropertyMappings {
		if m.Operation == "action" {
			got[m.PropertyKey] = m.Source
		}
	}

	want := map[string]string{
		"createFileAction":    "", // named slot — addressed by key
		"onUploadSuccessFile": "",
		"onClickEvent":        "OnClick", // keeps the MDL alias
	}
	if len(got) != len(want) {
		t.Fatalf("action mappings = %v, want %v", got, want)
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("mapping for %q has source %q, want %q", k, got[k], w)
		}
	}
}

// A named slot must not become a KnownProperty: that list is what MDL-WIDGET06
// warns "recognized but not yet persisted" about, and a slot that now persists
// would be warned about while working.
func TestGenerateDefJSON_NamedActionSlotIsNotReportedUnpersisted(t *testing.T) {
	def := GenerateDefJSON(&mpk.WidgetDefinition{
		ID:         "com.example.W",
		Name:       "W",
		Properties: []mpk.PropertyDef{{Key: "createFileAction", Type: "action"}},
	}, "W")

	for _, k := range def.KnownProperties {
		if k == "createFileAction" {
			t.Errorf("createFileAction is listed as a known-but-unmapped property — "+
				"MDL-WIDGET06 would warn that a slot mxcli now writes will be dropped; "+
				"knownProperties = %v", def.KnownProperties)
		}
	}
}

// --- reading the AST value for a named slot ---

func namedSlotAction(t *testing.T, propValue any) *ast.ActionV3 {
	t.Helper()
	w := &ast.WidgetV3{Name: "fu", Properties: map[string]any{"createFileAction": propValue}}
	got, err := namedActionSlotValue(w, "createFileAction")
	if err != nil {
		t.Fatalf("namedActionSlotValue: %v", err)
	}
	return got
}

// The straightforward case: the value parsed as an action.
func TestResolveMapping_NamedSlotReadsAnAction(t *testing.T) {
	got := namedSlotAction(t, &ast.ActionV3{Type: "microflow", Target: "M.ACT_CreateFile"})
	if got == nil {
		t.Fatal("no action resolved for createFileAction — the slot is addressed by key " +
			"and the mapping carries no Source, so nothing else can find the value")
	}
	if got.Type != "microflow" || got.Target != "M.ACT_CreateFile" {
		t.Errorf("resolved %+v, want microflow M.ACT_CreateFile", got)
	}
}

// `createFileAction: microflow M.X` parses as a DATASOURCE, because
// dataSourceExprV3 and actionExprV3 overlap on MICROFLOW/NANOFLOW/VARIABLE and
// the datasource alternative comes first in widgetPropertyV3. The grammar cannot
// tell them apart — only the def can, which is the same split fragmentArgValue
// already resolves in the executor ("the executor disambiguates using the
// parameter's declared kind").
func TestResolveMapping_NamedSlotAcceptsTheMicroflowFormThatParsesAsADataSource(t *testing.T) {
	cases := []struct {
		name string
		ds   *ast.DataSourceV3
		want ast.ActionV3
	}{{
		name: "microflow",
		ds:   &ast.DataSourceV3{Type: "microflow", Reference: "M.ACT_CreateFile"},
		want: ast.ActionV3{Type: "microflow", Target: "M.ACT_CreateFile"},
	}, {
		name: "nanoflow — FileUploader's own createFileAction default is a CallNanoflow",
		ds:   &ast.DataSourceV3{Type: "nanoflow", Reference: "FileUploader.ACT_Create"},
		want: ast.ActionV3{Type: "nanoflow", Target: "FileUploader.ACT_Create"},
	}, {
		name: "with arguments",
		ds: &ast.DataSourceV3{Type: "microflow", Reference: "M.ACT_CreateFile",
			Args: []ast.FlowArgV3{{Name: "Doc", Value: "$currentObject"}}},
		want: ast.ActionV3{Type: "microflow", Target: "M.ACT_CreateFile",
			Args: []ast.FlowArgV3{{Name: "Doc", Value: "$currentObject"}}},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := namedSlotAction(t, tc.ds)
			if got == nil {
				t.Fatal("a microflow/nanoflow written on a named action slot resolved to " +
					"nothing — it parses as a DataSourceV3 and the slot must accept that shape")
			}
			if got.Type != tc.want.Type || got.Target != tc.want.Target {
				t.Errorf("resolved %s %s, want %s %s", got.Type, got.Target, tc.want.Type, tc.want.Target)
			}
			if len(got.Args) != len(tc.want.Args) {
				t.Fatalf("resolved %d args, want %d — argument bindings are half of what "+
					"the slot is for", len(got.Args), len(tc.want.Args))
			}
			for i := range tc.want.Args {
				if got.Args[i].Name != tc.want.Args[i].Name {
					t.Errorf("arg %d = %q, want %q", i, got.Args[i].Name, tc.want.Args[i].Name)
				}
			}
		})
	}
}

// A datasource shape that is NOT a flow call is not an action. `database` and
// `association` sources have no action equivalent, and silently turning one into
// an empty action would write a NoAction over whatever the slot held.
func TestResolveMapping_NamedSlotRejectsANonFlowDataSource(t *testing.T) {
	for _, ds := range []*ast.DataSourceV3{
		{Type: "database", Reference: "M.Entity"},
		{Type: "association", Reference: "M.Assoc"},
		{Type: "selection", Reference: "grid1"},
	} {
		w := &ast.WidgetV3{Name: "fu", Properties: map[string]any{"createFileAction": ds}}
		_, err := namedActionSlotValue(w, "createFileAction")
		if err == nil {
			t.Errorf("a %q source on an action slot was accepted — it has no action form, "+
				"so accepting it writes an empty action over the slot", ds.Type)
		}
	}
}

// An unset slot resolves to no action rather than an empty one: SetAction is
// nil-guarded, and writing a NoAction would clear a slot the script never named.
func TestResolveMapping_NamedSlotUnsetResolvesToNothing(t *testing.T) {
	w := &ast.WidgetV3{Name: "fu", Properties: map[string]any{}}
	got, err := namedActionSlotValue(w, "createFileAction")
	if err != nil {
		t.Fatalf("namedActionSlotValue: %v", err)
	}
	if got != nil {
		t.Errorf("an unset slot resolved to %+v, want nothing", got)
	}
}

// --- what the validator accepts ---

// A named slot's own key IS its authorable name, so the validator must allow it.
// The rule it fell foul of — "an action mapping reads a fixed AST slot, so its
// storage key is not authorable" — is true only of a mapping that HAS a Source.
func TestValidatePluggableWidgetProperties_NamedActionSlotIsAuthorable(t *testing.T) {
	def := &WidgetDefinition{
		WidgetID: "com.mendix.widget.web.datagrid.Datagrid",
		MDLName:  "DATAGRID",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "onClick", Source: "OnClick", Operation: "action"},
			{PropertyKey: "onSelectionChange", Operation: "action"}, // named slot
		},
	}
	reg := &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{"DATAGRID": def}}

	w := &ast.WidgetV3{Name: "dg", Type: "datagrid", Properties: map[string]any{
		"onSelectionChange": &ast.ActionV3{Type: "microflow", Target: "M.ACT_Selected"},
	}}
	for _, v := range validatePluggableWidgetProperties(w, reg, "page M.P") {
		t.Errorf("named action slot rejected — its own key is the only way to write it:\n  [%s] %s",
			v.RuleID, v.Message)
	}

	// The aliased slot's storage key stays unauthorable: `onClick:` writes it,
	// `onClick` as a raw key would be read from nowhere.
	wStorage := &ast.WidgetV3{Name: "dg", Type: "datagrid", Properties: map[string]any{
		"onClick": "nope",
	}}
	var flagged bool
	for _, v := range validatePluggableWidgetProperties(wStorage, reg, "page M.P") {
		if v.RuleID == "MDL-WIDGET01" {
			flagged = true
		}
	}
	if !flagged {
		t.Error("a SOURCED action slot's storage key must still be refused — it reads " +
			"from the fixed AST slot, so a value written under the storage key is dropped")
	}
}

// The engine has to CALL the reader. namedActionSlotValue can be perfect and the
// slot still write nothing if resolveMapping never consults it — which is what
// happened: stubbing the call out left every other test in this file green.
func TestResolveMapping_EngineResolvesTheNamedSlotIntoAClientAction(t *testing.T) {
	// A close-page action, so the assertion is about the engine calling the
	// reader rather than about resolving a microflow against a backend.
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	w := &ast.WidgetV3{Name: "fu", Properties: map[string]any{
		"createFileAction": &ast.ActionV3{Type: "close"},
	}}

	ctx, err := e.resolveMapping(
		PropertyMapping{PropertyKey: "createFileAction", Operation: "action"}, w)
	if err != nil {
		t.Fatalf("resolveMapping: %v", err)
	}
	if ctx.Action == nil {
		t.Fatal("resolveMapping produced no client action for a named slot — " +
			"applyOperation would then call SetAction(key, nil) and write nothing")
	}
	if _, ok := ctx.Action.(*pages.ClosePageClientAction); !ok {
		t.Fatalf("client action is %T, want *pages.ClosePageClientAction", ctx.Action)
	}
}

// A mapping WITH a Source must not take the named-slot path: it reads a fixed
// AST slot, and reading its storage key instead would make `onClick:` stop
// working the moment a script also carried a property of that name.
func TestResolveMapping_SourcedActionMappingIsNotANamedSlot(t *testing.T) {
	e := &PluggableWidgetEngine{pageBuilder: &pageBuilder{}}
	w := &ast.WidgetV3{Name: "dg", Properties: map[string]any{
		"onClick": &ast.ActionV3{Type: "microflow", Target: "M.WrongOne"},
	}}

	ctx, err := e.resolveMapping(
		PropertyMapping{PropertyKey: "onClick", Source: "OnClick", Operation: "action"}, w)
	if err != nil {
		t.Fatalf("resolveMapping: %v", err)
	}
	if ctx.Action != nil {
		t.Errorf("a sourced mapping read the storage key and produced %v — it must read "+
			"the Action AST slot, which `onClick:`/`Action:` writes", ctx.Action)
	}
}
