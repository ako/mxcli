// SPDX-License-Identifier: Apache-2.0

package javaactions

import "os"

// WriteSourceIfChanged writes a generated .java/.js stub, and reports whether it
// actually differed from what was already there.
//
// Both halves matter, for different reasons.
//
// Not rewriting an identical file is ADR-0008's rule one level out from unit
// storage: a `create or modify` that changes nothing should leave the working
// tree alone, and an unconditional rewrite moves the file's mtime on every run —
// enough to defeat an incremental build's caching even though git sees no
// change.
//
// Reporting *whether* it changed is what lets the executor say "Unchanged
// javascript action" honestly. A code action's body does not live in its unit —
// the unit carries the signature, the source lives in
// `javascriptsource/<module>/actions/<name>.js` — so a body-only edit elides the
// unit write entirely. Judging the statement on unit writes alone would call
// that "Unchanged" while the user's edit had just landed in a file.
//
// A file that cannot be read is treated as different, so the write still
// happens: the failure direction is a redundant write, never a lost one.
func WriteSourceIfChanged(path string, source string) (changed bool, err error) {
	if existing, readErr := os.ReadFile(path); readErr == nil && string(existing) == source {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		return false, err
	}
	return true, nil
}
