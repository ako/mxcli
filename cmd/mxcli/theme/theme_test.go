// SPDX-License-Identifier: Apache-2.0

package theme

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// newProject fakes the parts of a Mendix project a theme touches: the .mpr, the
// three-line theme/web/main.scss Mendix ships, and the stock custom-variables.
func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "App.mpr"), "")
	write(t, filepath.Join(dir, "themesource", "atlas_core", "web", "main.scss"), "// atlas\n")
	write(t, filepath.Join(dir, "theme", "web", "main.scss"),
		"@import \"custom-variables\";\n@import \"theme-dark\";\n@import \"theme-neutral\";\n")
	write(t, filepath.Join(dir, "theme", "web", "custom-variables.scss"),
		"$brand-logo: false;\n:root {\n  --brand-primary: #264ae5;\n}\n")
	return dir
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDefaultThemeIsEmbeddedAndWellFormed(t *testing.T) {
	themes, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) == 0 {
		t.Fatal("no themes embedded")
	}
	def, err := Get("", DefaultName)
	if err != nil {
		t.Fatalf("the default theme must be embedded: %v", err)
	}
	if def.Title == "" || def.Version == "" || def.Summary == "" {
		t.Errorf("theme.json is missing display fields: %+v", def)
	}
	if len(def.Colorway) == 0 {
		t.Error("colorway is empty; the chart theme in P2 reads it")
	}
}

func TestApply_WritesTheThreeLayersAndTheFonts(t *testing.T) {
	dir := newProject(t)

	res, err := Apply(dir, DefaultName, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed() {
		t.Fatal("apply reported no changes")
	}

	// Layer 1: the palette lands in the file every module imports. It declares
	// the theme's own tokens; the Atlas variables are mapped from them one file
	// down, which is what lets a variant restate ~30 values instead of ~60.
	vars := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss"))
	if !strings.Contains(vars, "--mxt-brand: #0f6e6b") {
		t.Error("brand token not written")
	}
	atlasMap := read(t, filepath.Join(dir, "theme", "web", "_mxcli-atlas-map.scss"))
	if !strings.Contains(atlasMap, "--brand-primary: var(--mxt-brand)") {
		t.Error("Atlas wiring not written")
	}
	if !strings.Contains(vars, "$brand-logo: false;") {
		t.Error("Mendix's own content was dropped from custom-variables.scss")
	}

	// Layer 2: the partial, plus the one-line import from the file that
	// compiles last. Without the import the partial is dead weight.
	if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_mxcli-signal.scss")); err != nil {
		t.Errorf("Layer 2 partial not written: %v", err)
	}
	main := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
	if !strings.Contains(main, `@import "mxcli-signal"`) {
		t.Error("partial is not imported from theme/web/main.scss")
	}
	if !strings.Contains(main, `@import "theme-dark"`) {
		t.Error("Mendix's own imports were dropped from main.scss")
	}

	// Fonts are vendored, so the app renders correctly with no network.
	fonts, err := filepath.Glob(filepath.Join(dir, "theme", "web", "mxcli-fonts", "*.woff2"))
	if err != nil || len(fonts) == 0 {
		t.Errorf("no fonts vendored: %v", err)
	}
}

// Every url() the theme emits must resolve to a file the theme also ships. A
// typo here compiles clean and only shows up as a silent fallback to system-ui
// in the browser. Every theme is checked, and
// the filenames are derived from the partial's own @font-face loops rather
// than restated here, so a theme that changes its weights cannot drift from a
// hand-maintained list in this file.
func TestApply_EveryFontURLResolvesToAVendoredFile(t *testing.T) {
	themes, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	for _, th := range themes {
		t.Run(th.Name, func(t *testing.T) {
			dir := newProject(t)
			if _, err := Apply(dir, th.Name, Options{}); err != nil {
				t.Fatal(err)
			}
			partial := read(t, filepath.Join(dir, "theme", "web", "_mxcli-"+th.Name+".scss"))
			assertFontsResolve(t, dir, partial)
		})
	}
}

func TestApply_IsIdempotent(t *testing.T) {
	dir := newProject(t)
	if _, err := Apply(dir, DefaultName, Options{}); err != nil {
		t.Fatal(err)
	}
	before := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss"))

	res, err := Apply(dir, DefaultName, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Changed() {
		t.Errorf("second apply reported changes: %+v", res.Files)
	}
	if after := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss")); after != before {
		t.Error("second apply rewrote the file")
	}
}

