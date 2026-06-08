// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"context"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestCatalogCache_SaveLoadRoundtrip(t *testing.T) {
	cc := &CatalogCache{Path: filepath.Join(t.TempDir(), "cat.json"), TTL: time.Hour}
	items := []Content{{ContentID: 1, Publisher: "Mendix"}, {ContentID: 2, Publisher: "Acme"}}
	if err := cc.Save(items); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, fresh := cc.Load()
	if !fresh {
		t.Fatal("expected freshly-saved cache to be fresh")
	}
	if len(got) != 2 || got[0].ContentID != 1 || got[1].Publisher != "Acme" {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}

func TestCatalogCache_StalePastTTL(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	cc := &CatalogCache{
		Path: filepath.Join(t.TempDir(), "cat.json"),
		TTL:  time.Hour,
		now:  func() time.Time { return now },
	}
	if err := cc.Save([]Content{{ContentID: 1}}); err != nil {
		t.Fatal(err)
	}
	// Within TTL → fresh.
	if _, fresh := cc.Load(); !fresh {
		t.Error("expected fresh within TTL")
	}
	// Advance past TTL → stale.
	now = now.Add(2 * time.Hour)
	if _, fresh := cc.Load(); fresh {
		t.Error("expected stale past TTL")
	}
}

func TestCatalogCache_MissingFile(t *testing.T) {
	cc := &CatalogCache{Path: filepath.Join(t.TempDir(), "does-not-exist.json")}
	if _, fresh := cc.Load(); fresh {
		t.Error("missing cache must not be fresh")
	}
}

func TestSearchCached_HitAndMiss(t *testing.T) {
	var requests atomic.Int32
	client, _ := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(contentPage(1, 3, "Mendix Business Events"))) // short page -> end
	})
	cc := &CatalogCache{Path: filepath.Join(t.TempDir(), "cat.json"), TTL: time.Hour}

	// Miss: fetches the catalog and caches it.
	res, fromCache, err := SearchCached(context.Background(), client, cc, "business events", 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if fromCache {
		t.Error("first call should be a cache miss")
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 match, got %d", len(res.Items))
	}
	afterMiss := requests.Load()
	if afterMiss == 0 {
		t.Fatal("miss should have hit the API")
	}

	// Hit: served from cache, no new requests.
	res, fromCache, err = SearchCached(context.Background(), client, cc, "business events", 20, false)
	if err != nil {
		t.Fatal(err)
	}
	if !fromCache {
		t.Error("second call should be a cache hit")
	}
	if len(res.Items) != 1 {
		t.Errorf("cache hit lost the match: %+v", res.Items)
	}
	if requests.Load() != afterMiss {
		t.Errorf("cache hit must not call the API (requests %d -> %d)", afterMiss, requests.Load())
	}

	// refresh=true bypasses the cache and re-fetches.
	_, fromCache, err = SearchCached(context.Background(), client, cc, "business events", 20, true)
	if err != nil {
		t.Fatal(err)
	}
	if fromCache {
		t.Error("refresh must bypass the cache")
	}
	if requests.Load() <= afterMiss {
		t.Error("refresh should have re-fetched from the API")
	}
}
