// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#951 item 1: `docker check` must never modify the user's project.
//
// `mx update-widgets` rewrites the model (and turns an MPRv2 project into
// MPRv1), and `mx check` itself compiles the theme into theme-cache/ and writes
// deployment/sass/. Measured on an 11.13 project: an MPRv1 project's .mpr was
// rewritten permanently by a plain `docker check`, and theme-cache/ and
// deployment/ were touched on v1 and v2 alike, with or without
// --no-update-widgets. Check now runs both tools on a temporary copy.
//
// The stubs below do to the directory they are given what the real tools do,
// so these tests fail the moment a tool is pointed at the project itself.
package docker

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// treeState records every file's content hash and mtime, and every directory,
// under root.
func treeState(t *testing.T, root string) map[string]string {
	t.Helper()
	state := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			state[rel+"/"] = "dir"
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		state[rel] = hex.EncodeToString(sum[:]) + " " + info.ModTime().UTC().Format(time.RFC3339Nano)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return state
}

func diffStates(before, after map[string]string) []string {
	var out []string
	for k, v := range before {
		if a, ok := after[k]; !ok {
			out = append(out, "removed "+k)
		} else if a != v {
			out = append(out, "changed "+k)
		}
	}
	for k := range after {
		if _, ok := before[k]; !ok {
			out = append(out, "added "+k)
		}
	}
	return out
}

// addProjectDirs gives a fixture the directories mx touches or reads.
func addProjectDirs(t *testing.T, dir string) {
	t.Helper()
	for _, f := range []string{
		"widgets/Some.Widget.mpk",
		"theme-cache/web/theme.compiled.css",
		"theme/web/main.scss",
		"javasource/app/Action.java",
	} {
		p := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("original "+f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// stubTools replaces mx resolution and both mx invocations with stubs that
// mutate the project they are pointed at the way the real tools do, and record
// the paths they were given. Each widget named in staleWidgets gets a CE0463
// from mx check until update-widgets has rewritten the model it checks.
func stubTools(t *testing.T, staleWidgets ...string) (seen *[]string) {
	t.Helper()
	var paths []string
	origResolve, origUW, origCheck := resolveMxForCheck, updateWidgetsCmd, mxCheckCmd
	t.Cleanup(func() { resolveMxForCheck, updateWidgetsCmd, mxCheckCmd = origResolve, origUW, origCheck })

	resolveMxForCheck = func(string, string) (string, error) { return "mx", nil }
	updateWidgetsCmd = func(_, mprPath string, w, _ io.Writer) error {
		paths = append(paths, "update-widgets "+mprPath)
		dir := filepath.Dir(mprPath)
		// Rewrites the model (inlining units on v2) and drops mprcontents/.
		if err := os.WriteFile(mprPath, []byte("rewritten by update-widgets"), 0o644); err != nil {
			return err
		}
		return os.RemoveAll(filepath.Join(dir, "mprcontents"))
	}
	mxCheckCmd = func(_, mprPath string, _ []string, w, _ io.Writer) error {
		paths = append(paths, "check "+mprPath)
		dir := filepath.Dir(mprPath)
		// Compiles the theme and writes the sass entry point.
		if err := os.MkdirAll(filepath.Join(dir, "theme-cache/web"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "theme-cache/web/theme.compiled.css"), []byte("recompiled"), 0o644); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dir, "deployment/sass"), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "deployment/sass/main.scss"), []byte("x"), 0o644); err != nil {
			return err
		}
		// mx reports the project it loaded by path in some messages.
		fmt.Fprintf(w, "Loading %s\n", mprPath)
		if b, _ := os.ReadFile(mprPath); string(b) != "rewritten by update-widgets" && len(staleWidgets) > 0 {
			for _, el := range staleWidgets {
				fmt.Fprintln(w, ce0463Line(el))
			}
			fmt.Fprintf(w, "The app contains: %d errors.\n", len(staleWidgets))
			return fmt.Errorf("exit status 1")
		}
		fmt.Fprintln(w, "The app contains: 0 errors.")
		return nil
	}
	return &paths
}

