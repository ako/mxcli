// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// MxBuildCacheDir returns the cache directory for a specific MxBuild version.
// Layout: ~/.mxcli/mxbuild/{version}/
func MxBuildCacheDir(version string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".mxcli", "mxbuild", version), nil
}

// MxBuildCDNURL constructs the CDN download URL for MxBuild.
// arm64 -> https://cdn.mendix.com/runtime/arm64-mxbuild-{version}.tar.gz
// amd64 -> https://cdn.mendix.com/runtime/mxbuild-{version}.tar.gz
func MxBuildCDNURL(version, goarch string) string {
	switch goarch {
	case "arm64":
		return fmt.Sprintf("https://cdn.mendix.com/runtime/arm64-mxbuild-%s.tar.gz", version)
	default:
		return fmt.Sprintf("https://cdn.mendix.com/runtime/mxbuild-%s.tar.gz", version)
	}
}

// CachedMxBuildPath returns the path to a cached mxbuild binary for the given version,
// or empty string if not cached.
// On Windows, checks both "mxbuild.exe" and "mxbuild" (Linux binary cached for Docker).
func CachedMxBuildPath(version string) string {
	cacheDir, err := MxBuildCacheDir(version)
	if err != nil {
		return ""
	}
	return cachedBinaryPath(cacheDir, mxbuildBinaryNames())
}

// AnyCachedMxBuildPath searches for any cached mxbuild version.
// Returns the path to the newest cached mxbuild binary found, or empty string.
// On Windows, checks both "mxbuild.exe" and "mxbuild" (Linux binary cached for Docker).
func AnyCachedMxBuildPath() string {
	return anyCachedBinaryPath(mxbuildBinaryNames())
}

func cachedBinaryPath(cacheDir string, names []string) string {
	for _, name := range names {
		bin := filepath.Join(cacheDir, "modeler", name)
		if info, err := os.Stat(bin); err == nil && !info.IsDir() {
			return bin
		}
	}
	return ""
}

func anyCachedBinaryPath(names []string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	var matches []string
	for _, name := range names {
		pattern := filepath.Join(home, ".mxcli", "mxbuild", "*", "modeler", name)
		found, _ := filepath.Glob(pattern)
		matches = append(matches, found...)
	}
	return NewestVersionedPath(matches)
}

func globVersionedMatches(patterns []string) []string {
	var matches []string
	for _, pattern := range patterns {
		found, _ := filepath.Glob(pattern)
		matches = append(matches, found...)
	}
	return matches
}

func exactVersionedPath(paths []string, version string) string {
	if version == "" {
		return ""
	}
	var exact []string
	for _, path := range paths {
		if versionFromPath(path) == version {
			exact = append(exact, path)
		}
	}
	return NewestVersionedPath(exact)
}

// NewestVersionedPath selects the lexicographically-highest "versioned"
// directory from paths, where "versioned" means the grandparent directory
// name parses as a dotted numeric version (`11.9.0`). Paths whose version
// cannot be parsed compare as a pure lexicographic fallback, but always rank
// below any parseable version. Used by both the mx-binary resolver and the
// integration test harness.
func NewestVersionedPath(paths []string) string {
	var best string
	var bestVersion []int
	bestValid := false

	for _, path := range paths {
		versionParts, ok := parseVersionParts(versionFromPath(path))
		switch {
		case best == "":
			best = path
			bestVersion = versionParts
			bestValid = ok
		case ok && !bestValid:
			best = path
			bestVersion = versionParts
			bestValid = true
		case ok && bestValid:
			if cmp := compareVersionParts(versionParts, bestVersion); cmp > 0 || (cmp == 0 && path > best) {
				best = path
				bestVersion = versionParts
			}
		case !ok && !bestValid && path > best:
			best = path
		}
	}

	return best
}

func versionFromPath(path string) string {
	// macOS Studio Pro bundles: .../Mendix Studio Pro X.Y.Z*.app/Contents/modeler/<binary>
	if m := regexp.MustCompile(`Mendix Studio Pro (\d+\.\d+\.\d+)`).FindStringSubmatch(path); m != nil {
		return m[1]
	}
	versionDir := filepath.Dir(filepath.Dir(path))
	return filepath.Base(versionDir)
}

