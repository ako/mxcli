// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#584 — SystemAttrDef declared a Length and not one of the 115 String
// attributes populated it, so every System string read as unlimited. The cost
// was not cosmetic: a view entity selecting `u.Name` from System.User had its
// correct declaration (`String(100)`) refused and its wrong one (`String`)
// waved through, to fail the build later with CE6770.
//
// The lengths are MEASURED, from the System module's domain model inside the
// `deployment/model/model.mdp` mxbuild writes — the same model the runtime
// creates the System tables from. testdata/system_string_lengths.txt is that
// measurement; this test holds SystemEntities to it.
//
// Two searches this saves repeating, both already spent on #584: the Mendix
// Model SDK does NOT carry these (its gen/ describes metamodel TYPES, so
// System.User.Name is simply not in it), and reading them out of
// Mendix.Modeler.Core.dll means a decompiler. The .mdp costs one build for all
// 115 at once.
package meta

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var (
	mdpPath    = flag.String("mdp", "", "path to a built deployment/model/model.mdp to re-measure from")
	updateGold = flag.Bool("update", false, "rewrite testdata/system_string_lengths.txt from -mdp")
)

const goldenPath = "testdata/system_string_lengths.txt"

func TestSystemStringLengths(t *testing.T) {
	if *updateGold {
		t.Skip("-update regenerates the golden; see TestSystemStringLengthsUpdate")
	}
	golden, err := readGoldenLengths(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	declared := map[string]int{}
	for _, e := range SystemEntities {
		for _, a := range e.Attributes {
			if a.Type == "String" {
				declared[e.Name+"."+a.Name] = a.Length
			}
		}
	}

	for key, want := range golden {
		got, ok := declared[key]
		if !ok {
			t.Errorf("System.%s is measured at length %d but SystemEntities no longer declares it as a String", key, want)
			continue
		}
		if got != want {
			t.Errorf("System.%s: SystemEntities says String(%d), the deployed model says String(%d)\n"+
				"  Mendix decides CE6770 by ITS number, so mxcli disagreeing with it is a wrong answer, not a stale one.\n"+
				"  Re-measure with -mdp <deployment/model/model.mdp> -update rather than editing either side by hand.",
				key, got, want)
		}
	}
	// The other direction is what makes this a guard rather than a snapshot: a
	// String attribute added to SystemEntities without being measured has no
	// golden row, and would otherwise sit at length 0 — reading as "unlimited",
	// which is a claim, not an absence.
	for key := range declared {
		if _, ok := golden[key]; !ok {
			t.Errorf("System.%s is declared as a String but has no measured length.\n"+
				"  Build a project on the target Mendix version and re-measure:\n"+
				"    mxcli new Probe --version <v> --theme none --layout none --skip-init\n"+
				"    go test ./modelsdk/meta -run TestSystemStringLengths -mdp Probe/deployment/model/model.mdp -update",
				key)
		}
	}
}

// readGoldenLengths parses the measured table: entity<TAB>attribute<TAB>length,
// '#' comments, keyed as "Entity.Attribute".
func readGoldenLengths(path string) (map[string]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]int{}
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		parts := strings.Split(text, "\t")
		if len(parts) != 3 {
			return nil, fmt.Errorf("%s:%d: want entity<TAB>attribute<TAB>length, got %q", path, line, text)
		}
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: length: %w", path, line, err)
		}
		out[parts[0]+"."+parts[1]] = n
	}
	return out, sc.Err()
}

// formatGoldenLengths renders the table body in the golden's stable order.
func formatGoldenLengths(lengths map[string]int) string {
	keys := make([]string, 0, len(lengths))
	for k := range lengths {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		entity, attr, _ := strings.Cut(k, ".")
		fmt.Fprintf(&b, "%s\t%s\t%d\n", entity, attr, lengths[k])
	}
	return b.String()
}
