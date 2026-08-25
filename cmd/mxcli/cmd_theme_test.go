// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/theme"
	"github.com/spf13/cobra"
)

// runTheme drives the real cobra command rather than the package API. That
// distinction is the whole point of this file: the bug these tests cover lived
// in the command's argument handling, not in the theme package, so a test that
// calls theme.Resolve directly would keep passing while the CLI stayed broken.
func runTheme(t *testing.T, args ...string) (string, error) {
	t.Helper()
	for _, c := range []*cobra.Command{themeApplyCmd, themeRemoveCmd, themeCreateCmd, themeListCmd, themeShowCmd} {
		resetCmdFlags(c)
	}

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{"theme"}, args...))
	err := rootCmd.ExecuteContext(context.Background())
	return out.String(), err
}

// themeProject fakes the parts of a Mendix project a theme touches.
func themeProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for path, body := range map[string]string{
		"App.mpr":                              "",
		"themesource/atlas_core/web/main.scss": "// atlas\n",
		"theme/web/main.scss":                  "@import \"custom-variables\";\n@import \"theme-dark\";\n",
		"theme/web/custom-variables.scss":      ":root {\n  --brand-primary: #264ae5;\n}\n",
	} {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func installedThemes(t *testing.T, dir string) []string {
	t.Helper()
	got, err := theme.Installed(dir)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// `mxcli theme remove -p app.mpr` — the invocation the docs show — used to
// target the default theme regardless of what was installed. On a project
// themed with anything else it removed nothing, reported every file as
// unchanged and exited 0, leaving the theme fully in place.
func TestThemeRemove_BareInvocationRemovesTheInstalledTheme(t *testing.T) {
	dir := themeProject(t)
	if _, err := runTheme(t, "apply", "ledger", "-p", dir); err != nil {
		t.Fatalf("apply ledger: %v", err)
	}
	if got := installedThemes(t, dir); len(got) != 1 || got[0] != "ledger" {
		t.Fatalf("setup failed: installed = %v", got)
	}

	if _, err := runTheme(t, "remove", "-p", dir); err != nil {
		t.Fatalf("bare remove: %v", err)
	}

	if got := installedThemes(t, dir); len(got) != 0 {
		t.Errorf("bare remove left %v installed", got)
	}
	main := filepath.Join(dir, "theme", "web", "main.scss")
	body, err := os.ReadFile(main)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "mxcli:theme") {
		t.Errorf("main.scss still carries a theme block:\n%s", body)
	}
}

// Removing from a project that has no theme is a mistake worth reporting, not
// a no-op that exits 0.
func TestThemeRemove_UnthemedProjectErrors(t *testing.T) {
	dir := themeProject(t)

	_, err := runTheme(t, "remove", "-p", dir)
	if err == nil {
		t.Fatal("expected an error removing from an unthemed project")
	}
	if !strings.Contains(err.Error(), "no mxcli theme found") {
		t.Errorf("error should say what is missing, got: %v", err)
	}
}

// A bare `apply` refreshes what is installed. Silently switching a ledger
// project to the default is as surprising as the remove bug was.
func TestThemeApply_BareInvocationRefreshesTheInstalledTheme(t *testing.T) {
	dir := themeProject(t)
	if _, err := runTheme(t, "apply", "console", "-p", dir); err != nil {
		t.Fatalf("apply console: %v", err)
	}

	if _, err := runTheme(t, "apply", "-p", dir); err != nil {
		t.Fatalf("bare apply: %v", err)
	}

	got := installedThemes(t, dir)
	if len(got) != 1 || got[0] != "console" {
		t.Errorf("bare apply changed the installed theme to %v, want [console]", got)
	}
}

// An unthemed project is exactly when falling back to the default is right.
func TestThemeApply_BareInvocationInstallsTheDefaultWhenThereIsNone(t *testing.T) {
	dir := themeProject(t)

	if _, err := runTheme(t, "apply", "-p", dir); err != nil {
		t.Fatalf("bare apply: %v", err)
	}
	got := installedThemes(t, dir)
	if len(got) != 1 || got[0] != theme.DefaultName {
		t.Errorf("installed = %v, want [%s]", got, theme.DefaultName)
	}
}

// Switching themes must leave exactly one theme behind in every file it
// touches — including the shared Atlas map, where the outgoing block used to
// survive and the incoming one was appended beside it.
func TestThemeApply_SwitchingLeavesNoOrphanBlocks(t *testing.T) {
	dir := themeProject(t)
	for _, name := range []string{"signal", "ledger", "console"} {
		if _, err := runTheme(t, "apply", name, "-p", dir); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if got := installedThemes(t, dir); len(got) != 1 || got[0] != name {
			t.Fatalf("after apply %s, installed = %v", name, got)
		}
	}

	// And a bare remove then leaves nothing, which is the combination the
	// report flagged: switch once, then run the documented removal.
	if _, err := runTheme(t, "remove", "-p", dir); err != nil {
		t.Fatalf("bare remove after switching: %v", err)
	}
	if got := installedThemes(t, dir); len(got) != 0 {
		t.Errorf("remove after switching left %v", got)
	}
}

// captureStdout runs f with os.Stdout redirected. The theme commands print
// with fmt.Printf rather than cmd.Print, so rootCmd.SetOut does not see them.
func captureStdout(t *testing.T, f func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()

	runErr := f()
	os.Stdout = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out, runErr
}

// The whole point of a project-local theme is that the CLI treats it as a
// theme: created here, listed here, applied here. Before the local registry
// there was nowhere to put a fourth theme at all, so a design-derived palette
// had to be hand-edited into a generated block — which the digest fence then
// refuses to touch on the next apply.
func TestThemeCreate_ScaffoldsAThemeTheOtherCommandsThenAccept(t *testing.T) {
	dir := themeProject(t)

	out, err := captureStdout(t, func() error {
		_, e := runTheme(t, "create", "acme", "-p", dir, "--from", "console")
		return e
	})
	if err != nil {
		t.Fatalf("create: %v\n%s", err, out)
	}
	if !strings.Contains(out, "scaffolded from 'console'") {
		t.Errorf("create must say what it scaffolded from:\n%s", out)
	}

	listed, err := captureStdout(t, func() error {
		_, e := runTheme(t, "list", "-p", dir)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listed, "acme") || !strings.Contains(listed, "local") {
		t.Errorf("`theme list -p` must show the project's own theme, marked local:\n%s", listed)
	}

	// ...and must not show it without -p, or the listing depends on where the
	// shell happens to be.
	global, err := captureStdout(t, func() error {
		_, e := runTheme(t, "list")
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(global, "acme") {
		t.Errorf("`theme list` with no project must show only the built-ins:\n%s", global)
	}

	shown, err := captureStdout(t, func() error {
		_, e := runTheme(t, "show", "acme", "-p", dir)
		return e
	})
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(shown, "local to this project") || !strings.Contains(shown, "--mxt-brand") {
		t.Errorf("`theme show` must describe a local theme and print its token vocabulary:\n%s", shown)
	}

	if _, err := runTheme(t, "apply", "acme", "-p", dir); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := installedThemes(t, dir); len(got) != 1 || got[0] != "acme" {
		t.Fatalf("installed = %v; want [acme]", got)
	}
	if _, err := runTheme(t, "remove", "-p", dir); err != nil {
		t.Fatalf("bare remove of a local theme: %v", err)
	}
	if got := installedThemes(t, dir); len(got) != 0 {
		t.Errorf("remove left %v", got)
	}
}

func TestThemeCreate_SeedsFromADesignFileAndReportsWhatItRead(t *testing.T) {
	dir := themeProject(t)
	design := filepath.Join(dir, "canvas.dc.html")
	if err := os.WriteFile(design, []byte(
		"<style>:root{--mxt-brand:#7f5af0;}\n"+
			"@media (prefers-color-scheme: dark){:root{--mxt-brand:#a78bfa;}}</style>"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := captureStdout(t, func() error {
		_, e := runTheme(t, "create", "acme", "-p", dir, "--from", design)
		return e
	})
	if err != nil {
		t.Fatalf("create --from design: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Seeded 2 token(s)") {
		t.Errorf("create must report what it read:\n%s", out)
	}

	palette := filepath.Join(dir, filepath.FromSlash(theme.LocalThemesDir),
		"acme", "files", "theme", "web", "custom-variables.scss")
	body, err := os.ReadFile(palette)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "--mxt-brand: #7f5af0;") {
		t.Errorf("the design's brand colour did not reach the palette:\n%s", body)
	}
}
