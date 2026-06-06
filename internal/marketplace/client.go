// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	// pageSize is the server-side cap on the /v1/content `limit` parameter:
	// the API silently caps any larger value at 100 per page.
	pageSize = 100
	// maxSearchPages bounds how far keyword search paginates via `offset`
	// (pageSize*maxSearchPages items) so a sparse/no-match query doesn't keep
	// walking indefinitely as the catalog grows. The catalog is ~2300 items
	// (~23 pages) today, so this comfortably covers a full scan with margin.
	// (End-of-catalog normally terminates the loop first: an offset past the
	// end returns a short/empty page.)
	maxSearchPages = 50
)

// Client is a typed wrapper around the marketplace REST API. Callers
// obtain an authenticated http.Client via internal/auth.ClientFor and
// pass it here.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// New returns a marketplace client bound to the given HTTP client.
// The http.Client is expected to inject Mendix auth headers — use
// auth.ClientFor(ctx, profile) in production.
func New(httpClient *http.Client) *Client {
	return &Client{
		httpClient: httpClient,
		baseURL:    BaseURL,
	}
}

// NewWithBaseURL constructs a client pointed at a specific base URL.
// Used by tests to redirect at httptest.Server.
func NewWithBaseURL(httpClient *http.Client, baseURL string) *Client {
	return &Client{httpClient: httpClient, baseURL: baseURL}
}

// Search lists marketplace content matching a query. limit is the
// maximum number of results to return; pass 0 for the API default.
//
// The marketplace API accepts ?search= but does NOT filter server-side, and it
// caps `limit` at 100 per page. So keyword search paginates through the catalog
// with `offset` and filters client-side (case-insensitive substring match on the
// item name and publisher), stopping as soon as it has `limit` matches or it
// reaches the end of the catalog. With no query it returns a single page.
func (c *Client) Search(ctx context.Context, query string, limit int) (*ContentList, error) {
	if query == "" {
		// Plain listing: a single page (the server caps `limit` at pageSize).
		path := "/v1/content"
		if limit > 0 {
			path += "?limit=" + strconv.Itoa(limit)
		}
		var out ContentList
		if err := c.get(ctx, path, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}

	var matched []Content
	for page := range maxSearchPages {
		items, err := c.fetchContentPage(ctx, page*pageSize)
		if err != nil {
			return nil, err
		}
		matched = append(matched, filterItems(items, query)...)
		if limit > 0 && len(matched) >= limit {
			matched = matched[:limit]
			break
		}
		if len(items) < pageSize {
			break // reached the end of the catalog
		}
	}
	return &ContentList{Items: matched}, nil
}

// fetchContentPage returns one page of /v1/content starting at offset
// (pageSize items, the server's per-page maximum).
func (c *Client) fetchContentPage(ctx context.Context, offset int) ([]Content, error) {
	path := fmt.Sprintf("/v1/content?limit=%d&offset=%d", pageSize, offset)
	var out ContentList
	if err := c.get(ctx, path, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// filterItems returns items whose name or publisher contains query
// (case-insensitive substring match).
func filterItems(items []Content, query string) []Content {
	q := strings.ToLower(query)
	var matched []Content
	for _, item := range items {
		name := ""
		if item.LatestVersion != nil {
			name = strings.ToLower(item.LatestVersion.Name)
		}
		if strings.Contains(name, q) || strings.Contains(strings.ToLower(item.Publisher), q) {
			matched = append(matched, item)
		}
	}
	return matched
}

// Get returns the full detail for a single content item by ID.
func (c *Client) Get(ctx context.Context, contentID int) (*Content, error) {
	var out Content
	if err := c.get(ctx, fmt.Sprintf("/v1/content/%d", contentID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Versions returns all published versions for a content item, ordered
// newest first (per the API).
func (c *Client) Versions(ctx context.Context, contentID int) (*VersionList, error) {
	var out VersionList
	if err := c.get(ctx, fmt.Sprintf("/v1/content/%d/versions", contentID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Download streams the .mpk for the given version to dst and returns the
// suggested filename (from the CDN URL's last path segment).
//
// The flow is two hops (see reference_marketplace_download_api memory):
//  1. GET version.DownloadURL on marketplace.mendix.com WITH the MxToken
//     (the auth http.Client supplies it) but with redirects DISABLED, to
//     capture the 303 Location pointing at the public CDN.
//  2. GET that CDN URL with a PLAIN client — no token is sent to the CDN, and
//     the CDN host is not in the auth allowlist so the auth client would reject
//     it anyway.
func (c *Client) Download(ctx context.Context, v *Version, dst io.Writer) (filename string, err error) {
	if v == nil || v.DownloadURL == "" {
		return "", fmt.Errorf("marketplace: this version exposes no download URL")
	}

	// Step 1: resolve the 303 redirect using the auth client, without following it.
	req, err := http.NewRequestWithContext(ctx, "GET", v.DownloadURL, nil)
	if err != nil {
		return "", err
	}
	noRedirect := *c.httpClient
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noRedirect.Do(req)
	if err != nil {
		return "", fmt.Errorf("marketplace download (resolve): %w", err)
	}
	cdnURL := resp.Header.Get("Location")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusMovedPermanently {
		return "", fmt.Errorf("marketplace download (resolve): expected redirect, got HTTP %d", resp.StatusCode)
	}
	if cdnURL == "" {
		return "", fmt.Errorf("marketplace download (resolve): redirect carried no Location")
	}

	// Step 2: fetch the .mpk from the public CDN with a plain client.
	cdnReq, err := http.NewRequestWithContext(ctx, "GET", cdnURL, nil)
	if err != nil {
		return "", err
	}
	cdnResp, err := http.DefaultClient.Do(cdnReq)
	if err != nil {
		return "", fmt.Errorf("marketplace download (fetch): %w", err)
	}
	defer cdnResp.Body.Close()
	if cdnResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(cdnResp.Body, 256))
		return "", fmt.Errorf("marketplace download (fetch): HTTP %d: %s", cdnResp.StatusCode, strings.TrimSpace(string(body)))
	}
	if _, err := io.Copy(dst, cdnResp.Body); err != nil {
		return "", fmt.Errorf("marketplace download (stream): %w", err)
	}

	if u, perr := url.Parse(cdnURL); perr == nil {
		if i := strings.LastIndex(u.Path, "/"); i >= 0 && i+1 < len(u.Path) {
			filename = u.Path[i+1:]
		}
	}
	return filename, nil
}

func (c *Client) get(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("marketplace %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("marketplace %s: HTTP %d: %s", path, resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("marketplace %s: decode: %w", path, err)
	}
	return nil
}
