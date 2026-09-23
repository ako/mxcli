// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	_ "modernc.org/sqlite"
)

// The internals pages are the reference someone uses to build an independent
// reader or writer for the .mpr format — that is what they are for. They
// described a `UnitContents` table that has never existed in any .mpr, and a
// v1/v2 detection recipe that probes for it. Implemented as written the probe
// can never succeed, so it returns v2 for every project including genuine v1
// ones: a wrong answer rather than an error, which is the kind that survives
// testing (mendixlabs/mxcli#1072).
//
// Prose cannot be type-checked, but the identifiers in it can. These tests
// hold the pages to the schema of two real fixtures — the v1 project in this
// package's testdata and the v2 project at testdata/expr-checker — so a
// fabricated table or column fails the suite instead of a reader six months
// from now.
//
// The invariant they enforce, stated once: **on a page describing the .mpr, any
// identifier in the Unit/_MetaData/_Transaction namespace written in code font
// is a real table or column.** So a page that wants to say a column does NOT
// exist says it in prose — code font there would be indistinguishable from the
// defect. (The old pages did exactly that at mpr-v1-v2.md:35, "no
// `UnitContents`", which read as a v1/v2 difference rather than as a fiction.)

// repoRoot is the module root, two levels up from modelsdk/mpr.
const repoRoot = "../.."

const (
	v1Fixture = "testdata/v1-project/App.mpr"
	v2Fixture = repoRoot + "/testdata/expr-checker/minimal.mpr"
)

// mprSchema reads table -> column set straight out of an .mpr's SQLite
// catalog. Measured, never asserted from the docs under test.
func mprSchema(t *testing.T, path string) map[string]map[string]bool {
	t.Helper()
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", path))
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatalf("list tables in %s: %v", path, err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, n)
	}
	rows.Close()
	if len(tables) == 0 {
		t.Fatalf("%s has no tables — fixture missing or not an .mpr", path)
	}

	schema := make(map[string]map[string]bool, len(tables))
	for _, tbl := range tables {
		cols, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tbl))
		if err != nil {
			t.Fatalf("table_info(%s): %v", tbl, err)
		}
		set := map[string]bool{}
		for cols.Next() {
			var cid, notNull, pk int
			var name, colType string
			var dflt *string
			if err := cols.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
				t.Fatalf("scan column of %s: %v", tbl, err)
			}
			set[name] = true
		}
		cols.Close()
		schema[tbl] = set
	}
	return schema
}

// realMPRSchema unions the v1 and v2 fixtures: a name is real if either
// format has it. v2 drops Unit.Contents and adds _Transaction and
// _MetaData._FormatVersion, so neither fixture alone is the whole vocabulary.
func realMPRSchema(t *testing.T) (tables map[string]map[string]bool) {
	t.Helper()
	tables = map[string]map[string]bool{}
	for _, f := range []string{v1Fixture, v2Fixture} {
		for tbl, cols := range mprSchema(t, f) {
			if tables[tbl] == nil {
				tables[tbl] = map[string]bool{}
			}
			for c := range cols {
				tables[tbl][c] = true
			}
		}
	}
	return tables
}

// mprDocPages returns every reference page that describes the .mpr file.
// Discovered by content, not hardcoded, so a page added later is covered
// without anyone remembering to add it here. Proposals and plans are excluded
// on purpose: they record what was considered, not what the format is.
func mprDocPages(t *testing.T) []string {
	t.Helper()
	var pages []string
	for _, dir := range []string{
		filepath.Join(repoRoot, "docs-site", "src"),
		filepath.Join(repoRoot, "docs", "05-mdl-specification"),
	} {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(b), "mprcontents") || strings.Contains(string(b), ".mpr") {
				pages = append(pages, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if len(pages) == 0 {
		t.Fatal("found no reference pages describing the .mpr file — the walk is looking in the wrong place")
	}
	return pages
}

var (
	backtickedIdent = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_$]*)`")
	headingIdent    = regexp.MustCompile(`^#{1,6}\s+.*?\b([A-Za-z_][A-Za-z0-9_$]*)\s+Table\b`)
)

