// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// publishedBinaryRule is the rule ID for "a published OData service exposes a
// Binary attribute".
const publishedBinaryRule = "MDL110"

// A Binary attribute cannot be exposed by a published OData service. MxBuild
// 11.16.1 reports CE5013 "Binary attribute 'Photo' cannot be exposed through an
// OData entity." for the entity's own attribute and for one it inherits; check and
// exec accepted both. Every other attribute type (String, HashedString, Integer,
// Long, Decimal, Boolean, DateTime, AutoNumber, Enumeration) exposes clean.
//
// The attribute's type comes from the script, then the project, walking the
// generalization chain for an inherited one. A member whose type cannot be found
// (an association, an entity neither declares) is not judged.
func ValidatePublishedBinary(prog *ast.Program, open func() backend.FullBackend) []linter.Violation {
	if prog == nil {
		return nil
	}
	type scriptEntity struct {
		gen    string
		binary map[string]bool // lower-cased attribute names
		attrs  map[string]bool
	}
	script := map[string]*scriptEntity{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateEntityStmt:
			e := &scriptEntity{binary: map[string]bool{}, attrs: map[string]bool{}}
			if s.Generalization != nil {
				e.gen = qualifyIn(s.Name.Module, s.Generalization.String())
			}
			for _, a := range s.Attributes {
				e.attrs[strings.ToLower(a.Name)] = true
				if a.Type.Kind == ast.TypeBinary {
					e.binary[strings.ToLower(a.Name)] = true
				}
			}
			script[strings.ToLower(s.Name.String())] = e
		case *ast.AlterEntityStmt:
			if e := script[strings.ToLower(s.Name.String())]; e != nil && s.Operation == ast.AlterEntityAddAttribute && s.Attribute != nil {
				e.attrs[strings.ToLower(s.Attribute.Name)] = true
				if s.Attribute.Type.Kind == ast.TypeBinary {
					e.binary[strings.ToLower(s.Attribute.Name)] = true
				}
			}
		}
	}

	var lookup entityLookupBackend
	opened := false
	project := func() entityLookupBackend {
		if !opened {
			opened = true
			if open != nil {
				if b := open(); b != nil {
					lookup, _ = b.(entityLookupBackend)
				}
			}
		}
		return lookup
	}
	// isBinary answers whether entity (or a generalization) declares attr as
	// Binary, and whether the answer is known.
	isBinary := func(entity, attr string) (bool, bool) {
		seen := map[string]bool{}
		for cur := entity; cur != ""; {
			key := strings.ToLower(cur)
			if seen[key] {
				return false, false
			}
			seen[key] = true
			if e := script[key]; e != nil {
				if e.attrs[attr] {
					return e.binary[attr], true
				}
				cur = e.gen
				continue
			}
			b := project()
			if b == nil {
				return false, false
			}
			ent, ok := findEntityByQN(b, cur)
			if !ok {
				return false, false
			}
			for _, a := range ent.Attributes {
				if a != nil && strings.EqualFold(a.Name, attr) {
					_, bin := a.Type.(*domainmodel.BinaryAttributeType)
					return bin, true
				}
			}
			cur = ent.GeneralizationRef
		}
		return false, false
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		svc, ok := stmt.(*ast.CreateODataServiceStmt)
		if !ok {
			continue
		}
		for _, pe := range svc.Entities {
			if pe == nil {
				continue
			}
			entity := qualifyIn(svc.Name.Module, pe.Entity.String())
			for _, m := range pe.Members {
				if m == nil || m.Name == "" {
					continue
				}
				if bin, known := isBinary(entity, strings.ToLower(bareMemberName(m.Name))); !known || !bin {
					continue
				}
				out = append(out, linter.Violation{
					RuleID:   publishedBinaryRule,
					Severity: linter.SeverityError,
					Message: fmt.Sprintf("published odata service %s exposes %s, a Binary attribute of %s — mxbuild reports "+
						"CE5013 \"Binary attribute '%s' cannot be exposed through an OData entity\"",
						svc.Name, m.Name, entity, m.Name),
					Location: linter.Location{Module: svc.Name.Module, DocumentType: "published odata service", DocumentName: svc.Name.Name},
					Suggestion: "Leave the Binary attribute out of `expose (…)`. To serve file contents, publish a " +
						"REST operation that returns them, or expose a URL or an ID the client can download from.",
				})
			}
		}
	}
	return out
}
