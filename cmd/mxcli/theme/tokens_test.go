// SPDX-License-Identifier: Apache-2.0

package theme

import (
	"reflect"
	"testing"
)

func TestExtractTokens_ReadsAPlainRootBlock(t *testing.T) {
	got := ExtractTokens("x.css", `:root { --mxt-brand: #7f5af0; --mxt-ground: #fffffe; }`)
	want := TokenSet{"--mxt-brand": "#7f5af0", "--mxt-ground": "#fffffe"}
	if !reflect.DeepEqual(got.Base, want) {
		t.Errorf("Base = %v; want %v", got.Base, want)
	}
}

// The variant a declaration belongs to comes from the *enclosing* blocks, not
// from the one it sits in: the selector inside a prefers-color-scheme query is
// just :root, and reading only the innermost header would file every dark
// value as a base value and silently produce a one-palette theme.
func TestExtractTokens_ScopeComesFromTheEnclosingBlocks(t *testing.T) {
	got := ExtractTokens("x.css", `
:root { --mxt-brand: #0f6e6b; }
@media (prefers-color-scheme: dark) {
  :root { --mxt-brand: #2aa39f; --mxt-ground: #0e1116; }
}
:root.theme-dark { --mxt-line: #262c36; }
html[data-theme="light"] { --mxt-line: #dce1e7; }
`)
	if got.Base["--mxt-brand"] != "#0f6e6b" {
		t.Errorf("base brand = %q", got.Base["--mxt-brand"])
	}
	if got.Dark["--mxt-brand"] != "#2aa39f" || got.Dark["--mxt-ground"] != "#0e1116" {
		t.Errorf("dark = %v", got.Dark)
	}
	if got.Dark["--mxt-line"] != "#262c36" {
		t.Errorf(".theme-dark must read as dark: %v", got.Dark)
	}
	if got.Light["--mxt-line"] != "#dce1e7" {
		t.Errorf("[data-theme=light] must read as light: %v", got.Light)
	}
}

func TestExtractTokens_SurvivesAWholeHTMLPage(t *testing.T) {
	got := ExtractTokens("canvas.dc.html", `
<!doctype html><html><head><style>
  /* --mxt-brand: #000000; a commented-out token must not count */
  :root { --mxt-brand: #7f5af0; }
  .card::after { content: "}"; }
  :root { --mxt-ink: #14181f; }
</style></head>
<body><div class="x">--mxt-not-a-declaration</div></body></html>`)
	want := TokenSet{"--mxt-brand": "#7f5af0", "--mxt-ink": "#14181f"}
	if !reflect.DeepEqual(got.Base, want) {
		t.Errorf("Base = %v; want %v — a brace inside a string or a commented token must not derail the scan", got.Base, want)
	}
}

func TestExtractTokens_KeepsMultiPartValues(t *testing.T) {
	got := ExtractTokens("x.css", `:root {
  --mxt-shadow: 0 1px 2px 0 rgba(20, 24, 31, 0.08);
  --mxt-font: "IBM Plex Sans", system-ui, sans-serif;
}`)
	if got.Base["--mxt-shadow"] != "0 1px 2px 0 rgba(20, 24, 31, 0.08)" {
		t.Errorf("shadow = %q", got.Base["--mxt-shadow"])
	}
	if got.Base["--mxt-font"] != `"IBM Plex Sans", system-ui, sans-serif` {
		t.Errorf("font = %q", got.Base["--mxt-font"])
	}
}

func TestExtractTokens_IgnoresEverythingThatIsNotAThemeToken(t *testing.T) {
	got := ExtractTokens("x.css", `:root { --brand-primary: #264ae5; --color-bg: #fff; }`)
	if got.Count() != 0 {
		t.Errorf("only --mxt-* is the theme's vocabulary; got %v", got.Base)
	}
}

func TestApplyTokens_RewritesInPlaceAndReportsWhatItCouldNotFind(t *testing.T) {
	src := ":root {\n  /* brand */\n  --mxt-brand: #0f6e6b;\n  --mxt-ink: #14181f;\n}\n"
	out, unplaced := applyTokens(src, TokenSet{"--mxt-brand": "#7f5af0", "--mxt-radius": "12px"})

	if want := "  --mxt-brand: #7f5af0;"; !contains(out, want) {
		t.Errorf("out = %q; want it to contain %q", out, want)
	}
	if !contains(out, "/* brand */") || !contains(out, "--mxt-ink: #14181f;") {
		t.Errorf("rewriting must leave comments and untouched tokens alone:\n%s", out)
	}
	if len(unplaced) != 1 || unplaced[0] != "--mxt-radius" {
		t.Errorf("unplaced = %v; want [--mxt-radius]", unplaced)
	}
}

// A var() reference is a read, not a declaration. Substituting one would
// hardcode a colour into a rule, which is exactly what the palette/wiring
// split exists to prevent.
func TestApplyTokens_LeavesVarReferencesAlone(t *testing.T) {
	src := ":root {\n  --mxt-brand: #0f6e6b;\n}\n.btn {\n  color: var(--mxt-brand);\n}\n"
	out, _ := applyTokens(src, TokenSet{"--mxt-brand": "#7f5af0"})
	if !contains(out, "color: var(--mxt-brand);") {
		t.Errorf("var() reference was rewritten:\n%s", out)
	}
}

func contains(s, sub string) bool { return len(sub) == 0 || indexOf(s, sub) >= 0 }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
