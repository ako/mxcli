// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// externalModifyRule is the rule ID for "`modify attribute` on an external entity
// changes a mapped attribute away from the type the OData service publishes".
const externalModifyRule = "MDL112"

// externalEntitySourceType is the stored Source of an external (OData) entity.
const externalEntitySourceType = "Rest$ODataRemoteEntitySource"

// An external entity's mapped attribute keeps the type the service publishes.
// Measured on MxBuild 11.16.1 against a contract Product(Code Edm.String
// MaxLength 36 key, Label Edm.String MaxLength 100, Note Edm.String, Qty Edm.Int32),
// one project per case:
//
//	Label: string(100), Qty: integer, Qty: long              clean   (Integer and Long are one class)
//	Label: string(200) or string(unlimited), Note: string(50),
//	Code: string(50)                                          CE6621 "… has max length '200' which is not the
//	                                                                 same as max length in the OData service which is '100'"
//	Qty: decimal / boolean / datetime, Label: integer / enum  CE6616 "… is of type 'Decimal' which is not the same as
//	                                                                 property type in the OData service …"
//
// check and exec accepted every rejected case: `create or modify external entity`
// already refuses a type change (MDL-V1-REMOTETYPE) and `add attribute` an unmapped
// one (CE6612), but `modify attribute` was not judged.
//
// The published type is read from the contract stored on the project's consumed
// service. Not judged: an entity the script creates or rewrites, a service the script
// rewrites, a property the contract does not list, an Edm type outside the measured
// classes, and a key property with no MaxLength (the importer bounds it itself).
func ValidateExternalModifyAttribute(prog *ast.Program, open func() backend.FullBackend) []linter.Violation {
	if prog == nil || open == nil {
		return nil
	}
	var mods []*ast.AlterEntityStmt
	rewritten := map[string]bool{} // lower-cased entity QNs the script creates
	changedSvc := map[string]bool{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateEntityStmt:
			rewritten[strings.ToLower(s.Name.String())] = true
		case *ast.CreateODataClientStmt:
			changedSvc[strings.ToLower(s.Name.String())] = true
		case *ast.AlterODataClientStmt:
			changedSvc[strings.ToLower(s.Name.String())] = true
		case *ast.DropODataClientStmt:
			changedSvc[strings.ToLower(s.Name.String())] = true
		case *ast.AlterEntityStmt:
			if s.Operation == ast.AlterEntityModifyAttribute && !s.Calculated && s.AttributeName != "" {
				if !rewritten[strings.ToLower(s.Name.String())] {
					mods = append(mods, s)
				}
			}
		}
	}
	if len(mods) == 0 {
		return nil
	}
	b := open()
	lookup, ok := b.(entityLookupBackend)
	if b == nil || !ok {
		return nil
	}
	contracts := consumedServiceContracts(b)

	var out []linter.Violation
	for _, s := range mods {
		ent, found := findEntityByQN(lookup, s.Name.String())
		if !found || ent.Source != externalEntitySourceType || ent.RemoteServiceName == "" {
			continue
		}
		if changedSvc[strings.ToLower(ent.RemoteServiceName)] {
			continue
		}
		var stored *domainmodel.Attribute
		for _, a := range ent.Attributes {
			if a != nil && a.Name == s.AttributeName {
				stored = a
			}
		}
		if stored == nil || stored.RemoteName == "" {
			continue
		}
		doc := contracts[strings.ToLower(ent.RemoteServiceName)]
		if doc == nil {
			continue
		}
		et := doc.FindEntityType(ent.RemoteEntityName)
		if et == nil {
			continue
		}
		var prop *types.EdmProperty
		for _, p := range et.Properties {
			if p != nil && p.Name == stored.RemoteName {
				prop = p
			}
		}
		if prop == nil {
			continue
		}
		want := edmAttributeClass(prop.Type)
		got := astAttributeClass(s.DataType)
		if want == "" || got == "" {
			continue
		}
		loc := linter.Location{Module: s.Name.Module, DocumentType: "entity", DocumentName: s.Name.Name}
		if want != got {
			out = append(out, linter.Violation{
				RuleID: externalModifyRule, Severity: linter.SeverityError, Location: loc,
				Message: fmt.Sprintf("`alter entity %s modify attribute %s` makes it %s, but %s publishes %s as %s — "+
					"mxbuild reports CE6616 \"… is not the same as property type in the OData service\"",
					s.Name, s.AttributeName, got, ent.RemoteServiceName, prop.Name, prop.Type),
				Suggestion: fmt.Sprintf("An external entity's attribute types come from the service. Keep %s a %s attribute, "+
					"or re-import with `create or modify external entities from %s` after the service changes.",
					s.AttributeName, want, ent.RemoteServiceName),
			})
			continue
		}
		if want != "String" || prop.Type != "Edm.String" {
			continue
		}
		isKey := false
		for _, k := range et.KeyProperties {
			if k == prop.Name {
				isKey = true
			}
		}
		published, known := edmStringLength(prop.MaxLength)
		if !known || (isKey && prop.MaxLength == "") {
			continue
		}
		if s.DataType.Length != published {
			out = append(out, linter.Violation{
				RuleID: externalModifyRule, Severity: linter.SeverityError, Location: loc,
				Message: fmt.Sprintf("`alter entity %s modify attribute %s` gives it length %s, but %s publishes %s with "+
					"max length %s — mxbuild reports CE6621 \"… is not the same as max length in the OData service\"",
					s.Name, s.AttributeName, lengthText(s.DataType.Length), ent.RemoteServiceName, prop.Name, lengthText(published)),
				Suggestion: fmt.Sprintf("Use string(%s), the length the service publishes.", lengthText(published)),
			})
		}
	}
	return out
}

