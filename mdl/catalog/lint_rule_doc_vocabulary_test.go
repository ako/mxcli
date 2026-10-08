// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The bundled write-lint-rules skill is the ONLY documentation of the Starlark
// rule API, so its example tables are what a rule author copies. Nothing
// connected those tables to the values this package emits, and they drifted into
// fiction: `action_type` listed five Mendix *storage* names — CreateChangeAction,
// CommitAction, ShowFormAction, CloseFormAction, ShowHomeFormAction — that the
// catalog has never produced, and `source_type`, `target_type`, `element_type`,
// `access_type` and `data_type` were documented in the wrong case.
//
// Both failure modes are silent. A rule built from the documented values
// compiles, runs, matches nothing and reports a clean pass; nothing warns that
// the filter never had a chance. Measured on one real project, the action_type
// row inverted a shipped convention rule into 138 of 282 ACT_ microflows flagged,
// 49% false positives (mendixlabs/mxcli#1027), after which lint on that project
// was demoted to non-blocking and then stopped being run.
//
// mdl/catalog/lint_rule_vocabulary_test.go pins the bundled *rules* to these same
// vocabularies. This file pins the *documentation*, which is where the rule
// authors who are not in this repository get their values.

func lintRuleSkillDoc(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", ".claude", "skills", "mendix", "write-lint-rules", "SKILL.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// SKILL.md first, so a row it holds is the one docRowValues finds; then
	// the supporting files a long skill moves its detail into.
	extra, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.md"))
	for _, p := range extra {
		if filepath.Base(p) == "SKILL.md" {
			continue
		}
		more, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		b = append(append(b, '\n'), more...)
	}
	return string(b)
}

// docRowValues returns the quoted example values documented for a field, from
// the Markdown table row whose first cell is `field`. Every field this file
// checks appears in exactly one row.
//
// It fails rather than returning nothing: a renamed row would otherwise make
// every assertion below vacuously true, which is the same silent pass the bug is.
func docRowValues(t *testing.T, doc, field string) []string {
	t.Helper()
	row := regexp.MustCompile(`(?m)^\| ` + "`" + regexp.QuoteMeta(field) + "`" + ` \|.*$`).FindString(doc)
	if row == "" {
		t.Fatalf("no table row for %q in write-lint-rules/SKILL.md — the row was renamed or removed, "+
			"which silently disables this check", field)
	}
	var out []string
	for _, m := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(row, -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatalf("row for %q documents no example values: %s", field, row)
	}
	return out
}

// docSectionRowValues is docRowValues restricted to one "### <section>" of the
// doc, for a field name two structs share: widget.action_type and
// activity.action_type hold different vocabularies, and the first row in the
// file would otherwise answer for both.
func docSectionRowValues(t *testing.T, doc, section, field string) []string {
	t.Helper()
	start := strings.Index(doc, "\n### "+section+"\n")
	if start < 0 {
		t.Fatalf("no \"### %s\" section in write-lint-rules — renamed or removed, which silently disables this check", section)
	}
	body := doc[start+1:]
	if end := strings.Index(body[4:], "\n### "); end >= 0 {
		body = body[:end+4]
	}
	return docRowValues(t, body, field)
}

func assertDocumentedValuesExist(t *testing.T, field string, documented []string, real map[string]bool, produced string) {
	t.Helper()
	for _, v := range documented {
		if !real[v] {
			t.Errorf("write-lint-rules documents %s = %q, which %s never produces.\n"+
				"A rule filtering on it matches nothing and reports a clean pass.", field, v, produced)
		}
	}
}

// microflowActionLabels is every label getMicroflowActionType can return for a
// modelled action. The label IS the Go type name (the function derives it with
// %T), so the authoritative set is the set of types implementing MicroflowAction
// — which is spelled in exactly one place: the isMicroflowAction marker methods.
//
// Read from source rather than hand-listed, because a hand-list beside a doc list
// is two copies of the same table and drifts the same way the doc did.
func microflowActionLabels(t *testing.T) map[string]bool {
	t.Helper()
	path := filepath.Join("..", "..", "sdk", "microflows", "microflows_actions.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	labels := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "isMicroflowAction" || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		typ := fn.Recv.List[0].Type
		if star, ok := typ.(*ast.StarExpr); ok {
			typ = star.X
		}
		if id, ok := typ.(*ast.Ident); ok {
			labels[id.Name] = true
		}
	}

	// CONTROL: an AST walk that matched nothing would make every action_type
	// assertion pass. sdk/microflows models dozens of actions.
	if len(labels) < 40 {
		t.Fatalf("found only %d MicroflowAction implementors in %s — the scan is broken, "+
			"not the documentation", len(labels), path)
	}
	// CONTROL: the labeller must agree with the scan on a known action, or the
	// set is the right size and the wrong thing.
	if got := getMicroflowActionType(&microflows.ShowPageAction{}); !labels[got] {
		t.Fatalf("getMicroflowActionType returns %q, which the scan did not collect", got)
	}
	return labels
}