// TestMPRDocsNameOnlyRealUnitSchema fails when a reference page names an
// identifier in the .mpr's own namespace that no real .mpr has.
//
// The rule is deliberately narrow: only names that BEGIN with a real table
// name are checked, so the catalog tables these pages also mention (`REFS`
// and friends) are out of scope and need no allowlist. That is exactly the
// shape of the reported defect — `UnitContents` alongside the real `Unit` —
// and of its quieter half, a `UnitType` column on a table that has none.
func TestMPRDocsNameOnlyRealUnitSchema(t *testing.T) {
	tables := realMPRSchema(t)

	legit := map[string]bool{}
	var prefixes []string
	for tbl, cols := range tables {
		legit[tbl] = true
		prefixes = append(prefixes, tbl)
		for c := range cols {
			legit[c] = true
		}
	}
	sort.Strings(prefixes)

	inNamespace := func(name string) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
		return false
	}

	for _, page := range mprDocPages(t) {
		b, err := os.ReadFile(page)
		if err != nil {
			t.Fatalf("read %s: %v", page, err)
		}
		rel, _ := filepath.Rel(repoRoot, page)
		for i, line := range strings.Split(string(b), "\n") {
			var names []string
			for _, m := range backtickedIdent.FindAllStringSubmatch(line, -1) {
				names = append(names, m[1])
			}
			if m := headingIdent.FindStringSubmatch(line); m != nil {
				names = append(names, m[1])
			}
			for _, n := range names {
				if inNamespace(n) && !legit[n] {
					t.Errorf("%s:%d names %q, which is not a table or column of any real .mpr.\n"+
						"Measured tables: %s\nThese pages are what an independent reader/writer is built from.",
						rel, i+1, n, describeSchema(tables))
				}
			}
		}
	}
}

func describeSchema(tables map[string]map[string]bool) string {
	var names []string
	for t := range tables {
		names = append(names, t)
	}
	sort.Strings(names)
	var parts []string
	for _, n := range names {
		var cols []string
		for c := range tables[n] {
			cols = append(cols, c)
		}
		sort.Strings(cols)
		parts = append(parts, fmt.Sprintf("%s(%s)", n, strings.Join(cols, ", ")))
	}
	return strings.Join(parts, "; ")
}

// TestMPRFormatDocUnitColumnsAreReal reads the column table printed under the
// "Unit Table" heading in the format page and holds every documented column
// to the fixture. The prefix rule above cannot see `Name`, which the page
// listed as a Unit column: unit type and name are read out of the BSON
// contents (getTypeFromContents), never off a column.
func TestMPRFormatDocUnitColumnsAreReal(t *testing.T) {
	real := mprSchema(t, v1Fixture)["Unit"]
	if real == nil {
		t.Fatal("v1 fixture has no Unit table")
	}

	page := filepath.Join(repoRoot, "docs-site", "src", "internals", "mpr-format.md")
	b, err := os.ReadFile(page)
	if err != nil {
		t.Fatalf("read %s: %v", page, err)
	}

	documented := unitColumnTable(t, string(b))
	if len(documented) == 0 {
		t.Fatal("docs-site/src/internals/mpr-format.md no longer prints a column table under a Unit Table heading; " +
			"this test is then asserting nothing — re-point it or delete it")
	}
	for _, col := range documented {
		if !real[col] {
			t.Errorf("mpr-format.md documents Unit column %q; the v1 fixture's Unit table has no such column (%s)",
				col, describeSchema(map[string]map[string]bool{"Unit": real}))
		}
	}
}

