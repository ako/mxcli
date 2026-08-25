// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// The toolbox entry's bitmaps are the half of "expose as" MDL could not reach:
// Studio Pro fills them from PNG files, and until now a script could only ever
// preserve whatever was already there. ICON/IMAGE clauses author them.
//
// Nothing below Studio Pro validates them — mxbuild stores the bytes and
// `mx check` passes — so what these tests pin is the checking mxcli does itself.

// writePNG writes a w×h PNG and returns its path.
//
// The first pixel is derived from the file name so that two same-sized bitmaps
// have different bytes — otherwise a test asserting the four targets are routed
// separately would pass against a build that wrote one file into all four.
func writePNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var tint uint8
	for _, c := range name {
		tint += uint8(c)
	}
	img.Set(0, 0, color.RGBA{R: tint, G: 0xaf, B: 0x50, A: 0xff})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding png: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

// scriptDirCtx builds the minimal context the bitmap loader needs: the directory
// a relative path resolves against.
func scriptDirCtx(dir string) *ExecContext { return &ExecContext{ScriptDir: dir} }

// dirOf returns the directory the fixtures were written to. The tests pass
// absolute paths, so this only matters for the nil-context cases.
func dirOf(bitmaps []ast.ExposeBitmap) string {
	for _, b := range bitmaps {
		if b.Path != "" {
			return filepath.Dir(b.Path)
		}
	}
	return ""
}

func mergeWithBitmaps(t *testing.T, stored *javaactions.MicroflowActionInfo, bitmaps []ast.ExposeBitmap) (*javaactions.MicroflowActionInfo, []string) {
	t.Helper()
	var warnings []string
	out, err := mergeMicroflowActionInfo(scriptDirCtx(dirOf(bitmaps)), stored, "Ping", "My first module", false, bitmaps,
		func(msg string) { warnings = append(warnings, msg) })
	if err != nil {
		t.Fatalf("mergeMicroflowActionInfo: %v", err)
	}
	return out, warnings
}

func TestExposeBitmapsAreReadFromDisk(t *testing.T) {
	dir := t.TempDir()
	icon := writePNG(t, dir, "icon.png", 64, 64)
	iconDark := writePNG(t, dir, "icon-dark.png", 64, 64)
	img := writePNG(t, dir, "image.png", 256, 192)
	imgDark := writePNG(t, dir, "image-dark.png", 256, 192)

	got, warnings := mergeWithBitmaps(t, nil, []ast.ExposeBitmap{
		{Path: icon},
		{Path: iconDark, Dark: true},
		{Path: img, Image: true},
		{Path: imgDark, Image: true, Dark: true},
	})

	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none — every bitmap is the size Studio Pro asks for", warnings)
	}
	for _, tc := range []struct {
		field string
		data  []byte
	}{
		{"IconData", got.IconData},
		{"IconDataDark", got.IconDataDark},
		{"ImageData", got.ImageData},
		{"ImageDataDark", got.ImageDataDark},
	} {
		if len(tc.data) == 0 {
			t.Errorf("%s is empty — the file was not read", tc.field)
			continue
		}
		if !bytes.HasPrefix(tc.data, []byte("\x89PNG")) {
			t.Errorf("%s does not start with the PNG magic; the bytes were mangled", tc.field)
		}
	}
	// The four must not be the same file: mixing up the targets is the easy bug,
	// and every one of them being "some PNG" would hide it.
	if bytes.Equal(got.IconData, got.ImageData) {
		t.Error("IconData and ImageData hold the same bytes — the clauses were not routed separately")
	}
	if bytes.Equal(got.IconData, got.IconDataDark) {
		t.Error("IconData and IconDataDark hold the same bytes — DARK was ignored")
	}
}

// An omitted clause preserves; only DROP clears. Without this the ICON clause
// would have re-introduced the very data loss it was added alongside.
func TestExposeBitmapOmittedIsPreservedAndDropClears(t *testing.T) {
	dir := t.TempDir()
	stored := &javaactions.MicroflowActionInfo{
		Caption:   "Ping",
		IconData:  []byte("STORED-ICON"),
		ImageData: []byte("STORED-IMAGE"),
	}

	// No bitmap clauses at all.
	got, _ := mergeWithBitmaps(t, stored, nil)
	if !bytes.Equal(got.IconData, []byte("STORED-ICON")) || !bytes.Equal(got.ImageData, []byte("STORED-IMAGE")) {
		t.Fatalf("an absent ICON/IMAGE clause cleared a stored bitmap: %q / %q", got.IconData, got.ImageData)
	}

	// Replace the icon, leave the image alone.
	got, _ = mergeWithBitmaps(t, stored, []ast.ExposeBitmap{{Path: writePNG(t, dir, "new.png", 64, 64)}})
	if bytes.Equal(got.IconData, []byte("STORED-ICON")) {
		t.Error("ICON clause did not replace the stored icon")
	}
	if !bytes.Equal(got.ImageData, []byte("STORED-IMAGE")) {
		t.Errorf("ImageData = %q, want the stored bytes — only the icon was named", got.ImageData)
	}

	// DROP ICON clears exactly one.
	got, _ = mergeWithBitmaps(t, stored, []ast.ExposeBitmap{{Clear: true}})
	if len(got.IconData) != 0 {
		t.Errorf("IconData = %q, want empty after DROP ICON", got.IconData)
	}
	if !bytes.Equal(got.ImageData, []byte("STORED-IMAGE")) {
		t.Errorf("DROP ICON also cleared the image: %q", got.ImageData)
	}
}