// consumedServiceContracts parses the contract stored on each project consumed
// service, keyed by lower-cased qualified name.
func consumedServiceContracts(b backend.FullBackend) map[string]*types.EdmxDocument {
	out := map[string]*types.EdmxDocument{}
	modules, err := b.ListModules()
	if err != nil {
		return out
	}
	names := map[string]string{}
	for _, m := range modules {
		if m != nil {
			names[string(m.ID)] = m.Name
		}
	}
	services, err := b.ListConsumedODataServices()
	if err != nil {
		return out
	}
	for _, svc := range services {
		if svc == nil || svc.Metadata == "" || names[string(svc.ContainerID)] == "" {
			continue
		}
		if doc, err := types.ParseEdmx(svc.Metadata); err == nil && doc != nil {
			out[strings.ToLower(names[string(svc.ContainerID)]+"."+svc.Name)] = doc
		}
	}
	return out
}

// edmAttributeClass is the attribute-type class an Edm primitive maps to, as
// MxBuild compares them ("Integer/Long" is one class). "" for a type not measured.
func edmAttributeClass(edm string) string {
	switch edm {
	case "Edm.String":
		return "String"
	case "Edm.Int16", "Edm.Int32", "Edm.Int64", "Edm.Byte", "Edm.SByte":
		return "Integer/Long"
	case "Edm.Decimal", "Edm.Double", "Edm.Single":
		return "Decimal"
	case "Edm.Boolean":
		return "Boolean"
	case "Edm.DateTime", "Edm.DateTimeOffset", "Edm.Date":
		return "DateTime"
	}
	return ""
}

// astAttributeClass is the class of a type written in the script.
func astAttributeClass(dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeString:
		return "String"
	case ast.TypeInteger, ast.TypeLong:
		return "Integer/Long"
	case ast.TypeDecimal:
		return "Decimal"
	case ast.TypeBoolean:
		return "Boolean"
	case ast.TypeDateTime, ast.TypeDate:
		return "DateTime"
	case ast.TypeEnumeration:
		return "Enumeration"
	case ast.TypeBinary:
		return "Binary"
	}
	return ""
}

// edmStringLength is the attribute length an Edm.String MaxLength means: a number,
// or 0 (unlimited) for none or "max".
func edmStringLength(maxLength string) (int, bool) {
	if maxLength == "" || strings.EqualFold(maxLength, "max") {
		return 0, true
	}
	n, err := strconv.Atoi(maxLength)
	if err != nil {
		return 0, false
	}
	return n, true
}

func lengthText(n int) string {
	if n <= 0 {
		return "unlimited"
	}
	return strconv.Itoa(n)
}
