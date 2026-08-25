// SPDX-License-Identifier: Apache-2.0

package theme

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Create scaffolds a project-local theme.
//
// The three built-in themes cover a house style, not a brand. A theme derived
// from a design has to live somewhere mxcli's registry can see it, or the only
// way to use it is to hand-edit the generated block — which the digest fence
// then refuses to touch on the next apply. So a created theme is a first-class
// entry in LocalThemesDir: `theme list` shows it, `theme show` describes it,
// `theme apply <name>` installs it, `theme remove` takes it out again.
//
// Scaffolding from an existing theme rather than from nothing is what keeps
// this small. The Atlas wiring and the widget layer are copied byte for byte —
// they are the same in every theme and are where most of the hard-won detail
// lives — so a new theme is a palette and the recipes on top of it.
func Create(projectDir, name string, opts CreateOptions) (*CreateResult, error) {
	if err := validName(name); err != nil {
		return nil, err
	}
	if projectDir == "" {
		return nil, fmt.Errorf("a project is required to create a theme (pass -p)")
	}
	if err := requireMendixProject(projectDir); err != nil {
		return nil, err
	}

	base, tokens, err := opts.resolve(projectDir)
	if err != nil {
		return nil, err
	}
	if base == name {
		return nil, fmt.Errorf("cannot scaffold %q from itself; pass --base to pick a different starting point", name)
	}

	src, err := sourceFor(projectDir, base)
	if err != nil {
		return nil, err
	}
	baseTheme, err := src.load(base)
	if err != nil {
		return nil, err
	}

	dest := filepath.Join(projectDir, filepath.FromSlash(LocalThemesDir), name)
	if _, err := os.Stat(dest); err == nil && !opts.Force {
		return nil, fmt.Errorf("%s already exists; pass --force to overwrite it", dest)
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	res := &CreateResult{Name: name, Base: base, Dir: dest}
	if tokens != nil {
		res.Tokens = tokens
	}

	rewrite := newRewriter(baseTheme, name, opts.Title)

	// Tokens are validated against the base before anything is written. A
	// design that names --mxt-brand-color instead of --mxt-brand would
	// otherwise produce a theme that compiles, applies and changes nothing —
	// the failure this whole path exists to avoid.
	if tokens != nil {
		if err := validateTokens(src, baseTheme, tokens); err != nil {
			return nil, err
		}
	}

	root := src.filesRoot()
	walkErr := fs.WalkDir(src.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, root+"/")
		body, err := fs.ReadFile(src.fsys, p)
		if err != nil {
			return err
		}

		out := body
		if isBlockFile(rel) {
			text := rewrite.text(string(body))
			if tokens != nil {
				// The rewritten path, not the base's: the partial has just
				// been renamed to _mxcli-<name>.scss and that is what the
				// seeder matches on.
				text, err = seedTokens(rewrite.path(rel), text, baseTheme, name, tokens)
				if err != nil {
					return err
				}
			}
			out = []byte(text)
		}

		target := filepath.Join(dest, "files", filepath.FromSlash(rewrite.path(rel)))
		res.Files = append(res.Files, FileResult{Path: rewrite.path(rel), Action: ActionCreated})
		if opts.DryRun {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, out, 0o644)
	})
	if walkErr != nil {
		return nil, walkErr
	}

	manifest, err := rewrite.manifest(baseTheme, opts)
	if err != nil {
		return nil, err
	}
	res.Files = append(res.Files, FileResult{Path: "theme.json", Action: ActionCreated})
	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	if opts.DryRun {
		return res, nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dest, "theme.json"), manifest, 0o644); err != nil {
		return nil, err
	}
	return res, nil
}

// CreateOptions tunes Create.
type CreateOptions struct {
	// From is either the name of a theme to scaffold from, or the path to a
	// design artifact declaring --mxt-* tokens. One flag, because "create a
	// theme from X" is the sentence a user actually has in mind; which of the
	// two X is gets decided by whether it names a theme.
	From string
	// Base overrides the theme to scaffold from when From is an artifact.
	Base string
	// Title and Summary override what `theme list` and `theme show` display.
	Title, Summary string
	// Force overwrites an existing project-local theme of the same name.
	Force bool
	// DryRun reports what would be written without writing it.
	DryRun bool
}