// A file that is not a PNG is refused. Mendix would store the bytes and
// `mx check` would pass; the only symptom is a blank space in a toolbox nobody
// headless ever opens.
func TestExposeBitmapRefusesNonPNG(t *testing.T) {
	dir := t.TempDir()
	notPNG := filepath.Join(dir, "icon.jpg")
	if err := os.WriteFile(notPNG, []byte("\xff\xd8\xff\xe0 JFIF-ish but not a PNG"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := mergeMicroflowActionInfo(nil, nil, "Ping", "Cat", false,
		[]ast.ExposeBitmap{{Path: notPNG}}, nil)
	if err == nil {
		t.Fatal("a non-PNG was accepted; Studio Pro renders nothing for it")
	}
	if !strings.Contains(err.Error(), "PNG") {
		t.Errorf("error = %q, want it to say the file is not a PNG", err)
	}
}

// A missing file is an error, not a silently-empty icon.
func TestExposeBitmapRefusesMissingFile(t *testing.T) {
	_, err := mergeMicroflowActionInfo(nil, nil, "Ping", "Cat", false,
		[]ast.ExposeBitmap{{Path: "/nonexistent/icon.png"}}, nil)
	if err == nil {
		t.Fatal("a missing bitmap file was accepted")
	}
}

// The wrong size is a warning, not a refusal: Studio Pro's own wording is
// "should be", and it scales. But it has to be said — the size is invisible
// everywhere else in a headless workflow.
func TestExposeBitmapWarnsOnWrongSize(t *testing.T) {
	dir := t.TempDir()
	got, warnings := mergeWithBitmaps(t, nil, []ast.ExposeBitmap{
		{Path: writePNG(t, dir, "small.png", 32, 32)},
	})
	if len(got.IconData) == 0 {
		t.Fatal("a mis-sized icon was dropped; it should be written with a warning")
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one", warnings)
	}
	w := warnings[0]
	for _, want := range []string{"32x32", "64x64"} {
		if !strings.Contains(w, want) {
			t.Errorf("warning %q does not mention %s", w, want)
		}
	}
}

// The control: a correctly-sized image does not warn. Without it, "warns on
// wrong size" could be satisfied by warning on everything.
func TestExposeBitmapDoesNotWarnOnCorrectImageSize(t *testing.T) {
	dir := t.TempDir()
	_, warnings := mergeWithBitmaps(t, nil, []ast.ExposeBitmap{
		{Path: writePNG(t, dir, "image.png", 256, 192), Image: true},
	})
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none for a 256x192 image", warnings)
	}
}

// A relative bitmap path names a file next to the SCRIPT, not next to whoever
// invoked mxcli.
//
// Resolving against the working directory made a script runnable from exactly
// one place. That is not a theoretical complaint: it broke
// TestMxCheck_DoctypeScripts, whose working directory is this package, and it
// broke it only in CI — locally the example had always been run from its own
// directory.
func TestExposeBitmapResolvesAgainstTheScriptDirectory(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, dir, "icon.png", 64, 64)

	// The path is relative and the working directory is this package, which has
	// no icon.png — only ScriptDir can find it.
	got, err := mergeMicroflowActionInfo(scriptDirCtx(dir), nil, "Ping", "Cat", false,
		[]ast.ExposeBitmap{{Path: "icon.png"}}, nil)
	if err != nil {
		t.Fatalf("a relative path did not resolve against the script's directory: %v", err)
	}
	if len(got.IconData) == 0 {
		t.Fatal("IconData is empty")
	}

	// The control: with no ScriptDir the same relative path falls back to the
	// working directory and is not found, which is what CI reported.
	if _, err := mergeMicroflowActionInfo(nil, nil, "Ping", "Cat", false,
		[]ast.ExposeBitmap{{Path: "icon.png"}}, nil); err == nil {
		t.Error("expected the working-directory fallback to fail here; if it found a file, " +
			"this test proves nothing about ScriptDir")
	}
}
