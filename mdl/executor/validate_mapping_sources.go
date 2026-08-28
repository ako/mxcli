// SPDX-License-Identifier: Apache-2.0

// Reference validation for a mapping's schema source — the `with json structure
// …` / `with xml schema …` clause.
//
// Nothing resolved it: the name was written through to BSON verbatim, `mxcli
// check` passed, `mxcli check --references` passed, and the first sign of a typo
// was MxBuild:
//
//	[error] [CE1613] "The selected XML schema 'XGap.NoSuchThing' no longer
//	        exists." at Import mapping 'XGap.IMM_Xml'
//
// `exec` refuses these at the point it builds the document (resolveJsonStructure-
// Source / resolveXmlSchemaSource). This is the same question asked one tier
// earlier, so that `check -p` answers it too — which is the tier an author or an
// agent is actually looking at while the statement is still editable
// (ako/mxcli#259).
package executor

import (
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// validateMappingSources resolves every mapping's schema source against the
// project, ignoring sources the script itself creates.
//
// It runs in the --references pass because the answer lives in the project. A
// project whose list comes back empty disables the corresponding half rather
// than reporting every mapping as broken — for XML schemas that is the ORDINARY
// case, since MDL cannot create one and none of the nine demo apps in the corpus
// contains any.
func validateMappingSources(ctx *ExecContext, prog *ast.Program) []error {
	refs := mappingSourceRefs(prog)
	if len(refs) == 0 {
		return nil
	}

	jsonNames := scriptJsonStructures(prog)
	for _, name := range projectJsonStructures(ctx) {
		jsonNames[name] = true
	}
	// An XML schema cannot be created from MDL, so the script contributes none.
	xmlNames := projectXmlSchemas(ctx)

	return mappingSourceViolations(refs, jsonNames, xmlNames)
}

// mappingSourceViolations is the decision, separated from the project lookups so
// it can be tested without one.
//
// A source family whose set is EMPTY disables itself. That is not defensiveness:
// for XML schemas an empty set is the ordinary case (MDL cannot create one, and
// none of the nine demo apps has any), and for JSON structures an empty set
// means the project could not be read — in both, refusing would report every
// mapping as broken on the strength of having learned nothing.
func mappingSourceViolations(refs []mappingSourceRef, jsonNames, xmlNames map[string]bool) []error {
	var errs []error
	for _, ref := range refs {
		var known map[string]bool
		var label string
		switch ref.kind {
		case "JSON_STRUCTURE":
			known, label = jsonNames, "json structure"
		case "XML_SCHEMA":
			known, label = xmlNames, "xml schema"
		default:
			// A message definition resolves against a collection document and
			// has its own resolver in the create path.
			continue
		}
		if len(known) == 0 || known[ref.name] {
			continue
		}
		errs = append(errs, mdlerrors.NewValidationf("%s %s: %s %q does not exist; available: %s",
			ref.docKind, ref.doc, label, ref.name, strings.Join(sortedNames(known), ", ")))
	}
	return errs
}

// mappingSourceRef is one `with …` clause to resolve.
type mappingSourceRef struct {
	docKind string // "import mapping" / "export mapping"
	doc     string // the mapping's qualified name
	kind    string // SchemaKind
	name    string // the referenced document's qualified name
}

func mappingSourceRefs(prog *ast.Program) []mappingSourceRef {
	var out []mappingSourceRef
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateImportMappingStmt:
			if s.SchemaRef.Module != "" {
				out = append(out, mappingSourceRef{"import mapping", s.Name.String(), s.SchemaKind, s.SchemaRef.String()})
			}
		case *ast.CreateExportMappingStmt:
			if s.SchemaRef.Module != "" {
				out = append(out, mappingSourceRef{"export mapping", s.Name.String(), s.SchemaKind, s.SchemaRef.String()})
			}
		}
	}
	return out
}

// scriptJsonStructures collects the structures the script creates, which a
// mapping later in the same script may legitimately name before they exist in
// the project. This is the overwhelmingly common shape — create the structure,
// then the mapping over it — so getting it wrong would flag almost every script.
func scriptJsonStructures(prog *ast.Program) map[string]bool {
	out := map[string]bool{}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateJsonStructureStmt); ok && s.Name.Module != "" {
			out[s.Name.String()] = true
		}
	}
	return out
}

func projectJsonStructures(ctx *ExecContext) []string {
	all, err := ctx.Backend.ListJsonStructures()
	if err != nil {
		return nil
	}
	h, herr := getHierarchy(ctx)
	out := make([]string, 0, len(all))
	for _, js := range all {
		if js == nil {
			continue
		}
		module := ""
		if herr == nil {
			module = h.GetModuleName(h.FindModuleID(js.ContainerID))
		}
		if module == "" {
			// Without a module the name cannot be compared to a qualified
			// reference; contributing a bare name would make every reference
			// look unknown.
			continue
		}
		out = append(out, module+"."+js.Name)
	}
	return out
}

func projectXmlSchemas(ctx *ExecContext) map[string]bool {
	all, err := ctx.Backend.ListXmlSchemas()
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, xs := range all {
		if xs == nil || xs.Module == "" {
			continue
		}
		out[xs.Module+"."+xs.Name] = true
	}
	return out
}

func sortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
