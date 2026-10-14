// SPDX-License-Identifier: Apache-2.0

package executor

import "github.com/mendixlabs/mxcli/mdl/langver"

// LanguageChanges returns every change of meaning the executor gates on the
// language header: the ones decided at exec or check time, against the
// project, rather than by the visitor (whose list is visitor.LanguageChanges).
// The migration reference (mdl/migration) tabulates both lists, and
// TestExecutorLanguageChangesListsEveryDeclaredChange holds this one complete.
func LanguageChanges() []langver.Change {
	return []langver.Change{
		actionSlotRefused, galleryClickRefused, flowRebuildRefused, boundaryDropAmbiguous,
		remoteTypeChangeRefused, templateAttrBinding, viewAutoNumberRefused, tabPagePropDropped,
	}
}
