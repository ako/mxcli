// SPDX-License-Identifier: Apache-2.0

package docker

import "testing"

// A real ListObjectsV2 response for prefix "runtime/mxbuild-10.24.24." — the
// release Mendix respun three times, which is what makes "pick a build" a
// decision rather than a lookup. Every archive is shadowed by a .sha256 sidecar.
const listing10_24_24 = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
<Name>mx-cdn</Name><Prefix>runtime/mxbuild-10.24.24.</Prefix>
<KeyCount>6</KeyCount><MaxKeys>20</MaxKeys><IsTruncated>false</IsTruncated>
<Contents><Key>runtime/mxbuild-10.24.24.119349.tar.gz</Key></Contents>
<Contents><Key>runtime/mxbuild-10.24.24.119349.tar.gz.sha256</Key></Contents>
<Contents><Key>runtime/mxbuild-10.24.24.119564.tar.gz</Key></Contents>
<Contents><Key>runtime/mxbuild-10.24.24.119564.tar.gz.sha256</Key></Contents>
<Contents><Key>runtime/mxbuild-10.24.24.119653.tar.gz</Key></Contents>
<Contents><Key>runtime/mxbuild-10.24.24.119653.tar.gz.sha256</Key></Contents>
</ListBucketResult>`

// A truncated page, to prove the continuation token is read rather than ignored.
const listingTruncated = `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
<Name>mx-cdn</Name><Prefix>runtime/mxbuild-10.24</Prefix>
<NextContinuationToken>14OiJwODsCgR7yYEMps95YFPhA5XOi74</NextContinuationToken>
<KeyCount>1</KeyCount><MaxKeys>1</MaxKeys><IsTruncated>true</IsTruncated>
<Contents><Key>runtime/mxbuild-10.24.0.72725.tar.gz</Key></Contents>
</ListBucketResult>`

func TestParseCDNListing(t *testing.T) {
	got, err := parseCDNListing([]byte(listing10_24_24))
	if err != nil {
		t.Fatalf("parseCDNListing: %v", err)
	}
	if len(got.keys) != 6 {
		t.Errorf("keys = %d, want 6: %v", len(got.keys), got.keys)
	}
	if got.truncated {
		t.Error("truncated = true, want false")
	}

	tr, err := parseCDNListing([]byte(listingTruncated))
	if err != nil {
		t.Fatalf("parseCDNListing(truncated): %v", err)
	}
	if !tr.truncated {
		t.Error("truncated = false, want true — a page treated as complete resolves to the wrong build")
	}
	if tr.next != "14OiJwODsCgR7yYEMps95YFPhA5XOi74" {
		t.Errorf("next = %q, want the continuation token", tr.next)
	}
}

func TestHighestBuild(t *testing.T) {
	listing, err := parseCDNListing([]byte(listing10_24_24))
	if err != nil {
		t.Fatalf("parseCDNListing: %v", err)
	}

	if got := highestBuild(listing.keys, "mxbuild-", "10.24.24"); got != "10.24.24.119653" {
		t.Errorf("highestBuild = %q, want 10.24.24.119653 (the last respin)", got)
	}

	// The .sha256 sidecar sits next to every archive and must never be chosen.
	only := []string{"runtime/mxbuild-10.24.25.122571.tar.gz.sha256"}
	if got := highestBuild(only, "mxbuild-", "10.24.25"); got != "" {
		t.Errorf("highestBuild on a sidecar alone = %q, want empty", got)
	}

	// Build numbers are not zero-padded, so text sorting would put 99999 first.
	unpadded := []string{
		"runtime/mxbuild-10.24.25.99999.tar.gz",
		"runtime/mxbuild-10.24.25.122571.tar.gz",
	}
	if got := highestBuild(unpadded, "mxbuild-", "10.24.25"); got != "10.24.25.122571" {
		t.Errorf("highestBuild = %q, want 10.24.25.122571 — compare build numbers numerically, not as text", got)
	}

	// Without the trailing dot, 10.24.2 would swallow 10.24.20..10.24.26. The
	// prefix highestBuild builds must keep releases apart.
	neighbours := []string{
		"runtime/mxbuild-10.24.2.75382.tar.gz",
		"runtime/mxbuild-10.24.20.105674.tar.gz",
		"runtime/mxbuild-10.24.25.122571.tar.gz",
	}
	if got := highestBuild(neighbours, "mxbuild-", "10.24.2"); got != "10.24.2.75382" {
		t.Errorf("highestBuild = %q, want 10.24.2.75382 — 10.24.20 and 10.24.25 are different releases", got)
	}

	// arm64 archives carry their own stem; the amd64 stem must not match them.
	arm := []string{"runtime/arm64-mxbuild-10.24.25.122571.tar.gz"}
	if got := highestBuild(arm, "mxbuild-", "10.24.25"); got != "" {
		t.Errorf("amd64 stem matched an arm64 key: %q", got)
	}
	if got := highestBuild(arm, "arm64-mxbuild-", "10.24.25"); got != "10.24.25.122571" {
		t.Errorf("arm64 stem = %q, want 10.24.25.122571", got)
	}
}

func TestMxbuildKeyStemMatchesURL(t *testing.T) {
	// The listing prefix and the download URL must name the same artifact, or a
	// resolved version points at a file the downloader will not ask for.
	for _, arch := range []string{"amd64", "arm64"} {
		url := MxBuildCDNURL("10.24.25.122571", arch)
		want := "https://cdn.mendix.com/runtime/" + mxbuildKeyStem(arch) + "10.24.25.122571.tar.gz"
		if url != want {
			t.Errorf("arch %s: URL %q, stem builds %q", arch, url, want)
		}
	}
}

// A version that already carries a build number must be returned untouched and
// must not reach the network — this is the common path for a project-derived
// version, where _ProductVersion is already four parts.
func TestResolveCDNVersionPassesThroughFourPart(t *testing.T) {
	got, err := ResolveCDNVersion("10.24.25.122571", "amd64")
	if err != nil {
		t.Fatalf("ResolveCDNVersion: %v", err)
	}
	if got != "10.24.25.122571" {
		t.Errorf("= %q, want it returned unchanged", got)
	}
}

func TestResolveCDNVersionRejectsEmpty(t *testing.T) {
	if _, err := ResolveCDNVersion("  ", "amd64"); err == nil {
		t.Error("empty version accepted; want an error")
	}
}

// The "newest first" hint shown when a version cannot be resolved must order
// releases numerically. Sorted as text, 10.24.9 outranks 10.24.26 and the hint
// names a release two dozen patches old as the latest.
func TestCDNReleasesSortIsNumeric(t *testing.T) {
	in := []string{"10.24.9.81004", "10.24.26.123458", "10.24.2.75382", "10.24.20.105674"}
	got := append([]string(nil), in...)
	sortCDNReleases(got)
	want := []string{"10.24.26.123458", "10.24.20.105674", "10.24.9.81004", "10.24.2.75382"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted = %v, want %v", got, want)
		}
	}
}