func parseVersionParts(version string) ([]int, bool) {
	if version == "" {
		return nil, false
	}
	parts := strings.Split(version, ".")
	values := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func compareVersionParts(left, right []int) int {
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}
	for i := 0; i < maxLen; i++ {
		var l, r int
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		switch {
		case l < r:
			return -1
		case l > r:
			return 1
		}
	}
	return 0
}

// DownloadMxBuild downloads and extracts MxBuild for the given version.
// Returns the path to the mxbuild binary.
// If already cached, skips the download.
func DownloadMxBuild(version string, w io.Writer) (string, error) {
	// Check cache first
	if cached := CachedMxBuildPath(version); cached != "" {
		fmt.Fprintf(w, "  MxBuild %s already cached at %s\n", version, cached)
		return cached, nil
	}

	cacheDir, err := MxBuildCacheDir(version)
	if err != nil {
		return "", err
	}

	// Remove any partial cache from a previously interrupted download.
	if _, err := os.Stat(cacheDir); err == nil {
		fmt.Fprintf(w, "  Removing incomplete cache at %s...\n", cacheDir)
		os.RemoveAll(cacheDir)
	}

	url := MxBuildCDNURL(version, runtime.GOARCH)
	fmt.Fprintf(w, "  Downloading MxBuild %s for %s...\n", version, runtime.GOARCH)
	fmt.Fprintf(w, "  URL: %s\n", url)

	// Download
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("downloading mxbuild: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading mxbuild: HTTP %d from %s", resp.StatusCode, url)
	}

	// Report download size if available
	if resp.ContentLength > 0 {
		fmt.Fprintf(w, "  Size: %.1f MB\n", float64(resp.ContentLength)/(1024*1024))
	}

	// Extract tar.gz directly from response body
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}

	fmt.Fprintf(w, "  Extracting to %s...\n", cacheDir)
	if err := extractTarGz(resp.Body, cacheDir); err != nil {
		// Clean up on failure
		os.RemoveAll(cacheDir)
		return "", fmt.Errorf("extracting mxbuild: %w", err)
	}

	// Verify the binary exists (check all candidate names)
	var bin string
	for _, name := range mxbuildBinaryNames() {
		candidate := filepath.Join(cacheDir, "modeler", name)
		if _, err := os.Stat(candidate); err == nil {
			bin = candidate
			break
		}
	}
	if bin == "" {
		os.RemoveAll(cacheDir)
		return "", fmt.Errorf("mxbuild binary not found after extraction (looked in %s/modeler/)", cacheDir)
	}

	fmt.Fprintf(w, "  MxBuild cached at %s\n", bin)
	return bin, nil
}

// RuntimeCDNURL returns the CDN download URL for the Mendix runtime.
// The runtime is pure Java — no architecture-specific variants needed.
func RuntimeCDNURL(version string) string {
	return fmt.Sprintf("https://cdn.mendix.com/runtime/mendix-%s.tar.gz", version)
}

// RuntimeCacheDir returns the cache directory for a specific runtime version.
// Layout: ~/.mxcli/runtime/{version}/
func RuntimeCacheDir(version string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".mxcli", "runtime", version), nil
}

// CachedRuntimePath returns the path to a cached runtime for the given version,
// or empty string if not cached. Checks for runtime/launcher/runtimelauncher.jar.
func CachedRuntimePath(version string) string {
	cacheDir, err := RuntimeCacheDir(version)
	if err != nil {
		return ""
	}
	jar := filepath.Join(cacheDir, "runtime", "launcher", "runtimelauncher.jar")
	if info, err := os.Stat(jar); err == nil && !info.IsDir() {
		return cacheDir
	}
	return ""
}