func TestApply_RefusesWhenTheUserHasEditedTheBlock(t *testing.T) {
	dir := newProject(t)
	if _, err := Apply(dir, DefaultName, Options{}); err != nil {
		t.Fatal(err)
	}

	varsPath := filepath.Join(dir, "theme", "web", "custom-variables.scss")
	edited := strings.Replace(read(t, varsPath), "#0f6e6b", "#ff6b35", 1)
	write(t, varsPath, edited)

	_, err := Apply(dir, DefaultName, Options{})
	var modified *ErrBlockModified
	if !errors.As(err, &modified) {
		t.Fatalf("err = %v, want ErrBlockModified", err)
	}
	if modified.Path == "" {
		t.Error("the error must name the file so the user can find it")
	}
	if !strings.Contains(read(t, varsPath), "#ff6b35") {
		t.Fatal("the user's re-brand was overwritten")
	}
}

func TestApply_DryRunWritesNothing(t *testing.T) {
	dir := newProject(t)
	before := read(t, filepath.Join(dir, "theme", "web", "main.scss"))

	res, err := Apply(dir, DefaultName, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed() {
		t.Error("dry run should still report the changes it would make")
	}
	if read(t, filepath.Join(dir, "theme", "web", "main.scss")) != before {
		t.Error("dry run modified main.scss")
	}
	if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_mxcli-signal.scss")); !os.IsNotExist(err) {
		t.Error("dry run created the partial")
	}
}

func TestRemove_LeavesTheProjectAsItWas(t *testing.T) {
	dir := newProject(t)
	mainBefore := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
	varsBefore := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss"))

	if _, err := Apply(dir, DefaultName, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove(dir, DefaultName, Options{}); err != nil {
		t.Fatal(err)
	}

	if got := read(t, filepath.Join(dir, "theme", "web", "main.scss")); got != mainBefore {
		t.Errorf("main.scss not restored:\nwant %q\ngot  %q", mainBefore, got)
	}
	if got := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss")); got != varsBefore {
		t.Errorf("custom-variables.scss not restored:\nwant %q\ngot  %q", varsBefore, got)
	}
	if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_mxcli-signal.scss")); !os.IsNotExist(err) {
		t.Error("the partial should be deleted, not left empty")
	}
	if fonts, _ := filepath.Glob(filepath.Join(dir, "theme", "web", "mxcli-fonts", "*.woff2")); len(fonts) > 0 {
		t.Error("vendored fonts should be removed with the theme")
	}
	if _, err := os.Stat(filepath.Join(dir, "theme", "web", "mxcli-fonts")); !os.IsNotExist(err) {
		t.Error("the font directory should be pruned, not left empty")
	}
}

// A directory the theme owns must not be pruned when the user has put something
// of their own in it.
func TestRemove_KeepsADirectoryHoldingUserFiles(t *testing.T) {
	dir := newProject(t)
	if _, err := Apply(dir, DefaultName, Options{}); err != nil {
		t.Fatal(err)
	}
	mine := filepath.Join(dir, "theme", "web", "mxcli-fonts", "my-brand-font.woff2")
	write(t, mine, "not really a font")

	if _, err := Remove(dir, DefaultName, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mine); err != nil {
		t.Errorf("user's own file in a theme directory was deleted: %v", err)
	}
}

func TestApply_RefusesADirectoryThatIsNotAMendixProject(t *testing.T) {
	if _, err := Apply(t.TempDir(), DefaultName, Options{}); err == nil {
		t.Fatal("expected a refusal for a non-project directory")
	}
}

// A typo has to name what the user could have meant. Pointing at
// `mxcli theme list` was actively wrong once a project could own themes: that
// listing needs -p to see them, so a user who had just run `theme create` was
// sent to a command that would not show what they created.
func TestGet_UnknownThemeListsWhatIsActuallyAvailable(t *testing.T) {
	_, err := Get("", "nope")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	for _, want := range []string{"signal", "ledger", "console"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v; must name the available themes", err)
		}
	}
	if !strings.Contains(err.Error(), "-p") {
		t.Errorf("err = %v; without a project it must say the local ones are not listed", err)
	}

	// Inside a project the listing includes the project's own themes, marked,
	// and the message offers the command that makes one.
	dir := newProject(t)
	mustCreate(t, dir, "acme", CreateOptions{})
	_, err = Get(dir, "acmee")
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !strings.Contains(err.Error(), "acme (local)") {
		t.Errorf("err = %v; must list the project's own themes and mark them", err)
	}
	if !strings.Contains(err.Error(), "theme create") {
		t.Errorf("err = %v; must offer the command that adds one", err)
	}
	if strings.Contains(err.Error(), "run `mxcli theme list`") {
		t.Error("must not send the user to a listing that hides the theme they just made")
	}
}

