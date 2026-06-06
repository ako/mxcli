// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Sample responses mirror what the 2026-04 spike recorded from the real
// API. If Mendix changes the response shape, these sample strings are the
// first place to update.

const sampleContentList = `{
  "items": [
    {
      "contentId": 170,
      "publisher": "Mendix",
      "type": "Module",
      "categories": [{"name": "Utility"}],
      "supportCategory": "Platform",
      "licenseUrl": "http://www.apache.org/licenses/LICENSE-2.0.html",
      "isPrivate": false,
      "latestVersion": {
        "name": "Community Commons",
        "versionId": "0a03e65a-d94f-47fa-ac40-4e8e054fdcd4",
        "versionNumber": "11.5.0",
        "minSupportedMendixVersion": "10.24.0",
        "publicationDate": "2026-01-13T06:57:14.512Z"
      }
    }
  ]
}`

// sampleMultiContentList has two items: one matching "database", one not.
const sampleMultiContentList = `{
  "items": [
    {
      "contentId": 2888,
      "publisher": "Mendix",
      "type": "Module",
      "latestVersion": {
        "name": "Database Connector",
        "versionId": "aaaa",
        "versionNumber": "3.1.0",
        "minSupportedMendixVersion": "9.0.0",
        "publicationDate": "2025-06-01T00:00:00Z"
      }
    },
    {
      "contentId": 170,
      "publisher": "Mendix",
      "type": "Module",
      "latestVersion": {
        "name": "Community Commons",
        "versionId": "bbbb",
        "versionNumber": "11.5.0",
        "minSupportedMendixVersion": "10.24.0",
        "publicationDate": "2026-01-13T00:00:00Z"
      }
    },
    {
      "contentId": 999,
      "publisher": "ACME",
      "type": "Module",
      "latestVersion": {
        "name": "Advanced Database Tools",
        "versionId": "cccc",
        "versionNumber": "1.0.0",
        "minSupportedMendixVersion": "9.0.0",
        "publicationDate": "2024-01-01T00:00:00Z"
      }
    }
  ]
}`

const sampleContent = `{
  "contentId": 170,
  "publisher": "Mendix",
  "type": "Module",
  "categories": [{"name": "Utility"}],
  "supportCategory": "Platform",
  "licenseUrl": "http://www.apache.org/licenses/LICENSE-2.0.html",
  "isPrivate": false,
  "latestVersion": {
    "name": "Community Commons",
    "versionId": "0a03e65a-d94f-47fa-ac40-4e8e054fdcd4",
    "versionNumber": "11.5.0",
    "minSupportedMendixVersion": "10.24.0",
    "publicationDate": "2026-01-13T06:57:14.512Z"
  }
}`

const sampleVersions = `{
  "items": [
    {
      "name": "Community Commons",
      "versionId": "0a03e65a-d94f-47fa-ac40-4e8e054fdcd4",
      "versionNumber": "11.5.0",
      "minSupportedMendixVersion": "10.24.0",
      "publicationDate": "2026-01-13T06:57:14.512Z",
      "releaseNotes": "<p>We upgraded guava to 33.5.0-jre</p>"
    }
  ]
}`

func newMockServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	return NewWithBaseURL(ts.Client(), ts.URL), ts
}

// contentPage renders a /v1/content page of `n` filler items (ids based at
// `startID`), optionally appending one extra item named `special`.
func contentPage(startID, n int, special string) string {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"contentId":%d,"publisher":"Acme","type":"Module","latestVersion":{"name":"Filler %d","versionNumber":"1.0.0"}}`, startID+i, startID+i)
	}
	if special != "" {
		if n > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"contentId":999999,"publisher":"Mendix","type":"Module","latestVersion":{"name":%q,"versionNumber":"3.12.0"}}`, special)
	}
	b.WriteString(`]}`)
	return b.String()
}

// TestSearch_PaginatesPastFirstPage is the regression test for the bug where
// keyword search only looked at the first page: a match that lives on the second
// page (offset=100) must still be found. The server returns a full page of 100
// non-matching items at offset 0, and the match at offset 100.
func TestSearch_PaginatesPastFirstPage(t *testing.T) {
	var offsetsSeen []string
	client, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		off := r.URL.Query().Get("offset")
		offsetsSeen = append(offsetsSeen, off)
		switch off {
		case "0":
			_, _ = w.Write([]byte(contentPage(1, 100, ""))) // full page, no match
		case "100":
			_, _ = w.Write([]byte(contentPage(200, 5, "Mendix Business Events"))) // match here
		default:
			_, _ = w.Write([]byte(`{"items":[]}`))
		}
	})

	result, err := client.Search(context.Background(), "business events", 20)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].ContentID != 999999 {
		t.Fatalf("expected the second-page match (999999), got %+v", result.Items)
	}
	// It must have advanced past the first page (offset 0 was a full 100).
	if len(offsetsSeen) < 2 || offsetsSeen[1] != "100" {
		t.Errorf("expected pagination to request offset=100; offsets seen: %v", offsetsSeen)
	}
}

