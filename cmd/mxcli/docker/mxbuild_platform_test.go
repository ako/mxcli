// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeBinary drops a file whose first bytes are magic, so the format checks can
// be exercised without shipping real executables.
func writeBinary(t *testing.T, dir, name string, magic []byte) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, append(magic, []byte("padding-padding")...), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

var (
	elfMagic   = []byte{0x7F, 'E', 'L', 'F'}
	machoMagic = []byte{0xCF, 0xFA, 0xED, 0xFE}
	peMagic    = []byte{'M', 'Z', 0x90, 0x00}
)

func TestBinaryOS(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name  string
		magic []byte
		want  string
	}{
		{"elf", elfMagic, "linux"},
		{"macho", machoMagic, "darwin"},
		{"macho-fat", []byte{0xCA, 0xFE, 0xBA, 0xBE}, "darwin"},
		{"pe", peMagic, "windows"},
		{"shell wrapper is not classified", []byte("#!/b"), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := binaryOS(writeBinary(t, dir, c.name, c.magic)); got != c.want {
				t.Errorf("binaryOS = %q, want %q", got, c.want)
			}
		})
	}

	t.Run("too short to classify", func(t *testing.T) {
		p := filepath.Join(dir, "tiny")
		if err := os.WriteFile(p, []byte{0x7F}, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := binaryOS(p); got != "" {
			t.Errorf("binaryOS = %q, want empty for a 1-byte file", got)
		}
	})
}

// TestVerifyRunsOn_LinuxBinaryOnMac is the #916 failure itself: the cached CDN
// download is a Linux ELF, and macOS produced only `exec format error`. The
// message has to name the cause and a way out.
func TestVerifyRunsOn_LinuxBinaryOnMac(t *testing.T) {
	p := writeBinary(t, t.TempDir(), "mxbuild", elfMagic)

	err := verifyRunsOn(p, "darwin")
	if err == nil {
		t.Fatal("a Linux binary must be refused on darwin")
	}
	for _, want := range []string{"linux binary", "cannot run on darwin", "Studio Pro", "--mxbuild-path"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got:\n%v", want, err)
		}
	}
}

func TestVerifyRunsOn_MatchingAndUnknown(t *testing.T) {
	dir := t.TempDir()
	if err := verifyRunsOn(writeBinary(t, dir, "native", elfMagic), "linux"); err != nil {
		t.Errorf("a matching binary must be accepted: %v", err)
	}
	// A wrapper script has no magic to read; refusing it would block a working
	// setup on a guess.
	if err := verifyRunsOn(writeBinary(t, dir, "wrapper", []byte("#!/b")), "darwin"); err != nil {
		t.Errorf("an unclassifiable file must be allowed through: %v", err)
	}
}

// TestResolveMxBuildForLocal_ExplicitPathWins covers the half that left the
// reporter with no workaround: --mxbuild-path was documented as an override and
// the local path ignored it, calling DownloadMxBuild unconditionally.
func TestResolveMxBuildForLocal_ExplicitPathWins(t *testing.T) {
	dir := t.TempDir()
	explicit := writeBinary(t, dir, "mxbuild", elfMagic)

	got, err := resolveMxBuildForLocalOn("linux", explicit, "11.12.0", &bytes.Buffer{})
	if err != nil {
		t.Fatalf("explicit path rejected: %v", err)
	}
	if got != explicit {
		t.Errorf("resolved %q, want the explicit path %q", got, explicit)
	}
}

// TestResolveMxBuildForLocal_ExplicitPathMustRunHere — an override pointing at a
// foreign binary is refused rather than exec'd.
func TestResolveMxBuildForLocal_ExplicitPathMustRunHere(t *testing.T) {
	explicit := writeBinary(t, t.TempDir(), "mxbuild", machoMagic)

	if _, err := resolveMxBuildForLocalOn("linux", explicit, "11.12.0", &bytes.Buffer{}); err == nil {
		t.Fatal("a darwin binary passed via --mxbuild-path must be refused on linux")
	}
}

// TestResolveMxBuildForLocal_NonLinuxWithoutStudioProRefuses pins the behaviour
// that could not be reached from CI before: on a host the CDN has no build for,
// resolution must fail with guidance instead of downloading a Linux binary and
// exec'ing it. No network is touched — NativeMxBuildForSetup returns the error
// before any download is attempted.
func TestResolveMxBuildForLocal_NonLinuxWithoutStudioProRefuses(t *testing.T) {
	// A version no Studio Pro install will match, so the darwin branch reaches
	// its "no native mxbuild" outcome on any machine.
	var out bytes.Buffer
	_, err := resolveMxBuildForLocalOn("darwin", "", "99.99.99", &out)
	if err == nil {
		t.Fatal("darwin without Studio Pro must refuse, not download a Linux binary")
	}
	for _, want := range []string{"Linux binary", "darwin"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got:\n%v", want, err)
		}
	}
	if !strings.Contains(err.Error(), "--mxbuild-path") && !strings.Contains(err.Error(), "Studio Pro") {
		t.Errorf("error should offer a way forward, got:\n%v", err)
	}
	if out.Len() > 0 {
		t.Errorf("nothing should have been downloaded, but progress was written:\n%s", out.String())
	}
}
