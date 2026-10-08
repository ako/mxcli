// SPDX-License-Identifier: Apache-2.0

package rules

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ImportRangeObjectMappingRule (MDL-MAP04) is the model-level half of check's
// MDL-MAP04 (mendixlabs/mxcli#1319).
//
// An import over an OBJECT-rooted mapping that stores Studio Pro's First —
// ImportMappingCall.ForceSingleOccurrence = true, or a ConstantRange with
// SingleObject = true — is a valid model: mx check reports 0 errors and the app
// builds. The activity throws the moment it runs:
//
//	key not found: Path(QName(None,),None,)
//
// a message naming no microflow, mapping or element. check refuses the
// statement in a script, but an activity already in the model — written by
// v0.24.0 or earlier, by `exec --no-check`, or by hand — was found by nothing
// before it ran.
//
// The flags alone do not decide it: a LIST-rooted mapping stores the same pair
// for Studio Pro's legitimate First. The rule keys on the mapping's root shape,
// decided exactly as the builder and the check-time rule decide it
// (types.JsonMappingRootIsList), and a shape that cannot be established — an
// XML or message-definition mapping, a missing mapping — is left alone.
//
// It reuses check's ID on purpose: the same code for the same defect, whether
// it is found in the script or already in the model.
type ImportRangeObjectMappingRule struct{}

func NewImportRangeObjectMappingRule() *ImportRangeObjectMappingRule {
	return &ImportRangeObjectMappingRule{}
}

func (r *ImportRangeObjectMappingRule) ID() string                       { return "MDL-MAP04" }
func (r *ImportRangeObjectMappingRule) Name() string                     { return "ImportRangeOnObjectMapping" }
func (r *ImportRangeObjectMappingRule) Category() string                 { return "correctness" }
func (r *ImportRangeObjectMappingRule) DefaultSeverity() linter.Severity { return linter.SeverityError }

func (r *ImportRangeObjectMappingRule) Description() string {
	return "Import activity stores a First range over an object-rooted mapping (throws at runtime)"
}

// mappingReader is the part of the backend the rule needs beyond LintReader.
// It is asserted rather than added to LintReader so the readers that do not
// carry mappings (test doubles, a catalog-only run) simply skip the rule.
type mappingReader interface {
	types.JsonStructureLookup
	GetImportMappingByQualifiedName(moduleName, name string) (*model.ImportMapping, error)
}

// mappingShapeFunc answers "is this mapping list-rooted", with known=false when
// the shape cannot be established.
type mappingShapeFunc func(qualifiedName string) (list, known bool)

func (r *ImportRangeObjectMappingRule) Check(ctx *linter.LintContext) []linter.Violation {
	mr, ok := ctx.Reader().(mappingReader)
	if !ok || mr == nil {
		return nil
	}
	shape := cachedMappingShape(mr)

	var violations []linter.Violation
	for mf := range ctx.Microflows() {
		if ctx.IsExcluded(mf.ModuleName) {
			continue
		}
		full, err := ctx.FullMicroflow(model.ID(mf.ID))
		if err != nil || full == nil || full.ObjectCollection == nil {
			continue
		}
		findFirstOnObjectMappings(full.ObjectCollection.Objects, mf, shape, r, &violations)
	}
	return violations
}

// cachedMappingShape resolves each mapping once per lint run.
func cachedMappingShape(mr mappingReader) mappingShapeFunc {
	type answer struct{ list, known bool }
	cache := map[string]answer{}
	return func(qn string) (bool, bool) {
		if a, ok := cache[qn]; ok {
			return a.list, a.known
		}
		var a answer
		if mod, name, ok := strings.Cut(qn, "."); ok {
			if im, err := mr.GetImportMappingByQualifiedName(mod, name); err == nil && im != nil {
				a.list, a.known = types.JsonMappingRootIsList(mr, im)
			}
		}
		cache[qn] = a
		return a.list, a.known
	}
}

func findFirstOnObjectMappings(objects []microflows.MicroflowObject, mf linter.Microflow, shape mappingShapeFunc, r *ImportRangeObjectMappingRule, violations *[]linter.Violation) {
	for _, obj := range objects {
		switch act := obj.(type) {
		case *microflows.ActionActivity:
			h, kind := mappingResultHandling(act.Action)
			if h == nil || !storesFirst(h) {
				continue
			}
			mapping := string(h.MappingID)
			if list, known := shape(mapping); !known || list {
				continue
			}
			*violations = append(*violations, r.violation(mf, kind, mapping, h))
		case *microflows.LoopedActivity:
			if act.ObjectCollection != nil {
				findFirstOnObjectMappings(act.ObjectCollection.Objects, mf, shape, r, violations)
			}
		}
	}
}

// mappingResultHandling returns the ImportMappingCall an action carries, and
// what to call the activity in a message.
func mappingResultHandling(a microflows.MicroflowAction) (*microflows.ResultHandlingMapping, string) {
	switch a := a.(type) {
	case *microflows.ImportXmlAction:
		return a.ResultHandling, "import from mapping"
	case *microflows.RestCallAction:
		if h, ok := a.ResultHandling.(*microflows.ResultHandlingMapping); ok {
			return h, "rest call … returns mapping"
		}
	}
	return nil, ""
}

// storesFirst reports either of Studio Pro's First flags. Each one alone was
// measured failing at runtime on an object-rooted mapping (ako/mxcli#242), so
// neither needs the other to be reported.
func storesFirst(h *microflows.ResultHandlingMapping) bool {
	return (h.ForceSingleOccurrence != nil && *h.ForceSingleOccurrence) ||
		(h.RangeSingleObject != nil && *h.RangeSingleObject)
}

func (r *ImportRangeObjectMappingRule) violation(mf linter.Microflow, kind, mapping string, h *microflows.ResultHandlingMapping) linter.Violation {
	var flags []string
	if h.ForceSingleOccurrence != nil && *h.ForceSingleOccurrence {
		flags = append(flags, "ForceSingleOccurrence = true")
	}
	if h.RangeSingleObject != nil && *h.RangeSingleObject {
		flags = append(flags, "Range = First")
	}
	where := ""
	if h.ResultVariable != "" {
		where = fmt.Sprintf(" into $%s", h.ResultVariable)
	}
	return linter.Violation{
		RuleID:   r.ID(),
		Severity: r.DefaultSeverity(),
		Message: fmt.Sprintf("%s '%s.%s': `%s %s`%s stores Studio Pro's First (%s), but %s is object-rooted — "+
			"it already returns one object, and First narrows a LIST. mx check reports 0 errors; the activity "+
			"throws when it runs (key not found: Path(QName(None,),None,))",
			mf.DocumentNounTitle(), mf.ModuleName, mf.Name, kind, mapping, where,
			strings.Join(flags, ", "), mapping),
		Location: linter.Location{
			Module:       mf.ModuleName,
			DocumentType: mf.DocumentNoun(),
			DocumentName: mf.Name,
			DocumentID:   mf.ID,
		},
		Suggestion: "Drop the range: an object-rooted mapping binds an object without one — " +
			"`$X = import from mapping " + mapping + "($Json);` — and rewrite the activity with `create or modify`. " +
			"In Studio Pro, set the activity's Range to All.",
	}
}