// The project is untouched in every mode: the default on a clean project (one
// check), the default on a project with a stale widget (the re-check runs
// update-widgets — on the copy), --no-update-widgets, and --update-widgets.
func TestCheck_DoesNotModifyProject(t *testing.T) {
	for _, tc := range []struct {
		name          string
		fixture       func(*testing.T) string
		skipUpdate    bool
		updateFirst   bool
		staleWidgets  []string
		wantUWOnACopy bool
	}{
		{"v1", v1Fixture, false, false, nil, false},
		{"v2", v2Fixture, false, false, nil, false},
		{"v1 stale widget", v1Fixture, false, false, []string{"Data grid 2 'dg'"}, true},
		{"v2 stale widget", v2Fixture, false, false, []string{"Data grid 2 'dg'"}, true},
		{"v1 --no-update-widgets", v1Fixture, true, false, []string{"Data grid 2 'dg'"}, false},
		{"v2 --no-update-widgets", v2Fixture, true, false, []string{"Data grid 2 'dg'"}, false},
		{"v1 --update-widgets", v1Fixture, false, true, []string{"Data grid 2 'dg'"}, true},
		{"v2 --update-widgets", v2Fixture, false, true, []string{"Data grid 2 'dg'"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mprPath := tc.fixture(t)
			projectDir := filepath.Dir(mprPath)
			addProjectDirs(t, projectDir)
			seen := stubTools(t, tc.staleWidgets...)

			tmpRoot := t.TempDir()
			t.Setenv("TMPDIR", tmpRoot)

			before := treeState(t, projectDir)
			var out bytes.Buffer
			err := Check(CheckOptions{
				ProjectPath:        mprPath,
				SkipUpdateWidgets:  tc.skipUpdate,
				UpdateWidgetsFirst: tc.updateFirst,
				Stdout:             &out,
				Stderr:             io.Discard,
			})
			// The stored project fails exactly when it has a CE0463 and the
			// mode checks it as stored.
			if wantErr := len(tc.staleWidgets) > 0 && !tc.updateFirst; (err != nil) != wantErr {
				t.Fatalf("Check error = %v, want error %v\n%s", err, wantErr, out.String())
			}
			after := treeState(t, projectDir)
			if d := diffStates(before, after); len(d) > 0 {
				t.Errorf("docker check modified the project:\n  %s", strings.Join(d, "\n  "))
			}

			// Every tool ran, and none of them on the project itself.
			if len(*seen) == 0 {
				t.Fatal("no mx tool was invoked")
			}
			for _, s := range *seen {
				if strings.HasPrefix(strings.Fields(s)[1], projectDir+string(filepath.Separator)) {
					t.Errorf("mx was pointed at the project itself: %s", s)
				}
			}
			ranUW := false
			for _, s := range *seen {
				ranUW = ranUW || strings.HasPrefix(s, "update-widgets ")
			}
			if ranUW != tc.wantUWOnACopy {
				t.Errorf("update-widgets ran = %v, want %v (%v)", ranUW, tc.wantUWOnACopy, *seen)
			}
			if first := strings.Fields((*seen)[0])[0]; tc.updateFirst != (first == "update-widgets") {
				t.Errorf("first mx step = %q; update-widgets comes first only with --update-widgets (%v)", first, *seen)
			}

			// The output names the project, not the copy, and says what happened.
			got := out.String()
			if strings.Contains(got, tmpRoot) {
				t.Errorf("output leaks the temporary copy's path:\n%s", got)
			}
			if !strings.Contains(got, "Loading "+mprPath) {
				t.Errorf("mx output not reported against the original path:\n%s", got)
			}
			if tc.updateFirst && !strings.Contains(got, "normalised on a temporary copy") {
				t.Errorf("output does not say widgets were normalised on a copy:\n%s", got)
			}
			if tc.wantUWOnACopy && !tc.updateFirst && !strings.Contains(got, staleCE0463Label) {
				t.Errorf("output does not label the stale widget's CE0463:\n%s", got)
			}

			// The copy is cleaned up.
			if entries, _ := os.ReadDir(tmpRoot); len(entries) != 0 {
				t.Errorf("temporary copy left behind in %s: %v", tmpRoot, entries)
			}
		})
	}
}

