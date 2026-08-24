// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"errors"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	mdltypes "github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// issue #242. `rest call … returns mapping M.IMM as Module.Entity` built a
// document every static check accepts — mxcli check, mx check 0 errors, mxbuild
// clean — and that throws the moment the activity runs:
//
//	MicroflowException: key not found: Path(QName(None,),None,)
//	  at com.mendix.integration.importer.mapping.MappingCache.storeValueMappingElement
//
// The REST branch mirrored BOTH ImportMappingCall flags onto the result
// variable's cardinality. They are not that axis:
//
//	ForceSingleOccurrence  "the mapping yields many, bind one"
//	Range.SingleObject     Studio Pro's First/All range
//
// Measured on 11.13.0 over an object-rooted mapping, all four combinations,
// same mapping and same JSON in one boot:
//
//	FSO=true  Range=true   FAIL  key not found: Path(QName(None,),None,)  <- shipped
//	FSO=true  Range=false  FAIL  key not found: Path(QName(None,),None,)
//	FSO=false Range=false  PASS  imported
//	FSO=false Range=true   PASS  imported
//
// So ForceSingleOccurrence alone drives the exception — which is why "write both
// range pointers false", the obvious reading of the sibling fix in #192, is the
// right outcome for the wrong reason: blanket-false also strips the flag from
// PrivateCloudData.REST_GetEnvironmentByUUID, a Studio Pro document that binds
// one object out of a LIST-rooted mapping and stores ForceSingleOccurrence=true.
// The flag is therefore inferred from the mapping's root, not from the call site.

func restMappingBuilder(root *mdltypes.JsonStructure, mappingErr error) *flowBuilder {
	return &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		varTypes:     map[string]string{},
		declaredVars: map[string]string{},
		measurer:     &layoutMeasurer{},
		backend: &mock.MockBackend{
			GetImportMappingByQualifiedNameFunc: func(string, string) (*model.ImportMapping, error) {
				if mappingErr != nil {
					return nil, mappingErr
				}
				return &model.ImportMapping{JsonStructure: "M.Payload"}, nil
			},
			GetJsonStructureByQualifiedNameFunc: func(string, string) (*mdltypes.JsonStructure, error) {
				return root, nil
			},
		},
	}
}

func jsonRoot(kind string) *mdltypes.JsonStructure {
	return &mdltypes.JsonStructure{Elements: []*mdltypes.JsonElement{{ElementType: kind}}}
}

func buildRestMapping(t *testing.T, fb *flowBuilder, isList bool) *microflows.ResultHandlingMapping {
	t.Helper()
	fb.addRestCallAction(&ast.RestCallStmt{
		OutputVariable: "Result",
		Method:         ast.HttpMethodGet,
		URL:            &ast.LiteralExpr{Kind: ast.LiteralString, Value: "https://example.com"},
		Result: ast.RestResult{
			Type:         ast.RestResultMapping,
			MappingName:  ast.QualifiedName{Module: "M", Name: "IMM"},
			ResultEntity: ast.QualifiedName{Module: "M", Name: "Token"},
			IsList:       isList,
		},
	})
	action, ok := fb.objects[0].(*microflows.ActionActivity).Action.(*microflows.RestCallAction)
	if !ok {
		t.Fatalf("built %T, want *microflows.RestCallAction", fb.objects[0].(*microflows.ActionActivity).Action)
	}
	h, ok := action.ResultHandling.(*microflows.ResultHandlingMapping)
	if !ok {
		t.Fatalf("ResultHandling is %T, want *microflows.ResultHandlingMapping", action.ResultHandling)
	}
	return h
}