// The Layer-1 file is imported once per module, so a CSS rule there is emitted
// once per module too. Declarations only.
//
// Checked on the file as written, not on the asset: the palette's selector is
// templated now, so the asset carries a placeholder and only the applied file
// shows what actually lands in the project.
func TestLayer1BlockContainsNoRules(t *testing.T) {
	dir := newProject(t)
	if _, err := Apply(dir, DefaultName, Options{}); err != nil {
		t.Fatal(err)
	}
	body := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss"))
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, "{") && !strings.HasPrefix(trimmed, ":root") {
			t.Errorf("custom-variables.scss must hold declarations only, found selector: %q", trimmed)
		}
	}
}

// ---------------------------------------------------------------------------
// Variants and the multi-theme registry
// ---------------------------------------------------------------------------

func TestAllThemesAreWellFormed(t *testing.T) {
	themes, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) < 3 {
		t.Fatalf("expected signal, ledger and console; got %d", len(themes))
	}
	for _, th := range themes {
		if th.DefaultVariant != VariantLight && th.DefaultVariant != VariantDark {
			t.Errorf("%s: defaultVariant %q is neither light nor dark", th.Name, th.DefaultVariant)
		}
		if th.AltVariant() == th.DefaultVariant {
			t.Errorf("%s: alt variant equals the default", th.Name)
		}
		if len(th.Colorway) == 0 || th.Summary == "" || th.Title == "" {
			t.Errorf("%s: incomplete theme.json: %+v", th.Name, th)
		}
	}
}

// The Atlas wiring is what makes a palette swap cheap, so every theme has to
// run through the same one. Shipped per theme (a theme package is meant to be
// self-contained), which is exactly why it can drift.
func TestSharedPartialsAreIdenticalInEveryTheme(t *testing.T) {
	themes, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	for _, shared := range []string{"_mxcli-atlas-map.scss", "_mxcli-widgets.scss"} {
		var reference []byte
		var referenceName string
		for _, th := range themes {
			body, err := assetsFS.ReadFile("assets/" + th.Name + "/files/theme/web/" + shared)
			if err != nil {
				t.Fatalf("%s ships no %s: %v", th.Name, shared, err)
			}
			if reference == nil {
				reference, referenceName = body, th.Name
				continue
			}
			if string(body) != string(reference) {
				t.Errorf("%s's %s has drifted from %s's", th.Name, shared, referenceName)
			}
		}
	}
}

// The widget layer exists because Sass bakes these colours before any custom
// property exists, so a rule that reintroduces a literal defeats the point.
func TestWidgetLayerResolvesEveryColourThroughAToken(t *testing.T) {
	themes, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile(`(color|background|background-color|border-color|outline-color|box-shadow)\s*:\s*[^;]*(#[0-9a-fA-F]{3,8}|\brgba?\()`)
	for _, th := range themes {
		body, err := assetsFS.ReadFile("assets/" + th.Name + "/files/theme/web/_mxcli-widgets.scss")
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if literal.MatchString(trimmed) {
				t.Errorf("%s _mxcli-widgets.scss:%d reintroduces a literal colour: %q",
					th.Name, i+1, trimmed)
			}
		}
		// And it has to actually carry the fix that prompted the layer.
		if !strings.Contains(string(body), ".pagination-bar") {
			t.Errorf("%s: no rule for the Data Grid 2 pager caption", th.Name)
		}
	}
}

// A palette that pins Atlas leaves to literal colours cannot survive a variant
// flip: the ink stays near-black on a near-black ground. Every theme must go
// through --mxt-* instead, which is what the Atlas map exists for.
func TestPalettesDeclareOnlyThemeTokens(t *testing.T) {
	themes, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	for _, th := range themes {
		body, err := assetsFS.ReadFile("assets/" + th.Name + "/files/theme/web/custom-variables.scss")
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "--") {
				continue
			}
			if !strings.HasPrefix(trimmed, "--mxt-") {
				t.Errorf("%s custom-variables.scss:%d declares an Atlas variable directly: %q",
					th.Name, i+1, trimmed)
			}
		}
	}
}

