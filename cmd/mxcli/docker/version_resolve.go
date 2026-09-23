// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// The Mendix CDN is an S3 bucket that answers the ListObjectsV2 API, which is
// how a partial version is turned into the artifact that actually exists.
const (
	cdnBucketURL  = "https://cdn.mendix.com/"
	cdnKeyPrefix  = "runtime/"
	cdnListMaxKey = 1000
)

// ResolveCDNVersion maps a user-supplied Mendix version onto the version string
// the CDN publishes an artifact for.
//
// Mendix 11 publishes three-part names (mxbuild-11.13.0.tar.gz). Mendix 9 and 10
// publish FOUR parts, with a build number: the release called "10.24.25" is
// mxbuild-10.24.25.122571.tar.gz. A three-part 10.x version therefore names no
// artifact at all, and every probe of one 404s — which reads as "Mendix 10 is not
// on the CDN" rather than "that is not its name". Both the mxbuild and the
// runtime archive follow this, so one resolved string serves both.
//
// A version that already carries a build number is returned untouched, and so is
// one the CDN serves as given — the listing call only happens when a download
// would otherwise fail. A project-derived version needs none of this: the MPR's
// _ProductVersion already carries all four parts.
//
// Where several builds exist for one release (10.24.24 has three), the highest
// build number wins: they are respins of the same release and the last one is
// what Mendix ships.
func ResolveCDNVersion(version, goarch string) (string, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return "", fmt.Errorf("empty Mendix version")
	}
	// Already exact — a build number is present.
	if len(strings.Split(version, ".")) >= 4 {
		return version, nil
	}
	// Published under the name as given (every supported Mendix 11).
	if cdnHasArtifact(MxBuildCDNURL(version, goarch)) {
		return version, nil
	}

	stem := mxbuildKeyStem(goarch)
	keys, err := listCDNKeys(cdnKeyPrefix + stem + version + ".")
	if err != nil {
		return "", fmt.Errorf("resolving Mendix %s against the CDN: %w", version, err)
	}
	resolved := highestBuild(keys, stem, version)
	if resolved == "" {
		return "", fmt.Errorf("no MxBuild archive published for Mendix %s "+
			"(tried %s and a CDN listing of %s*)",
			version, MxBuildCDNURL(version, goarch), stem+version+".")
	}
	return resolved, nil
}

// mxbuildKeyStem is the filename stem for an architecture, matching MxBuildCDNURL.
func mxbuildKeyStem(goarch string) string {
	if goarch == "arm64" {
		return "arm64-mxbuild-"
	}
	return "mxbuild-"
}

// cdnHasArtifact reports whether the CDN serves the given URL. A transport error
// or any non-200 counts as absent: the caller falls back to a listing, and a real
// outage surfaces there with a better message than a HEAD failure would give.
func cdnHasArtifact(url string) bool {
	resp, err := http.Head(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// s3ListResult is the subset of ListObjectsV2 output this needs.
type s3ListResult struct {
	IsTruncated           bool   `xml:"IsTruncated"`
	NextContinuationToken string `xml:"NextContinuationToken"`
	Contents              []struct {
		Key string `xml:"Key"`
	} `xml:"Contents"`
}

// listCDNKeys returns every object key under a prefix, following continuation
// tokens. The prefixes used here match a handful of keys, but a truncated
// response that was treated as complete would silently resolve to the wrong
// build, so the loop is not optional.
func listCDNKeys(prefix string) ([]string, error) {
	var keys []string
	token := ""
	for {
		url := fmt.Sprintf("%s?list-type=2&prefix=%s&max-keys=%d",
			cdnBucketURL, prefix, cdnListMaxKey)
		if token != "" {
			url += "&continuation-token=" + token
		}
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d listing %s", resp.StatusCode, prefix)
		}
		parsed, err := parseCDNListing(body)
		if err != nil {
			return nil, err
		}
		keys = append(keys, parsed.keys...)
		if !parsed.truncated || parsed.next == "" {
			return keys, nil
		}
		token = parsed.next
	}
}

type cdnListing struct {
	keys      []string
	truncated bool
	next      string
}

// parseCDNListing extracts the object keys from one ListObjectsV2 response.
func parseCDNListing(body []byte) (cdnListing, error) {
	var res s3ListResult
	if err := xml.Unmarshal(body, &res); err != nil {
		return cdnListing{}, fmt.Errorf("parsing CDN listing: %w", err)
	}
	out := cdnListing{truncated: res.IsTruncated, next: res.NextContinuationToken}
	for _, c := range res.Contents {
		out.keys = append(out.keys, c.Key)
	}
	return out, nil
}

// highestBuild picks the highest-numbered build of one release from a set of
// object keys, and returns the full four-part version.
//
// It matches `<stem><version>.<digits>.tar.gz` exactly: the `.sha256` sidecar
// next to every archive must not be mistaken for an artifact, and a prefix
// without the trailing dot would let 10.24.2 match 10.24.20 through 10.24.26.
// Comparison is numeric, because build numbers are not zero-padded and sorting
// them as text puts 99999 above 122571.
func highestBuild(keys []string, stem, version string) string {
	want := stem + version + "."
	best := -1
	for _, key := range keys {
		name := key
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if !strings.HasPrefix(name, want) || !strings.HasSuffix(name, ".tar.gz") {
			continue
		}
		build := strings.TrimSuffix(strings.TrimPrefix(name, want), ".tar.gz")
		n, err := strconv.Atoi(build)
		if err != nil {
			continue // not a build number (e.g. a suffixed variant)
		}
		if n > best {
			best = n
		}
	}
	if best < 0 {
		return ""
	}
	return version + "." + strconv.Itoa(best)
}

// CDNReleasesFor lists the full versions published for a partial one, newest
// first. Used to show what was available when a version cannot be resolved.
func CDNReleasesFor(partial, goarch string) []string {
	stem := mxbuildKeyStem(goarch)
	keys, err := listCDNKeys(cdnKeyPrefix + stem + partial)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, key := range keys {
		name := key
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if !strings.HasPrefix(name, stem) || !strings.HasSuffix(name, ".tar.gz") {
			continue
		}
		v := strings.TrimSuffix(strings.TrimPrefix(name, stem), ".tar.gz")
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sortCDNReleases(out)
	return out
}

// sortCDNReleases orders versions newest first, numerically. Sorting them as
// text puts 10.24.9 above 10.24.26, so the hint would name a release two dozen
// patches old as the latest.
func sortCDNReleases(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		li, oki := parseVersionParts(versions[i])
		lj, okj := parseVersionParts(versions[j])
		if !oki || !okj {
			return versions[i] > versions[j]
		}
		return compareVersionParts(li, lj) > 0
	})
}
