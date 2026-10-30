// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// Files beside the model (ako/mxcli#1618, mendixlabs/mxcli#1634).
//
// `create java action … as $$ … $$` stores the action in the model and writes
// its body to javasource/<module>/actions/<Name>.java; `exec` named only the
// model element. A workflow that commits "the model" — the .mpr and
// mprcontents/ — left the body behind, and a clone got an action with no logic,
// with nothing in the output to say a second place had been written.
//
// So every file or folder a statement changes beside the model is named on its
// own line after the statement's report, and a program run that changed any
// ends with one line saying to stage them. The changes come from the backend,
// which records each at the moment it touches disk (backend.SourceFileReporter):
// the report cannot claim a write that did not happen, and an identical rewrite,
// which leaves the file alone, is not reported at all.

// SetSourceFileReport turns the report of files changed beside the model on
// (the default) or off. Off is for a caller whose writes are temporary — `mxcli
// test` injects a Java action and deletes its source again at cleanup, and
// telling its user to commit that file would be wrong.
func (e *Executor) SetSourceFileReport(on bool) {
	e.noSourceFileReport = !on
}

// reportSourceFileChanges prints, one per line, the files and folders beside
// the model that the statement just run changed, and keeps them for the run's
// summary. A backend that writes none does not implement the reporter.
func (e *Executor) reportSourceFileChanges(b backend.FullBackend, w io.Writer) {
	r, ok := b.(backend.SourceFileReporter)
	if !ok {
		return
	}
	// Taken even when not reported, so a later statement does not report it.
	changes := r.TakeSourceFileChanges()
	if e.noSourceFileReport {
		return
	}
	for _, c := range changes {
		fmt.Fprintln(w, sourceFileChangeLine(c))
	}
	e.sourceFiles.add(changes)
}

// sourceFileChangeLine is the line one change is reported with.
func sourceFileChangeLine(c backend.SourceFileChange) string {
	switch {
	case c.Removed:
		return fmt.Sprintf("Removed %s (%s; commit the deletion with the model)", c.Path, c.What)
	case c.From != "":
		return fmt.Sprintf("Moved %s to %s (%s; commit both paths with the model)", c.From, c.Path, c.What)
	default:
		return fmt.Sprintf("Wrote %s (%s; commit it with the model)", c.Path, c.What)
	}
}

// sourceFileLog collects a program run's changes beside the model for the
// summary line at its end. Its lifetime is the mutation tally's: the run that
// owns the tally owns this, so a nested EXECUTE SCRIPT adds to its caller's
// summary instead of printing its own.
type sourceFileLog struct {
	active  bool
	changes []backend.SourceFileChange
}

// begin starts collecting for a program run.
func (l *sourceFileLog) begin() {
	l.active = true
	l.changes = nil
}

// add records changes for the summary. Outside a program run — one statement
// in the REPL — there is no summary, and nothing is kept.
func (l *sourceFileLog) add(changes []backend.SourceFileChange) {
	if l.active {
		l.changes = append(l.changes, changes...)
	}
}

// flush writes the summary, if the run changed anything beside the model, and
// stands the log down. A path written more than once counts once; the folders
// are the top-level ones, in the order first touched.
func (l *sourceFileLog) flush(w io.Writer) {
	defer func() { l.active, l.changes = false, nil }()
	if !l.active || len(l.changes) == 0 {
		return
	}
	paths := map[string]bool{}
	var roots []string
	seenRoot := map[string]bool{}
	for _, c := range l.changes {
		paths[c.Path] = true
		root := c.Path
		if i := strings.Index(root, "/"); i >= 0 {
			root = root[:i+1]
		}
		if !seenRoot[root] {
			seenRoot[root] = true
			roots = append(roots, root)
		}
	}
	fmt.Fprintf(w, "Files changed outside the model: %d (%s). Stage them with the .mpr.\n",
		len(paths), strings.Join(roots, ", "))
}