// TestSearch_StopsAtEndOfCatalog: a short first page (< pageSize) means the
// catalog is exhausted, so search must not keep requesting further offsets.
func TestSearch_StopsAtEndOfCatalog(t *testing.T) {
	var calls int
	client, _ := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(contentPage(1, 3, ""))) // only 3 items, < pageSize
	})
	if _, err := client.Search(context.Background(), "nomatch", 20); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 page fetch at end-of-catalog, got %d", calls)
	}
}

func TestSearch_PassesQueryAndLimit(t *testing.T) {
	var gotPath, gotQuery string
	client, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleMultiContentList))
	})

	// The API ignores ?search= and caps limit at 100, so we paginate with
	// limit=pageSize + offset and filter client-side.
	result, err := client.Search(context.Background(), "database", 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotPath != "/v1/content" {
		t.Errorf("path: got %q, want /v1/content", gotPath)
	}
	// Paginated request shape: limit=pageSize, offset=0 on the first page.
	if !strings.Contains(gotQuery, "limit="+strconv.Itoa(pageSize)) {
		t.Errorf("expected API to receive limit=%d, got query %q", pageSize, gotQuery)
	}
	if !strings.Contains(gotQuery, "offset=0") {
		t.Errorf("expected offset=0 on first page, got query %q", gotQuery)
	}
	// Client-side filter: only items whose name contains "database"
	if len(result.Items) != 2 {
		t.Errorf("expected 2 filtered items (Database Connector + Advanced Database Tools), got %d", len(result.Items))
	}
}

func TestSearch_ClientSideFiltering(t *testing.T) {
	client, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleMultiContentList))
	})

	result, err := client.Search(context.Background(), "community", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ContentID != 170 {
		t.Errorf("expected only Community Commons (170), got %+v", result.Items)
	}
}

func TestSearch_ClientSideFiltering_NoMatch(t *testing.T) {
	client, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleMultiContentList))
	})

	result, err := client.Search(context.Background(), "zzzznonexistent", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 results for nonexistent query, got %d", len(result.Items))
	}
}

func TestSearch_ClientSideFiltering_LimitApplied(t *testing.T) {
	client, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleMultiContentList))
	})

	// 2 items match "database", but user wants only 1
	result, err := client.Search(context.Background(), "database", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Errorf("expected limit of 1 applied after filtering, got %d items", len(result.Items))
	}
}

func TestSearch_PublisherFiltering(t *testing.T) {
	client, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleMultiContentList))
	})

	result, err := client.Search(context.Background(), "acme", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ContentID != 999 {
		t.Errorf("expected ACME item (999), got %+v", result.Items)
	}
}

func TestSearch_NoQueryOrLimit(t *testing.T) {
	var gotQuery string
	client, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(sampleContentList))
	})

	if _, err := client.Search(context.Background(), "", 0); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "" {
		t.Errorf("expected empty query when no search or limit, got %q", gotQuery)
	}
}

func TestGet_ParsesContentDetail(t *testing.T) {
	client, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/content/170" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(sampleContent))
	})

	got, err := client.Get(context.Background(), 170)
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentID != 170 || got.Publisher != "Mendix" {
		t.Errorf("unexpected content: %+v", got)
	}
	if got.LatestVersion == nil || got.LatestVersion.VersionNumber != "11.5.0" {
		t.Errorf("latestVersion not parsed: %+v", got.LatestVersion)
	}
	if len(got.Categories) != 1 || got.Categories[0].Name != "Utility" {
		t.Errorf("categories not parsed: %+v", got.Categories)
	}
}

func TestVersions_ParsesList(t *testing.T) {
	client, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/content/170/versions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(sampleVersions))
	})

	got, err := client.Versions(context.Background(), 170)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected 1 version, got %d", len(got.Items))
	}
	v := got.Items[0]
	if v.VersionNumber != "11.5.0" {
		t.Errorf("versionNumber: %q", v.VersionNumber)
	}
	if v.MinSupportedMendixVersion != "10.24.0" {
		t.Errorf("minSupportedMendixVersion: %q", v.MinSupportedMendixVersion)
	}
	if !strings.Contains(v.ReleaseNotes, "guava") {
		t.Errorf("releaseNotes: %q", v.ReleaseNotes)
	}
	if v.PublicationDate.IsZero() {
		t.Error("publicationDate did not parse")
	}
	expected := time.Date(2026, 1, 13, 6, 57, 14, 512000000, time.UTC)
	if !v.PublicationDate.Equal(expected) {
		t.Errorf("publicationDate: got %v, want %v", v.PublicationDate, expected)
	}
}

func TestGet_HTTPErrorIsReported(t *testing.T) {
	client, _ := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	_, err := client.Get(context.Background(), 99999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status: %v", err)
	}
}

func TestGet_InvalidJSONReported(t *testing.T) {
	client, _ := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	})

	_, err := client.Get(context.Background(), 170)
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error should mention decode: %v", err)
	}
}

func TestNew_UsesDefaultBaseURL(t *testing.T) {
	c := New(http.DefaultClient)
	if c.baseURL != BaseURL {
		t.Errorf("expected default BaseURL %q, got %q", BaseURL, c.baseURL)
	}
	if c.baseURL != "https://marketplace-api.mendix.com" {
		t.Errorf("default BaseURL unexpected: %q", c.baseURL)
	}
}
