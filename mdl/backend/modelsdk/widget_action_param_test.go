// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Bug 1: a widget action calling a microflow with arguments must serialize its
// parameter mappings. clientActionToGen dropped them (built MicroflowSettings
// from the name only), so a parameterized button/row invoked its microflow with
// no argument and silently no-op'd at runtime, while mx check stayed clean.
func TestClientActionToGen_MicroflowParameterMappings(t *testing.T) {
	action := &pages.MicroflowClientAction{
		BaseElement:   model.BaseElement{TypeName: "Forms$MicroflowAction"},
		MicroflowName: "M.ACT_Submit",
		ParameterMappings: []*pages.MicroflowParameterMapping{
			{ParameterName: "Form", Variable: "$Form"},    // variable ref
			{ParameterName: "Count", Expression: "1 + 2"}, // expression
		},
	}

	el, err := clientActionToGen(action)
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	act, ok := el.(*genPg.MicroflowClientAction)
	if !ok {
		t.Fatalf("got %T, want *genPg.MicroflowClientAction", el)
	}
	settings, ok := act.MicroflowSettings().(*genPg.MicroflowSettings)
	if !ok {
		t.Fatalf("MicroflowSettings is %T, want *genPg.MicroflowSettings", act.MicroflowSettings())
	}
	items := settings.ParameterMappingsItems()
	if len(items) != 2 {
		t.Fatalf("ParameterMappings count = %d, want 2 (mappings were dropped)", len(items))
	}

	got := map[string]string{} // Parameter BY_NAME → Expression
	for _, it := range items {
		pm, ok := it.(*genPg.MicroflowParameterMapping)
		if !ok {
			t.Fatalf("mapping is %T, want *genPg.MicroflowParameterMapping", it)
		}
		got[pm.ParameterQualifiedName()] = pm.Expression()
	}
	if got["M.ACT_Submit.Form"] != "$Form" {
		t.Errorf("Form mapping = %q, want expression $Form (BY_NAME M.ACT_Submit.Form)", got["M.ACT_Submit.Form"])
	}
	if got["M.ACT_Submit.Count"] != "1 + 2" {
		t.Errorf("Count mapping = %q, want expression '1 + 2'", got["M.ACT_Submit.Count"])
	}
}

// No arguments → an empty parameter-mapping list (still valid; datasource callers
// pass nil the same way).
func TestClientActionToGen_MicroflowNoParams(t *testing.T) {
	el, err := clientActionToGen(&pages.MicroflowClientAction{MicroflowName: "M.ACT_NoArg"})
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	settings := el.(*genPg.MicroflowClientAction).MicroflowSettings().(*genPg.MicroflowSettings)
	if n := len(settings.ParameterMappingsItems()); n != 0 {
		t.Errorf("ParameterMappings count = %d, want 0", n)
	}
}