// DownloadRuntime downloads and extracts the Mendix runtime for the given version.
// Returns the cache directory path (containing runtime/launcher/runtimelauncher.jar).
// If already cached, skips the download.
// The tarball extracts to {version}/runtime/... so we strip the top-level directory.
func DownloadRuntime(version string, w io.Writer) (string, error) {
	// Check cache first
	if cached := CachedRuntimePath(version); cached != "" {
		fmt.Fprintf(w, "  Mendix runtime %s already cached at %s\n", version, cached)
		return cached, nil
	}

	cacheDir, err := RuntimeCacheDir(version)
	if err != nil {
		return "", err
	}

	// Remove any partial cache from a previously interrupted download.
	if _, err := os.Stat(cacheDir); err == nil {
		fmt.Fprintf(w, "  Removing incomplete cache at %s...\n", cacheDir)
		os.RemoveAll(cacheDir)
	}

	url := RuntimeCDNURL(version)
	fmt.Fprintf(w, "  Downloading Mendix runtime %s...\n", version)
	fmt.Fprintf(w, "  URL: %s\n", url)

	// Download
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("downloading runtime: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading runtime: HTTP %d from %s", resp.StatusCode, url)
	}

	// Report download size if available
	if resp.ContentLength > 0 {
		fmt.Fprintf(w, "  Size: %.1f MB\n", float64(resp.ContentLength)/(1024*1024))
	}

	// Extract tar.gz directly from response body, stripping the top-level directory
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("creating cache directory: %w", err)
	}

	fmt.Fprintf(w, "  Extracting to %s...\n", cacheDir)
	if err := extractTarGzStrip1(resp.Body, cacheDir); err != nil {
		// Clean up on failure
		os.RemoveAll(cacheDir)
		return "", fmt.Errorf("extracting runtime: %w", err)
	}

	// Verify the launcher jar exists
	jar := filepath.Join(cacheDir, "runtime", "launcher", "runtimelauncher.jar")
	if _, err := os.Stat(jar); err != nil {
		os.RemoveAll(cacheDir)
		return "", fmt.Errorf("runtime launcher not found after extraction (expected %s)", jar)
	}

	fmt.Fprintf(w, "  Runtime cached at %s\n", cacheDir)
	return cacheDir, nil
}

// extractTarGzStrip1 extracts a tar.gz stream to the target directory,
// stripping the first path component (equivalent to tar --strip-components=1).
func extractTarGzStrip1(r io.Reader, targetDir string) error {
	return extractTarGzInto(r, targetDir, true)
}

// extractTarGz extracts a tar.gz stream to the target directory.
func extractTarGz(r io.Reader, targetDir string) error {
	return extractTarGzInto(r, targetDir, false)
}

// extractTarGzInto extracts a tar.gz stream into targetDir, optionally
// stripping the first path component.
//
// Every write goes through *os.Root, so containment is decided by the kernel
// resolving the path (openat2 RESOLVE_BENEATH on Linux) rather than by
// comparing strings. That distinction is the whole fix. The lexical guard this
// replaced — a "does the joined path still start with targetDir" test, plus a
// lexical resolution of each symlink's target — looks airtight entry by entry
// and is escapable in two steps, because it resolves a link against its
// LEXICAL parent while the OS resolves it against its REAL one:
//
//	sub/            (dir)
//	sub/up -> ..    lexically targetDir; allowed, and correct — it IS targetDir
//	sub/up/w -> ..  lexically targetDir/sub; allowed. Really targetDir/w -> the
//	                PARENT of targetDir, because sub/up is already targetDir
//	sub/up/w/pwned  no ".." in the name, passes every check, lands outside
//
// Measured before the fix: `pwned` written to targetDir's parent, and with one
// more link in the chain, into any sibling directory. Both are refused now, and
// TestExtractTarGz_RefusesChainedUpDirSymlinks holds the archives.
//
// The symlink target check is kept as well, so a hostile archive cannot leave
// an outward-pointing symlink in the cache for whatever reads it later — os.Root
// stops US following it, not mxbuild or the JVM. It resolves the parent with
// EvalSymlinks for the reason above.
//
// Neither archive this extracts contains a symlink at all: measured on the
// 11.13.0 CDN tarballs, the runtime is 3,646 files + 279 directories and mxbuild
// is 33,188 + 5,036, with no other entry type in either. So refusing one cannot
// break a real download.
//
// CodeQL go/unsafe-unzip-symlink, alerts 4-7 (both functions, both the header
// name and the link name).
func extractTarGzInto(r io.Reader, targetDir string, strip1 bool) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	root, err := os.OpenRoot(targetDir)
	if err != nil {
		return fmt.Errorf("opening extraction root %s: %w", targetDir, err)
	}
	defer root.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		name, ok := tarEntryPath(header.Name, strip1)
		if !ok {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := root.MkdirAll(name, 0755); err != nil {
				return fmt.Errorf("creating directory %s: %w", name, err)
			}
		case tar.TypeReg:
			if err := mkdirAllParent(root, name); err != nil {
				return fmt.Errorf("creating parent directory for %s: %w", name, err)
			}
			f, err := root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode())
			if err != nil {
				return fmt.Errorf("creating file %s: %w", name, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("writing file %s: %w", name, err)
			}
			f.Close()
		case tar.TypeSymlink:
			if err := mkdirAllParent(root, name); err != nil {
				return fmt.Errorf("creating parent directory for symlink %s: %w", name, err)
			}
			if !symlinkStaysInside(targetDir, name, header.Linkname) {
				continue
			}
			_ = root.Remove(name) // replace an existing entry; absent is fine
			if err := root.Symlink(header.Linkname, name); err != nil {
				return fmt.Errorf("creating symlink %s: %w", name, err)
			}
		}
	}

	return nil
}