// TestSkillDocumentsRealActionTypes is the reported defect: every documented
// action_type was a storage name the catalog never emits.
func TestSkillDocumentsRealActionTypes(t *testing.T) {
	doc := lintRuleSkillDoc(t)
	assertDocumentedValuesExist(t, "action_type",
		docSectionRowValues(t, doc, "activity", "action_type"),
		microflowActionLabels(t),
		"getMicroflowActionType")
}

// TestSkillDocumentsRealActivityTypes covers the sibling column. An activity_type
// label is derived the same way, from the MicroflowObject's Go type.
func TestSkillDocumentsRealActivityTypes(t *testing.T) {
	doc := lintRuleSkillDoc(t)
	real := map[string]bool{}
	for _, obj := range []microflows.MicroflowObject{
		&microflows.ActionActivity{}, &microflows.ExclusiveSplit{}, &microflows.ExclusiveMerge{},
		&microflows.LoopedActivity{}, &microflows.InheritanceSplit{},
		&microflows.StartEvent{}, &microflows.EndEvent{}, &microflows.Annotation{},
	} {
		real[getMicroflowObjectType(obj)] = true
	}
	assertDocumentedValuesExist(t, "activity_type",
		docRowValues(t, doc, "activity_type"), real, "getMicroflowObjectType")
}

// TestSkillDocumentsRealRefObjectTypes covers the second half of the report:
// source_type and target_type were documented lower-case ("microflow", "page")
// and are emitted upper-case.
func TestSkillDocumentsRealRefObjectTypes(t *testing.T) {
	doc := lintRuleSkillDoc(t)

	set := func(vals []string) map[string]bool {
		m := map[string]bool{}
		for _, v := range vals {
			m[v] = true
		}
		return m
	}

	assertDocumentedValuesExist(t, "source_type",
		docRowValues(t, doc, "source_type"), set(RefSourceObjectTypes), "buildReferences")
	assertDocumentedValuesExist(t, "target_type",
		docRowValues(t, doc, "target_type"), set(RefTargetObjectTypes), "buildReferences")
}

// TestSkillDocumentsRealPermissionVocabulary covers permission.element_type and
// permission.access_type, which had the same lower-cased examples. They were not
// in the report — they are the same defect in the same tables, found by checking
// the siblings rather than only the two rows that were named.
func TestSkillDocumentsRealPermissionVocabulary(t *testing.T) {
	doc := lintRuleSkillDoc(t)

	set := func(vals []string) map[string]bool {
		m := map[string]bool{}
		for _, v := range vals {
			m[v] = true
		}
		return m
	}

	assertDocumentedValuesExist(t, "element_type",
		docRowValues(t, doc, "element_type"), set(PermissionElementTypes), "buildPermissions")
	assertDocumentedValuesExist(t, "access_type",
		docRowValues(t, doc, "access_type"), set(PermissionAccessTypes), "buildPermissions")
}

// TestSkillDocumentsRealAttributeDataTypes covers attribute.data_type, documented
// as "string"/"integer"/"datetime" and emitted as String/Integer/DateTime. This is
// the most-used filter in a lint rule, so it is the one whose silence costs most.
func TestSkillDocumentsRealAttributeDataTypes(t *testing.T) {
	doc := lintRuleSkillDoc(t)

	real := map[string]bool{}
	for _, at := range []domainmodel.AttributeType{
		&domainmodel.StringAttributeType{}, &domainmodel.IntegerAttributeType{},
		&domainmodel.LongAttributeType{}, &domainmodel.DecimalAttributeType{},
		&domainmodel.BooleanAttributeType{}, &domainmodel.DateTimeAttributeType{},
		&domainmodel.DateAttributeType{}, &domainmodel.EnumerationAttributeType{},
		&domainmodel.AutoNumberAttributeType{}, &domainmodel.BinaryAttributeType{},
		&domainmodel.HashedStringAttributeType{},
	} {
		real[at.GetTypeName()] = true
	}

	assertDocumentedValuesExist(t, "data_type",
		docRowValues(t, doc, "data_type"), real, "AttributeType.GetTypeName")
}

