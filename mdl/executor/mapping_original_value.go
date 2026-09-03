// SPDX-License-Identifier: Apache-2.0

// Carrying a mapping value element's OriginalValue through a rewrite
// (ako/mxcli#379).
//
// OriginalValue is the sample parsed out of the JSON structure's snippet
// ("42", "\"Widget\""). mxcli wrote it empty on every element, on the strength
// of a measurement over two mappings a blank app ships (#882) — and a rewrite
// therefore DELETED it from every mapping that had one.
//
// The wider measurement says neither "always empty" nor "always copy the
// sample" is right. Across 3,042 value elements whose structure carries a
// sample, 2,322 (76%) store it and 720 do not — and the split is PER DOCUMENT,
// not per element:
//
//	145 mappings carry the sample on EVERY element
//	107 carry it on NONE
//	  2 are mixed
//
// So which one a mapping gets is a property of how and when it was authored,
// which mxcli cannot compute. Choosing a global default is wrong for roughly
// half the corpus either way.
//
// A REWRITE does not have to choose. It knows what was stored, so it carries it
// — guard-don't-drop, ADR-0005. That leaves #882's actual decision intact: a
// NEWLY created mapping still writes empty, which is what that issue was about.
package executor

import "github.com/mendixlabs/mxcli/model"

// carryImportOriginalValues copies each stored element's OriginalValue onto the
// rebuilt element at the same JsonPath.
//
// Matching is by JsonPath because that is what identifies an element against
// the schema: names can be renamed and order can change, but a value element
// bound to a different path is a different element.
func carryImportOriginalValues(rebuilt, stored *model.ImportMapping) {
	if rebuilt == nil || stored == nil {
		return
	}
	byPath := map[string]string{}
	collectImportOriginalValues(stored.Elements, byPath)
	if len(byPath) == 0 {
		return
	}
	applyImportOriginalValues(rebuilt.Elements, byPath)
}

func collectImportOriginalValues(elems []*model.ImportMappingElement, out map[string]string) {
	for _, e := range elems {
		if e == nil {
			continue
		}
		if e.OriginalValue != "" {
			out[e.JsonPath] = e.OriginalValue
		}
		collectImportOriginalValues(e.Children, out)
	}
}

func applyImportOriginalValues(elems []*model.ImportMappingElement, byPath map[string]string) {
	for _, e := range elems {
		if e == nil {
			continue
		}
		// Only fill an element the rebuild left empty: a statement that somehow
		// set one should win over what was stored.
		if e.OriginalValue == "" {
			if v, ok := byPath[e.JsonPath]; ok {
				e.OriginalValue = v
			}
		}
		applyImportOriginalValues(e.Children, byPath)
	}
}

// carryExportOriginalValues is the export twin. An export mapping's value
// elements hardcoded "" in the codec writer rather than carrying the field at
// all, so this needed the semantic type to reach the writer as well.
func carryExportOriginalValues(rebuilt, stored *model.ExportMapping) {
	if rebuilt == nil || stored == nil {
		return
	}
	byPath := map[string]string{}
	collectExportOriginalValues(stored.Elements, byPath)
	if len(byPath) == 0 {
		return
	}
	applyExportOriginalValues(rebuilt.Elements, byPath)
}

func collectExportOriginalValues(elems []*model.ExportMappingElement, out map[string]string) {
	for _, e := range elems {
		if e == nil {
			continue
		}
		if e.OriginalValue != "" {
			out[e.JsonPath] = e.OriginalValue
		}
		collectExportOriginalValues(e.Children, out)
	}
}

func applyExportOriginalValues(elems []*model.ExportMappingElement, byPath map[string]string) {
	for _, e := range elems {
		if e == nil {
			continue
		}
		if e.OriginalValue == "" {
			if v, ok := byPath[e.JsonPath]; ok {
				e.OriginalValue = v
			}
		}
		applyExportOriginalValues(e.Children, byPath)
	}
}