// tarEntryPath turns a tar header name into a path relative to the extraction
// root, reporting false for an entry that must be skipped.
//
// Containment is filepath.IsLocal, which rejects an absolute path, a path that
// escapes through a ".." ELEMENT, and (on Windows) a reserved device name. The
// substring test it replaces — strings.Contains(name, "..") — also dropped
// legitimate files: "foo..bar" never made it out of the archive, silently.
func tarEntryPath(name string, strip1 bool) (string, bool) {
	name = strings.TrimPrefix(name, "./")
	if strip1 {
		// Everything up to the first "/" is the archive's top-level directory.
		idx := strings.IndexByte(name, '/')
		if idx < 0 {
			return "", false // the top-level entry itself
		}
		name = name[idx+1:]
	}
	name = strings.TrimSuffix(name, "/")
	if name == "" || name == "." {
		return "", false
	}
	local := filepath.FromSlash(name)
	if !filepath.IsLocal(local) {
		return "", false
	}
	return local, true
}

// mkdirAllParent creates the parent directory of a root-relative path.
func mkdirAllParent(root *os.Root, name string) error {
	dir := filepath.Dir(name)
	if dir == "." || dir == string(os.PathSeparator) {
		return nil
	}
	return root.MkdirAll(dir, 0755)
}

// symlinkStaysInside reports whether a symlink placed at rel (relative to
// targetDir) and pointing at linkname would resolve inside targetDir.
//
// The parent is resolved with EvalSymlinks, not joined lexically: an earlier
// entry may have made it a symlink, and then the lexical and the real parent
// disagree. Resolving lexically is what let a two-link chain out. Both sides are
// evaluated so a targetDir that is itself reached through a link (/tmp on macOS)
// compares equal to itself.
func symlinkStaysInside(targetDir, rel, linkname string) bool {
	if linkname == "" {
		return false
	}
	root, err := filepath.EvalSymlinks(targetDir)
	if err != nil {
		return false
	}
	parent, err := filepath.EvalSymlinks(filepath.Join(targetDir, filepath.Dir(rel)))
	if err != nil {
		return false
	}
	if !pathWithin(root, parent) {
		return false
	}
	dest := linkname
	if !filepath.IsAbs(dest) {
		dest = filepath.Join(parent, dest)
	}
	return pathWithin(root, filepath.Clean(dest))
}

// pathWithin reports whether p is root or lives under it. The separator matters:
// a bare prefix test puts /cache/runtime-evil inside /cache/runtime.
func pathWithin(root, p string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+string(os.PathSeparator))
}
