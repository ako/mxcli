// SPDX-License-Identifier: Apache-2.0

// The measuring half of ako/mxcli#584's guard: reads the System module's domain
// model out of a built `deployment/model/model.mdp` and rewrites the golden.
//
// The .mdp is not one BSON document — it is a STREAM of them, each prefixed by
// its own 4-byte length, so unmarshalling the file whole fails with "invalid
// document length". Modules arrive as `Projects$ModuleImpl` documents carrying
// only a Name, and each module's documents follow it; the domain model is the
// first `DomainModels$DomainModel` after the System module's own document.
//
// Run it only when re-measuring — without -mdp it skips.
package meta

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestSystemStringLengthsUpdate(t *testing.T) {
	if *mdpPath == "" {
		t.Skip("no -mdp given; this test re-measures from a built deployment/model/model.mdp")
	}
	raw, err := os.ReadFile(*mdpPath)
	if err != nil {
		t.Fatalf("read %s: %v", *mdpPath, err)
	}
	measured, err := systemStringLengthsFromMDP(raw)
	if err != nil {
		t.Fatalf("parse %s: %v", *mdpPath, err)
	}
	if len(measured) == 0 {
		t.Fatalf("%s carries no System module domain model — is it a deploy build?", *mdpPath)
	}

	// Only the attributes SystemEntities declares: the deployed model carries
	// more (11.14.0 has 29 members mxcli's 11.6.4-era table does not know), and
	// adding those is a separate change with a version-gating question of its
	// own — a member that does not exist in the target version is CE1613, which
	// a length can never be.
	keep := map[string]int{}
	var missing []string
	for _, e := range SystemEntities {
		for _, a := range e.Attributes {
			if a.Type != "String" {
				continue
			}
			key := e.Name + "." + a.Name
			length, ok := measured[key]
			if !ok {
				missing = append(missing, key)
				continue
			}
			keep[key] = length
		}
	}
	if len(missing) > 0 {
		t.Errorf("declared as String but absent from %s: %s\n"+
			"  Either the attribute is gone in this Mendix version, or the build is not a deploy build.",
			*mdpPath, strings.Join(missing, ", "))
	}

	if !*updateGold {
		t.Logf("measured %d String attributes; pass -update to rewrite %s", len(keep), goldenPath)
		golden, err := readGoldenLengths(goldenPath)
		if err != nil {
			t.Fatalf("read golden: %v", err)
		}
		for key, want := range keep {
			if got, ok := golden[key]; !ok || got != want {
				t.Errorf("System.%s: golden says %d, %s says %d", key, golden[key], *mdpPath, want)
			}
		}
		return
	}

	old, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	header, _, found := strings.Cut(string(old), "# entity\tattribute\tlength\n")
	if !found {
		t.Fatalf("%s has lost its header line; restore it before regenerating", goldenPath)
	}
	body := header + "# entity\tattribute\tlength\n" + formatGoldenLengths(keep)
	if err := os.WriteFile(goldenPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	t.Logf("rewrote %s with %d measured lengths — update the header's version line by hand", goldenPath, len(keep))
}

// systemStringLengthsFromMDP returns "Entity.Attribute" -> String length for the
// System module's domain model in a deployed model.mdp.
func systemStringLengthsFromMDP(raw []byte) (map[string]int, error) {
	out := map[string]int{}
	inSystem := false
	for off := 0; off+4 <= len(raw); {
		size := int(binary.LittleEndian.Uint32(raw[off : off+4]))
		if size < 5 || off+size > len(raw) {
			break
		}
		var doc bson.M
		if err := bson.Unmarshal(raw[off:off+size], &doc); err == nil {
			switch ty, _ := doc["$Type"].(string); ty {
			case "Projects$ModuleImpl":
				name, _ := doc["Name"].(string)
				inSystem = name == "System"
			case "DomainModels$DomainModel":
				if inSystem {
					collectStringLengths(doc, out)
					inSystem = false
				}
			}
		}
		off += size
	}
	return out, nil
}

func collectStringLengths(domainModel bson.M, out map[string]int) {
	entities, _ := domainModel["Entities"].(bson.A)
	for _, ev := range entities {
		entity, _ := ev.(bson.M)
		entityName, _ := entity["UnqualifiedName"].(string)
		attrs, _ := entity["Attributes"].(bson.A)
		for _, av := range attrs {
			attr, _ := av.(bson.M)
			attrType, _ := attr["Type"].(bson.M)
			if ty, _ := attrType["$Type"].(string); ty != "DomainModels$StringAttributeType" {
				continue
			}
			attrName, _ := attr["Name"].(string)
			// A String attribute with no Length property is unlimited, the same
			// as an explicit 0 — Mendix omits the default. The width of the
			// stored integer is not fixed (mxcli has already been bitten by a
			// gen-declared int32 stored as int64, #585), so accept either rather
			// than reading every length as 0 through a failed assertion.
			out[entityName+"."+attrName] = bsonInt(attrType["Length"])
		}
	}
}

// bsonInt reads an integer property whatever width it was stored at.
func bsonInt(v any) int {
	switch n := v.(type) {
	case int32:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}