// TestSkillDocumentsRealRefKinds pins the ref_kind row both ways: whatever it
// documents must be a kind buildReferences emits, and every RefKind constant
// must be documented — read from the declarations, so a new kind (commit,
// ako/mxcli#963) cannot ship without a line a rule author can find.
func TestSkillDocumentsRealRefKinds(t *testing.T) {
	doc := lintRuleSkillDoc(t)

	declared := constantsWithPrefix(t, "builder_references.go", "RefKind")
	// CONTROL: a scan that collected nothing would pass whatever the row said.
	if len(declared) < 20 {
		t.Fatalf("found only %d RefKind* constants — the scan is broken", len(declared))
	}
	real := map[string]bool{}
	for _, v := range declared {
		real[v] = true
	}

	documented := docRowValues(t, doc, "ref_kind")
	assertDocumentedValuesExist(t, "ref_kind", documented, real, "buildReferences")

	for name, v := range declared {
		if !slices.Contains(documented, v) {
			t.Errorf("%s = %q is emitted but the skill's ref_kind row does not document it", name, v)
		}
	}
}

// TestEveryObjectTypeConstantIsInAPublishedList is the other half of the pin.
// The exported vocabularies are only authoritative if they are complete: a
// constant the builders emit but neither list names is a value no documentation
// can mention and no rule author can discover.
//
// Read from the declarations rather than hand-listed, so adding a constant
// without publishing it fails here instead of quietly narrowing the vocabulary.
func TestEveryObjectTypeConstantIsInAPublishedList(t *testing.T) {
	for _, tc := range []struct {
		file, prefix string
		published    []string
	}{
		{"builder_references.go", "RefObject", append(append([]string{}, RefSourceObjectTypes...), RefTargetObjectTypes...)},
		{"builder_permissions.go", "PermissionElement", PermissionElementTypes},
		{"builder_permissions.go", "AccessType", PermissionAccessTypes},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			known := map[string]bool{}
			for _, v := range tc.published {
				known[v] = true
			}

			declared := constantsWithPrefix(t, tc.file, tc.prefix)
			// CONTROL: a scan that collected nothing would pass this test whatever
			// the lists said.
			if len(declared) < 4 {
				t.Fatalf("found only %d %s* constants in %s — the scan is broken", len(declared), tc.prefix, tc.file)
			}
			for name, value := range declared {
				if !known[value] {
					t.Errorf("%s = %q is emitted but appears in no published vocabulary, "+
						"so nothing can document it", name, value)
				}
			}
		})
	}
}

// constantsWithPrefix returns the name→value pairs of the string constants in a
// file of this package whose name starts with prefix.
func constantsWithPrefix(t *testing.T, file, prefix string) map[string]string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}

	out := map[string]string{}
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			name := vs.Names[0].Name
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			out[name] = strings.Trim(lit.Value, `"`)
		}
	}
	return out
}

// The skill documented widget.container_type as "page" / "snippet"; the
// builder has only ever written "PAGE" / "SNIPPET", so a rule filtering on the
// documented value matched no widget. The real values are read from the
// ContainerType argument (the 6th) of every widgets_data insert.
func TestSkillDocumentsRealWidgetContainerTypes(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "builder_pages.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	real := map[string]bool{}
	ast.Inspect(parsed, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 6 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Exec" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "widgetStmt" {
			return true
		}
		if lit, ok := call.Args[5].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			real[strings.Trim(lit.Value, `"`)] = true
		}
		return true
	})
	// CONTROL: both a page and a snippet insert must have been found.
	if len(real) < 2 {
		t.Fatalf("found container types %v in builder_pages.go — the scan is broken", real)
	}
	assertDocumentedValuesExist(t, "container_type",
		docRowValues(t, lintRuleSkillDoc(t), "container_type"), real, "buildPages / buildSnippets")
}

