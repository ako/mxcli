// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// fileUploaderBSON is the shape a Studio Pro-authored File Uploader 2.5.0 is
// stored in: its schema declares TWO datasource properties, `associatedFiles`
// and `associatedImages`, and in files mode only the first is configured.
func fileUploaderBSON() map[string]any {
	propType := func(id, key string) map[string]any {
		return map[string]any{
			"$ID":         id,
			"PropertyKey": key,
			"ValueType":   map[string]any{"Type": "DataSource"},
		}
	}
	return map[string]any{
		"$Type": "CustomWidgets$CustomWidget",
		"Name":  "upFiles",
		"Type": map[string]any{
			"WidgetId": "com.mendix.widget.web.fileuploader.FileUploader",
			"ObjectType": map[string]any{"PropertyTypes": []any{
				propType("pt-files", "associatedFiles"),
				propType("pt-images", "associatedImages"),
				map[string]any{"$ID": "pt-mode", "PropertyKey": "uploadMode", "ValueType": map[string]any{"Type": "Enumeration"}},
			}},
		},
		"Object": map[string]any{"Properties": []any{
			map[string]any{"TypePointer": "pt-files", "Value": map[string]any{"DataSource": dbSource("Uploads.UploadedFile")}},
			map[string]any{"TypePointer": "pt-images", "Value": map[string]any{"DataSource": dsBSON(dsTypeDatabase)}},
			map[string]any{"TypePointer": "pt-mode", "Value": map[string]any{"PrimitiveValue": "files"}},
		}},
	}
}

func describeRawWidget(t *testing.T, w map[string]any) string {
	t.Helper()
	var buf bytes.Buffer
	ctx := (&Executor{}).newExecContext(context.Background())
	ctx.Output = &buf
	raw := parseRawWidget(ctx, w)
	if len(raw) != 1 {
		t.Fatalf("parsed %d widgets, want 1", len(raw))
	}
	outputWidgetMDLV3(ctx, raw[0], 1)
	return buf.String()
}

// #1199: a widget whose SCHEMA declares several datasources is described with
// the named key even when only one is configured. The builder refuses the
// generic clause on such a widget whatever is configured ("exposes 2
// datasources, so a generic `datasource:` clause is ambiguous"), so the generic
// spelling made the describe output of every File Uploader page unexecutable.
func TestDescribe_SchemaMultiSourceWidgetUsesNamedKey(t *testing.T) {
	got := describeRawWidget(t, fileUploaderBSON())
	if !strings.Contains(got, "associatedFiles: database from Uploads.UploadedFile") {
		t.Errorf("describe did not name the configured datasource's key:\n%s", got)
	}
	if strings.Contains(strings.ToLower(got), "datasource:") {
		t.Errorf("describe emitted a generic `DataSource:` clause the builder rejects as ambiguous:\n%s", got)
	}
}

// A linked datasource is filled by the platform and never authored, so it does
// not make a widget multi-source: one linked plus one authorable keeps the
// generic clause.
func TestDescribe_LinkedDataSourceDoesNotCountAsAuthorable(t *testing.T) {
	w := fileUploaderBSON()
	pts := w["Type"].(map[string]any)["ObjectType"].(map[string]any)["PropertyTypes"].([]any)
	pts[1].(map[string]any)["ValueType"] = map[string]any{"Type": "DataSource", "IsLinked": true}
	got := describeRawWidget(t, w)
	if !strings.Contains(got, "DataSource: database from Uploads.UploadedFile") {
		t.Errorf("single authorable datasource should keep the generic clause:\n%s", got)
	}
}

// A widget with a hand-written embedded definition picks its datasource
// mapping by mode, so the builder accepts the generic clause there and its
// output must not change: a database-mode ComboBox declares two datasources.
func TestDescribe_EmbeddedDefinitionWidgetKeepsGenericClause(t *testing.T) {
	w := fileUploaderBSON()
	w["Type"].(map[string]any)["WidgetId"] = "com.mendix.widget.web.combobox.Combobox"
	if got := describeRawWidget(t, w); !strings.Contains(got, "DataSource: database from Uploads.UploadedFile") {
		t.Errorf("embedded-definition widget should keep the generic clause:\n%s", got)
	}
}

// The other half of the #1199 round trip: what DESCRIBE now emits for a
// multi-source widget with ONE configured datasource — its named key, the other
// left unset — is what the builder accepts. Before, it was offered the generic
// clause and refused it as ambiguous.
func TestBuild_OneNamedDataSourceOnMultiSourceWidget(t *testing.T) {
	e := multiSourceEngine(t)
	w := &ast.WidgetV3{
		Name: "cb",
		Type: "pluggablewidget",
		Properties: map[string]any{
			"WidgetType":                      "com.mendix.widget.web.combobox.Combobox",
			"optionsSourceDatabaseDataSource": &ast.DataSourceV3{Type: "database", Reference: "Sales.Customer"},
		},
	}
	widget, err := e.Build(multiSourceDef(), w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := renderedWidgetStrings(t, widget); !strings.Contains(got, "Sales.Customer") {
		t.Error("named datasource was not applied")
	}
}
