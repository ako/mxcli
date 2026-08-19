// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

// The Mendix CDN publishes Linux mxbuild archives only — MxBuildCDNURL branches
// on GOARCH, never GOOS, so on an arm64 Mac it fetches a Linux *aarch64* ELF.
// The architecture matches, which is why nothing notices until exec:
//
//	fork/exec ~/.mxcli/mxbuild/11.12.0/modeler/mxbuild: exec format error
//
// `setup mxbuild` has always known this (NativeMxBuildForSetup) and `docker
// build` resolves Studio Pro before the cache (resolveMxBuild). The local loop
// did neither: it called DownloadMxBuild directly, so on macOS and Windows it
// executed whatever Linux binary the cache held. (issue #916)

// ResolveMxBuildForLocal picks the mxbuild that `run --local` / `test --local`
// can actually execute on this host, and reports why when none can.
//
// Order, and why:
//
//  1. An explicit --mxbuild-path wins. It was documented as an override and was
//     silently ignored by the local path, so a user hitting the platform
//     mismatch had no way out.
//  2. On a non-Linux host, Studio Pro BEFORE the cache. The cache may legitimately
//     hold a Linux binary — Windows keeps one for Docker builds — and on macOS it
//     is exactly what a previous `run --local` downloaded. Preferring it is the bug.
//  3. Otherwise (Linux) the cache or a CDN download, unchanged.
//
// Whatever is chosen is checked for executability on this host before it is
// handed back, so a stale or hand-placed foreign binary fails with an
// explanation rather than a raw exec error.
func ResolveMxBuildForLocal(explicitPath, version string, w io.Writer) (string, error) {
	return resolveMxBuildForLocalOn(runtime.GOOS, explicitPath, version, w)
}

// resolveMxBuildForLocalOn is ResolveMxBuildForLocal with the host OS injected,
// so the macOS and Windows branches are testable from any host — the platform
// mismatch this fixes cannot otherwise be exercised in CI.
func resolveMxBuildForLocalOn(goos, explicitPath, version string, w io.Writer) (string, error) {
	if explicitPath != "" {
		resolved, err := resolveMxBuild(explicitPath, version)
		if err != nil {
			return "", err
		}
		if err := verifyRunsOn(resolved, goos); err != nil {
			return "", err
		}
		return resolved, nil
	}

	if native, guidance, err := NativeMxBuildForSetup(goos, version); err != nil {
		// Non-Linux host with no Studio Pro: downloading would cache something
		// that cannot run. Say so, with the same guidance `setup mxbuild` gives.
		return "", fmt.Errorf("%w\n  %s", err, guidance)
	} else if native != "" {
		if err := verifyRunsOn(native, goos); err != nil {
			return "", err
		}
		return native, nil
	}

	downloaded, err := DownloadMxBuild(version, w)
	if err != nil {
		return "", err
	}
	if err := verifyRunsOn(downloaded, goos); err != nil {
		return "", err
	}
	return downloaded, nil
}

// binaryOS reports the OS an executable is built for, from its magic bytes, or
// "" when the format is not recognised (a shell-script wrapper, or anything
// else this does not need to be clever about).
func binaryOS(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var magic [4]byte
	if n, err := f.Read(magic[:]); err != nil || n < 4 {
		return ""
	}
	switch {
	case magic[0] == 0x7F && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F':
		return "linux"
	case magic[0] == 'M' && magic[1] == 'Z':
		return "windows"
	}
	// Mach-O, thin (feedface/feedfacf) or fat (cafebabe), either endianness.
	switch be := uint32(magic[0])<<24 | uint32(magic[1])<<16 | uint32(magic[2])<<8 | uint32(magic[3]); be {
	case 0xFEEDFACE, 0xFEEDFACF, 0xCEFAEDFE, 0xCFFAEDFE, 0xCAFEBABE, 0xBEBAFECA:
		return "darwin"
	}
	return ""
}

// verifyRunsHere refuses a binary built for another operating system.
//
// The raw failure is `fork/exec …: exec format error`, which names neither the
// cause nor a remedy — the reporter of #916 had to run `file` on the cached
// binary to find out. An unrecognised format is allowed through: a script
// wrapper has no magic to read, and guessing wrong would block a working setup.
func verifyRunsHere(path string) error { return verifyRunsOn(path, runtime.GOOS) }

// verifyRunsOn is verifyRunsHere with the host OS injected, so the macOS case
// can be asserted from a Linux CI runner.
func verifyRunsOn(path, goos string) error {
	got := binaryOS(path)
	if got == "" || got == goos {
		return nil
	}
	msg := fmt.Sprintf("mxbuild at %s is a %s binary and cannot run on %s", path, got, goos)
	if got == "linux" && goos != "linux" {
		msg += "\n  The Mendix CDN only publishes Linux mxbuild, so a cached download cannot run here." +
			"\n  Install Mendix Studio Pro for this project's Mendix version, or pass --mxbuild-path" +
			"\n  pointing at its bundled mxbuild."
	} else {
		msg += "\n  Pass --mxbuild-path pointing at an mxbuild built for " + goos + "."
	}
	return fmt.Errorf("%s", msg)
}