// resolve works out the base theme and, if one was given, the design tokens.
func (o CreateOptions) resolve(projectDir string) (string, *Tokens, error) {
	base := o.Base
	from := strings.TrimSpace(o.From)

	// A bare --base with no --from is a scaffold-only create.
	if from == "" {
		if base == "" {
			base = DefaultName
		}
		return base, nil, nil
	}

	// --from names a theme: scaffold from it, no tokens.
	if _, err := sourceFor(projectDir, from); err == nil {
		if base != "" && base != from {
			return "", nil, fmt.Errorf("--from %s names a theme, so --base %s is ambiguous; pass one or the other", from, base)
		}
		return from, nil, nil
	}

	// --from is a path: read tokens out of it.
	raw, err := os.ReadFile(from)
	if err != nil {
		if os.IsNotExist(err) {
			// Name both readings, and list the themes actually visible from
			// here — `mxcli theme list` without -p would hide the project's own,
			// which is exactly the set a user working in a project means.
			return "", nil, fmt.Errorf("--from %q is neither a theme name nor a readable file\n\n%s\n\n"+
				"To seed a palette from a design, --from takes a path to a file declaring --mxt-* tokens.",
				from, availableThemes(projectDir))
		}
		return "", nil, err
	}
	tokens := ExtractTokens(filepath.Base(from), string(raw))
	if tokens.Count() == 0 {
		return "", nil, fmt.Errorf("%s declares no --mxt-* tokens\n\n"+
			"A design artifact seeds a theme by declaring the palette it wants, e.g.\n"+
			"  :root { --mxt-brand: #7f5af0; --mxt-ground: #fffffe; }\n"+
			"Run `mxcli theme show %s` for the full token list.", from, DefaultName)
	}
	if base == "" {
		base = DefaultName
	}
	return base, tokens, nil
}

// CreateResult is what Create wrote.
type CreateResult struct {
	Name   string
	Base   string
	Dir    string
	Files  []FileResult
	Tokens *Tokens
}

// rewriter carries the renames that turn a copy of one theme into another:
// the partial's filename, the mixin and @import identifiers built from the
// name, and the title as it reads in the file comments.
type rewriter struct {
	baseName, baseTitle string
	newName, newTitle   string
}

func newRewriter(base *Theme, newName, newTitle string) *rewriter {
	if newTitle == "" {
		newTitle = defaultTitle(newName)
	}
	return &rewriter{baseName: base.Name, baseTitle: base.Title, newName: newName, newTitle: newTitle}
}

