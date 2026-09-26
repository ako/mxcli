// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	_ "modernc.org/sqlite"
)

// A Starlark rule that compares a field to a value the API never emits does not
// error: it skips every item, reports nothing, and its output is byte-identical to
// a clean project. ARCH002/ARCH003 compared entity_type to "PERSISTENT" that way
// (#1164), and the rule-authoring skill documented microflow_type as "microflow" /
// "nanoflow" while the example rule every project gets said "Microflow" /
// "Nanoflow" -- neither of which the linter has ever returned (#1178).
//
// Two passes over the same skill file fixed instances and missed the next one,
// because nothing tied the documented literals to the API. These tests do: the
// emitted values and field names are observed by running a rule against a fixture
// holding every stored kind, and every documented literal must be one of them.

const (
	lintSkillPath = "../../.claude/skills/mendix/write-lint-rules/SKILL.md"
	lintRulesDir  = "../../.claude/lint-rules"
)

// observed is what a Starlark rule actually receives from entities() and
// microflows(): the field names on each struct and the distinct enum values.
type observed struct {
	entityFields, microflowFields []string
	entityTypes, microflowTypes   []string
}

func observeStarlarkAPI(t *testing.T) observed {
	t.Helper()
	src := `
RULE_ID = "TEST_OBSERVE"
RULE_NAME = "Observe"
DESCRIPTION = "reports every value the API hands a rule"
CATEGORY = "quality"
SEVERITY = "info"

def check():
    out = []
    for e in entities():
        out.append(violation(message="entity|" + e.entity_type + "|" + ",".join(dir(e))))
    for mf in microflows():
        out.append(violation(message="microflow|" + mf.microflow_type + "|" + ",".join(dir(mf))))
    return out
`
	path := filepath.Join(t.TempDir(), "observe.star")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}
	var o observed
	for _, v := range r.Check(linter.NewLintContextFromDB(everyKindFixtureDB(t))) {
		parts := strings.SplitN(v.Message, "|", 3)
		if len(parts) != 3 {
			t.Fatalf("unexpected message %q", v.Message)
		}
		fields := strings.Split(parts[2], ",")
		switch parts[0] {
		case "entity":
			o.entityTypes = appendUnique(o.entityTypes, parts[1])
			o.entityFields = fields
		case "microflow":
			o.microflowTypes = appendUnique(o.microflowTypes, parts[1])
			o.microflowFields = fields
		}
	}
	// The fixture holds three kinds of each; seeing fewer means the fixture or
	// the iterator dropped rows, and every assertion below would be vacuous.
	if len(o.entityTypes) != 3 || len(o.microflowTypes) != 3 {
		t.Fatalf("observed entity types %v and microflow types %v, want three of each",
			o.entityTypes, o.microflowTypes)
	}
	return o
}

