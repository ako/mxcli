// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/grammar/parser"
)

func (b *Builder) ExitCreateAssociationStatement(ctx *parser.CreateAssociationStatementContext) {
	names := ctx.AllQualifiedName()
	if len(names) < 3 {
		return
	}

	stmt := &ast.CreateAssociationStmt{
		Name:           buildQualifiedName(names[0]),
		Parent:         buildQualifiedName(names[1]),
		Child:          buildQualifiedName(names[2]),
		Type:           ast.AssocReference, // Default
		Owner:          ast.OwnerDefault,
		DeleteBehavior: ast.DeleteKeepReferences,
	}

	// Association options
	if opts := ctx.AssociationOptions(); opts != nil {
		optsCtx := opts.(*parser.AssociationOptionsContext)
		for _, opt := range optsCtx.AllAssociationOption() {
			optCtx := opt.(*parser.AssociationOptionContext)

			// TYPE
			if optCtx.TYPE() != nil {
				if optCtx.REFERENCE_SET() != nil {
					stmt.Type = ast.AssocReferenceSet
				}
			}

			// OWNER (grammar supports DEFAULT and BOTH)
			if optCtx.OWNER() != nil {
				if optCtx.BOTH() != nil {
					stmt.Owner = ast.OwnerBoth
				} else if optCtx.DEFAULT() != nil {
					stmt.Owner = ast.OwnerDefault
				}
			}

			// STORAGE
			if optCtx.STORAGE() != nil {
				if optCtx.COLUMN() != nil {
					stmt.Storage = ast.StorageColumn
				} else if optCtx.TABLE() != nil {
					stmt.Storage = ast.StorageTable
				}
			}

			// DELETE_BEHAVIOR
			if delBehavior := optCtx.DeleteBehavior(); delBehavior != nil {
				stmt.DeleteBehavior = buildDeleteBehavior(delBehavior)
			}

			// COMMENT
			if optCtx.COMMENT() != nil && optCtx.STRING_LITERAL() != nil {
				stmt.Comment = unquoteString(optCtx.STRING_LITERAL().GetText())
			}
		}
	}

	if createStmt := findParentCreateStatement(ctx); createStmt != nil {
		if createStmt.OR() != nil && (createStmt.MODIFY() != nil || createStmt.REPLACE() != nil) {
			stmt.CreateOrModify = true
		}
		stmt.FromAnchor, stmt.ToAnchor = b.anchorAnnotation(createStmt)
	}
	b.statements = append(b.statements, stmt)
}

// anchorAnnotation reads `@anchor(from: (x, y), to: (x, y))` off a CREATE
// ASSOCIATION statement — the association's line anchors, as a percentage of
// each entity box (0..100).
//
// No grammar rule was needed: `annotation*` is generic across every CREATE
// statement, `annotationParamName` already admits FROM and TO, and a
// parenthesised value is the existing `annotationParenValue` (which the
// microflow `@anchor(true: (from: right, to: left), …)` form uses). This is the
// same annotation name and the same two parameter names as the microflow-flow
// anchor, asking the same question — where does the connector attach — and
// differing only in the value type, because an association's endpoint is a
// continuous point on the box rather than one of four sides.
//
// Either end may be omitted; a nil result means "say nothing", which preserves
// whatever is stored rather than resetting it. (issue #872)
func (b *Builder) anchorAnnotation(createStmt *parser.CreateStatementContext) (from, to *ast.Position) {
	for _, annCtx := range createStmt.AllAnnotation() {
		ann := annCtx.(*parser.AnnotationContext)
		if !strings.EqualFold(ann.AnnotationName().GetText(), "anchor") {
			continue
		}
		params := ann.AnnotationParams()
		if params == nil {
			continue
		}
		for _, p := range params.(*parser.AnnotationParamsContext).AllAnnotationParam() {
			paramCtx := p.(*parser.AnnotationParamContext)
			nameCtx := paramCtx.AnnotationParamName()
			if nameCtx == nil {
				continue
			}
			pt := b.annotationParenPoint(paramCtx)
			if pt == nil {
				continue
			}
			switch strings.ToLower(nameCtx.GetText()) {
			case "from":
				from = pt
			case "to":
				to = pt
			}
		}
	}
	return from, to
}

// annotationParenPoint reads a `(x, y)` parenthesised annotation value into a
// Position. Returns nil for any other shape — including the microflow anchor's
// `(from: right, to: left)`, whose params are named rather than positional, so
// the two `@anchor` forms cannot be confused for one another.
func (b *Builder) annotationParenPoint(paramCtx *parser.AnnotationParamContext) *ast.Position {
	paren := paramCtx.AnnotationParenValue()
	if paren == nil {
		return nil
	}
	inner := paren.(*parser.AnnotationParenValueContext).AnnotationParams()
	if inner == nil {
		return nil
	}
	coords := inner.(*parser.AnnotationParamsContext).AllAnnotationParam()
	if len(coords) != 2 {
		return nil
	}
	for _, c := range coords {
		if c.(*parser.AnnotationParamContext).AnnotationParamName() != nil {
			return nil // named, not a coordinate pair
		}
	}
	x, okX := anchorCoord(coords[0].GetText())
	y, okY := anchorCoord(coords[1].GetText())
	if !okX || !okY {
		b.addErrorWithExample(
			fmt.Sprintf("anchor coordinate (%s, %s) is not a whole number — Mendix stores line anchors as two integers, "+
				"a percentage of the entity box (0..100), and refuses to LOAD a project whose anchor is anything else",
				strings.TrimSpace(coords[0].GetText()), strings.TrimSpace(coords[1].GetText())),
			"@anchor(from: (0, 54), to: (100, 54))")
		return nil
	}
	return &ast.Position{X: x, Y: y}
}

