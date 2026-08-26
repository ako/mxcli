// SPDX-License-Identifier: Apache-2.0

// Package executor — validation rule commands (CREATE VALIDATION RULE).
//
// A Mendix validation rule is anonymous and lives on the ENTITY, keyed by the
// attribute it constrains, so the statement names its target rather than the
// rule. Only the two constraints nothing else can express are handled here:
// RegEx and Range. Required and Unique are already authorable as attribute
// constraints (`not null error '…'` / `unique error '…'`) on CREATE ENTITY and
// ALTER ENTITY, and are refused here with a pointer at that syntax rather than
// given a second, drift-prone spelling.
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// execCreateValidationRule handles CREATE VALIDATION RULE FOR Module.Entity.Attribute.
func execCreateValidationRule(ctx *ExecContext, s *ast.CreateValidationRuleStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	moduleName, entityName, attrName, err := splitAttributeQN(s.Attribute)
	if err != nil {
		return err
	}
	if s.Feedback == "" {
		return mdlerrors.NewValidation("validation rule: FEEDBACK message is required")
	}

	module, err := findModule(ctx, moduleName)
	if err != nil {
		return err
	}
	dm, err := ctx.Backend.GetDomainModel(module.ID)
	if err != nil {
		return mdlerrors.NewBackend("get domain model", err)
	}

	var entity *domainmodel.Entity
	for _, ent := range dm.Entities {
		if strings.EqualFold(ent.Name, entityName) {
			entity = ent
			break
		}
	}
	if entity == nil {
		return mdlerrors.NewNotFound("entity", moduleName+"."+entityName)
	}

	var attr *domainmodel.Attribute
	for _, a := range entity.Attributes {
		if strings.EqualFold(a.Name, attrName) {
			attr = a
			break
		}
	}
	if attr == nil {
		return mdlerrors.NewNotFoundMsg("attribute", attrName,
			fmt.Sprintf("attribute '%s' not found on entity %s.%s", attrName, moduleName, entityName))
	}

	info, ruleType, err := validationRuleInfoFor(ctx, s)
	if err != nil {
		return err
	}

	attrQN := fmt.Sprintf("%s.%s.%s", moduleName, entity.Name, attr.Name)
	setAttributeValidationRuleInfo(entity, attr, attrQN, ruleType, s.Feedback, info, authoringLanguage(ctx))

	if err := ctx.Backend.UpdateEntity(dm.ID, entity); err != nil {
		return mdlerrors.NewBackend("create validation rule", err)
	}
	invalidateHierarchy(ctx)
	invalidateDomainModelsCache(ctx)

	fmt.Fprintf(ctx.Output, "Created %s validation rule on %s\n", ruleType, attrQN)
	return nil
}

// validationRuleInfoFor builds the rule payload for a statement, resolving and
// checking anything it references.
func validationRuleInfoFor(ctx *ExecContext, s *ast.CreateValidationRuleStmt) (domainmodel.ValidationRuleInfo, string, error) {
	switch s.Kind {
	case ast.ValidationRuleRegEx:
		qn := s.RegularExpression
		if qn.Module == "" || qn.Name == "" {
			return nil, "", mdlerrors.NewValidation(
				"validation rule: REGEX takes the qualified name of a regular expression document, e.g. REGEX MyModule.EmailPattern")
		}
		// The reference is stored by name, so a typo would produce a document
		// that loads and validates nothing. Fail here instead.
		if findRegularExpression(ctx, qn.Module, qn.Name) == nil {
			return nil, "", mdlerrors.NewNotFoundMsg("regular expression", qn.String(),
				fmt.Sprintf("regular expression %s not found — create it first with CREATE REGULAR EXPRESSION", qn.String()))
		}
		info := &domainmodel.RegexValidationRuleInfo{RegularExpressionQualifiedName: qn.String()}
		info.ID = model.ID(types.GenerateID())
		return info, "RegEx", nil

	case ast.ValidationRuleRange:
		if s.Min == nil && s.Max == nil {
			return nil, "", mdlerrors.NewValidation("validation rule: RANGE needs at least one of FROM or TO")
		}
		info := &domainmodel.RangeValidationRuleInfo{
			MinValue:    s.Min,
			MaxValue:    s.Max,
			UseMinValue: s.Min != nil,
			UseMaxValue: s.Max != nil,
		}
		info.ID = model.ID(types.GenerateID())
		return info, "Range", nil

	default:
		return nil, "", mdlerrors.NewValidation("validation rule: expected REGEX or RANGE")
	}
}

