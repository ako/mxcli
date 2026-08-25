// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// Custom object handling (#264): a microflow resolves a mapping element's object
// instead of Create/Find. 56 of the 327 mapping documents in the demo apps use
// it — the third-largest gap, and the only one with a sub-language of its own.
//
// MDL spells it as a modifier on `find`, which is what it means:
//
//	find AgentCommons.Tool
//	     by AgentCommons.Version_GetMicroflowTools ( Version: parent )
//	     = Tools { ... }
//
// The four parameter sources, and how Studio Pro stores each (measured across
// 161 parameter mappings in the demo apps):
//
//	MDL             JsonValueElementPath / XmlValueElementPath   LevelOfParent
//	Param: parent        "(parent)"      "(parent)"                  -1
//	Param: parameter     "(parameter)"   "(parameter)"               -1
//	Param: parent(2)     ""              ""                           2
//	Param: a/b/c         the value path  ""                          -1
//
// The marker sources write the same marker into BOTH path properties; only an
// explicit value path is JSON-only.

// customHandlerParamSources is the set the executor accepts, named so a typo
// gets a list instead of silence.
var customHandlerParamSources = []string{"parent", "parameter", "parent(N)", "a/b/c (a member path)"}

// buildCustomHandler converts the `by ...` clause to the semantic model,
// resolving the microflow and each parameter.
//
// elementPath is the element's own JsonPath: an explicit value path is resolved
// RELATIVE to it, because the value a custom handler keys on lives inside the
// object being resolved. An absolute path is not authorable — no demo mapping
// needs one, and accepting a raw path would let a typo through unchecked (#259).
func buildCustomHandler(def *ast.MappingCustomHandlerDef, moduleName, elementPath string,
	b backend.FullBackend,
) (*model.MappingMicroflowCall, error) {
	if def == nil {
		return nil, nil
	}
	microflow := def.Microflow
	if microflow == "" {
		return nil, fmt.Errorf("`by` needs a microflow to call")
	}
	if !strings.Contains(microflow, ".") {
		microflow = moduleName + "." + microflow
	}
	if err := requireMicroflow(microflow, b); err != nil {
		return nil, err
	}

	call := &model.MappingMicroflowCall{Microflow: microflow}
	for _, p := range def.Parameters {
		mp := &model.MappingMicroflowParameter{
			// A parameter is referenced against the microflow that declares it.
			Parameter:     microflow + "." + p.Parameter,
			Source:        p.Source,
			LevelOfParent: -1,
		}
		switch p.Source {
		case "parent", "parameter":
		case "ancestor":
			if p.Level < 1 {
				return nil, fmt.Errorf("parameter %q: parent(N) needs a level of 1 or more", p.Parameter)
			}
			mp.LevelOfParent = p.Level
		case "path":
			if elementPath == "" {
				return nil, fmt.Errorf("parameter %q: a member path needs the element to be bound "+
					"to a schema first", p.Parameter)
			}
			mp.ValuePath = elementPath + "|" + strings.ReplaceAll(p.Path, "/", "|")
		default:
			return nil, fmt.Errorf("parameter %q: unknown source %q; expected one of: %s",
				p.Parameter, p.Source, strings.Join(customHandlerParamSources, ", "))
		}
		call.Parameters = append(call.Parameters, mp)
	}
	return call, nil
}

// requireMicroflow refuses an unresolvable microflow rather than writing the
// name through — mxbuild reports one as CE1613 and the handler is silently gone.
func requireMicroflow(qualified string, b backend.FullBackend) error {
	parts := strings.SplitN(qualified, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("microflow %q is not a qualified name", qualified)
	}
	mfs, err := b.ListMicroflows()
	if err != nil {
		// Nothing to check against — take the name at face value rather than
		// refusing something that may well be right.
		return nil
	}
	for _, mf := range mfs {
		if strings.EqualFold(mf.Name, parts[1]) {
			return nil
		}
	}
	return fmt.Errorf("microflow %q not found", qualified)
}

// customHandlerParamText renders one parameter for DESCRIBE.
func customHandlerParamText(p *model.MappingMicroflowParameter, elementPath string) string {
	name := p.Parameter
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	switch p.Source {
	case "parameter":
		return name + ": parameter"
	case "ancestor":
		return fmt.Sprintf("%s: parent(%d)", name, p.LevelOfParent)
	case "path":
		rel := p.ValuePath
		if elementPath != "" {
			rel = strings.TrimPrefix(rel, elementPath+"|")
		}
		return name + ": " + strings.ReplaceAll(rel, "|", "/")
	default:
		return name + ": parent"
	}
}

// customHandlerText renders the whole `by ...` clause for DESCRIBE, or "" when
// the element has no custom handler.
func customHandlerText(call *model.MappingMicroflowCall, elementPath string) string {
	if call == nil || call.Microflow == "" {
		return ""
	}
	parts := make([]string, 0, len(call.Parameters))
	for _, p := range call.Parameters {
		parts = append(parts, customHandlerParamText(p, elementPath))
	}
	return fmt.Sprintf(" by %s(%s)", call.Microflow, strings.Join(parts, ", "))
}

// sourceFromStored recovers a parameter's source from the stored path marker and
// level — the document distinguishes the four shapes that way rather than with a
// kind field.
func sourceFromStored(jsonPath string, level int) string {
	switch {
	case jsonPath == "(parent)":
		return "parent"
	case jsonPath == "(parameter)":
		return "parameter"
	case level >= 1:
		return "ancestor"
	case jsonPath != "" && jsonPath != "-":
		return "path"
	default:
		// "-" and "" with level -1: an ancestor reference whose level did not
		// survive, or a shape this reader does not model. Treat as parent, the
		// only source that needs no extra information.
		return "parent"
	}
}