// TestCopyProjectForCheck_SkipsOutputs pins that build output, VCS metadata and
// caches are not copied: a large project's deployment/ or .git can dwarf the
// model, and mx check needs neither.
func TestCopyProjectForCheck_SkipsOutputs(t *testing.T) {
	src := t.TempDir()
	for _, f := range []string{
		"App.mpr", "mprcontents/ab/cd/x.mxunit", "widgets/W.mpk",
		"themesource/m/web/main.scss", "theme/web/main.scss", "javasource/a/B.java",
		"deployment/run/x.jar", ".git/HEAD", "releases/App.mda", "theme-cache/web/t.css",
		".mxcli/catalog.db", "theme/node_modules/pkg/index.js", ".mendix-cache/x",
	} {
		p := filepath.Join(src, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(f), 0o644)
	}
	dst := t.TempDir()
	if err := copyProjectTree(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"App.mpr", "mprcontents/ab/cd/x.mxunit", "widgets/W.mpk", "themesource/m/web/main.scss", "theme/web/main.scss", "javasource/a/B.java"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err != nil {
			t.Errorf("%s not copied: %v", f, err)
		}
	}
	for _, f := range []string{"deployment", ".git", "releases", "theme-cache", ".mxcli", "theme/node_modules", ".mendix-cache"} {
		if _, err := os.Stat(filepath.Join(dst, f)); err == nil {
			t.Errorf("%s was copied; it is output, VCS metadata or a cache", f)
		}
	}
}