// unitColumnTable pulls the first cell of every body row of the first
// markdown table that follows a heading naming the Unit table.
func unitColumnTable(t *testing.T, md string) []string {
	t.Helper()
	lines := strings.Split(md, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "#") && regexp.MustCompile(`\bUnit\s+Table\b`).MatchString(l) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}
	var cols []string
	inTable := false
	for _, l := range lines[start+1:] {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "#") {
			break
		}
		if !strings.HasPrefix(trimmed, "|") {
			if inTable {
				break
			}
			continue
		}
		inTable = true
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		first := strings.TrimSpace(cells[0])
		first = strings.Trim(first, "`")
		if first == "" || first == "Column" || strings.HasPrefix(first, "-") || strings.HasPrefix(first, ":") {
			continue
		}
		cols = append(cols, first)
	}
	return cols
}

// TestMPRVersionDetectionRestsOnContentsColumn is the measured basis for the
// corrected detection prose. Detection is by directory first
// (Open: os.Stat of mprcontents/), with the Unit.Contents column as the
// reconciliation fallback for a .mpr copied away from its folder. Both halves
// are only sound because the column is present in exactly one of the two
// formats — the property the docs' old "UnitContents table exists" recipe was
// reaching for and got wrong.
func TestMPRVersionDetectionRestsOnContentsColumn(t *testing.T) {
	if got := mprSchema(t, v1Fixture)["Unit"]["Contents"]; !got {
		t.Error("v1 fixture's Unit table has no Contents column; v1 contents are stored there, not in a separate table")
	}
	if got := mprSchema(t, v2Fixture)["Unit"]["Contents"]; got {
		t.Error("v2 fixture's Unit table has a Contents column; v2 stores contents in mprcontents/, so the column should be absent")
	}
	if _, ok := mprSchema(t, v1Fixture)["UnitContents"]; ok {
		t.Error("v1 fixture has a UnitContents table — the docs' claim would be correct and this whole file is wrong")
	}

	r1, err := Open(filepath.Join("testdata", "v1-project", "App.mpr"))
	if err != nil {
		t.Fatalf("open v1 fixture: %v", err)
	}
	defer r1.Close()
	if r1.Version() != MPRVersionV1 {
		t.Errorf("v1 fixture detected as %v, want %v", r1.Version(), MPRVersionV1)
	}

	r2, err := Open(filepath.Join(repoRoot, "testdata", "expr-checker", "minimal.mpr"))
	if err != nil {
		t.Fatalf("open v2 fixture: %v", err)
	}
	defer r2.Close()
	if r2.Version() != MPRVersionV2 {
		t.Errorf("v2 fixture detected as %v, want %v", r2.Version(), MPRVersionV2)
	}
}

// --- Unit types -------------------------------------------------------------
//
// The same pages map a document's BSON $Type to a document kind. Those were
// wrong in the same way and for the same reason: several rows named the
// TypeScript SDK's qualified name instead of the storage name Mendix actually
// writes — `Pages$Page` for what every real unit calls `Forms$Page` — and one
// page lowercased half the table, which matters because $Type is
// case-sensitive. Selecting on either spelling matches zero units: a wrong
// answer rather than an error, the same failure mode as the UnitContents
// detection recipe above.

// realUnitTypes returns every distinct $Type across both fixtures. The v1
// fixture's units are Unit.Contents blobs; the v2 fixture's are .mxunit files.
func realUnitTypes(t *testing.T) map[string]bool {
	t.Helper()
	types := map[string]bool{}

	add := func(raw []byte) {
		if v, err := bson.Raw(raw).LookupErr("$Type"); err == nil {
			if s, ok := v.StringValueOK(); ok && s != "" {
				types[s] = true
			}
		}
	}

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", v1Fixture))
	if err != nil {
		t.Fatalf("open v1 fixture: %v", err)
	}
	rows, err := db.Query(`SELECT Contents FROM Unit`)
	if err != nil {
		db.Close()
		t.Fatalf("read v1 contents: %v", err)
	}
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			t.Fatalf("scan v1 contents: %v", err)
		}
		add(blob)
	}
	rows.Close()
	db.Close()

	v2Dir := filepath.Join(repoRoot, "testdata", "expr-checker", "mprcontents")
	err = filepath.Walk(v2Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".mxunit") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		add(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", v2Dir, err)
	}

	if len(types) < 20 {
		t.Fatalf("only %d distinct $Type values across both fixtures — the fixtures are not being read", len(types))
	}
	return types
}

