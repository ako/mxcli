// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A model with a boolean and an enumeration attribute, the second inherited —
// the Account_Overview shape (System.User.Active bound from a specialization).
func visibleWhenPB(entityContext string) *pageBuilder {
	const (
		modID  = model.ID("mod-m")
		baseID = model.ID("e-base")
		subID  = model.ID("e-sub")
	)
	return &pageBuilder{
		entityContext:    entityContext,
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "M"}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: modID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: baseID}, Name: "Job", Attributes: []*domainmodel.Attribute{
						{Name: "Status", Type: &domainmodel.EnumerationAttributeType{EnumerationRef: "M.EventStatus"}},
						{Name: "IsLocal", Type: &domainmodel.BooleanAttributeType{}},
						{Name: "Title", Type: &domainmodel.StringAttributeType{}},
					}},
					{BaseElement: model.BaseElement{ID: subID}, Name: "SpecialJob", GeneralizationRef: "M.Job"},
				},
			}},
			enumerations: []*model.Enumeration{{
				ContainerID: modID, Name: "EventStatus",
				Values: []model.EnumerationValue{{Name: "Running"}, {Name: "Completed"}, {Name: "Error"}},
			}},
		},
	}
}

func buildVisibleWhen(t *testing.T, pb *pageBuilder, attr string, values ...string) (*pages.ConditionalVisibilitySettings, error) {
	t.Helper()
	w := &ast.WidgetV3{Type: "container", Name: "c1", Properties: map[string]any{
		"VisibleWhen": &ast.VisibleWhenV3{Attribute: attr, Values: values},
	}}
	built, err := pb.buildWidgetV3(w)
	if err != nil {
		return nil, err
	}
	return built.(*pages.Container).ConditionalVisibility, nil
}

// Studio Pro stores EVERY value with its flag, in enumeration order, plus
// "(empty)" for an enumeration — measured on Administration.ScheduledEvents
// (Running true; Completed, Error, Stopped, (empty) false). A boolean gets
// "true" then "false" (Administration.Account_Edit, FeedbackModule.ShareFeedback).
func TestVisibleWhen_WritesEveryValue(t *testing.T) {
	cvs, err := buildVisibleWhen(t, visibleWhenPB("M.SpecialJob"), "Status", "Running", "empty")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if cvs == nil {
		t.Fatal("no ConditionalVisibilitySettings — the condition was dropped (widget always visible)")
	}
	if cvs.Attribute != "M.Job.Status" {
		t.Errorf("Attribute = %q, want M.Job.Status (the DECLARING entity)", cvs.Attribute)
	}
	got := []string{}
	for _, c := range cvs.Conditions {
		got = append(got, c.Value+"="+map[bool]string{true: "T", false: "F"}[c.Visible])
	}
	if want := "Running=T Completed=F Error=F (empty)=T"; strings.Join(got, " ") != want {
		t.Errorf("Conditions = %s, want %s", strings.Join(got, " "), want)
	}
	if cvs.Expression != "" {
		t.Errorf("Expression = %q, want empty", cvs.Expression)
	}

	cvs, err = buildVisibleWhen(t, visibleWhenPB("M.Job"), "IsLocal", "false")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	got = got[:0]
	for _, c := range cvs.Conditions {
		got = append(got, c.Value+"="+map[bool]string{true: "T", false: "F"}[c.Visible])
	}
	if want := "true=F false=T"; strings.Join(got, " ") != want {
		t.Errorf("boolean Conditions = %s, want %s", strings.Join(got, " "), want)
	}
}

func TestVisibleWhen_Refusals(t *testing.T) {
	for _, tc := range []struct {
		name, ctx, attr string
		values          []string
		want            string
	}{
		{"unknown value", "M.Job", "Status", []string{"Paused"}, "Paused"},
		{"not boolean or enum", "M.Job", "Title", []string{"x"}, "Boolean or enumeration"},
		{"unknown attribute", "M.Job", "Nope", []string{"true"}, "Nope"},
		{"no data container", "", "IsLocal", []string{"true"}, "data container"},
		{"empty on a boolean", "M.Job", "IsLocal", []string{"empty"}, "empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildVisibleWhen(t, visibleWhenPB(tc.ctx), tc.attr, tc.values...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// DESCRIBE reads the stored conditions back as the visible values.
func TestDescribe_VisibleWhen(t *testing.T) {
	stored := map[string]any{
		"$Type": "Forms$DivContainer",
		"Name":  "c1",
		"ConditionalVisibilitySettings": map[string]any{
			"$Type":     "Forms$ConditionalVisibilitySettings",
			"Attribute": "System.ScheduledEventInformation.Status",
			"Conditions": []any{int32(2),
				map[string]any{"$Type": "Enumerations$Condition", "AttributeValue": "Running", "EditableVisible": true},
				map[string]any{"$Type": "Enumerations$Condition", "AttributeValue": "Completed", "EditableVisible": false},
				map[string]any{"$Type": "Enumerations$Condition", "AttributeValue": "(empty)", "EditableVisible": true},
			},
			"Expression": "",
		},
		"Widgets": []any{int32(2)},
	}
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	raw := parseRawWidget(ctx, stored)
	outputWidgetMDLV3(ctx, raw[0], 1)
	got := buf.String()
	if !strings.Contains(got, `Visible: "Status" in (Running, empty)`) { // Status is a keyword, so quoted
		t.Errorf("describe output lacks the attribute condition:\n%s", got)
	}
	if _, errs := visitor.Build("create page M.P (Title: 'x', Layout: A.L) {\n" + got + "}\n"); len(errs) > 0 {
		t.Errorf("describe output does not parse: %v\n%s", errs, got)
	}
}
