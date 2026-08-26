// SPDX-License-Identifier: Apache-2.0

package theme

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The registry used to be embed-only, which is what made a design-derived
// theme unusable: there was nowhere to put a fourth theme, so the only route
// was hand-editing a generated block — which the digest fence then refuses to
// touch on the next apply.

func TestList_IncludesTheProjectsOwnThemes(t *testing.T) {
	dir := newProject(t)
	mustCreate(t, dir, "acme", CreateOptions{})

	themes, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	var found *Theme
	for i := range themes {
		if themes[i].Name == "acme" {
			found = &themes[i]
		}
	}
	if found == nil {
		t.Fatalf("a theme in %s must be listed; got %v", LocalThemesDir, names(themes))
	}
	if !found.Local {
		t.Error("a project-local theme must be marked Local, or `theme list` cannot tell the user where it came from")
	}

	// Without a project, the embedded set and nothing else.
	embedded, err := List("")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range embedded {
		if e.Name == "acme" {
			t.Error("List(\"\") must not reach into a project")
		}
	}
}

func TestGet_PrefersTheProjectsCopyOverTheBuiltIn(t *testing.T) {
	dir := newProject(t)
	// A local theme named after a built-in shadows it: "signal, but with our
	// brand" is the obvious thing to want, and it must not silently apply the
	// embedded one instead.
	mustCreate(t, dir, "ledger", CreateOptions{Base: DefaultName, Title: "House Ledger"})

	local, err := Get(dir, "ledger")
	if err != nil {
		t.Fatal(err)
	}
	if !local.Local || local.Title != "House Ledger" {
		t.Errorf("the project's own ledger must shadow the built-in: %+v", local)
	}

	builtIn, err := Get("", "ledger")
	if err != nil {
		t.Fatal(err)
	}
	if builtIn.Local || builtIn.Title == "House Ledger" {
		t.Errorf("Get(\"\") must still return the embedded ledger: %+v", builtIn)
	}
}

func TestApply_InstallsAProjectLocalTheme(t *testing.T) {
	dir := newProject(t)
	mustCreate(t, dir, "acme", CreateOptions{})

	res, err := Apply(dir, "acme", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed() {
		t.Fatal("applying a project-local theme wrote nothing")
	}

	main := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
	if !strings.Contains(main, `@import "mxcli-acme"`) {
		t.Errorf("main.scss must import the created theme's partial:\n%s", main)
	}
	if !strings.Contains(main, "mxcli:theme:begin acme") {
		t.Error("a project-local theme must be fenced like any other")
	}
	if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_mxcli-acme.scss")); err != nil {
		t.Errorf("the partial must be renamed to the new theme: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_mxcli-signal.scss")); err == nil {
		t.Error("the base theme's partial must not be written under its own name")
	}

	// Resolve and Remove have to see it too, or a bare `theme remove` on a
	// project themed locally silently removes nothing.
	if got, err := Resolve(dir, ""); err != nil || got != "acme" {
		t.Errorf("Resolve = %q, %v; want acme", got, err)
	}
	if _, err := Remove(dir, "acme", Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, filepath.Join(dir, "theme", "web", "main.scss")), "mxcli:theme") {
		t.Error("remove left the block behind")
	}
}

func TestApply_ALocalThemeReplacesAnInstalledBuiltIn(t *testing.T) {
	dir := newProject(t)
	if _, err := Apply(dir, DefaultName, Options{}); err != nil {
		t.Fatal(err)
	}
	mustCreate(t, dir, "acme", CreateOptions{})
	if _, err := Apply(dir, "acme", Options{}); err != nil {
		t.Fatal(err)
	}

	main := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
	if strings.Contains(main, "mxcli:theme:begin "+DefaultName) {
		t.Error("the built-in theme's block survived; two themes now map the same Atlas variables")
	}
	if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_mxcli-"+DefaultName+".scss")); err == nil {
		t.Error("the built-in theme's partial survived")
	}
	if got, err := Installed(dir); err != nil || len(got) != 1 || got[0] != "acme" {
		t.Errorf("Installed = %v, %v; want [acme]", got, err)
	}
}