// One row per value the catalog builder stores: EntityType from
// mdl/catalog/builder_modules.go, MicroflowType from builder_microflows.go (all
// three flavours share one table).
func everyKindFixtureDB(t *testing.T) catalog.CatalogDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	stmts := []string{
		`CREATE TABLE modules (Id TEXT, Name TEXT, Source TEXT)`,
		`INSERT INTO modules VALUES ('m1', 'Sales', '')`,
		`CREATE TABLE entities (
			Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			EntityType TEXT, Description TEXT, Generalization TEXT,
			AttributeCount INTEGER, AccessRuleCount INTEGER, ValidationRuleCount INTEGER,
			HasEventHandlers INTEGER, IsExternal INTEGER,
			HasCreatedDate INTEGER, HasChangedDate INTEGER,
			HasOwner INTEGER, HasChangedBy INTEGER)`,
		`INSERT INTO entities VALUES
			('e1','A','Sales.A','Sales','','PERSISTENT','','',0,0,0,0,0,0,0,0,0),
			('e2','B','Sales.B','Sales','','NON_PERSISTENT','','',0,0,0,0,0,0,0,0,0),
			('e3','C','Sales.C','Sales','','VIEW','','',0,0,0,0,0,0,0,0,0)`,
		`CREATE TABLE microflows (
			Id TEXT, Name TEXT, QualifiedName TEXT, ModuleName TEXT, Folder TEXT,
			MicroflowType TEXT, Description TEXT, ReturnType TEXT,
			ParameterCount INTEGER, ActivityCount INTEGER, Complexity INTEGER)`,
		`INSERT INTO microflows VALUES
			('f1','MF','Sales.MF','Sales','','MICROFLOW','','',0,0,1),
			('f2','NF','Sales.NF','Sales','','NANOFLOW','','',0,0,1),
			('f3','RU','Sales.RU','Sales','','RULE','','',0,0,1)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("fixture %q: %v", s, err)
		}
	}
	return catalog.WrapSqlDB(db)
}

// The skill's entity and microflow tables must name exactly the fields the
// struct exposes, and the enum rows must list exactly the values it emits:
// a documented value that never occurs is a rule that silently reports nothing,
// and an undocumented one ("RULE") is a flavour a rule silently mistreats.
func TestLintSkillDocumentsWhatTheAPIEmits(t *testing.T) {
	o := observeStarlarkAPI(t)
	skill, err := os.ReadFile(lintSkillPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		section, enumField string
		fields, values     []string
	}{
		{"entity", "entity_type", o.entityFields, o.entityTypes},
		{"microflow", "microflow_type", o.microflowFields, o.microflowTypes},
	} {
		t.Run(tc.section, func(t *testing.T) {
			rows := skillTableRows(t, string(skill), tc.section)
			var documented []string
			for name := range rows {
				documented = append(documented, name)
			}
			assertSameSet(t, "fields in the skill's "+tc.section+" table", documented, tc.fields)

			row, ok := rows[tc.enumField]
			if !ok {
				t.Fatalf("skill's %s table has no %s row", tc.section, tc.enumField)
			}
			assertSameSet(t, "values documented for "+tc.enumField+" in the skill",
				quotedLiterals(row), tc.values)
		})
	}
}

// The comment headers of the shipped rules are what a new rule is copied from
// (example_microflow.star exists for exactly that, and `mxcli init` ships it into
// every project). Each `.entity_type` / `.microflow_type` line there must list
// exactly the emitted values.
func TestShippedRuleHeadersDocumentWhatTheAPIEmits(t *testing.T) {
	o := observeStarlarkAPI(t)
	want := map[string][]string{"entity_type": o.entityTypes, "microflow_type": o.microflowTypes}
	header := regexp.MustCompile(`^#\s+\.(entity_type|microflow_type)\s+-\s+(.*)$`)

	seen := 0
	for _, path := range shippedRules(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			m := header.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			seen++
			assertSameSet(t, filepath.Base(path)+":"+strconv.Itoa(i+1)+" ."+m[1],
				quotedLiterals(m[2]), want[m[1]])
		}
	}
	if seen == 0 {
		t.Fatal("no .entity_type / .microflow_type header lines found -- the pattern no longer matches the files")
	}
}

// Every literal a shipped rule compares one of these fields to must be a value
// the API emits. This is the #1164 shape directly: `entity_type == "PERSISTENT"`.
func TestShippedRulesCompareOnlyToEmittedValues(t *testing.T) {
	o := observeStarlarkAPI(t)
	want := map[string][]string{"entity_type": o.entityTypes, "microflow_type": o.microflowTypes}
	cmp := regexp.MustCompile(`\.(entity_type|microflow_type)\s*(?:==|!=|in|not in)\s*(\[[^\]]*\]|\([^)]*\)|"[^"]*")`)

	seen := 0
	for _, path := range shippedRules(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range cmp.FindAllStringSubmatch(string(data), -1) {
			seen++
			for _, lit := range quotedLiterals(m[2]) {
				if !slices.Contains(want[m[1]], lit) {
					t.Errorf("%s compares .%s to %q, which the linter never returns (it returns %v): "+
						"the rule silently skips every item", filepath.Base(path), m[1], lit, want[m[1]])
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no comparisons found -- the pattern no longer matches the shipped rules")
	}
}

func shippedRules(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(lintRulesDir, "*.star"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no shipped rules under %s: %v", lintRulesDir, err)
	}
	return paths
}

// skillTableRows returns the "### <section>" table of the skill as
// property name -> the rest of the row.
func skillTableRows(t *testing.T, skill, section string) map[string]string {
	t.Helper()
	_, after, ok := strings.Cut(skill, "\n### "+section+"\n")
	if !ok {
		t.Fatalf("skill has no ### %s section", section)
	}
	row := regexp.MustCompile("^\\|\\s*`([a-z_]+)`\\s*\\|(.*)$")
	rows := map[string]string{}
	for _, line := range strings.Split(after, "\n") {
		if strings.HasPrefix(line, "### ") {
			break
		}
		if m := row.FindStringSubmatch(line); m != nil {
			rows[m[1]] = m[2]
		}
	}
	if len(rows) == 0 {
		t.Fatalf("### %s section has no property rows", section)
	}
	return rows
}

func quotedLiterals(s string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(s, -1) {
		out = appendUnique(out, m[1])
	}
	return out
}

func assertSameSet(t *testing.T, what string, got, want []string) {
	t.Helper()
	g, w := slices.Clone(got), slices.Clone(want)
	sort.Strings(g)
	sort.Strings(w)
	if !slices.Equal(g, w) {
		t.Errorf("%s: documented %v, API emits %v", what, g, w)
	}
}

func appendUnique(s []string, v string) []string {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}
