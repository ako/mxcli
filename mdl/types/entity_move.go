// SPDX-License-Identifier: Apache-2.0

package types

// MovedAssociation reports one association that a cross-module entity move
// converted into a cross-module association, and the qualified name it holds
// afterwards.
//
// The old and new names are both carried because the conversion is asymmetric and
// the caller must not guess which happened. Mendix stores an association in the
// module of its FROM entity, so:
//
//   - the FROM (parent) entity moved → the cross-association travels with it, and
//     its qualified name changes to the target module;
//   - the TO (child) entity moved → the cross-association stays in the source
//     module and its qualified name does not change.
//
// Measured on a blank 11.13 app: moving the parent left 9 of 33 CE1613s naming the
// association, moving the child left 0. A reference sweep driven by the module
// names alone would therefore corrupt the second case, and one driven by these
// fields is a no-op there instead of a special case (ako/mxcli#605).
type MovedAssociation struct {
	// Name is the association's own (unqualified) name, for reporting to the user.
	Name string
	// OldQualifiedName is what it was called before the move.
	OldQualifiedName string
	// NewQualifiedName is what it is called after it. Equal to OldQualifiedName
	// when the association did not change module.
	NewQualifiedName string
}

// Moved reports whether the association's qualified name changed, i.e. whether
// references to it elsewhere in the project have to be rewritten.
func (m MovedAssociation) Moved() bool {
	return m.OldQualifiedName != m.NewQualifiedName
}

// MovedAssociationNames returns the association names, for the user-facing
// "Converted N association(s)" report.
func MovedAssociationNames(ms []MovedAssociation) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.Name)
	}
	return out
}