// Renaming the mixin is what makes a scaffolded theme a *different* theme.
// Two themes both defining @mixin mxcli-signal-dark collide the moment both
// exist, and main.scss would @import a partial that is not there.
func TestCreate_RenamesEveryIdentifierBuiltFromTheThemeName(t *testing.T) {
	dir := newProject(t)
	mustCreate(t, dir, "acme", CreateOptions{Base: DefaultName})

	files := filepath.Join(dir, filepath.FromSlash(LocalThemesDir), "acme", "files", "theme", "web")
	partial := read(t, filepath.Join(files, "_mxcli-acme.scss"))
	if !strings.Contains(partial, "@mixin mxcli-acme-dark") {
		t.Error("the alt-palette mixin must be renamed to the new theme")
	}
	if strings.Contains(partial, "mxcli-signal") {
		t.Errorf("the base theme's identifiers survived in the partial:\n%s", firstMatch(partial, `.*mxcli-signal.*`))
	}
	main := read(t, filepath.Join(files, "main.scss"))
	if !strings.Contains(main, `@import "mxcli-acme"`) || strings.Contains(main, `@import "mxcli-signal"`) {
		t.Errorf("main.scss must import the renamed partial:\n%s", main)
	}
	// The apply-time placeholders must survive the copy. Expanding any of them
	// at create time would bake in whatever variant the scaffold was made
	// under, or pin the theme to a fixed selector so it could never join a
	// switchable set.
	if !strings.Contains(main, "{{SHARED_HEAD}}") {
		t.Error("{{SHARED_HEAD}} was expanded at create time; the shared imports must stay templated")
	}
	for _, want := range []string{"{{VARIANT}}", "{{SCOPE}}", "{{SCOPED}}"} {
		if !strings.Contains(partial, want) {
			t.Errorf("%s was expanded in the partial at create time", want)
		}
	}
	palette := read(t, filepath.Join(files, "custom-variables.scss"))
	if !strings.Contains(palette, "{{PALETTE_SCOPE}}") {
		t.Error("{{PALETTE_SCOPE}} was expanded at create time; the palette could never be scoped")
	}
}

func TestCreate_SharedPartialsAreCopiedByteForByte(t *testing.T) {
	dir := newProject(t)
	mustCreate(t, dir, "acme", CreateOptions{Base: "console"})

	src := embeddedSource("console")
	for _, shared := range []string{"_mxcli-atlas-map.scss", "_mxcli-widgets.scss"} {
		want, err := src.fsys.(interface{ ReadFile(string) ([]byte, error) }).ReadFile(
			src.filesRoot() + "/theme/web/" + shared)
		if err != nil {
			t.Fatal(err)
		}
		got := read(t, filepath.Join(dir, filepath.FromSlash(LocalThemesDir), "acme", "files", "theme", "web", shared))
		if got != string(want) {
			t.Errorf("%s must be copied verbatim — it is identical in every theme and is where most of the detail lives", shared)
		}
	}
}

func TestCreate_SeedsThePaletteFromADesignArtifact(t *testing.T) {
	dir := newProject(t)
	design := filepath.Join(dir, "design.css")
	write(t, design, `
:root {
  --mxt-brand: #7f5af0;
  --mxt-ground: #fffffe;
  --mxt-radius: 12px;
}
@media (prefers-color-scheme: dark) {
  :root { --mxt-ground: #16161a; --mxt-brand: #a78bfa; }
}
`)

	res := mustCreate(t, dir, "acme", CreateOptions{From: design})
	if res.Tokens == nil || res.Tokens.Count() != 5 {
		t.Fatalf("expected 5 tokens read, got %+v", res.Tokens)
	}

	files := filepath.Join(dir, filepath.FromSlash(LocalThemesDir), "acme", "files", "theme", "web")
	palette := read(t, filepath.Join(files, "custom-variables.scss"))
	for _, want := range []string{"--mxt-brand: #7f5af0;", "--mxt-ground: #fffffe;", "--mxt-radius: 12px;"} {
		if !strings.Contains(palette, want) {
			t.Errorf("palette is missing %q:\n%s", want, palette)
		}
	}
	// A token the design did not mention keeps the base theme's value, so a
	// three-colour design still yields a complete, working palette.
	if !strings.Contains(palette, "--mxt-ink:") {
		t.Error("tokens the design did not name must survive from the base theme")
	}
	// The section comments have to survive too, or a reseeded palette is an
	// unreadable wall of declarations.
	if !strings.Contains(palette, "surfaces and ink") {
		t.Error("the palette's section comments were lost")
	}

	partial := read(t, filepath.Join(files, "_mxcli-acme.scss"))
	mixin := betweenBraces(t, partial, "@mixin mxcli-acme-dark")
	if !strings.Contains(mixin, "--mxt-ground: #16161a;") || !strings.Contains(mixin, "--mxt-brand: #a78bfa;") {
		t.Errorf("the dark block must seed the alt-palette mixin:\n%s", mixin)
	}
	// The rules below the mixin read tokens through var(); substituting a
	// value there would hardcode a colour into a rule.
	below := partial[strings.Index(partial, mixin)+len(mixin):]
	if strings.Contains(below, "#a78bfa") {
		t.Error("a seeded value leaked out of the mixin into the rules")
	}
}

