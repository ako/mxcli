// SPDX-License-Identifier: Apache-2.0

package backend

// SourceFileChange is a file or folder beside the model — in the project
// directory, but not the .mpr or mprcontents/ — that a write created, changed,
// moved or removed.
//
// It exists because the model is not the whole project. A Java or JavaScript
// action's body lives in javasource/ or javascriptsource/, not in its unit, so
// a workflow that commits "the model" leaves it behind, and a clone gets an
// action with no logic (ako/mxcli#1618, mendixlabs/mxcli#1634). The writer is
// the only code that knows what it touched; it records each change at the
// moment it touches disk, and `exec` names them.
type SourceFileChange struct {
	// Path is project-relative and slash-separated, e.g.
	// "javasource/probe/actions/JA_Hello.java". A folder ends in "/".
	Path string
	// From is the path the file was moved from, for a rename; empty otherwise.
	From string
	// Removed is set when the file or folder was deleted.
	Removed bool
	// What names the content for a reader, e.g. "Java action body".
	What string
}

// SourceFileReporter is implemented by backends that write files beside the
// model and can say which. A backend that writes none need not implement it.
type SourceFileReporter interface {
	// TakeSourceFileChanges returns the changes made since the previous call,
	// in the order they were made, and forgets them.
	TakeSourceFileChanges() []SourceFileChange
}