func TestApplyVariant_AutoShipsBothPalettes(t *testing.T) {
	dir := newProject(t)
	if _, err := Apply(dir, DefaultName, Options{Variant: VariantAuto}); err != nil {
		t.Fatal(err)
	}
	main := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
	if !strings.Contains(main, "$mxcli-theme-variant: auto;") {
		t.Errorf("variant not written into main.scss:\n%s", main)
	}
	partial := read(t, filepath.Join(dir, "theme", "web", "_mxcli-signal.scss"))
	if !strings.Contains(partial, "prefers-color-scheme") {
		t.Error("auto must follow the OS")
	}
	// The explicit-class path has to outrank Mendix's own _theme-dark.scss,
	// which also declares :root.theme-dark.
	if !strings.Contains(partial, ":root.theme-dark") {
		t.Error("auto must honour an explicit theme-dark class")
	}
}

func TestApplyVariant_PinnedIsWrittenThrough(t *testing.T) {
	for _, v := range []Variant{VariantLight, VariantDark} {
		dir := newProject(t)
		if _, err := Apply(dir, DefaultName, Options{Variant: v}); err != nil {
			t.Fatal(err)
		}
		main := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
		if !strings.Contains(main, "$mxcli-theme-variant: "+string(v)+";") {
			t.Errorf("variant %q not written into main.scss:\n%s", v, main)
		}
		if strings.Contains(main, "{{VARIANT}}") {
			t.Errorf("variant placeholder left unexpanded for %q", v)
		}
	}
}

func TestParseVariant_RejectsNonsense(t *testing.T) {
	if _, err := ParseVariant("sepia"); err == nil {
		t.Fatal("expected an error for an unknown variant")
	}
	for _, ok := range []string{"auto", "light", "dark"} {
		if _, err := ParseVariant(ok); err != nil {
			t.Errorf("ParseVariant(%q) = %v", ok, err)
		}
	}
}

// Two themes at once would both map the Atlas leaves, and which palette won
// would come down to SCSS import order rather than to what was asked for.
func TestApply_RemovesThePreviousTheme(t *testing.T) {
	dir := newProject(t)
	if _, err := Apply(dir, "ledger", Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_mxcli-ledger.scss")); err != nil {
		t.Fatalf("ledger not applied: %v", err)
	}

	if _, err := Apply(dir, "signal", Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_mxcli-ledger.scss")); !os.IsNotExist(err) {
		t.Error("ledger's partial survived a switch to signal")
	}
	main := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
	if strings.Contains(main, "mxcli-ledger") {
		t.Errorf("ledger is still imported after switching to signal:\n%s", main)
	}
	if !strings.Contains(main, "mxcli-signal") {
		t.Error("signal is not imported")
	}
	// Ledger's fonts must go too, or the binary's payload accumulates in the
	// project every time someone tries a theme.
	if got, _ := filepath.Glob(filepath.Join(dir, "theme", "web", "mxcli-fonts", "source-*.woff2")); len(got) > 0 {
		t.Errorf("ledger's fonts survived the switch: %v", got)
	}
}

func TestSwitcherMDL_TargetsTheRequestedModule(t *testing.T) {
	mdl := SwitcherMDL("Ops", nil)
	if strings.Contains(mdl, "{{MODULE}}") {
		t.Error("module placeholder left unexpanded")
	}
	for _, want := range []string{
		"create or modify javascript action Ops.ToggleAppTheme",
		"create or modify javascript action Ops.SetAppTheme",
		"create or modify javascript action Ops.ApplyStoredTheme",
		"create or replace nanoflow Ops.ACT_ToggleTheme",
		SwitcherStorageKey,
	} {
		if !strings.Contains(mdl, want) {
			t.Errorf("switcher MDL is missing %q", want)
		}
	}
	// The class has to land on the root element: popups and modals render at
	// <body>, outside any page container, and must follow the theme too.
	if !strings.Contains(mdl, "document.documentElement") {
		t.Error("the theme class must be set on the root element")
	}
}

// SCSS is not compiled by the Go tests, so a theme whose variant mixin is named
// differently from the one it includes would ship broken and only fail at the
// user's next build. Assert the naming contract instead.
func TestEveryThemeDefinesAndIncludesItsAltPaletteMixin(t *testing.T) {
	themes, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	for _, th := range themes {
		body, err := assetsFS.ReadFile(
			"assets/" + th.Name + "/files/theme/web/_mxcli-" + th.Name + ".scss")
		if err != nil {
			t.Fatalf("%s ships no theme partial: %v", th.Name, err)
		}
		assertPartialStructure(t, string(body), th.Name, th.AltVariant())
	}
}

