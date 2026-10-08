// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// AddParameter appends a page or snippet parameter (mendixlabs/mxcli#1234).
//
// Before this existed the only way to add one was CREATE OR REPLACE, which
// rebuilds the whole document from describe output and so drops whatever
// describe does not round-trip. Here the parameter is appended to the stored
// document and nothing else is touched.
func (m *Mutator) AddParameter(p backend.PageParameterSpec) error {
	if m.containerType == backend.ContainerLayout {
		return fmt.Errorf("a layout has no parameters; add parameters applies to pages and snippets")
	}
	if names := documentParameterNames(m.rawData); contains(names, p.Name) {
		return fmt.Errorf("parameter $%s already exists", p.Name)
	}
	// Parameters and variables are referenced the same way ($Name), so a
	// shared name would make every reference ambiguous.
	for _, v := range bsonnav.DGetArrayElements(bsonnav.DGet(m.rawData, "Variables")) {
		if vDoc, ok := v.(bson.D); ok && bsonnav.DGetString(vDoc, "Name") == p.Name {
			return fmt.Errorf("$%s is already declared as a variable on this %s", p.Name, m.containerType)
		}
	}

	doc, err := m.deps.SerializeParameter(m.containerType, p)
	if err != nil {
		return fmt.Errorf("build parameter $%s: %w", p.Name, err)
	}
	if doc == nil {
		return fmt.Errorf("build parameter $%s: the engine produced nothing", p.Name)
	}

	if bsonnav.DGet(m.rawData, "Parameters") != nil {
		elements := bsonnav.DGetArrayElements(bsonnav.DGet(m.rawData, "Parameters"))
		bsonnav.DSetArray(m.rawData, "Parameters", append(elements, doc))
	} else {
		m.rawData = append(m.rawData, bson.E{Key: "Parameters", Value: bson.A{int32(3), doc}})
	}
	return nil
}

// DropParameter removes a parameter by name. It refuses while the document still
// uses the parameter: a data source bound to it, or an expression naming it,
// would be left pointing at nothing — valid-looking BSON that mx check rejects.
func (m *Mutator) DropParameter(name string) error {
	if m.containerType == backend.ContainerLayout {
		return fmt.Errorf("a layout has no parameters; drop parameters applies to pages and snippets")
	}
	elements := bsonnav.DGetArrayElements(bsonnav.DGet(m.rawData, "Parameters"))
	var kept []any
	found := false
	for _, elem := range elements {
		if doc, ok := elem.(bson.D); ok && bsonnav.DGetString(doc, "Name") == name {
			found = true
			continue
		}
		kept = append(kept, elem)
	}
	if !found {
		names := documentParameterNames(m.rawData)
		if len(names) == 0 {
			return fmt.Errorf("parameter $%s not found: this %s has no parameters", name, m.containerType)
		}
		return fmt.Errorf("parameter $%s not found; this %s declares $%s", name, m.containerType, strings.Join(names, ", $"))
	}

	var uses []string
	for _, e := range m.rawData {
		if e.Key == "Parameters" {
			continue
		}
		collectParameterUses(e.Key, e.Value, name, &uses)
	}
	if len(uses) > 0 {
		sort.Strings(uses)
		return fmt.Errorf("parameter $%s is still used by this %s (%s); remove those uses first",
			name, m.containerType, strings.Join(dedupe(uses), ", "))
	}

	bsonnav.DSetArray(m.rawData, "Parameters", kept)
	return nil
}

// documentParameterNames lists the declared parameter names in stored order.
func documentParameterNames(raw bson.D) []string {
	var out []string
	for _, p := range bsonnav.DGetArrayElements(bsonnav.DGet(raw, "Parameters")) {
		if doc, ok := p.(bson.D); ok {
			if n := bsonnav.DGetString(doc, "Name"); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// collectParameterUses walks a document subtree for references to a parameter:
// a Forms$PageVariable whose PageParameter/SnippetParameter names it (how a data
// view, an input or a call argument binds to it), or an expression that mentions
// $name. Each use is recorded by the key it was found under.
func collectParameterUses(key string, v any, name string, uses *[]string) {
	switch val := v.(type) {
	case bson.D:
		for _, e := range val {
			collectParameterUses(e.Key, e.Value, name, uses)
		}
	case bson.A:
		for _, item := range val {
			collectParameterUses(key, item, name, uses)
		}
	case []any:
		for _, item := range val {
			collectParameterUses(key, item, name, uses)
		}
	case string:
		switch {
		case key == "PageParameter" || key == "SnippetParameter":
			if val == name || strings.HasSuffix(val, "."+name) {
				*uses = append(*uses, key)
			}
		case strings.Contains(key, "Expression") || strings.Contains(key, "XPath"):
			if mentionsVariable(val, name) {
				*uses = append(*uses, key)
			}
		}
	}
}

// mentionsVariable reports whether an expression names $name as a whole token —
// `$Customer/Name` does, `$CustomerList` does not.
func mentionsVariable(expr, name string) bool {
	needle := "$" + name
	for i := 0; ; {
		j := strings.Index(expr[i:], needle)
		if j < 0 {
			return false
		}
		end := i + j + len(needle)
		if end == len(expr) || !isIdentChar(expr[end]) {
			return true
		}
		i = end
	}
}

func isIdentChar(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func dedupe(sorted []string) []string {
	var out []string
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}
