// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	mdltypes "github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// upstream #881. An import activity's Range — Studio Pro's All / First / Custom
// — was neither authorable nor described, so all three settings round-tripped
// identically and describe→edit→exec silently rewrote the activity.
//
// The trap this file guards is the one the first fix fell into: the Range and
// the RESULT VARIABLE's cardinality are SEPARATE axes. Conflating them made
// `all` write a ListType against an object-rooted mapping, which mxbuild rejects
// with CE0243 ("the mapping used to return 'List of X' but now returns 'X'").
// Mendix's own FeedbackModule.SUB_Feedback_PostToAppInsights is the proof: it
// stores ConstantRange{SingleObject:false} against an ObjectType variable.

func importRangeBuilder(rootIsArray bool) *flowBuilder {
	return &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{},
		backend: &mock.MockBackend{
			GetImportMappingByQualifiedNameFunc: func(string, string) (*model.ImportMapping, error) {
				return &model.ImportMapping{JsonStructure: "M.Payload"}, nil
			},
			GetJsonStructureByQualifiedNameFunc: func(string, string) (*mdltypes.JsonStructure, error) {
				kind := "Object"
				if rootIsArray {
					kind = "Array"
				}
				return &mdltypes.JsonStructure{Elements: []*mdltypes.JsonElement{{ElementType: kind}}}, nil
			},
		},
	}
}

func buildImportRange(t *testing.T, rootIsArray bool, stmt *ast.ImportFromMappingStmt) *microflows.ResultHandlingMapping {
	t.Helper()
	fb := importRangeBuilder(rootIsArray)
	stmt.Mapping = ast.QualifiedName{Module: "M", Name: "IMM"}
	stmt.SourceVariable = "P"
	stmt.OutputVariable = "R"
	fb.addImportFromMappingAction(stmt)
	action, ok := fb.objects[0].(*microflows.ActionActivity).Action.(*microflows.ImportXmlAction)
	if !ok {
		t.Fatalf("built %T, want *microflows.ImportXmlAction", fb.objects[0].(*microflows.ActionActivity).Action)
	}
	if action.ResultHandling == nil {
		t.Fatal("ResultHandling nil")
	}
	return action.ResultHandling
}

// `all` sets the RANGE and nothing else. Against an object-rooted mapping the
// bound variable stays a single object — writing a list there is exactly the
// CE0243 the axis separation exists to prevent.
func TestImportRange_AllLeavesVariableCardinalityToTheMapping(t *testing.T) {
	h := buildImportRange(t, false, &ast.ImportFromMappingStmt{All: true})
	if microflows.RangeSingleObjectOf(h) {
		t.Error("range SingleObject = true, want false — `all` is Studio Pro's All")
	}
	if !h.SingleObject {
		t.Error("SingleObject = false, want true: an object-rooted mapping binds an " +
			"OBJECT even when the range is All (mxbuild rejects the list with CE0243)")
	}

	// The same statement against a list-rooted mapping binds a list.
	h = buildImportRange(t, true, &ast.ImportFromMappingStmt{All: true})
	if h.SingleObject {
		t.Error("SingleObject = true, want false for a list-rooted mapping")
	}
}

// `first` is the one form that DOES pin the variable: "first" means one object,
// not a one-element list, so it overrides the mapping's own shape.
func TestImportRange_FirstBindsASingleObjectEvenForAListMapping(t *testing.T) {
	h := buildImportRange(t, true, &ast.ImportFromMappingStmt{First: true})
	if !microflows.RangeSingleObjectOf(h) {
		t.Error("range SingleObject = false, want true — `first` is Studio Pro's First")
	}
	if !h.SingleObject {
		t.Error("SingleObject = false, want true — `first` binds one object")
	}
	if h.ForceSingleOccurrence == nil || !*h.ForceSingleOccurrence {
		t.Errorf("ForceSingleOccurrence = %v, want explicit true", h.ForceSingleOccurrence)
	}
}

// A limit or an offset selects Mendix's CustomRange. It is a range setting only:
// it must not drag the variable's cardinality with it, or an object-rooted
// mapping gets a ListType and CE0243 again.
func TestImportRange_LimitIsARangeNotACardinality(t *testing.T) {
	h := buildImportRange(t, false, &ast.ImportFromMappingStmt{
		LimitExpr:  &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: "10"},
		OffsetExpr: &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: "5"},
	})
	if h.LimitExpression != "10" || h.OffsetExpression != "5" {
		t.Errorf("limit/offset = %q/%q, want 10/5", h.LimitExpression, h.OffsetExpression)
	}
	if !h.SingleObject {
		t.Error("SingleObject = false, want true — the mapping is object-rooted, and the " +
			"range does not change what the mapping returns")
	}
}

// Saying nothing must keep writing what mxcli always wrote: cardinality inferred
// from the mapping's root, range mirroring it. Every hand-written script that
// predates the syntax depends on this.
func TestImportRange_UnauthoredKeepsTheOldInference(t *testing.T) {
	h := buildImportRange(t, true, &ast.ImportFromMappingStmt{})
	if h.RangeSingleObject != nil {
		t.Errorf("RangeSingleObject = %v, want nil (unauthored)", *h.RangeSingleObject)
	}
	if h.SingleObject {
		t.Error("SingleObject = true, want false — a list-rooted mapping still infers a list")
	}
	if microflows.RangeSingleObjectOf(h) {
		t.Error("the stored range must fall back to the inferred cardinality when unauthored")
	}
}

// DESCRIBE always emits one of the three forms — never nothing. Omitting it
// would leave the builder inferring on re-exec, and an object-rooted mapping set
// to All (Studio Pro's default, shipped in the blank app) would come back as
// First. That silent rewrite is what #881 reported.
func TestFormatImportMappingRange(t *testing.T) {
	first, no := true, false
	for _, tc := range []struct {
		name string
		h    *microflows.ResultHandlingMapping
		want string
	}{
		{"first", &microflows.ResultHandlingMapping{SingleObject: true, RangeSingleObject: &first}, " first"},
		{"all", &microflows.ResultHandlingMapping{SingleObject: false, RangeSingleObject: &no}, " all"},
		{
			// Mendix's own SUB_Feedback_PostToAppInsights: range All, variable an
			// object. Describing this as `first` — which reading SingleObject does —
			// changes the activity on re-exec.
			"all against an object variable",
			&microflows.ResultHandlingMapping{SingleObject: true, RangeSingleObject: &no},
			" all",
		},
		{"limit", &microflows.ResultHandlingMapping{LimitExpression: "10"}, " limit 10"},
		{"limit+offset", &microflows.ResultHandlingMapping{LimitExpression: "10", OffsetExpression: "5"}, " limit 10 offset 5"},
		{"offset only", &microflows.ResultHandlingMapping{OffsetExpression: "5"}, " offset 5"},
		{
			// No explicit range recorded (a document written before #881): fall back
			// to the variable's cardinality rather than emitting nothing.
			"legacy single object", &microflows.ResultHandlingMapping{SingleObject: true}, " first",
		},
	} {
		if got := formatImportMappingRange(tc.h); got != tc.want {
			t.Errorf("%s: formatImportMappingRange = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := formatImportMappingRange(nil); got != "" {
		t.Errorf("nil handling = %q, want empty", got)
	}
}
