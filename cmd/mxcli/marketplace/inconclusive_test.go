// SPDX-License-Identifier: Apache-2.0

// mxcli-chat FINDINGS §16: on an untouched blank app, `marketplace diff` accused
// the user of editing an Atlas_Core snippet, an Atlas_Web_Content building block
// and four FeedbackModule elements. Nobody had touched them — DESCRIBE renders
// those types imperfectly, and two imperfect renderings can differ. Worse,
// `--save-edits` wrote the snippet out as an empty `{ }` body, so the file
// offered as a rescue would have emptied it on replay.
package marketplace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The real rendering from the finding.
const emptySnippetMDL = `create or modify snippet Atlas_Core.FeedbackWidget (Folder: 'Web') { }`

const readOnlyBuildingBlockMDL = `-- Building blocks are read-only; they cannot be created via MDL.
create building block Atlas_Web_Content.Master_Detail (
  DataSource: database from ,
)`

func TestClassify_AnEmptyBodyIsNotEvidenceOfAnEdit(t *testing.T) {
	f := classify(ElementKey{Type: "SNIPPET", Name: "Atlas_Core.FeedbackWidget"},
		Element{MDL: emptySnippetMDL},
		Element{MDL: `create or modify snippet Atlas_Core.FeedbackWidget (Folder: 'Web', Caption: 'x') { }`},
		true, true)
	if f.Verdict != Unknown {
		t.Errorf("verdict = %s, want unknown — an empty body cannot show that the user edited anything", f.Verdict)
	}
	if f.Reason == "" {
		t.Error("an unknown verdict without a reason tells the user nothing")
	}
}

func TestClassify_AReadOnlyDescribeIsNotEvidenceOfAnEdit(t *testing.T) {
	f := classify(ElementKey{Type: "BUILDING_BLOCK", Name: "Atlas_Web_Content.Master_Detail"},
		Element{MDL: readOnlyBuildingBlockMDL},
		Element{MDL: strings.Replace(readOnlyBuildingBlockMDL, "database from ,", "database from Module.Entity,", 1)},
		true, true)
	if f.Verdict != Unknown {
		t.Errorf("verdict = %s, want unknown — this output is informational, not re-executable MDL", f.Verdict)
	}
}

// The negative half, and the reason equality is checked first: an element whose
// two renderings are identical is unchanged, whatever its type. Marking every
// building block unknown would make `verified` false on every project and drain
// the signal from it.
func TestClassify_IdenticalRenderingsAreUnchangedEvenWhenInconclusive(t *testing.T) {
	for _, mdl := range []string{emptySnippetMDL, readOnlyBuildingBlockMDL} {
		f := classify(ElementKey{Type: "X", Name: "M.N"}, Element{MDL: mdl}, Element{MDL: mdl}, true, true)
		if f.Verdict != Unchanged {
			t.Errorf("verdict = %s for identical output, want unchanged", f.Verdict)
		}
	}
}

// And a real edit to a normally-describable element must still be Modified —
// a guard that made everything unknown would pass the tests above.
func TestClassify_ARealEditIsStillReported(t *testing.T) {
	f := classify(ElementKey{Type: "ENTITY", Name: "Administration.Account"},
		Element{MDL: "create or modify persistent entity Administration.Account (\n  Name: String,\n  Mine: String,\n)"},
		Element{MDL: "create or modify persistent entity Administration.Account (\n  Name: String,\n)"},
		true, true)
	if f.Verdict != Modified {
		t.Errorf("verdict = %s, want modified — this is a genuine local edit", f.Verdict)
	}
}

// The saved file is offered as a rescue; it must never be a deletion.
func TestSaveEdits_RefusesToWriteAnEmptyBodyAsAnEdit(t *testing.T) {
	dir := t.TempDir()
	rep := &Report{Findings: []Finding{
		{Key: ElementKey{Type: "SNIPPET", Name: "Atlas_Core.FeedbackWidget"},
			Verdict: OnlyInstalled, InstalledMDL: emptySnippetMDL},
		{Key: ElementKey{Type: "ENTITY", Name: "M.Real"},
			Verdict: Modified, InstalledMDL: "create or modify persistent entity M.Real (\n  Name: String,\n)"},
	}}
	written, unsaved, err := SaveEdits(dir, rep)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range written {
		if strings.Contains(w, "snippet") {
			body, _ := os.ReadFile(w)
			t.Errorf("wrote %s, which replays as an empty snippet:\n%s", filepath.Base(w), body)
		}
	}
	if len(unsaved) != 1 || !strings.Contains(unsaved[0], "FeedbackWidget") {
		t.Errorf("unsaved = %v, want the snippet reported rather than silently dropped", unsaved)
	}
	if len(written) != 1 {
		t.Errorf("written = %v, want the real entity still saved", written)
	}
}
