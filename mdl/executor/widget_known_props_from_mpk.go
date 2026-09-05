// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// enrichKnownPropertiesFromMPK fills each definition's KnownProperties from the
// widget's INSTALLED package, so the property validator recognises every
// property the widget actually declares.
//
// # Why
//
// mxcli reads a widget from two places. DESCRIBE WIDGET parses the project's
// .mpk (version-accurate, and the only place a Marketplace widget appears). The
// property validator reads the WidgetDefinition. For most widgets those agree,
// because the .def.json cache is GENERATED from the .mpk and its
// knownProperties already carry everything unmapped.
//
// Nine widgets are the exception. COMBOBOX, GALLERY, IMAGE, BARCODESCANNER, the
// four data-grid filters and DROPDOWNSORT have hand-crafted definitions in
// sdk/widgets/definitions/ and are deliberately never extracted per-project, so
// their .def.json never exists and their hand-written property list is whatever
// someone typed. Measured against the packages in testdata/expr-checker:
//
//	combobox        73 properties in the .mpk,  7 mapped + 4 known
//	gallery         44                         12
//	image           37                         14
//	barcodescanner  12                          1
//
// So DESCRIBE emitted an example it labels "parses as written" — and it does
// parse — naming properties the validator then rejected as nonexistent. Because
// exec refuses a script with errors, BARCODESCANNER could not be placed from MDL
// in any legal form: name the five properties and exec refuses, omit them and
// mxbuild reports CE0463 "the definition of this widget has changed".
//
// # Known, not allowed
//
// A .mpk property with no mapping is added to KnownProperties, NOT to the
// allowed set. That is the honest distinction the validator already draws:
// knownUnmappedProperties turns it into MDL-WIDGET06 — "recognized but not yet
// persisted by mxcli; a non-default value will be dropped" — rather than
// silently accepting a value nothing writes. Promoting them to "allowed" would
// trade a false error for a silent drop.
//
// MDL-WIDGET01 keeps its job: a name no package declares is still an error, so
// typos are still caught.
//
// # Applied to every definition, not to a list of nine
//
// Recomputing KnownProperties for a generated definition produces what
// generation already put there, so the enrichment is idempotent where it is
// redundant. Naming the nine would be the same hand-maintained-list defect one
// layer up — the defect this whole line of work exists to remove.
func enrichKnownPropertiesFromMPK(r *WidgetRegistry, projectPath string) {
	if r == nil || projectPath == "" {
		return
	}
	byID := mpkPropertiesByWidgetID(filepath.Dir(projectPath))
	if len(byID) == 0 {
		return
	}
	for _, def := range r.byWidgetID {
		props, ok := byID[def.WidgetID]
		if !ok {
			continue
		}
		allowed, _ := allowedWidgetProperties(def)
		seen := make(map[string]bool, len(def.KnownProperties))
		for _, k := range def.KnownProperties {
			seen[strings.ToLower(k)] = true
		}
		var added []string
		for _, p := range props {
			if p.Key == "" || p.IsSystem {
				continue
			}
			l := strings.ToLower(p.Key)
			if allowed[l] || seen[l] {
				continue
			}
			seen[l] = true
			added = append(added, p.Key)
		}
		if len(added) == 0 {
			continue
		}
		sort.Strings(added)
		def.KnownProperties = append(def.KnownProperties, added...)
	}
}

// mpkPropertiesByWidgetID parses every package in the project's widgets/ folder
// once and indexes the properties by widget id.
//
// ParseAll rather than ParseMPK: a bundled package (Charts.mpk) carries many
// widgets and ParseMPK returns only the first, which is the #679 bug — here it
// would silently leave every chart but one unenriched.
func mpkPropertiesByWidgetID(projectDir string) map[string][]mpk.PropertyDef {
	matches, err := filepath.Glob(filepath.Join(projectDir, "widgets", "*.mpk"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	out := make(map[string][]mpk.PropertyDef, len(matches))
	for _, path := range matches {
		defs, err := mpk.ParseAll(path)
		if err != nil {
			continue // a package we cannot read enriches nothing; it is not an error
		}
		for _, d := range defs {
			if d == nil || d.ID == "" {
				continue
			}
			out[d.ID] = d.Properties
		}
	}
	return out
}
