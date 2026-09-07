// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	moddefs "github.com/mendixlabs/mxcli/modelsdk/widgets/definitions"
	sdkdefs "github.com/mendixlabs/mxcli/sdk/widgets/definitions"
	"github.com/mendixlabs/mxcli/sdk/widgets/mpk"
)

// A .def.json literal for an enumeration property must be the enumeration
// **key**, not the caption Studio Pro shows for it. Gallery's `pagingPosition`
// declares
//
//	<enumerationValue key="bottom">Below grid</enumerationValue>
//
// and the definition carried the caption's first word, "below". Mendix stores
// what we write and then rejects the widget with CE0463 "the definition of this
// widget has changed" — the error names the widget version, so nothing points at
// the value (mendixlabs/mxcli#1035).
//
// Nothing else catches this: the value is a plain string on both sides, `mxcli
// check` has no enum set for widget-level literals, and the failure only appears
// once mxbuild loads the page. It also hid for a year behind Gallery's
// hidden-property reset, which put `pagingPosition` back to its declared default
// for the default `pagination: buttons` — so only a non-default pagination
// reached the wrong value.
//
// The .mpk is the authority for the key set, so this test reads the packages in
// testdata rather than a hand-kept list.
func TestDefJSONEnumLiteralsAreDeclaredMPKKeys(t *testing.T) {
	enums := enumValuesFromTestdataMPKs(t)

	// Positive control: a scan that learned nothing would pass silently.
	if len(enums) < 50 {
		t.Fatalf("only %d enumeration properties learned from testdata .mpk files; "+
			"the scan is not reaching the packages, so a pass proves nothing", len(enums))
	}

	checked := 0
	for _, src := range []struct {
		name string
		read func() (map[string][]byte, error)
	}{
		{"sdk/widgets/definitions", func() (map[string][]byte, error) { return readDefJSON(sdkdefsFS{}) }},
		{"modelsdk/widgets/definitions", func() (map[string][]byte, error) { return readDefJSON(moddefsFS{}) }},
	} {
		files, err := src.read()
		if err != nil {
			t.Fatalf("%s: %v", src.name, err)
		}
		if len(files) == 0 {
			t.Fatalf("%s: no .def.json files found", src.name)
		}
		for file, data := range files {
			var def WidgetDefinition
			if err := json.Unmarshal(data, &def); err != nil {
				t.Errorf("%s/%s: %v", src.name, file, err)
				continue
			}
			for _, m := range collectLiterals(&def) {
				key := def.WidgetID + "." + m.property
				allowed, ok := enums[key]
				if !ok {
					continue // not an enumeration, or the package is not in testdata
				}
				checked++
				if !containsEnumKey(allowed, m.value) {
					t.Errorf("%s/%s: %s %s = %q is not a declared enumeration key (permitted: %s)\n"+
						"a caption is not a key; Mendix rejects the stored widget with CE0463",
						src.name, file, m.property, m.field, m.value, strings.Join(allowed, "|"))
				}
			}
		}
	}

	if checked == 0 {
		t.Fatal("no enumeration literal was compared; the test cannot detect anything")
	}
	t.Logf("compared %d enumeration literals against %d declared properties", checked, len(enums))
}

type defLiteral struct {
	property string // widget property key
	field    string // "value" or "default" — which .def.json field it came from
	value    string
}

// collectLiterals returns every literal a .def.json states for a top-level
// widget property: `value` (always written) and `default` (written when MDL
// says nothing). Object-list item properties carry their own enumValues and are
// validated by MDL-WIDGET08, so they are out of scope here.
func collectLiterals(def *WidgetDefinition) []defLiteral {
	var out []defLiteral
	add := func(ms []PropertyMapping) {
		for _, m := range ms {
			if m.Value != "" {
				out = append(out, defLiteral{m.PropertyKey, "value", m.Value})
			}
			if m.Default != "" {
				out = append(out, defLiteral{m.PropertyKey, "default", m.Default})
			}
		}
	}
	add(def.PropertyMappings)
	for _, mode := range def.Modes {
		add(mode.PropertyMappings)
	}
	return out
}

// enumValuesFromTestdataMPKs indexes every enumeration property declared by the
// widget packages in testdata, keyed "<widgetID>.<propertyKey>".
func enumValuesFromTestdataMPKs(t *testing.T) map[string][]string {
	t.Helper()
	dir := filepath.Join("..", "..", "testdata", "expr-checker", "widgets")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read widget testdata: %v", err)
	}
	mpk.ClearCache()
	out := map[string][]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mpk") {
			continue
		}
		defs, err := mpk.ParseMPKAll(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // a package we cannot parse teaches nothing; not this test's subject
		}
		for _, d := range defs {
			for _, p := range append(append([]mpk.PropertyDef{}, d.Properties...), d.SystemProps...) {
				if p.Type == "enumeration" && len(p.EnumValues) > 0 {
					vals := append([]string{}, p.EnumValues...)
					sort.Strings(vals)
					out[d.ID+"."+p.Key] = vals
				}
			}
		}
	}
	return out
}

func containsEnumKey(vals []string, want string) bool {
	for _, v := range vals {
		if v == want {
			return true
		}
	}
	return false
}

// The two definition sets are separate embed.FS values in separate packages;
// these adapters keep the loop above from repeating the read.
type defFS interface {
	ReadDir(string) ([]os.DirEntry, error)
	ReadFile(string) ([]byte, error)
}

type sdkdefsFS struct{}

func (sdkdefsFS) ReadDir(n string) ([]os.DirEntry, error) { return sdkdefs.EmbeddedFS.ReadDir(n) }
func (sdkdefsFS) ReadFile(n string) ([]byte, error)       { return sdkdefs.EmbeddedFS.ReadFile(n) }

type moddefsFS struct{}

func (moddefsFS) ReadDir(n string) ([]os.DirEntry, error) { return moddefs.EmbeddedFS.ReadDir(n) }
func (moddefsFS) ReadFile(n string) ([]byte, error)       { return moddefs.EmbeddedFS.ReadFile(n) }

func readDefJSON(fs defFS) (map[string][]byte, error) {
	entries, err := fs.ReadDir(".")
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".def.json") {
			continue
		}
		data, err := fs.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		out[e.Name()] = data
	}
	return out, nil
}
