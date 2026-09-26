// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// These tests parse the statement and dispatch it through the registry,
// rather than hand-building a DescribeFragmentFromStmt. The defect they pin
// was a casing contract between two layers: the visitor stores ContainerType
// as "PAGE"/"SNIPPET" and the handler switched on "page"/"snippet", so every
// `describe fragment from ...` fell through the switch with no widgets and
// reported the widget as missing. A test that builds the AST in lowercase
// agrees with the handler and cannot see that.

func fragmentFromCtx(t *testing.T) (*ExecContext, *bytes.Buffer) {
	t.Helper()
	mod := mkModule("Shop")
	pg := mkPage(mod.ID, "Product_Edit")
	sn := mkSnippet(mod.ID, "Product_Card")

	widget := func(name string) map[string]any {
		return map[string]any{
			"$Type":   "Forms$DivContainer",
			"Name":    name,
			"Widgets": []any{int32(2)},
		}
	}
	raw := map[model.ID]map[string]any{
		pg.ID: {
			"$Type": "Forms$Page",
			"FormCall": map[string]any{
				"Arguments": []any{int32(2), map[string]any{
					"Widgets": []any{int32(2), widget("pageBox")},
				}},
			},
		},
		sn.ID: {
			"$Type":   "Forms$Snippet",
			"Widgets": []any{int32(2), widget("snippetBox")},
		},
	}

	ctx, buf := newMockCtx(t, withHierarchy(mkHierarchy(mod)))
	mb := ctx.Backend.(*mock.MockBackend)
	mb.ListPagesFunc = func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil }
	mb.ListSnippetsFunc = func() ([]*pages.Snippet, error) { return []*pages.Snippet{sn}, nil }
	mb.GetRawUnitFunc = func(id model.ID) (map[string]any, error) { return raw[id], nil }
	return ctx, buf
}

func runParsed(t *testing.T, ctx *ExecContext, src string) error {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("want 1 statement, got %d", len(prog.Statements))
	}
	return NewRegistry().Dispatch(ctx, prog.Statements[0])
}

func TestDescribeFragmentFrom_Parsed(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"page", `describe fragment from page Shop.Product_Edit widget pageBox;`, "pageBox"},
		{"snippet", `describe fragment from snippet Shop.Product_Card widget snippetBox;`, "snippetBox"},
		{"uppercase keywords", `DESCRIBE FRAGMENT FROM PAGE Shop.Product_Edit WIDGET pageBox;`, "pageBox"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, buf := fragmentFromCtx(t)
			if err := runParsed(t, ctx, tc.src); err != nil {
				t.Fatalf("execute: %v", err)
			}
			if !strings.Contains(buf.String(), tc.want) {
				t.Fatalf("output does not describe widget %q:\n%s", tc.want, buf.String())
			}
		})
	}
}

// A missing widget must name what was looked for — the widget and the
// container — so the reader can tell a typo from a wrong page.
func TestDescribeFragmentFrom_MissingWidgetNamesItAndContainer(t *testing.T) {
	ctx, _ := fragmentFromCtx(t)
	err := runParsed(t, ctx, `describe fragment from page Shop.Product_Edit widget noSuchBox;`)
	if err == nil {
		t.Fatal("want an error for a missing widget")
	}
	for _, want := range []string{"noSuchBox", "page", "Shop.Product_Edit"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A missing container is its own error, not a missing widget.
func TestDescribeFragmentFrom_MissingContainer(t *testing.T) {
	ctx, _ := fragmentFromCtx(t)
	err := runParsed(t, ctx, `describe fragment from snippet Shop.NoSuchSnippet widget snippetBox;`)
	if err == nil || !strings.Contains(err.Error(), "snippet not found") {
		t.Fatalf("want snippet-not-found, got %v", err)
	}
}