func TestCreate_UnknownTokenIsAnErrorNotASilentNoOp(t *testing.T) {
	dir := newProject(t)
	design := filepath.Join(dir, "design.css")
	// --mxt-brand-color is the plausible typo: nothing reads it, so without
	// this check the theme applies cleanly and renders unchanged.
	write(t, design, ":root { --mxt-brand-color: #7f5af0; --mxt-ground: #fff; }")

	_, err := Create(dir, "acme", CreateOptions{From: design})
	if err == nil {
		t.Fatal("an unrecognised token must be reported, not written into a theme nothing reads")
	}
	if !strings.Contains(err.Error(), "--mxt-brand-color") {
		t.Errorf("the error must name the offending token: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, filepath.FromSlash(LocalThemesDir), "acme")); statErr == nil {
		t.Error("nothing must be written when validation fails")
	}
}

func TestCreate_FromNamesEitherTaneThemeOrAFile(t *testing.T) {
	dir := newProject(t)

	res := mustCreate(t, dir, "from-theme", CreateOptions{From: "console"})
	if res.Base != "console" || res.Tokens != nil {
		t.Errorf("--from console must scaffold from console with no tokens: %+v", res)
	}

	_, err := Create(dir, "from-nothing", CreateOptions{From: "consoel"})
	if err == nil || !strings.Contains(err.Error(), "neither a theme name") {
		t.Errorf("a --from that is neither must say so plainly: %v", err)
	}
}

func TestCreate_RefusesToOverwriteWithoutForce(t *testing.T) {
	dir := newProject(t)
	mustCreate(t, dir, "acme", CreateOptions{})

	edited := filepath.Join(dir, filepath.FromSlash(LocalThemesDir), "acme", "files", "theme", "web", "custom-variables.scss")
	write(t, edited, "/* hand tuned */\n:root { --mxt-brand: #123456; }\n")

	if _, err := Create(dir, "acme", CreateOptions{}); err == nil {
		t.Fatal("re-creating over an existing theme must be refused")
	}
	if !strings.Contains(read(t, edited), "hand tuned") {
		t.Error("the refusal must leave the existing theme untouched")
	}
	if _, err := Create(dir, "acme", CreateOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(read(t, edited), "hand tuned") {
		t.Error("--force must overwrite")
	}
}

func TestCreate_DryRunWritesNothing(t *testing.T) {
	dir := newProject(t)
	res, err := Create(dir, "acme", CreateOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) == 0 {
		t.Error("a dry run must still report what it would write")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(LocalThemesDir))); !os.IsNotExist(err) {
		t.Error("--dry-run wrote to disk")
	}
}

func TestCreate_RejectsNamesThatWalkOutOfTheThemesFolder(t *testing.T) {
	dir := newProject(t)
	for _, name := range []string{"../escape", "a/b", ".hidden", "", "."} {
		if _, err := Create(dir, name, CreateOptions{}); err == nil {
			t.Errorf("Create(%q) must be refused", name)
		}
		if _, err := Get(dir, name); err == nil {
			t.Errorf("Get(%q) must be refused", name)
		}
	}
}

func TestCreate_ManifestDescribesTheNewThemeNotTheBase(t *testing.T) {
	dir := newProject(t)
	mustCreate(t, dir, "acme", CreateOptions{Base: "ledger", Summary: "House style"})

	got, err := Get(dir, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Acme" || got.Summary != "House style" {
		t.Errorf("manifest = %+v; want title Acme, summary House style", got)
	}
	if got.DefaultVariant != mustGet(t, "ledger").DefaultVariant {
		t.Error("the scaffold's default variant must carry across, or the alt mixin seeds the wrong palette")
	}
	for _, f := range got.Files {
		if strings.Contains(f.Path, "_mxcli-ledger") {
			t.Errorf("theme.json still points at the base theme's partial: %s", f.Path)
		}
	}
	// Every file the manifest promises must actually be there — `theme show`
	// reads this list, and apply walks the tree.
	paths, err := assetPaths(dir, "acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got.Files {
		if strings.HasSuffix(f.Path, "/") {
			continue // a directory entry, e.g. the fonts
		}
		if !paths[f.Path] {
			t.Errorf("theme.json lists %s but the tree does not have it", f.Path)
		}
	}
}

func mustCreate(t *testing.T, dir, name string, opts CreateOptions) *CreateResult {
	t.Helper()
	res, err := Create(dir, name, opts)
	if err != nil {
		t.Fatalf("Create(%q): %v", name, err)
	}
	return res
}

func mustGet(t *testing.T, name string) *Theme {
	t.Helper()
	got, err := Get("", name)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func names(themes []Theme) []string {
	var out []string
	for _, t := range themes {
		out = append(out, t.Name)
	}
	return out
}

func firstMatch(s, pattern string) string {
	if m := regexp.MustCompile(pattern).FindString(s); m != "" {
		return m
	}
	return "(no match)"
}

// betweenBraces returns the body of the first block opened after header.
func betweenBraces(t *testing.T, s, header string) string {
	t.Helper()
	i := strings.Index(s, header)
	if i < 0 {
		t.Fatalf("%q not found", header)
	}
	depth := 0
	start := -1
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '{':
			if depth == 0 {
				start = j + 1
			}
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start:j]
			}
		}
	}
	t.Fatalf("unbalanced block after %q", header)
	return ""
}