var docTypeName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*\$[A-Za-z][A-Za-z0-9]*$`)

// unitTypeTables collects the first column of every markdown table under a
// heading naming unit types, across both pages that print one.
func unitTypeTables(t *testing.T) map[string][]string {
	t.Helper()
	pages := []string{
		filepath.Join(repoRoot, "docs-site", "src", "internals", "mpr-format.md"),
		filepath.Join(repoRoot, "docs", "05-mdl-specification", "10-bson-mapping.md"),
	}
	out := map[string][]string{}
	for _, page := range pages {
		b, err := os.ReadFile(page)
		if err != nil {
			t.Fatalf("read %s: %v", page, err)
		}
		rel, _ := filepath.Rel(repoRoot, page)
		lines := strings.Split(string(b), "\n")
		start := -1
		for i, l := range lines {
			if strings.HasPrefix(l, "#") && strings.Contains(l, "Unit Types") {
				start = i
				break
			}
		}
		if start < 0 {
			t.Errorf("%s no longer has a Unit Types heading; this test is asserting nothing about it", rel)
			continue
		}
		var found []string
		for _, l := range lines[start+1:] {
			trimmed := strings.TrimSpace(l)
			if strings.HasPrefix(trimmed, "#") {
				break // next heading — stop, but keep every table until then
			}
			if !strings.HasPrefix(trimmed, "|") {
				continue
			}
			cells := strings.Split(strings.Trim(trimmed, "|"), "|")
			name := strings.Trim(strings.TrimSpace(cells[0]), "`")
			if docTypeName.MatchString(name) {
				found = append(found, name)
			}
		}
		if len(found) == 0 {
			t.Errorf("%s prints no $Type rows under Unit Types", rel)
		}
		out[rel] = found
	}
	return out
}

// TestDocumentedUnitTypesUseStorageNames holds every documented $Type to the
// spelling real units carry.
//
// The check is deliberately keyed on the LOCAL name (the part after the `$`),
// case-insensitively, because the fixtures cannot prove a type absent — a
// blank project simply has no business-event service, so demanding that every
// documented type appear would fail on rows that are perfectly correct. But
// when a fixture DOES have a type with the same local name, the documented
// row must match it exactly: that catches `Pages$Page` against `Forms$Page`
// and every lowercased spelling, with no false positives on legitimately
// absent types.
//
// The limit is worth stating: a documented type whose local name appears
// nowhere in the fixtures is not checked at all. `CustomWidgets$customwidget`
// was one such row and had to be removed by hand — it is a widget element
// inside a page's tree (`CustomWidgets$CustomWidget`), never a unit.
func TestDocumentedUnitTypesUseStorageNames(t *testing.T) {
	real := realUnitTypes(t)

	byLocal := map[string][]string{}
	for full := range real {
		local := strings.ToLower(full[strings.Index(full, "$")+1:])
		byLocal[local] = append(byLocal[local], full)
	}
	for k := range byLocal {
		sort.Strings(byLocal[k])
	}

	for page, documented := range unitTypeTables(t) {
		for _, d := range documented {
			local := strings.ToLower(d[strings.Index(d, "$")+1:])
			candidates, known := byLocal[local]
			if !known {
				continue // no unit of this kind in either fixture — unprovable here
			}
			if real[d] {
				continue
			}
			t.Errorf("%s documents $Type %q; real units spell it %s.\n"+
				"$Type is the storage name and is case-sensitive — selecting on the SDK's "+
				"qualified name matches zero units.",
				page, d, strings.Join(candidates, " or "))
		}
	}
}
