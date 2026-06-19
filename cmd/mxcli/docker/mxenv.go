// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// systemFreeTypePath returns the path to the system libfreetype.so.6, or "" if
// none is found. It globs the common (multiarch) library locations so it works
// on amd64, arm64, and other Linux distros without a hardcoded arch path.
func systemFreeTypePath() string {
	patterns := []string{
		"/usr/lib/*/libfreetype.so.6", // Debian/Ubuntu multiarch (x86_64, aarch64, …)
		"/usr/lib/libfreetype.so.6",
		"/lib/*/libfreetype.so.6",
		"/usr/local/lib/libfreetype.so.6",
	}
	for _, pat := range patterns {
		matches, _ := filepath.Glob(pat)
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && !fi.IsDir() {
				return m
			}
		}
	}
	return ""
}

// skiaFreeTypeLib returns the system libfreetype to preload to work around the
// libSkiaSharp "undefined symbol: FT_Get_BDF_Property" crash, or "" when the
// workaround does not apply.
//
// Root cause: mx/mxbuild run under the Temurin JVM, whose bundled libfreetype
// is stripped and lacks FT_Get_BDF_Property. Skia then loads the JVM's FreeType
// instead of the system one (which has the symbol) and aborts. Preloading the
// system libfreetype makes it load first, fixing mx build/run/check while
// keeping Skia working (unlike moving libSkiaSharp.so aside, which disables it).
//
// Returns "" on non-Linux hosts or when no system libfreetype is found, so
// callers leave the environment untouched everywhere the bug can't occur.
func skiaFreeTypeLib() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	return systemFreeTypePath()
}

// PrepareMxCommand injects the FreeType LD_PRELOAD workaround into cmd's
// environment before it runs an mx/mxbuild binary. It is a no-op when the
// workaround does not apply (non-Linux, no system libfreetype) or when an
// LD_PRELOAD already references libfreetype (e.g. the user exported it). When a
// non-libfreetype LD_PRELOAD is already present, the system libfreetype is
// prepended so it loads first.
func PrepareMxCommand(cmd *exec.Cmd) {
	lib := skiaFreeTypeLib()
	if lib == "" {
		return
	}
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	if merged, changed := injectLDPreload(env, lib); changed {
		cmd.Env = merged
	}
}

// injectLDPreload returns env with lib added to LD_PRELOAD, and whether it
// changed anything. If an LD_PRELOAD already references libfreetype it is left
// untouched (changed=false); a non-libfreetype LD_PRELOAD gets lib prepended so
// it loads first. env is not mutated; a copy is returned when changed.
func injectLDPreload(env []string, lib string) (result []string, changed bool) {
	existing := ""
	idx := -1
	for i, e := range env {
		if v, ok := strings.CutPrefix(e, "LD_PRELOAD="); ok {
			existing = v
			idx = i
		}
	}
	if strings.Contains(existing, "libfreetype.so") {
		return env, false
	}
	value := lib
	if existing != "" {
		value = lib + ":" + existing
	}
	entry := "LD_PRELOAD=" + value
	out := append([]string(nil), env...)
	if idx >= 0 {
		out[idx] = entry
	} else {
		out = append(out, entry)
	}
	return out, true
}