// `theme remove` deletes what a theme owns, so a hand-authored tree that
// claimed a file inside the themes folder would delete theme sources the first
// time the project switched themes.
func TestApply_RefusesAThemeThatWritesIntoTheThemesFolder(t *testing.T) {
	dir := newProject(t)
	mustCreate(t, dir, "acme", CreateOptions{})

	rogue := filepath.Join(dir, filepath.FromSlash(LocalThemesDir), "acme",
		"files", filepath.FromSlash(LocalThemesDir), "acme", "theme.json")
	write(t, rogue, "{}")

	_, err := Apply(dir, "acme", Options{})
	if err == nil {
		t.Fatal("a theme owning files under the themes folder must be refused")
	}
	if !strings.Contains(err.Error(), LocalThemesDir) {
		t.Errorf("the refusal must name the folder: %v", err)
	}
}

// A scaffolded theme is SCSS that has to compile, and none of the Go tests run
// Sass. The naming half of that contract is the part a copy can plausibly get
// wrong — a partial still defining @mixin mxcli-signal-dark while main.scss
// includes mxcli-acme-dark compiles to nothing and fails at the user's build,
// not in CI — so a created theme is held to exactly the structural contract
// every built-in one is.
func TestCreate_ScaffoldSatisfiesTheSameStructuralContractAsABuiltIn(t *testing.T) {
	for _, base := range []string{"signal", "ledger", "console"} {
		t.Run(base, func(t *testing.T) {
			dir := newProject(t)
			mustCreate(t, dir, "acme", CreateOptions{Base: base})

			got, err := Get(dir, "acme")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Apply(dir, "acme", Options{}); err != nil {
				t.Fatal(err)
			}

			partial := read(t, filepath.Join(dir, "theme", "web", "_mxcli-acme.scss"))
			assertPartialStructure(t, partial, "acme", got.AltVariant())
			assertFontsResolve(t, dir, partial)

			// main.scss must import exactly the partials that were written.
			main := read(t, filepath.Join(dir, "theme", "web", "main.scss"))
			for _, m := range regexp.MustCompile(`@import "(mxcli-[^"]+)"`).FindAllStringSubmatch(main, -1) {
				if _, err := os.Stat(filepath.Join(dir, "theme", "web", "_"+m[1]+".scss")); err != nil {
					t.Errorf("main.scss imports %q but no such partial was written", m[1])
				}
			}
		})
	}
}

