// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// DefaultCacheTTL is how long a cached catalog listing is served before a
// refresh is triggered. Marketplace content changes slowly, so a day keeps
// search instant without going far out of date; `--refresh` overrides it.
const DefaultCacheTTL = 24 * time.Hour

// CatalogCache is an on-disk cache of the full marketplace catalog listing,
// used to make repeat searches instant (the Content API has no server-side
// search, so an uncached search scans the whole catalog).
type CatalogCache struct {
	Path string        // cache file location
	TTL  time.Duration // freshness window; defaults to DefaultCacheTTL when zero
	now  func() time.Time
}

type catalogCacheFile struct {
	FetchedAt time.Time `json:"fetchedAt"`
	Items     []Content `json:"items"`
}

func (cc *CatalogCache) clock() time.Time {
	if cc.now != nil {
		return cc.now()
	}
	return time.Now()
}

func (cc *CatalogCache) ttl() time.Duration {
	if cc.TTL > 0 {
		return cc.TTL
	}
	return DefaultCacheTTL
}

// Load returns the cached items and whether the cache is fresh (exists, parses,
// and is within TTL). A missing/corrupt/stale cache returns fresh=false.
func (cc *CatalogCache) Load() (items []Content, fresh bool) {
	data, err := os.ReadFile(cc.Path)
	if err != nil {
		return nil, false
	}
	var f catalogCacheFile
	if json.Unmarshal(data, &f) != nil {
		return nil, false
	}
	if cc.clock().Sub(f.FetchedAt) > cc.ttl() {
		return f.Items, false
	}
	return f.Items, true
}

// Save writes items to the cache with the current timestamp (file mode 0600,
// matching the auth store). Failure to write is the caller's to ignore — the
// cache is an optimization, not a source of truth.
func (cc *CatalogCache) Save(items []Content) error {
	data, err := json.Marshal(catalogCacheFile{FetchedAt: cc.clock(), Items: items})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cc.Path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(cc.Path, data, 0o600)
}

// SearchCached behaves like Client.Search but serves matches from the on-disk
// catalog cache when it is fresh, avoiding the per-query API scan. On a cache
// miss (or when refresh is true) it fetches the full catalog via client.ListAll,
// refreshes the cache, then filters. The returned bool reports whether the
// result came from cache.
func SearchCached(ctx context.Context, client *Client, cache *CatalogCache, query string, limit int, refresh bool) (*ContentList, bool, error) {
	if !refresh {
		if items, fresh := cache.Load(); fresh {
			return &ContentList{Items: limitItems(filterItems(items, query), limit)}, true, nil
		}
	}
	items, err := client.ListAll(ctx)
	if err != nil {
		return nil, false, err
	}
	_ = cache.Save(items) // best-effort
	return &ContentList{Items: limitItems(filterItems(items, query), limit)}, false, nil
}

// limitItems truncates items to at most limit (limit <= 0 means no limit).
func limitItems(items []Content, limit int) []Content {
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}
