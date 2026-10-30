// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/canon"
	mversion "github.com/mendixlabs/mxcli/modelsdk/version"
)

// typeFloorProjectsEnv lists more projects, path-list separated, for
// TestTypeFloor_StoredProjectsHoldNoUndeclaredType to scan: the measurement
// behind the type floor's data was this test run over Studio Pro-authored
// projects (blank apps from `mx create-project` on 9.24.30, 10.24.24, 11.13.0
// and 11.15.0, and Marketplace modules) — ako/mxcli#1774.
const typeFloorProjectsEnv = "MXCLI_TYPEFLOOR_PROJECTS"

// A project of version V holds only types V has, so the type floor must accept
// every unit stored in one: a refusal here is a row of
// mendixmodelsdk_type_versions.json that is wrong for storage, and on the
// write path it would refuse rewriting what Studio Pro wrote. Types the SDK
// says were deleted at or before V are logged: they are the evidence for not
// enforcing the deletion data.
func TestTypeFloor_StoredProjectsHoldNoUndeclaredType(t *testing.T) {
	projects := []string{
		filepath.Join("testdata", "v1-project", "App.mpr"),                   // 9.24.30
		filepath.Join("..", "..", "testdata", "expr-checker", "minimal.mpr"), // 11.6.6
	}
	if extra := os.Getenv(typeFloorProjectsEnv); extra != "" {
		projects = append(projects, filepath.SplitList(extra)...)
	}
	for _, path := range projects {
		if path == "" {
			continue
		}
		t.Run(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), func(t *testing.T) {
			r, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			pv := r.ProjectVersion()
			v := mversion.Version{Major: pv.MajorVersion, Minor: pv.MinorVersion, Patch: pv.PatchVersion}
			ids, err := r.ListAllUnitIDs()
			if err != nil {
				t.Fatal(err)
			}
			types := map[string]bool{}
			deletedHeld := map[string]bool{}
			for _, id := range ids {
				raw, err := r.GetRawUnitBytes(id)
				if err != nil || len(raw) == 0 {
					continue
				}
				if err := canon.UndeclaredTypeError(raw, &v); err != nil {
					t.Errorf("stored unit %s: %v", id, err)
				}
				collectTypes(bson.Raw(raw), types)
			}
			for typ := range types {
				if d, ok := canon.TypeDeleted(typ); ok && v.Compare(d) >= 0 {
					deletedHeld[typ+" (SDK: deleted "+d.String()+")"] = true
				}
			}
			// Positive control: the walk reaches the data. Two majors down,
			// a real project holds types that did not exist.
			low := mversion.Version{Major: v.Major - 2}
			refusedLow := 0
			for _, id := range ids {
				if raw, err := r.GetRawUnitBytes(id); err == nil && canon.UndeclaredTypeError(raw, &low) != nil {
					refusedLow++
				}
			}
			if refusedLow == 0 {
				t.Errorf("control: no unit of this %s project is refused at %s — the floor sees nothing", v, low)
			}
			held := make([]string, 0, len(deletedHeld))
			for k := range deletedHeld {
				held = append(held, k)
			}
			sort.Strings(held)
			t.Logf("Mendix %s: %d units, %d distinct $Types; stored types the SDK calls deleted: %s",
				v, len(ids), len(types), strings.Join(held, ", "))
		})
	}
}

func collectTypes(raw bson.Raw, into map[string]bool) {
	elems, err := raw.Elements()
	if err != nil {
		return
	}
	for _, e := range elems {
		val := e.Value()
		switch val.Type {
		case bson.TypeString:
			if e.Key() == "$Type" {
				into[val.StringValue()] = true
			}
		case bson.TypeEmbeddedDocument:
			collectTypes(val.Document(), into)
		case bson.TypeArray:
			collectTypes(bson.Raw(val.Array()), into)
		}
	}
}