// TestCopyProjectForCheck_TempDirInsideProject: with $TMPDIR inside the project
// the walk meets its own copy; it must not copy the copy into itself.
func TestCopyProjectForCheck_TempDirInsideProject(t *testing.T) {
	proj := t.TempDir()
	mpr := filepath.Join(proj, "App.mpr")
	if err := os.WriteFile(mpr, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(proj, "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	work, cleanup, err := copyProjectToTemp(mpr)
	if err != nil {
		t.Fatalf("copyProjectToTemp: %v", err)
	}
	defer cleanup()
	if b, err := os.ReadFile(work); err != nil || string(b) != "model" {
		t.Fatalf("copy of the model = %q, %v", b, err)
	}
	filepath.WalkDir(filepath.Dir(work), func(p string, d fs.DirEntry, err error) error {
		if err == nil && strings.HasPrefix(d.Name(), "mxcli-copy-") {
			t.Errorf("the copy contains a copy of itself: %s", p)
			return filepath.SkipDir
		}
		return nil
	})
}

func TestCopyProjectForCheck_MissingProject(t *testing.T) {
	if _, _, err := copyProjectToTemp(filepath.Join(t.TempDir(), "nope.mpr")); err == nil {
		t.Error("want an error for a project file that does not exist")
	}
}

// TestWithProjectCopy_WritesStayOnTheCopy: fn gets a copy it may write into, the
// original is untouched, and the copy is gone once fn returns.
func TestWithProjectCopy_WritesStayOnTheCopy(t *testing.T) {
	proj := t.TempDir()
	mpr := filepath.Join(proj, "App.mpr")
	if err := os.WriteFile(mpr, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	var seen string
	err := WithProjectCopy(mpr, func(work string) error {
		seen = work
		if work == mpr {
			t.Error("fn was handed the original project")
		}
		return os.WriteFile(filepath.Join(filepath.Dir(work), "theme-cache.txt"), []byte("mx wrote this"), 0o644)
	})
	if err != nil {
		t.Fatalf("WithProjectCopy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "theme-cache.txt")); err == nil {
		t.Error("a write into the copy reached the original project")
	}
	if _, err := os.Stat(seen); err == nil {
		t.Errorf("the copy %s was not removed", seen)
	}
	if err := WithProjectCopy(filepath.Join(proj, "nope.mpr"), func(string) error { return nil }); err == nil {
		t.Error("want an error for a project file that does not exist")
	}
}

// TestWithProjectCopy_SkipsNamedFoldersButNeverTheProject: a folder inside the
// project that the caller names is left out of the copy; naming the project's own
// folder (an output directory of ".") skips nothing.
func TestWithProjectCopy_SkipsNamedFoldersButNeverTheProject(t *testing.T) {
	proj := t.TempDir()
	mpr := filepath.Join(proj, "App.mpr")
	for p, content := range map[string]string{
		"App.mpr":             "model",
		"dist/Old.mpk":        "earlier package",
		"widgets/w.mpk":       "a widget",
		"userlib/dist/ok.jar": "a deeper folder of the same name",
	} {
		full := filepath.Join(proj, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(work, rel string) bool {
		_, err := os.Stat(filepath.Join(filepath.Dir(work), filepath.FromSlash(rel)))
		return err == nil
	}

	err := WithProjectCopy(mpr, func(work string) error {
		if exists(work, "dist/Old.mpk") {
			t.Error("the skipped folder was copied")
		}
		if !exists(work, "widgets/w.mpk") || !exists(work, "userlib/dist/ok.jar") || !exists(work, "App.mpr") {
			t.Error("something outside the skipped folder was not copied")
		}
		return nil
	}, filepath.Join(proj, "dist"), filepath.Join(t.TempDir(), "elsewhere"))
	if err != nil {
		t.Fatal(err)
	}

	err = WithProjectCopy(mpr, func(work string) error {
		if !exists(work, "App.mpr") || !exists(work, "dist/Old.mpk") {
			t.Error("naming the project folder itself must skip nothing")
		}
		return nil
	}, proj)
	if err != nil {
		t.Fatal(err)
	}
}

// mendixlabs/mxcli#1629: with an mx of the project's minor but another patch,
// `mx update-widgets` refuses the project while `mx check` accepts it. Measured
// with 10.24.18's mx on a project at 10.24.13: update-widgets prints "Project is
// in different Mendix version - 10.24.13.86719" and exits 4; mx check reports 0
// errors. The check carried on, as it should — and then closed with a note that
// widget definitions "were normalised on a temporary copy", a few lines after
// the warning that they were not. Since #646 update-widgets runs first only with
// --update-widgets; the default's re-check is TestCheck_FailedRecheckLeaves
// CE0463Unlabelled.
func TestCheck_FailedUpdateWidgetsIsNotReportedAsNormalised(t *testing.T) {
	mprPath := v1Fixture(t)
	stubTools(t)
	updateWidgetsCmd = func(_, _ string, w, _ io.Writer) error {
		fmt.Fprintln(w, "Project is in different Mendix version - 10.24.13.86719")
		return fmt.Errorf("exit status 4")
	}
	t.Setenv("TMPDIR", t.TempDir())

	var out bytes.Buffer
	if err := Check(CheckOptions{ProjectPath: mprPath, UpdateWidgetsFirst: true, Stdout: &out, Stderr: io.Discard}); err != nil {
		t.Fatalf("a failed update-widgets must not stop the check: %v\n%s", err, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "The app contains: 0 errors.") {
		t.Fatalf("mx check did not run after update-widgets failed:\n%s", got)
	}
	if strings.Contains(got, "were normalised") {
		t.Errorf("output says widget definitions were normalised after update-widgets failed:\n%s", got)
	}
	if !strings.Contains(got, "not normalised") {
		t.Errorf("output does not say the check ran without normalisation:\n%s", got)
	}
}

// docker check never downloads MxBuild: it resolves an mx that is installed or
// cached, and stops when there is none. The docs said it downloaded on first use,
// so on a fresh clone the reader met "mx not found; specify --mxbuild-path
// pointing to Mendix installation directory" (measured with an empty HOME) and
// nothing naming the command that does download.
func TestCheck_MissingMxNamesSetupNotADownload(t *testing.T) {
	mprPath := v1Fixture(t)
	seen := stubTools(t)
	resolveMxForCheck = func(string, string) (string, error) {
		return "", fmt.Errorf("mx not found; specify --mxbuild-path pointing to Mendix installation directory")
	}

	err := Check(CheckOptions{ProjectPath: mprPath, Stdout: io.Discard, Stderr: io.Discard})
	if err == nil {
		t.Fatal("Check succeeded without an mx")
	}
	if len(*seen) != 0 {
		t.Errorf("mx ran without having been found: %v", *seen)
	}
	for _, want := range []string{"does not download", "mxcli setup mxbuild -p " + mprPath} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not say %q:\n%v", want, err)
		}
	}
}
