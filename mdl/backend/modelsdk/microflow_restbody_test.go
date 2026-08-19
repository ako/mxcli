// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestRestRequestHandling_MappingVariableName guards the REST `body mapping X
// from $var` clause. The export-mapping source variable is stored under
// "MappingVariableName"; reading the wrong "ParameterVariable" key left it empty,
// so the renderer emitted `body mapping X` without the grammar-mandatory
// `from $var` — invalid MDL that broke the DESCRIBE roundtrip.
func TestRestRequestHandling_MappingVariableName(t *testing.T) {
	doc := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "rq-1"},
		{Key: "$Type", Value: "Microflows$MappingRequestHandling"},
		{Key: "ContentType", Value: "Json"},
		{Key: "MappingId", Value: "DatahubAPI.EM_EventRequest"},
		{Key: "MappingVariableName", Value: "eventRequest"},
	})
	h, ok := restRequestHandlingFromRaw(doc).(*microflows.MappingRequestHandling)
	if !ok {
		t.Fatalf("restRequestHandlingFromRaw → not a MappingRequestHandling")
	}
	if string(h.MappingID) != "DatahubAPI.EM_EventRequest" {
		t.Errorf("MappingID = %q", h.MappingID)
	}
	if h.ParameterVariable != "eventRequest" {
		t.Errorf("ParameterVariable = %q, want eventRequest (from MappingVariableName; empty → invalid 'body mapping' with no 'from')", h.ParameterVariable)
	}
}

// propNames lists an element's BSON keys, for asserting on what a writer emits.
func propNames(e interface{ Properties() []element.Property }) []string {
	var out []string
	for _, p := range e.Properties() {
		out = append(out, p.Name())
	}
	return out
}

// The WRITE side of the same clause. The reader above has known since #843 that
// Mendix stores the variable as "MappingVariableName" — the writer never did,
// and emitted "ParameterVariable" instead.
//
// generated/metamodel is decisive: MicroflowsMappingRequestHandling has exactly
// three properties — contentType, mappingId, mappingVariableName. So mxcli both
// omitted a real property and wrote one the type does not own, and per the
// overlay rule an unknown property is the shape mxbuild tolerates and Studio Pro
// refuses to open. An empty ContentType is not a valid enum member either
// (Json | Xml).
//
// Measured against a Studio Pro microflow (ako/TestApp, 11.13.0):
//
//	{ContentType: 'Json', MappingId: 'Mappings.Export_mapping',
//	 MappingVariableName: 'NewItem'}
func TestRestRequestHandling_WritesMappingVariableName(t *testing.T) {
	e := restRequestHandlingToGen(&microflows.MappingRequestHandling{
		MappingID:         "Mappings.Export_mapping",
		ParameterVariable: "NewItem",
	})
	if e == nil {
		t.Fatal("restRequestHandlingToGen returned nil")
	}
	names := propNames(e)
	var hasRight, hasWrong bool
	for _, n := range names {
		switch n {
		case "MappingVariableName":
			hasRight = true
		case "ParameterVariable":
			hasWrong = true
		}
	}
	if !hasRight {
		t.Errorf("missing MappingVariableName; got %v", names)
	}
	if hasWrong {
		t.Errorf("emits ParameterVariable, which MicroflowsMappingRequestHandling does not own; got %v", names)
	}
}

// The discriminator has to agree with the sub-element. It was hardcoded to
// "Custom" for every handler, so an export-mapping body claimed to be a custom
// template. Studio Pro writes Mapping / FormData / Binary / Custom to match.
func TestRestCallAction_RequestHandlingTypeMatchesHandler(t *testing.T) {
	for _, tc := range []struct {
		name string
		rh   microflows.RequestHandling
		want string
	}{
		{"custom", &microflows.CustomRequestHandling{Template: "x"}, "Custom"},
		{"mapping", &microflows.MappingRequestHandling{MappingID: "M.EMM"}, "Mapping"},
		{"binary", &microflows.BinaryRequestHandling{Expression: "$D/Contents"}, "Binary"},
		{"formdata", &microflows.FormDataRequestHandling{}, "FormData"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := restCallActionToGen(&microflows.RestCallAction{
				HttpConfiguration: &microflows.HttpConfiguration{HttpMethod: microflows.HttpMethodPost},
				RequestHandling:   tc.rh,
				ResultHandling:    &microflows.ResultHandlingNone{},
			})
			got := ""
			for _, p := range e.Properties() {
				if p.Name() == "RequestHandlingType" {
					if w, ok := p.(element.WritableProperty); ok {
						got, _ = w.BSONValue().(string)
					}
				}
			}
			if got != tc.want {
				t.Errorf("RequestHandlingType = %q, want %q", got, tc.want)
			}
		})
	}
}