// customHandlerValueMembers returns the member paths (relative to the element)
// that a custom handler keys on, i.e. its value-path parameters.
func customHandlerValueMembers(def *ast.MappingCustomHandlerDef) []string {
	if def == nil {
		return nil
	}
	var out []string
	for _, p := range def.Parameters {
		if p.Source == "path" && p.Path != "" {
			out = append(out, p.Path)
		}
	}
	return out
}

// addCustomHandlerValueElements appends the attribute-less value elements a
// value-path parameter needs.
//
// Mendix requires the value the handler keys on to EXIST as a value mapping
// element under the object being resolved — it is how the runtime knows where to
// read the argument from — and rejects a parameter pointing at an unmapped
// member with CE0281 "Unknown value element 'x' selected for parameter 'y' of
// object handling microflow". Studio Pro adds the element when you pick the
// value; MDL has no syntax for an attribute-less value element, so it is added
// here. Measured on GenAICommons.Chunk_FindByIndex in
// MxGenAIConnector.IM_CohereEmbed_Response.
//
// An element already mapping that member is left alone: the parameter may key on
// a value the author also maps to an attribute.
func addCustomHandlerValueElements(elem *model.ImportMappingElement,
	def *ast.MappingCustomHandlerDef, idx *jsonSchemaIndex, childPath string,
) error {
	for _, member := range customHandlerValueMembers(def) {
		want := childPath + "|" + strings.ReplaceAll(member, "/", "|")
		mapped := false
		for _, c := range elem.Children {
			if c.JsonPath == want {
				mapped = true
				break
			}
		}
		if mapped {
			continue
		}
		ve := &model.ImportMappingElement{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "ImportMappings$ValueMappingElement",
			},
			Kind:           "Value",
			JsonPath:       want,
			Nillable:       true,
			MaxOccurs:      1,
			FractionDigits: -1,
			TotalDigits:    -1,
			MaxLength:      -1,
			DataType:       "String",
		}
		if js, _ := idx.resolvePath(childPath, member); js != nil {
			ve.ExposedName = js.ExposedName
			ve.JsonPath = js.Path
			ve.MinOccurs = js.MinOccurs
			ve.MaxOccurs = js.MaxOccurs
			ve.Nillable = js.Nillable
			// The type has to MIRROR the schema element: Mendix cross-validates
			// the two and reports CE5015 ("Attribute type 'String' does not match
			// schema type 'Integer'") on a mismatch. There is no attribute to
			// derive it from here — the element exists only to feed the handler.
			if js.PrimitiveType != "" {
				ve.DataType = js.PrimitiveType
			}
		} else if idx.resolvable() {
			return fmt.Errorf("parameter value %q is not a member of the JSON structure at %s",
				member, childPath)
		} else {
			ve.ExposedName = member
		}
		elem.Children = append(elem.Children, ve)
	}
	return nil
}

// resolveSchemaRoot finds the element a `root a/b/c` clause selects (#267).
//
// Unlike a member reference, this path MAY pass through an array: the mapping is
// then rooted at the array's item, which is what Studio Pro stores —
// `root choices/message` over an array-valued `choices` reaches
// "(Object)|choices|(Object)|message" (OpenAI_API.IM_OpenAI). A value reference
// cannot do that (many items cannot collapse into one value, CE0256), which is
// why this walks the index itself rather than reusing resolvePath.
func resolveSchemaRoot(idx *jsonSchemaIndex, path string) (*types.JsonElement, error) {
	root := idx.root()
	if root == nil {
		return nil, fmt.Errorf("the schema source has no root to start from")
	}
	current := root
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		// Step into an array's item before looking for the next member: the
		// item is where the members live.
		if current.ElementType == "Array" {
			if item, ok := idx.byPath[current.Path+"|(Object)"]; ok {
				current = item
			}
		}
		next := idx.resolve(current.Path, seg)
		if next == nil {
			known := idx.memberNames(current.Path)
			if len(known) == 0 {
				return nil, fmt.Errorf("root %q: %q is not a member of the schema at %s, "+
					"which has no members there", path, seg, current.Path)
			}
			return nil, fmt.Errorf("root %q: %q is not a member of the schema at %s; available: %s",
				path, seg, current.Path, strings.Join(known, ", "))
		}
		current = next
	}
	// A root landing on an array means the mapping is rooted at its ITEM, the
	// same collapse an array-rooted structure gets (#248).
	if current.ElementType == "Array" {
		if item, ok := idx.byPath[current.Path+"|(Object)"]; ok {
			current = item
		}
	}
	return current, nil
}

// schemaRootClause renders the `root a/b/c` clause for DESCRIBE, or "" when the
// mapping starts at the structure's own root.
//
// Derived from the stored path rather than remembered: the "(Object)" markers an
// array contributes are dropped, which is the inverse of how the clause resolves.
func schemaRootClause(rootPath string) string {
	if rootPath == "" || rootPath == "(Object)" || rootPath == "(Array)" ||
		rootPath == "(Array)|(Object)" {
		return ""
	}
	segs := strings.Split(rootPath, "|")
	var out []string
	for i, seg := range segs {
		if i == 0 || seg == "(Object)" || seg == "(Array)" {
			continue
		}
		out = append(out, seg)
	}
	if len(out) == 0 {
		return ""
	}
	return " root " + strings.Join(out, "/")
}