// assertPartialStructure checks the contract a theme partial has to satisfy
// for its SCSS to compile into a working theme.
//
// It is a helper rather than an inline block because a *scaffolded* theme has
// to satisfy exactly the same contract, and the naming half of it is the one
// thing a copy can plausibly get wrong: a partial that still defines
// @mixin mxcli-signal-dark while main.scss includes mxcli-acme-dark compiles
// to nothing and fails at the user's build, not in CI.
func assertPartialStructure(t *testing.T, src, name string, alt Variant) {
	t.Helper()
	mixin := "mxcli-" + name + "-" + string(alt)
	if !strings.Contains(src, "@mixin "+mixin+" {") {
		t.Errorf("%s does not define @mixin %s", name, mixin)
	}
	if !strings.Contains(src, "@include "+mixin+";") {
		t.Errorf("%s defines %s but never includes it", name, mixin)
	}
	if !strings.Contains(src, "@include mxcli-atlas-map;") {
		t.Errorf("%s never includes the Atlas map", name)
	}
	// Every block that declares tokens must do so at the theme's own scope, or
	// two themes sharing a stylesheet would both be live at once.
	if !strings.Contains(src, "#{$mxcli-"+name+"-scope}") {
		t.Errorf("%s declares its tokens on a fixed selector rather than its scope", name)
	}
	if !strings.Contains(src, "@include mxcli-"+name+"-skin;") {
		t.Errorf("%s never includes its skin mixin", name)
	}
	// The alt palette must be reachable both ways: from the OS preference
	// and from an explicit class a switcher sets.
	if !strings.Contains(src, "prefers-color-scheme: "+string(alt)) {
		t.Errorf("%s does not follow the OS into its alt palette", name)
	}
	if !strings.Contains(src, "&.theme-"+string(alt)+" {") {
		t.Errorf("%s does not honour an explicit theme-%s class", name, alt)
	}
}

// assertFontsResolve checks that every font the partial asks for is actually
// on disk beside it. A missing woff2 is silent — the browser falls back to the
// stack's next family and the page just looks slightly wrong.
func assertFontsResolve(t *testing.T, projectDir, partial string) {
	t.Helper()
	names := fontRefs(partial)
	if len(names) == 0 {
		t.Error("no font URLs found; they must be relative to theme.compiled.css at the web root")
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(projectDir, "theme", "web", "mxcli-fonts", name)); err != nil {
			t.Errorf("%s is referenced by a @font-face loop but not vendored", name)
		}
	}
}

// fontRefs resolves every mxcli-fonts URL in a partial to concrete filenames.
//
// The scan is positional because each @font-face loop has its own weight list
// — the sans family ships 400-700 and the mono one 400-600 — so a URL has to
// be expanded against the @each it actually sits under. Cross-producing every
// list against every URL invents files no theme ever shipped.
func fontRefs(partial string) []string {
	each := regexp.MustCompile(`@each\s+\$weight\s+in\s+\(([^)]+)\)`)
	url := regexp.MustCompile(`url\("\./mxcli-fonts/([^"]+)"\)`)

	var out []string
	var weights []string
	for _, line := range strings.Split(partial, "\n") {
		if m := each.FindStringSubmatch(line); m != nil {
			weights = nil
			for _, w := range strings.Split(m[1], ",") {
				if w = strings.TrimSpace(w); w != "" {
					weights = append(weights, w)
				}
			}
		}
		m := url.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if !strings.Contains(m[1], "#{$weight}") {
			out = append(out, m[1])
			continue
		}
		for _, w := range weights {
			out = append(out, strings.ReplaceAll(m[1], "#{$weight}", w))
		}
	}
	return out
}

// The storage key the docs and the switcher's JavaScript refer to must be the
// same string; before this was templated, the constant could drift from the
// generated code while the test that checked it still passed.
func TestSwitcherStorageKeyIsSubstitutedNotHardcoded(t *testing.T) {
	mdl := SwitcherMDL("Ops", nil)
	if strings.Contains(mdl, "{{STORAGE_KEY}}") {
		t.Error("storage-key placeholder left unexpanded")
	}
	if got := strings.Count(mdl, `"`+SwitcherStorageKey+`"`); got != 4 {
		t.Errorf("storage key appears %d times in the generated JavaScript, want 4", got)
	}
}

