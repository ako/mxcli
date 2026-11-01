// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
)

// ako/mxcli#1640: every write gives a list with a registered owner/key marker
// that marker, the stored lists a rewrite passes through included
// (canon.NormalizeListMarkers). That is only safe if each registration is what
// Studio Pro writes, so this census holds every one of them against the
// Studio Pro-authored fixtures: a document Studio Pro wrote must be a fixed
// point of the normalization.

// studioProUnits lists the .mxunit files of the Studio Pro-authored fixtures.
func studioProUnits(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, root := range []string{
		"../../../testdata/testapp/TestApp/mprcontents",
		"../../../testdata/pedapp/mprcontents",
	} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(p, ".mxunit") {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(out) < 1000 {
		t.Fatalf("found only %d fixture units", len(out))
	}
	return out
}

// markerCensus counts, per "Owner.Key", the markers stored for every list.
func markerCensus(t *testing.T) map[string]map[int32]int {
	t.Helper()
	census := map[string]map[int32]int{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case bson.D:
			owner := ""
			for _, e := range x {
				if e.Key == "$Type" {
					owner, _ = e.Value.(string)
				}
			}
			for _, e := range x {
				if a, ok := e.Value.(bson.A); ok && len(a) > 0 {
					if m, ok := a[0].(int32); ok && owner != "" {
						k := owner + "." + e.Key
						if census[k] == nil {
							census[k] = map[int32]int{}
						}
						census[k][m]++
					}
				}
				walk(e.Value)
			}
		case bson.A:
			for _, e := range x {
				walk(e)
			}
		}
	}
	for _, p := range studioProUnits(t) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		walk(doc)
	}
	return census
}

func TestIssue1640_RegisteredListMarkersAreWhatStudioProStores(t *testing.T) {
	census := markerCensus(t)
	var wrong, measured []string
	for key, byMarker := range census {
		owner, prop, _ := strings.Cut(key, ".")
		want, ok := canon.PropertyListMarker(owner, prop)
		if !ok {
			continue
		}
		total := 0
		for _, n := range byMarker {
			total += n
		}
		if byMarker[want] != total {
			wrong = append(wrong, key)
			t.Errorf("%s is registered as %d, but Studio Pro stores %v", key, want, byMarker)
			continue
		}
		measured = append(measured, key)
	}
	sort.Strings(measured)
	t.Logf("%d registered owner/key markers agree with every stored list: %s", len(measured), strings.Join(measured, ", "))

	// And a Studio Pro document passes through the normalization unchanged.
	changed := 0
	for _, p := range studioProUnits(t) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if out := canon.NormalizeListMarkers(raw); string(out) != string(raw) {
			changed++
			if changed <= 5 {
				t.Errorf("normalizing changes the Studio Pro unit %s", p)
			}
		}
	}
	if changed > 0 {
		t.Errorf("%d Studio Pro units change under the normalization", changed)
	}
}