// The activity property columns of mendixlabs/mxcli#1267 store Mendix's own
// spellings, and two of them are easy to get wrong from memory:
// CustomWithoutRollBack has a capital B, and retrieve_source is lower-case
// where every other activity vocabulary is PascalCase. A rule comparing to
// "CustomWithoutRollback" or "Database" matches nothing and passes silently, so
// each documented value is held to the set the builder writes.
func TestSkillDocumentsRealActivityPropertyVocabulary(t *testing.T) {
	doc := lintRuleSkillDoc(t)
	set := func(vals ...string) map[string]bool {
		m := map[string]bool{}
		for _, v := range vals {
			m[v] = true
		}
		return m
	}

	errorHandling := set(
		string(microflows.ErrorHandlingTypeRollback), string(microflows.ErrorHandlingTypeCustom),
		string(microflows.ErrorHandlingTypeCustomWithoutRollback), string(microflows.ErrorHandlingTypeContinue),
		string(microflows.ErrorHandlingTypeAbort))
	// CONTROL: the stored spelling, measured on TestApp. If the constant ever
	// changes, every rule documented against it changes meaning.
	if !errorHandling["CustomWithoutRollBack"] {
		t.Fatal("ErrorHandlingTypeCustomWithoutRollback is no longer the stored \"CustomWithoutRollBack\"")
	}
	documented := docRowValues(t, doc, "error_handling_type")
	assertDocumentedValuesExist(t, "error_handling_type", documented, errorHandling, "describeFlowObject")
	if !slices.Contains(documented, "CustomWithoutRollBack") {
		t.Error("error_handling_type row does not document CustomWithoutRollBack, the value a rule most often misspells")
	}

	assertDocumentedValuesExist(t, "log_level", docRowValues(t, doc, "log_level"), set(
		string(microflows.LogLevelTrace), string(microflows.LogLevelDebug), string(microflows.LogLevelInfo),
		string(microflows.LogLevelWarning), string(microflows.LogLevelError), string(microflows.LogLevelCritical)),
		"describeFlowObject")
	assertDocumentedValuesExist(t, "commit_type", docRowValues(t, doc, "commit_type"), set(
		string(microflows.CommitTypeYes), string(microflows.CommitTypeYesWithoutEvents), string(microflows.CommitTypeNo)),
		"describeFlowObject")
	assertDocumentedValuesExist(t, "retrieve_source", docRowValues(t, doc, "retrieve_source"),
		set(RetrieveSourceDatabase, RetrieveSourceAssociation), "describeFlowObject")

	// CONTROL: the builder writes exactly these retrieve sources, so the set
	// above is the writer's and not a copy of the documentation.
	for src, want := range map[microflows.RetrieveSource]string{
		&microflows.DatabaseRetrieveSource{}:    RetrieveSourceDatabase,
		&microflows.AssociationRetrieveSource{}: RetrieveSourceAssociation,
	} {
		r := describeFlowObject(&microflows.ActionActivity{Action: &microflows.RetrieveAction{Source: src}}, nil)
		if r.retrieveSource != want {
			t.Errorf("builder writes retrieve_source %q for %T, want %q", r.retrieveSource, src, want)
		}
	}
}

// widget.action_type is the RAW stored $Type of a widget's primary action
// (mendixlabs/mxcli#1268). The issue's own example compared it to
// "DeleteAction", which no widget stores — the storage name is
// "Forms$DeleteClientAction" — so a rule written from memory matched nothing
// and passed. Every documented value must be a client-action storage name the
// codec knows.
func TestSkillDocumentsRealWidgetActionTypes(t *testing.T) {
	documented := docSectionRowValues(t, lintRuleSkillDoc(t), "widget", "action_type")
	real := map[string]bool{"": true} // a widget with no action property
	for _, v := range documented {
		if _, ok := codec.DefaultRegistry.Lookup(v); ok && strings.HasPrefix(v, "Forms$") && strings.HasSuffix(v, "Action") {
			real[v] = true
		}
	}
	// CONTROL: the lookup must accept the stored name and refuse the
	// qualified-looking one, or the set above proves nothing.
	if _, ok := codec.DefaultRegistry.Lookup("Forms$DeleteClientAction"); !ok {
		t.Fatal("codec registry does not know Forms$DeleteClientAction — the lookup is broken")
	}
	if _, ok := codec.DefaultRegistry.Lookup("DeleteAction"); ok {
		t.Fatal("codec registry accepts the bare \"DeleteAction\" — the lookup is not a vocabulary")
	}
	assertDocumentedValuesExist(t, "widget.action_type", documented, real, "widgetPrimaryAction")
	if !slices.Contains(documented, "Forms$DeleteClientAction") {
		t.Error("widget.action_type does not document Forms$DeleteClientAction, the value the delete-button rule needs")
	}
}

// The skill tells rule authors that a delete action has no confirmation, so
// has_confirmation is always false for one, and that only microflow, nanoflow
// and workflow calls carry one. That is a fact about the metamodel, pinned here
// so that a Mendix version adding one to the delete action fails this test
// instead of leaving the documented rule quietly wrong.
func TestWidgetConfirmationOnlyOnFlowCalls(t *testing.T) {
	has := func(v any) bool {
		_, ok := reflect.TypeOf(v).MethodByName("ConfirmationInfo")
		return ok
	}
	if has(&pages.DeleteClientAction{}) {
		t.Error("Forms$DeleteClientAction now has a ConfirmationInfo — widgetPrimaryAction and the skill's delete rule need revisiting")
	}
	for name, v := range map[string]any{
		"MicroflowSettings":        &pages.MicroflowSettings{},
		"CallNanoflowClientAction": &pages.CallNanoflowClientAction{},
		"CallWorkflowClientAction": &pages.CallWorkflowClientAction{},
	} {
		if !has(v) {
			t.Errorf("%s has no ConfirmationInfo — widgetPrimaryAction reads it there", name)
		}
	}
}