// defaultTitle turns "acme-dark" into "Acme Dark".
func defaultTitle(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// path renames the base theme's partial, which is the only filename carrying
// the theme's name.
func (r *rewriter) path(rel string) string {
	return strings.ReplaceAll(rel, "_mxcli-"+r.baseName+".scss", "_mxcli-"+r.newName+".scss")
}

// text renames the identifiers built from the theme name — the alt-palette
// mixin and the @import of the partial — and retitles the prose.
//
// The identifier rename must happen for the theme to work at all: the partial
// defines @mixin mxcli-<name>-<alt> and main.scss imports "mxcli-<name>". Two
// themes sharing a mixin name would collide the moment both were on disk.
func (r *rewriter) text(s string) string {
	s = strings.ReplaceAll(s, "mxcli-"+r.baseName, "mxcli-"+r.newName)
	if r.baseTitle != "" && r.baseTitle != r.newTitle {
		s = regexp.MustCompile(`\b`+regexp.QuoteMeta(r.baseTitle)+`\b`).ReplaceAllString(s, r.newTitle)
	}
	return s
}

// manifest writes the new theme's theme.json, derived from the base's so the
// file descriptions stay accurate.
func (r *rewriter) manifest(base *Theme, opts CreateOptions) ([]byte, error) {
	t := *base
	t.Name = r.newName
	t.Title = r.newTitle
	t.Local = false
	t.Version = "1"
	if opts.Summary != "" {
		t.Summary = opts.Summary
	}
	t.Description = fmt.Sprintf("Project-local theme, scaffolded from %s by `mxcli theme create`. "+
		"Edit the palette in files/theme/web/custom-variables.scss and the dark palette in "+
		"files/theme/web/_mxcli-%s.scss, then re-run `mxcli theme apply %s`.",
		base.Title, r.newName, r.newName)

	t.Files = append([]FileSpec(nil), base.Files...)
	for i := range t.Files {
		t.Files[i].Path = r.path(t.Files[i].Path)
		t.Files[i].Purpose = r.text(t.Files[i].Purpose)
	}

	out, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// seedTokens writes the extracted palette into the two files that carry one:
// custom-variables.scss holds the default variant, and the alt-palette mixin
// in the theme's partial holds the other.
func seedTokens(rel, text string, base *Theme, newName string, tokens *Tokens) (string, error) {
	switch {
	case path.Base(rel) == "custom-variables.scss":
		set := tokens.forVariant(base.DefaultVariant)
		out, unplaced := applyTokens(text, set)
		return appendTokens(out, unplaced, set, tokens.Source)

	case path.Base(rel) == "_mxcli-"+newName+".scss":
		// Only the mixin body may be rewritten. The rest of the partial is
		// rules that *read* tokens through var(), and a value substituted
		// there would hardcode a colour into a rule — exactly the mistake the
		// palette/wiring split exists to prevent.
		//
		// The body is bounded by the first `}` at column 0, which holds
		// because an alt-palette mixin is a flat list of declarations. A
		// nested block inside one would truncate the span; the structural
		// contract asserted in create_test.go is what would catch that.
		mixin := regexp.MustCompile(`(?s)(@mixin\s+mxcli-` + regexp.QuoteMeta(newName) + `-\w+\s*\{)(.*?)(\n\})`)
		m := mixin.FindStringSubmatchIndex(text)
		if m == nil {
			return "", fmt.Errorf("%s: no alt-palette mixin to seed", rel)
		}
		alt := base.AltVariant()
		set := tokens.forVariant(alt)
		body := text[m[4]:m[5]]
		seeded, unplaced := applyTokens(body, set)
		// Only a token the design *scoped* to this variant is worth adding to
		// the mixin. A base-only token the mixin does not already restate —
		// --mxt-radius, say — is variant-independent: it lives in the palette,
		// and copying it here would mean retuning it twice.
		unplaced = onlyScoped(unplaced, tokens.scoped(alt))
		if len(unplaced) > 0 {
			var b strings.Builder
			b.WriteString("\n\n  /* from ")
			b.WriteString(tokens.Source)
			b.WriteString(" */\n")
			for _, n := range unplaced {
				fmt.Fprintf(&b, "  %s: %s;", n, set[n])
				b.WriteString("\n")
			}
			seeded += strings.TrimRight(b.String(), "\n")
		}
		return text[:m[4]] + seeded + text[m[5]:], nil
	}
	return text, nil
}

// validateTokens refuses names the base theme does not declare.
//
// An unrecognised --mxt-* token is not a harmless extra: nothing reads it, so
// the theme applies cleanly and renders unchanged, and the only symptom is
// that the design did not take. Naming the typo at create time is the whole
// point of validating at all.
func validateTokens(src source, base *Theme, tokens *Tokens) error {
	names, err := knownTokens(src)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, n := range names {
		known[n] = true
	}

	var unknown []string
	for _, set := range []TokenSet{tokens.Base, tokens.Dark, tokens.Light} {
		for _, n := range set.Names() {
			if !known[n] {
				unknown = append(unknown, n)
			}
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	unknown = dedupe(unknown)
	return fmt.Errorf("%s declares %d token(s) theme %q does not recognise: %s\n\n"+
		"Nothing reads an unknown token, so the theme would apply and render unchanged.\n"+
		"Run `mxcli theme show %s` for the tokens it does read.",
		tokens.Source, len(unknown), base.Name, strings.Join(unknown, ", "), base.Name)
}

// onlyScoped keeps the names the design declared inside a variant block.
func onlyScoped(names []string, scoped TokenSet) []string {
	var out []string
	for _, n := range names {
		if _, ok := scoped[n]; ok {
			out = append(out, n)
		}
	}
	return out
}

func dedupe(sorted []string) []string {
	out := sorted[:0]
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}
