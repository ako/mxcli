// SPDX-License-Identifier: Apache-2.0

// Command propdefaults folds property-set measurements into the table
// canon.CompletePropertySets fills gaps from
// (modelsdk/canon/studiopro_property_defaults.json).
//
// The measurements come from the doctype gate, which records what `mx convert`
// added to and removed from every unit mxcli wrote:
//
//	MX_BINARY=~/.mxcli/mxbuild/<version>/modeler/mx \
//	MXCLI_PROPERTY_SETS_RECORD=/tmp/ps.jsonl \
//	go test -tags integration -run TestMxCheck_DoctypeScripts ./mdl/executor/
//	go run ./internal/propertysets/cmd/propdefaults -record /tmp/ps.jsonl
//
// Run the gate once per Mendix version to measure (append to the same file);
// each version's gaps get that version as their floor, and a key a newer
// version no longer declares gets an upper bound. Review the diff: a value is
// only added if Mendix wrote the same one everywhere it was measured.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/mendixlabs/mxcli/internal/propertysets"
)

func main() {
	table := flag.String("table", "modelsdk/canon/studiopro_property_defaults.json", "the defaults table to update")
	record := flag.String("record", "", "JSON-lines file the gate wrote (MXCLI_PROPERTY_SETS_RECORD)")
	flag.Parse()
	if *record == "" {
		fmt.Fprintln(os.Stderr, "propdefaults: -record is required")
		os.Exit(2)
	}
	if err := run(*table, *record); err != nil {
		fmt.Fprintln(os.Stderr, "propdefaults:", err)
		os.Exit(1)
	}
}

func run(tablePath, recordPath string) error {
	var existing []propertysets.Entry
	raw, err := os.ReadFile(tablePath)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &existing); err != nil {
		return fmt.Errorf("%s: %w", tablePath, err)
	}

	f, err := os.Open(recordPath)
	if err != nil {
		return err
	}
	defer f.Close()
	perVersion := map[string][]*propertysets.Result{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var rec propertysets.Recording
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			return fmt.Errorf("%s: %w", recordPath, err)
		}
		if rec.Version == "" || rec.Result == nil {
			return fmt.Errorf("%s: a line without version or result", recordPath)
		}
		perVersion[rec.Version] = append(perVersion[rec.Version], rec.Result)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	byVersion := map[string]*propertysets.Result{}
	for v, rs := range perVersion {
		byVersion[v] = propertysets.Combine(rs)
		fmt.Printf("%s: %d scripts, %d gaps, %d extras\n", v, len(rs), len(byVersion[v].Gaps), len(byVersion[v].Extras))
		for _, e := range byVersion[v].Extras {
			fmt.Printf("  extra (fix the writer): %s.%s ×%d\n", e.Type, e.Key, e.Count)
		}
	}

	entries, varying := propertysets.Merge(existing, byVersion)
	for _, v := range varying {
		fmt.Printf("  varying, not in the table (fix the writer): %s\n", v)
	}
	out, err := propertysets.MarshalTable(entries)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tablePath, out, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s: %d entries (was %d)\n", tablePath, len(entries), len(existing))
	return nil
}
