// SPDX-License-Identifier: Apache-2.0

package model

import "sync"

// DefaultTextLanguage is the language a Texts$Text is built under when nothing
// has told the process otherwise. It is the pre-#970 behaviour, so an unset
// process behaves exactly as mxcli always did.
const DefaultTextLanguage = "en_US"

var (
	textLangMu sync.RWMutex
	textLang   = DefaultTextLanguage
)

// AuthoringLanguage is the language code a writer stores a bare string under
// when it has to build a Texts$Text and the caller handed it no language.
//
// Why this is process-level rather than a parameter. Mendix has no
// language-neutral text: every Texts$Translation carries a LanguageCode, so the
// leaf that turns a Go string into a Texts$Text must name one. Those leaves sit
// at the bottom of the widget/navigation serializers, and threading a language
// down to them would change the signature of **183 functions** across five
// packages (113 in sdk/mpr alone) — nearly every widget writer in the codebase,
// to carry a value that is constant for the entire run. Measured, not estimated;
// see the #970 investigation.
//
// The language genuinely is ambient for a write: it is a property of the project
// being written, mxcli writes one project per process, and it never changes
// mid-write. That is the same shape as MXCLI_ENGINE and MXCLI_ALWAYS_WRITE, which
// this codebase already reads from the process.
//
// The contract: SetAuthoringLanguage is called once, when a project's settings
// are first resolved, BEFORE any write. Reads are cheap and safe from any
// goroutine (page widget extraction runs in parallel). An empty or unset value
// means DefaultTextLanguage, so nothing that forgets to set it regresses.
//
// This governs CREATION of a new text only. Editing an existing Texts$Text keeps
// whatever LanguageCode is stored — that is where the language belongs once a
// text exists, and no ambient default should override it.
func AuthoringLanguage() string {
	textLangMu.RLock()
	defer textLangMu.RUnlock()
	return textLang
}

// SetAuthoringLanguage sets the language returned by AuthoringLanguage. An empty
// code resets to DefaultTextLanguage rather than storing an empty LanguageCode,
// which no Mendix document has and Studio Pro would not resolve.
func SetAuthoringLanguage(code string) {
	textLangMu.Lock()
	defer textLangMu.Unlock()
	if code == "" {
		code = DefaultTextLanguage
	}
	textLang = code
}