// outputEntityValidationRules emits CREATE VALIDATION RULE statements for the
// rules DESCRIBE ENTITY cannot render as attribute constraints.
//
// Required and Unique appear inline on the attribute (`not null error '…'`), so
// only RegEx and Range are written here. A rule whose payload did not survive
// the read is reported as a comment rather than skipped: silence would read as
// "this entity has no such rule", which is the failure this whole area is about.
func outputEntityValidationRules(ctx *ExecContext, entity *domainmodel.Entity, moduleName, entityName string, attrNames map[model.ID]string) {
	for _, vr := range entity.ValidationRules {
		if vr == nil || (vr.Type != "RegEx" && vr.Type != "Range") {
			continue
		}

		attrName := attrNames[vr.AttributeID]
		if attrName == "" {
			attrName = extractAttrNameFromQualified(string(vr.AttributeID))
		}
		if attrName == "" {
			fmt.Fprintf(ctx.Output, "-- %s validation rule on an unresolved attribute (%s) — not rendered\n",
				vr.Type, vr.AttributeID)
			continue
		}
		target := fmt.Sprintf("%s.%s.%s", moduleName, entityName, attrName)

		constraint, ok := describeValidationConstraint(vr)
		if !ok {
			fmt.Fprintf(ctx.Output, "-- %s validation rule on %s — not expressible in MDL, left unchanged\n",
				vr.Type, target)
			continue
		}

		feedback := ""
		if vr.ErrorMessage != nil {
			feedback = vr.ErrorMessage.GetTranslation("en_US")
		}
		fmt.Fprintf(ctx.Output, "\ncreate validation rule for %s\n    %s\n    feedback '%s';\n",
			target, constraint, escapeMDLString(feedback))
	}
}

// describeValidationConstraint renders the constraint clause, or false when the
// rule has a shape MDL cannot author (an attribute-bounded range, say).
func describeValidationConstraint(vr *domainmodel.ValidationRule) (string, bool) {
	switch info := vr.Rule.(type) {
	case *domainmodel.RegexValidationRuleInfo:
		if info.RegularExpressionQualifiedName == "" {
			return "", false
		}
		return "regex " + info.RegularExpressionQualifiedName, true

	case *domainmodel.RangeValidationRuleInfo:
		// MDL has no syntax for a bound that points at another attribute.
		if info.MinAttributeQualifiedName != "" || info.MaxAttributeQualifiedName != "" {
			return "", false
		}
		switch {
		case info.MinValue != nil && info.MaxValue != nil:
			return fmt.Sprintf("range from %s to %s", *info.MinValue, *info.MaxValue), true
		case info.MinValue != nil:
			return fmt.Sprintf("range from %s", *info.MinValue), true
		case info.MaxValue != nil:
			return fmt.Sprintf("range to %s", *info.MaxValue), true
		}
		return "", false

	default:
		return "", false
	}
}

// escapeMDLString doubles single quotes, matching Mendix expression quoting.
func escapeMDLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// splitAttributeQN splits Module.Entity.Attribute.
//
// ast.QualifiedName keeps only a module and a name, so the entity segment
// arrives inside Name ("Entity.Attribute") — the statement needs all three.
func splitAttributeQN(qn ast.QualifiedName) (module, entity, attribute string, err error) {
	parts := strings.Split(qn.String(), ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", mdlerrors.NewValidationf(
			"validation rule: expected a fully qualified attribute Module.Entity.Attribute, got %q", qn.String())
	}
	return parts[0], parts[1], parts[2], nil
}

// setAttributeValidationRuleInfo replaces any rule of the same type on the same
// attribute and appends the new one.
//
// Same add/replace semantics as setAttributeValidationRule (which handles the
// payload-free Required/Unique rules), extended to carry a rule payload. An
// attribute may hold several rules of DIFFERENT types at once — a Required and
// a RegEx, say — so only the matching type is displaced.
func setAttributeValidationRuleInfo(entity *domainmodel.Entity, attr *domainmodel.Attribute, attrQualifiedName, ruleType, feedback string, info domainmodel.ValidationRuleInfo, lang string) {
	kept := entity.ValidationRules[:0]
	for _, vr := range entity.ValidationRules {
		if vr != nil && vr.Type == ruleType && ruleTargetsAttribute(string(vr.AttributeID), attr) {
			continue
		}
		kept = append(kept, vr)
	}
	entity.ValidationRules = kept

	vr := &domainmodel.ValidationRule{
		AttributeID: model.ID(attrQualifiedName),
		Type:        ruleType,
		Rule:        info,
	}
	vr.ID = model.ID(types.GenerateID())
	vr.ErrorMessage = &model.Text{Translations: map[string]string{lang: feedback}}
	vr.ErrorMessage.ID = model.ID(types.GenerateID())
	entity.ValidationRules = append(entity.ValidationRules, vr)
}