// The scaffold renames the identifiers built from the theme name — the mixin,
// the @import, the partial's filename. It did not rename the vendored font
// licence, so two themes scaffolded from signal both shipped OFL-signal.txt
// (ako/mxcli-ledger #140).
//
// Nothing collided: the content is identical because they genuinely ship the
// same fonts. It bites on the edit the scaffold exists to invite — a brand
// theme that changes its fonts then ships IBM Plex's licence for fonts that
// are not IBM Plex, and the filename is what would have caught it.
func TestCreate_RenamesTheFontLicenceToTheCreatedTheme(t *testing.T) {
	dir := newProject(t)
	mustCreate(t, dir, "acme", CreateOptions{Base: DefaultName})

	fonts := filepath.Join(dir, filepath.FromSlash(LocalThemesDir), "acme", "files", "theme", "web", "mxcli-fonts")
	entries, err := os.ReadDir(fonts)
	if err != nil {
		t.Fatal(err)
	}
	var licences []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "OFL") {
			licences = append(licences, e.Name())
		}
	}
	if len(licences) != 1 || licences[0] != "OFL-acme.txt" {
		t.Errorf("licences = %v; want exactly [OFL-acme.txt] — a base theme's name here "+
			"survives a font change and then covers fonts it does not license", licences)
	}

	// theme.json's prose names the licence, so `theme show` must not describe
	// the new theme's fonts under a filename that is no longer there.
	got, err := Get(dir, "acme")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range got.Files {
		if strings.Contains(f.Purpose, "OFL-"+DefaultName) {
			t.Errorf("theme.json still names the base theme's licence: %q", f.Purpose)
		}
	}
	// And the manifest's paths must match what is on disk.
	paths, err := assetPaths(dir, "acme")
	if err != nil {
		t.Fatal(err)
	}
	if !paths["theme/web/mxcli-fonts/OFL-acme.txt"] {
		t.Errorf("the renamed licence is not in the theme's file set: %v", len(paths))
	}
}

// `theme create` reported paths relative to the scaffold, which read as paths
// in the app: theme/web/custom-variables.scss and theme/web/main.scss listed as
// "created" by the one command whose job is to write into theme/. A project
// with its own hand-written versions of both saw them reported as overwritten
// when nothing had been touched (ako/mxcli-ledger #139).
func TestCreate_ReportsPathsInsideTheScaffoldNotTheApp(t *testing.T) {
	dir := newProject(t)
	res := mustCreate(t, dir, "acme", CreateOptions{})

	for _, f := range res.Files {
		if f.Path == "theme/web/custom-variables.scss" || f.Path == "theme/web/main.scss" {
			t.Errorf("reported %q, which is an app path this command did not write", f.Path)
		}
		// Every reported path must resolve under the scaffold directory.
		if _, err := os.Stat(filepath.Join(res.Dir, filepath.FromSlash(f.Path))); err != nil {
			t.Errorf("reported %q but it is not in the scaffold: %v", f.Path, err)
		}
	}
	// theme.json sits at the scaffold root, the rest under files/ — so a caller
	// prefixing them all with the same root cannot mislabel either.
	var sawManifest bool
	for _, f := range res.Files {
		if f.Path == "theme.json" {
			sawManifest = true
		} else if !strings.HasPrefix(f.Path, "files/") {
			t.Errorf("%q is neither the manifest nor under files/", f.Path)
		}
	}
	if !sawManifest {
		t.Error("theme.json was not reported")
	}
}