// ---------------------------------------------------------------------------
// Regressions reported from the RssReader test build (MXCLI-FINDINGS 15-17)
// ---------------------------------------------------------------------------

// Finding 15. `theme remove` with no name fell back to the default theme, so on
// a project themed with any other one it removed nothing, reported every file
// as unchanged and exited 0 — a silent no-op on the documented invocation.
func TestResolve_FindsTheInstalledThemeNotTheDefault(t *testing.T) {
	dir := newProject(t)
	if _, err := Apply(dir, "ledger", Options{}); err != nil {
		t.Fatal(err)
	}

	installed, err := Installed(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0] != "ledger" {
		t.Fatalf("Installed = %v, want [ledger]", installed)
	}

	// This is the call `theme remove` makes with no argument. Falling back to
	// the default here is exactly the bug.
	got, err := Resolve(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ledger" {
		t.Errorf("Resolve = %q, want ledger", got)
	}
	if got == DefaultName {
		t.Error("resolved to the default theme rather than the installed one")
	}
}

func TestResolve_UnthemedProjectIsAnErrorNotASilentDefault(t *testing.T) {
	dir := newProject(t)

	if _, err := Resolve(dir, ""); err == nil {
		t.Fatal("expected an error for a project with no theme")
	}
	// `apply` may still fall back — a project with no theme is exactly when
	// installing the default is right.
	got, err := Resolve(dir, DefaultName)
	if err != nil || got != DefaultName {
		t.Errorf("Resolve(fallback) = (%q, %v), want (%q, nil)", got, err, DefaultName)
	}
}

