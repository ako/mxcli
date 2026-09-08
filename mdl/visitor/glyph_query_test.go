// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestShowGlyphs_Parses(t *testing.T) {
	for _, tc := range []struct {
		src  string
		like string
	}{
		{"show glyphs;", ""},
		{"list glyphs;", ""},
		{"show glyphs like 'star';", "star"},
		{"SHOW GLYPHS LIKE 'user';", "user"},
	} {
		prog, errs := Build(tc.src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", tc.src, errs[0])
		}
		if len(prog.Statements) != 1 {
			t.Fatalf("%s: got %d statements", tc.src, len(prog.Statements))
		}
		s, ok := prog.Statements[0].(*ast.ShowStmt)
		if !ok {
			t.Fatalf("%s: got %T", tc.src, prog.Statements[0])
		}
		if s.ObjectType != ast.ShowGlyphs {
			t.Errorf("%s: ObjectType = %v, want ShowGlyphs", tc.src, s.ObjectType)
		}
		if s.Like != tc.like {
			t.Errorf("%s: Like = %q, want %q", tc.src, s.Like, tc.like)
		}
	}
}

// The subject is a code or a name, and both have to survive the visitor —
// `describe glyph 57350` when reading a menu someone else wrote,
// `describe glyph 'star'` when writing one.
func TestDescribeGlyph_Parses(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"describe glyph 57350;", "57350"},
		{"describe glyph 'star';", "star"},
		{"DESCRIBE GLYPH 'star-empty';", "star-empty"},
	} {
		prog, errs := Build(tc.src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", tc.src, errs[0])
		}
		s, ok := prog.Statements[0].(*ast.DescribeStmt)
		if !ok {
			t.Fatalf("%s: got %T", tc.src, prog.Statements[0])
		}
		if s.ObjectType != ast.DescribeGlyph {
			t.Errorf("%s: ObjectType = %v, want DescribeGlyph", tc.src, s.ObjectType)
		}
		if s.Qualifier != tc.want {
			t.Errorf("%s: Qualifier = %q, want %q", tc.src, s.Qualifier, tc.want)
		}
	}
}

// CONTROL: adding the GLYPHS token must not shadow GLYPH in the icon clause it
// was originally added for. ANTLR's maximal munch should keep them apart, and
// this is the test that says so out loud.
func TestGlyphKeywordStillParsesAnIconClause(t *testing.T) {
	prog, errs := Build(`create or modify menu M.Nav (
  menu item 'Home' page M.Home icon glyph 57377;
)`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	s, ok := prog.Statements[0].(*ast.CreateMenuStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	if len(s.Items) != 1 || s.Items[0].IconCode != 57377 {
		t.Errorf("the icon clause no longer parses: %+v", s.Items)
	}
}