// anchorCoord parses one anchor coordinate. Mendix stores the pair as two
// INTEGERS and its loader rejects anything else outright — a hand-patched
// "0.5;50" fails with StorageLoadException before validation even runs — so a
// non-integer must be refused here rather than silently truncated to 0.
func anchorCoord(text string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0, false
	}
	return v, true
}

// ExitAlterAssociationAction handles ALTER ASSOCIATION ... SET ... actions.
func (b *Builder) ExitAlterAssociationAction(ctx *parser.AlterAssociationActionContext) {
	// Walk up to the parent AlterStatement to get the association's qualified name
	parent := ctx.GetParent()
	for parent != nil {
		if alterStmt, ok := parent.(*parser.AlterStatementContext); ok {
			if alterStmt.ASSOCIATION() == nil {
				return
			}
			qn := alterStmt.QualifiedName()
			if qn == nil {
				return
			}
			name := buildQualifiedName(qn)

			// SET DELETE_BEHAVIOR
			if ctx.DELETE_BEHAVIOR() != nil {
				if delBehavior := ctx.DeleteBehavior(); delBehavior != nil {
					b.statements = append(b.statements, &ast.AlterAssociationStmt{
						Name:           name,
						Operation:      ast.AlterAssociationSetDeleteBehavior,
						DeleteBehavior: buildDeleteBehavior(delBehavior),
					})
				}
				return
			}

			// SET OWNER
			if ctx.OWNER() != nil {
				owner := ast.OwnerDefault
				if ctx.BOTH() != nil {
					owner = ast.OwnerBoth
				}
				b.statements = append(b.statements, &ast.AlterAssociationStmt{
					Name:      name,
					Operation: ast.AlterAssociationSetOwner,
					Owner:     owner,
				})
				return
			}

			// SET STORAGE
			if ctx.STORAGE() != nil {
				storage := ast.StorageTable
				if ctx.COLUMN() != nil {
					storage = ast.StorageColumn
				}
				b.statements = append(b.statements, &ast.AlterAssociationStmt{
					Name:      name,
					Operation: ast.AlterAssociationSetStorage,
					Storage:   storage,
				})
				return
			}

			// SET ANCHOR FROM (x, y) TO (x, y)
			if ctx.ANCHOR() != nil {
				pts := ctx.AllAnchorPoint()
				if len(pts) == 2 {
					b.statements = append(b.statements, &ast.AlterAssociationStmt{
						Name:       name,
						Operation:  ast.AlterAssociationSetAnchor,
						FromAnchor: b.buildAnchorPoint(pts[0]),
						ToAnchor:   b.buildAnchorPoint(pts[1]),
					})
				}
				return
			}

			// SET COMMENT
			if ctx.COMMENT() != nil && ctx.STRING_LITERAL() != nil {
				b.statements = append(b.statements, &ast.AlterAssociationStmt{
					Name:      name,
					Operation: ast.AlterAssociationSetComment,
					Comment:   unquoteString(ctx.STRING_LITERAL().GetText()),
				})
				return
			}

			return
		}
		parent = parent.GetParent()
	}
}

// ----------------------------------------------------------------------------
// Query Statements (SHOW/DESCRIBE)
// ----------------------------------------------------------------------------

// ExitShowStatement handles SHOW MODULES/ENTITIES/ASSOCIATIONS/etc.

// buildAnchorPoint reads the `(x, y)` of a SET ANCHOR clause.
func (b *Builder) buildAnchorPoint(ctx parser.IAnchorPointContext) *ast.Position {
	if ctx == nil {
		return nil
	}
	nums := ctx.(*parser.AnchorPointContext).AllNUMBER_LITERAL()
	if len(nums) != 2 {
		return nil
	}
	x, okX := anchorCoord(nums[0].GetText())
	y, okY := anchorCoord(nums[1].GetText())
	if !okX || !okY {
		b.addErrorWithExample(
			fmt.Sprintf("anchor coordinate (%s, %s) is not a whole number — Mendix stores line anchors as two integers, "+
				"a percentage of the entity box (0..100), and refuses to LOAD a project whose anchor is anything else",
				nums[0].GetText(), nums[1].GetText()),
			"alter association Module.A_B set anchor from (0, 54) to (100, 54);")
		return nil
	}
	return &ast.Position{X: x, Y: y}
}