// The reported case: an object-rooted mapping bound with `as Entity`. There is
// no occurrence to force, and forcing one is what the runtime refuses.
func TestRestMapping_ObjectRootedAsEntityDoesNotForceSingleOccurrence(t *testing.T) {
	h := buildRestMapping(t, restMappingBuilder(jsonRoot("Object"), nil), false)

	if h.ForceSingleOccurrence == nil || *h.ForceSingleOccurrence {
		t.Errorf("ForceSingleOccurrence = %v, want explicit false — true makes the "+
			"importer resolve an occurrence path that an object root does not have, "+
			"and the activity throws `key not found: Path(QName(None,),None,)`",
			h.ForceSingleOccurrence)
	}
	// The variable's cardinality is a separate axis and is untouched: `as Entity`
	// still binds an object, or mxbuild rejects the list with CE0243.
	if !h.SingleObject {
		t.Error("SingleObject = false, want true — `as Entity` binds one object")
	}
	if microflows.RangeSingleObjectOf(h) {
		t.Error("range SingleObject = true, want false — MDL's REST syntax has no " +
			"range keyword, so the range is All, which is what every reference " +
			"document stores")
	}
}

// PrivateCloudData.REST_GetEnvironmentByUUID: a LIST-rooted mapping bound to a
// single object. Here the flag is the whole point — it is what picks one out of
// many — and a fix that wrote false unconditionally would silently retype the
// activity on the next describe→exec round trip.
func TestRestMapping_ListRootedAsEntityKeepsForceSingleOccurrence(t *testing.T) {
	h := buildRestMapping(t, restMappingBuilder(jsonRoot("Array"), nil), false)

	if h.ForceSingleOccurrence == nil || !*h.ForceSingleOccurrence {
		t.Errorf("ForceSingleOccurrence = %v, want explicit true — binding one object "+
			"out of a list-rooted mapping is exactly what the flag means",
			h.ForceSingleOccurrence)
	}
	if !h.SingleObject {
		t.Error("SingleObject = false, want true — the call site said `as Entity`")
	}
	if microflows.RangeSingleObjectOf(h) {
		t.Error("range SingleObject = true, want false")
	}
}

// MendixSSO.RetrieveUserRoles: `as list of` binds the whole list, so nothing is
// forced to a single occurrence. This combination already worked and is the
// false-positive control — it must not move.
func TestRestMapping_AsListOfBindsTheListUnchanged(t *testing.T) {
	for _, root := range []string{"Array", "Object"} {
		h := buildRestMapping(t, restMappingBuilder(jsonRoot(root), nil), true)
		if h.SingleObject {
			t.Errorf("%s root: SingleObject = true, want false for `as list of`", root)
		}
		if h.ForceSingleOccurrence == nil || *h.ForceSingleOccurrence {
			t.Errorf("%s root: ForceSingleOccurrence = %v, want explicit false",
				root, h.ForceSingleOccurrence)
		}
		if microflows.RangeSingleObjectOf(h) {
			t.Errorf("%s root: range SingleObject = true, want false", root)
		}
	}
}

// A mapping the backend cannot resolve must read as object-rooted. Guessing
// "list" writes the flag that fails at RUN time; guessing "object" against a
// list-rooted mapping is caught by mxbuild as CE0243 at BUILD time. Between two
// wrong guesses, take the one someone finds.
func TestRestMapping_UnresolvableMappingDoesNotForceSingleOccurrence(t *testing.T) {
	h := buildRestMapping(t, restMappingBuilder(nil, errors.New("no such mapping")), false)
	if h.ForceSingleOccurrence == nil || *h.ForceSingleOccurrence {
		t.Errorf("ForceSingleOccurrence = %v, want explicit false when the mapping "+
			"cannot be resolved", h.ForceSingleOccurrence)
	}
}

// The occurrence bound is the fallback for mappings with no JSON structure (XML
// schema, message definition), where the root element's own MaxOccurs answers.
func TestMappingRootIsList_FallsBackToOccurrenceBound(t *testing.T) {
	fb := &flowBuilder{}
	for _, tc := range []struct {
		name      string
		maxOccurs int
		want      bool
	}{
		{"single", 1, false},
		{"unbounded", -1, true},
		{"bounded list", 5, true},
	} {
		im := &model.ImportMapping{Elements: []*model.ImportMappingElement{{MaxOccurs: tc.maxOccurs}}}
		if got := fb.mappingRootIsList(im); got != tc.want {
			t.Errorf("%s: mappingRootIsList = %v, want %v", tc.name, got, tc.want)
		}
	}
	if fb.mappingRootIsList(nil) {
		t.Error("nil mapping reported as list-rooted")
	}
}
