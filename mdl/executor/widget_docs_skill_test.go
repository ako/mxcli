// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// The generated widget docs are read by agents and by nothing else — no Go code
// loads them back — so their content IS the contract. These tests pin the three
// facts that were missing, each measured on a 42-widget project before the fix:
// 134 enumeration rows named 0 permitted values, 23 object rows exposed 0
// children, and 75 descriptions were cut mid-sentence at 77 characters.

func enumProp() mpk.PropertyDef {
	return mpk.PropertyDef{
		Key: "itemSelectionMethod", Type: "enumeration", Required: true,
		DefaultValue: "checkbox", EnumValues: []string{"checkbox", "rowClick"},
		Category: "General::General", Description: "Selection method",
	}
}

func objectProp() mpk.PropertyDef {
	return mpk.PropertyDef{
		Key: "columns", Type: "object", Required: true, IsList: true,
		Category: "General::Columns",
		Children: []mpk.PropertyDef{
			{Key: "showContentAs", Type: "enumeration", Required: true,
				DefaultValue: "attribute", EnumValues: []string{"attribute", "dynamicText"}},
			{Key: "header", Type: "textTemplate"},
			{Key: "hidden", Type: "boolean", IsSystem: true},
		},
	}
}

// An enumeration that shows its default but not its alternatives cannot be
// filled in from the doc. `mxcli widget describe` reads the same PropertyDef and
// always printed `{checkbox|rowClick}`; only this renderer dropped it.
func TestWidgetDocRendersEnumerationValues(t *testing.T) {
	doc := widgetDocMarkdown(&mpk.WidgetDefinition{
		ID: "com.x.Y", Name: "Y", Version: "1.0", IsPluggable: true,
		Properties: []mpk.PropertyDef{enumProp()},
	}, nil, "Y")

	for _, want := range []string{"`checkbox`", "`rowClick`"} {
		if !strings.Contains(doc, want) {
			t.Errorf("enumeration value %s missing; the reader cannot know what is allowed:\n%s", want, doc)
		}
	}
}

// An `object` row that says only "object, required" leaves the reader unable to
// write a single entry of it. The children are on the same struct.
func TestWidgetDocRendersNestedObjectChildren(t *testing.T) {
	doc := widgetDocMarkdown(&mpk.WidgetDefinition{
		ID: "com.x.Grid", Name: "Grid", Version: "1.0", IsPluggable: true,
		Properties: []mpk.PropertyDef{objectProp()},
	}, nil, "Grid")

	for _, want := range []string{"showContentAs", "header", "sub-properties below"} {
		if !strings.Contains(doc, want) {
			t.Errorf("%q missing from the rendered doc:\n%s", want, doc)
		}
	}
	if !strings.Contains(doc, "↳") {
		t.Error("children are not visibly nested under their parent")
	}
	// System properties are not authorable in MDL and were already excluded at
	// the top level; the recursion must not reintroduce them.
	if strings.Contains(doc, "hidden") {
		t.Error("a system sub-property was rendered")
	}
}

// Descriptions were cut at 77 characters plus an ellipsis, which reliably
// removed the operative half ("Must include '%d' to denote number posit...").
func TestWidgetDocKeepsWholeDescriptions(t *testing.T) {
	long := "Must include '%d' to denote number position, and the row count is substituted at runtime before the string reaches assistive technology."
	doc := widgetDocMarkdown(&mpk.WidgetDefinition{
		ID: "com.x.Y", Name: "Y", Version: "1.0", IsPluggable: true,
		Properties: []mpk.PropertyDef{{Key: "selectedCountTemplatePlural", Type: "textTemplate", Description: long}},
	}, nil, "Y")

	if strings.Contains(doc, "...") {
		t.Error("a description was truncated")
	}
	if !strings.Contains(doc, "assistive technology") {
		t.Error("the end of the description was lost")
	}
}

// A pipe in a description silently splits the markdown row into extra columns,
// so the table stops parsing from that row on.
func TestWidgetDocEscapesPipesInCells(t *testing.T) {
	doc := widgetDocMarkdown(&mpk.WidgetDefinition{
		ID: "com.x.Y", Name: "Y", Version: "1.0", IsPluggable: true,
		Properties: []mpk.PropertyDef{{Key: "mode", Type: "string", Description: "one | two"}},
	}, nil, "Y")

	for _, line := range strings.Split(doc, "\n") {
		if strings.Contains(line, "`mode`") {
			if strings.Count(line, "|")-strings.Count(line, `\|`) != 8 {
				t.Errorf("row has the wrong column count, an unescaped pipe leaked: %q", line)
			}
		}
	}
}

// The skill front page must be a real skill: frontmatter, and a description that
// names the project's own widgets — the thing a hand-written skill cannot do,
// and what lets a reader rule it in or out without opening it.
func TestWidgetSkillMarkdownIsADiscoverableSkill(t *testing.T) {
	skill := widgetSkillMarkdown(
		[]string{"| `PLUGGABLEWIDGET` | [BADGE](badge.md) | `com.x.Badge` | Badge | 3 |"},
		[]string{"Badge", "Data grid 2"}, nil)

	if !strings.HasPrefix(skill, "---\nname: widgets\ndescription: ") {
		t.Fatalf("no Agent Skills frontmatter:\n%s", skill[:min(200, len(skill))])
	}
	for _, want := range []string{"Badge", "Data grid 2"} {
		if !strings.Contains(skill, want) {
			t.Errorf("the description does not name %q, so the listing cannot answer what this project has", want)
		}
	}
	// It routes onward rather than trying to be the whole reference.
	if !strings.Contains(skill, "mxcli widget describe") {
		t.Error("no pointer to the always-fresh command")
	}
	if !strings.Contains(skill, "[BADGE](badge.md)") {
		t.Error("the index does not link the per-widget files, so nothing tells a reader they exist")
	}
}

// End to end: `widget docs` must write a usable skill into every skills tree,
// retire the old `_index.md`, and never leave the pre-#906 name behind.
func TestRegenerateWidgetDocsWritesSkillToBothTrees(t *testing.T) {
	dir := t.TempDir()
	projectPath := filepath.Join(dir, "App.mpr")
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{".ai-context", ".claude"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	const fixture = "../../testdata/expr-checker/widgets/Charts.mpk"
	src, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("Charts.mpk fixture not available: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "widgets", "Charts.mpk"), src, 0o644); err != nil {
		t.Fatal(err)
	}

	// A project upgraded from an older mxcli still carries the underscore index.
	for _, d := range []string{".ai-context", ".claude"} {
		wd := filepath.Join(dir, d, "skills", "widgets")
		if err := os.MkdirAll(wd, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(wd, "_index.md"), []byte("# old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := RegenerateWidgetDocs(projectPath); err != nil {
		t.Fatalf("RegenerateWidgetDocs: %v", err)
	}

	for _, d := range []string{".ai-context", ".claude"} {
		wd := filepath.Join(dir, d, "skills", "widgets")
		body, err := os.ReadFile(filepath.Join(wd, "SKILL.md"))
		if err != nil {
			t.Errorf("%s/SKILL.md not written: %v", d, err)
			continue
		}
		if !strings.HasPrefix(string(body), "---\nname: widgets\n") {
			t.Errorf("%s/SKILL.md has no frontmatter", d)
		}
		if _, err := os.Stat(filepath.Join(wd, "_index.md")); !os.IsNotExist(err) {
			t.Errorf("%s/_index.md survived; a stale second index sits beside SKILL.md", d)
		}
	}
}