// Finding 16. Switching themes replaced the block in custom-variables.scss and
// main.scss but appended to _mxcli-atlas-map.scss, leaving the outgoing theme's
// block in place and doubling the file — two themes mapping the same Atlas
// variables in one file, resolved by source order rather than intent.
func TestApply_SwitchingLeavesExactlyOneBlockInEveryFile(t *testing.T) {
	dir := newProject(t)
	for _, name := range []string{"signal", "ledger", "console", "signal"} {
		if _, err := Apply(dir, name, Options{}); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		installed, err := Installed(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(installed) != 1 || installed[0] != name {
			t.Fatalf("after apply %s, Installed = %v, want [%s]", name, installed, name)
		}

		blocks, err := filepath.Glob(filepath.Join(dir, "theme", "web", "*.scss"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range blocks {
			body := read(t, f)
			if n := strings.Count(body, beginMarker); n > 1 {
				t.Errorf("after apply %s, %s carries %d theme blocks",
					name, filepath.Base(f), n)
			}
		}
	}
}

// Finding 17. The topbar language selector was unreadable in every dark palette
// (1.13:1). Atlas paints it at (0,3,0) from --bg-color-secondary with a #fff
// fallback; the guard was a bare .current-language-text at (0,1,0), which only
// won on layouts that do not nest the selector under .navbar-brand.
func TestAtlasMap_LanguageSelectorMatchesAtlasSpecificityAndUsesTheRailToken(t *testing.T) {
	themes, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	for _, th := range themes {
		body, err := assetsFS.ReadFile(
			"assets/" + th.Name + "/files/theme/web/_mxcli-atlas-map.scss")
		if err != nil {
			t.Fatal(err)
		}
		src := string(body)
		if !strings.Contains(src, ".navbar-brand .widget-language-selector .current-language-text") {
			t.Errorf("%s: does not match Atlas's own (0,3,0) selector, so its rule cannot win", th.Name)
		}
		if !strings.Contains(src, "var(--mxt-rail-ink-active, var(--mxt-rail-ink))") {
			t.Errorf("%s: topbar text must resolve through the rail token", th.Name)
		}
		// `color: inherit` inherits body ink, which is dark on a dark rail —
		// wrong even at the right specificity. Match the declaration, not the
		// comment that explains why it is wrong.
		for _, line := range strings.Split(src, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.HasPrefix(trimmed, "color: inherit") {
				t.Errorf("%s: still declares color: inherit for topbar text", th.Name)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Switchable sets — several themes in one stylesheet, selected by a class
// ---------------------------------------------------------------------------

// The invariant the whole mechanism rests on: exactly one palette is live at
// any time, and which one does not depend on import order. The default theme
// claims :root minus every other skin's class, so the scopes are mutually
// exclusive by construction rather than by specificity.
func TestScopeFor_ScopesAreMutuallyExclusive(t *testing.T) {
	names := []string{"signal", "ledger", "console"}

	def := scopeFor(names, 0)
	if !strings.HasPrefix(def.Palette, ":root:not(.mxt-ledger):not(.mxt-console)") {
		t.Errorf("the default must exclude every other skin, got %q", def.Palette)
	}
	if !strings.Contains(def.Palette, ":root.mxt-signal") {
		t.Errorf("the default must also match its own class, got %q", def.Palette)
	}
	if !def.Default || !def.Shared {
		t.Errorf("default scope = %+v", def)
	}
	for i, want := range map[int]string{1: ":root.mxt-ledger", 2: ":root.mxt-console"} {
		if got := scopeFor(names, i); got.Palette != want {
			t.Errorf("scopeFor(%d) = %q, want %q", i, got.Palette, want)
		} else if got.Default {
			t.Errorf("only the first theme is the default; %d claims it too", i)
		}
	}

	// A lone theme is unscoped, so a single-theme project's CSS is exactly what
	// it was before switchable sets existed.
	if only := scopeFor([]string{"signal"}, 0); only.Palette != ":root" || only.Shared {
		t.Errorf("single theme scope = %+v, want plain :root, unshared", only)
	}
}

func TestApplySet_EachPaletteIsScopedAndTheSharedLayersAreWrittenOnce(t *testing.T) {
	dir := newProject(t)
	names := []string{"signal", "ledger", "console"}
	set, err := ApplySet(dir, names, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Themes) != 3 {
		t.Fatalf("installed %d themes, want 3", len(set.Themes))
	}

	// All three palettes land in the one file every module imports, each on its
	// own selector.
	vars := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss"))
	for _, n := range names[1:] {
		if !strings.Contains(vars, ":root.mxt-"+n+" {") {
			t.Errorf("%s's palette is not scoped to its own class:\n%s", n, vars)
		}
	}
	if strings.Count(vars, "--mxt-brand:") != 3 {
		t.Errorf("expected three brand declarations, one per theme; got %d",
			strings.Count(vars, "--mxt-brand:"))
	}

	// The shared layers are imported once, from the default theme's block. Three
	// copies would be identical rules, so nothing would look wrong — it would
	// just triple the stylesheet.
	main := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
	for _, shared := range []string{"mxcli-atlas-map", "mxcli-recipes", "mxcli-widgets"} {
		if got := strings.Count(main, `@import "`+shared+`"`); got != 1 {
			t.Errorf("%s imported %d times, want exactly 1:\n%s", shared, got, main)
		}
	}
	// ...and every theme partial is imported, or its palette is dead weight.
	for _, n := range names {
		if !strings.Contains(main, `@import "mxcli-`+n+`"`) {
			t.Errorf("main.scss never imports mxcli-%s", n)
		}
	}
	// The shared imports must precede the theme partials that rely on the map.
	if strings.Index(main, `@import "mxcli-atlas-map"`) > strings.Index(main, `@import "mxcli-console"`) {
		t.Error("the Atlas map must be imported before the theme partials that include it")
	}

	if got, err := InstalledOrder(dir); err != nil || len(got) != 3 || got[0] != "signal" {
		t.Errorf("InstalledOrder = %v, %v; want signal first", got, err)
	}
}

// Only a theme sharing the stylesheet gets its skin scoped. Scoping it in the
// single-theme case would raise its specificity and start winning against app
// CSS that used to beat it on source order.
func TestApplySet_SkinIsScopedOnlyWhenTheStylesheetIsShared(t *testing.T) {
	solo := newProject(t)
	if _, err := Apply(solo, "ledger", Options{}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(solo, "theme", "web", "_mxcli-ledger.scss")); !strings.Contains(got, "$mxcli-ledger-scoped: false !default;") {
		t.Error("a lone theme must not scope its skin")
	}

	shared := newProject(t)
	if _, err := ApplySet(shared, []string{"signal", "ledger"}, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(shared, "theme", "web", "_mxcli-ledger.scss")); !strings.Contains(got, "$mxcli-ledger-scoped: true !default;") {
		t.Error("a theme sharing the stylesheet must scope its skin, or it applies under every other theme")
	}
}

func TestApplySet_IsIdempotentAndReversible(t *testing.T) {
	dir := newProject(t)
	mainBefore := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
	varsBefore := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss"))
	names := []string{"signal", "ledger", "console"}

	if _, err := ApplySet(dir, names, Options{}); err != nil {
		t.Fatal(err)
	}
	set, err := ApplySet(dir, names, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if set.Changed() {
		t.Error("re-applying the same set reported changes")
	}

	for _, n := range names {
		if _, err := Remove(dir, n, Options{}); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(t, filepath.Join(dir, "theme", "web", "main.scss")); got != mainBefore {
		t.Errorf("main.scss not restored:\nwant %q\ngot  %q", mainBefore, got)
	}
	if got := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss")); got != varsBefore {
		t.Errorf("custom-variables.scss not restored:\nwant %q\ngot  %q", varsBefore, got)
	}
}

// Narrowing a set has to remove what dropped out — otherwise a theme the user
// removed is still selectable, and its fonts are still deployed.
func TestApplySet_NarrowingRemovesTheThemesThatLeft(t *testing.T) {
	dir := newProject(t)
	if _, err := ApplySet(dir, []string{"signal", "ledger", "console"}, Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplySet(dir, []string{"ledger"}, Options{}); err != nil {
		t.Fatal(err)
	}

	if got, err := Installed(dir); err != nil || len(got) != 1 || got[0] != "ledger" {
		t.Fatalf("Installed = %v, %v; want [ledger]", got, err)
	}
	for _, gone := range []string{"signal", "console"} {
		if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_mxcli-"+gone+".scss")); err == nil {
			t.Errorf("%s's partial survived the narrowing", gone)
		}
	}
	// Back to a lone theme: unscoped again, so the CSS is what a single-theme
	// project has always had.
	if got := read(t, filepath.Join(dir, "theme", "web", "custom-variables.scss")); !strings.Contains(got, "\n:root {") {
		t.Error("the surviving theme's palette must go back to a bare :root")
	}
}

func TestApplySet_RefusesADuplicateName(t *testing.T) {
	dir := newProject(t)
	if _, err := ApplySet(dir, []string{"signal", "ledger", "signal"}, Options{}); err == nil {
		t.Fatal("a repeated theme must be refused: its scope would be ambiguous")
	}
}

// The generated switcher must only ever offer skins whose CSS is in the page.
func TestSwitcherMDL_SkinActionsMatchTheInstalledSet(t *testing.T) {
	solo := SwitcherMDL("Ops", []string{"signal"})
	if strings.Contains(solo, "CycleAppSkin") {
		t.Error("a set of one must not generate a cycle action that cycles nothing")
	}

	multi := SwitcherMDL("Ops", []string{"signal", "ledger", "console"})
	for _, want := range []string{"Ops.SetAppSkin", "Ops.CycleAppSkin", "Ops.ACT_CycleSkin",
		`["signal", "ledger", "console"]`, `"` + SkinStorageKey + `"`} {
		if !strings.Contains(multi, want) {
			t.Errorf("generated switcher is missing %q", want)
		}
	}
	if strings.Contains(multi, "{{") {
		t.Error("a placeholder was left unexpanded in the generated MDL")
	}
	// The skin axis must be stored separately from the light/dark axis, or
	// picking a theme silently discards the variant choice.
	if SkinStorageKey == SwitcherStorageKey {
		t.Error("skin and variant must not share a storage key")
	}
}

// Every theme vendors a different font family, so they carry different SIL OFL
// licence texts. They used to land at one path, mxcli-fonts/OFL.txt, which was
// survivable for a single theme — the last apply won — and wrong for a set:
// installing three left one licence covering three families, and every apply
// rewrote it, so the set was never idempotent. Two symptoms, one cause.
func TestThemesDoNotClaimTheSameVerbatimPathWithDifferentContent(t *testing.T) {
	themes, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]map[string]string{} // path -> digest -> theme
	for _, th := range themes {
		src := embeddedSource(th.Name)
		root := src.filesRoot()
		err := fs.WalkDir(src.fsys, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel := strings.TrimPrefix(p, root+"/")
			if isBlockFile(rel) {
				return nil // fenced per theme, so sharing a path is fine
			}
			body, err := fs.ReadFile(src.fsys, p)
			if err != nil {
				return err
			}
			sum := fmt.Sprintf("%x", sha256.Sum256(body))
			if byPath[rel] == nil {
				byPath[rel] = map[string]string{}
			}
			byPath[rel][sum] = th.Name
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for rel, digests := range byPath {
		if len(digests) > 1 {
			var owners []string
			for _, n := range digests {
				owners = append(owners, n)
			}
			sort.Strings(owners)
			t.Errorf("%s is written with different content by %s — installing them as a set "+
				"leaves whichever applied last, and the apply never settles",
				rel, strings.Join(owners, ", "))
		}
	}
}
