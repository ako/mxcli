// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// A data view over a flow the project does not contain — Feedback v4.0.2's
// excluded ShareFeedback_Logo, over FeedbackModule.DS_FeedbackForm — has no
// entity in scope for DESCRIBE or for exec. Every binding inside it still
// carries its full name in storage (`FeedbackModule.Feedback.Subject`), but
// DESCRIBE printed the bare attribute, which exec cannot qualify: written
// anyway, a bare `ImageB64` image parameter left a project `mx check` could not
// LOAD. So inside such a container the qualified name is kept.
func unresolvedContextDataView(ds map[string]any) map[string]any {
	attrRef := func(qn string) map[string]any {
		return map[string]any{"$Type": "DomainModels$AttributeRef", "Attribute": qn, "EntityRef": nil}
	}
	return map[string]any{
		"$Type":      "Forms$DataView",
		"Name":       "dataView5",
		"DataSource": ds,
		"Widgets": []any{int32(2),
			map[string]any{"$Type": "Forms$TextBox", "Name": "feedback_subject",
				"AttributeRef": attrRef("FeedbackModule.Feedback.Subject")},
			map[string]any{"$Type": "Forms$DynamicText", "Name": "text1",
				"Content": map[string]any{"$Type": "Forms$ClientTemplate",
					"Template": map[string]any{"$Type": "Texts$Text", "Items": []any{int32(3),
						map[string]any{"$Type": "Texts$Translation", "LanguageCode": "en_US", "Text": "Image: {1}"}}},
					"Parameters": []any{int32(2),
						map[string]any{"$Type": "Forms$ClientTemplateParameter", "AttributeRef": attrRef("FeedbackModule.Feedback.ImageB64")}},
				}},
			map[string]any{"$Type": "Forms$TextBox", "Name": "textBox1",
				"AttributeRef": attrRef("FeedbackModule.Feedback.SubmitterEmail"),
				"ConditionalVisibilitySettings": map[string]any{
					"$Type":     "Forms$ConditionalVisibilitySettings",
					"Attribute": "FeedbackModule.Feedback._showEmail",
					"Conditions": []any{int32(2),
						map[string]any{"$Type": "Enumerations$Condition", "AttributeValue": "true", "EditableVisible": true},
						map[string]any{"$Type": "Enumerations$Condition", "AttributeValue": "false", "EditableVisible": false}},
				}},
		},
	}
}

func describeWidget(t *testing.T, w map[string]any) string {
	t.Helper()
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	for _, rw := range parseRawWidget(ctx, w) {
		outputWidgetMDLV3(ctx, rw, 1)
	}
	return buf.String()
}

func TestDescribe_UnresolvedFlowContext_KeepsQualifiedBindings(t *testing.T) {
	got := describeWidget(t, unresolvedContextDataView(map[string]any{
		"$Type": "Forms$NanoflowSource", "Nanoflow": "FeedbackModule.DS_FeedbackForm",
	}))
	for _, want := range []string{
		"Attribute: FeedbackModule.Feedback.Subject",
		"{1} = FeedbackModule.Feedback.ImageB64",
		"Attribute: FeedbackModule.Feedback.SubmitterEmail",
		"Visible: FeedbackModule.Feedback._showEmail in (true)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output lacks %q — a bare binding under an unresolvable flow cannot be qualified on exec:\n%s", want, got)
		}
	}
	if _, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + got + "}\n"); len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, got)
	}
}

// Where the entity IS known the short form is unchanged.
func TestDescribe_ResolvedContext_KeepsShortBindings(t *testing.T) {
	got := describeWidget(t, unresolvedContextDataView(map[string]any{
		"$Type":     "Forms$DataViewSource",
		"EntityRef": map[string]any{"$Type": "DomainModels$DirectEntityRef", "Entity": "FeedbackModule.Feedback"},
		"SourceVariable": map[string]any{"$Type": "Forms$PageVariable", "PageParameter": "Feedback"},
	}))
	for _, want := range []string{"Attribute: Subject", "{1} = ImageB64", "Visible: _showEmail in (true)"} {
		if !strings.Contains(got, want) {
			t.Errorf("describe output lacks %q:\n%s", want, got)
		}
	}
}
