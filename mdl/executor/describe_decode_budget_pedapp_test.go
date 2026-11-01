// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/mprread"
)

// A describe found its document, and the snippets, flows and pages it names,
// by listing every document of the type, and the listing decoded each one. A
// source catalog build describes every document, so it paid that once per
// document: 40% of a first build's CPU on FactoryManagement (#1850).
//
// The measure: describe each kind of document, add more documents of every
// kind, describe again. What a describe decodes must not grow with documents
// it does not print.
func TestDescribeDecodesOnlyWhatItPrints(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	if err := agreeExec(t, exec, `mdl 1;
create persistent entity MyFirstModule.DescProbe (Name: String(50));
create snippet MyFirstModule.DescSnippet ( Params: ( $P: MyFirstModule.DescProbe ) ) {
  dynamictext txtName (Content: 'probe')
};
create microflow MyFirstModule.DescFlow () returns Boolean as $Ok
begin
  declare $Ok Boolean = true;
  return $Ok;
end;
create nanoflow MyFirstModule.DescNano () returns Boolean as $Ok
begin
  declare $Ok Boolean = true;
  return $Ok;
end;
create page MyFirstModule.DescPage (Title: 'P', Layout: Atlas_Core.Atlas_Default, Params: ( $P: MyFirstModule.DescProbe )) {
  snippetcall sc1 (Snippet: MyFirstModule.DescSnippet, Params: (P = $P))
  actionbutton btn1 (Caption: 'Go', Action: call nanoflow MyFirstModule.DescNano)
};
`); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}

	kinds := []string{
		"snippet MyFirstModule.DescSnippet",
		"page MyFirstModule.DescPage",
		"microflow MyFirstModule.DescFlow",
		"nanoflow MyFirstModule.DescNano",
	}
	measure := func() map[string]int64 {
		decoded := map[string]int64{}
		for _, k := range kinds {
			before := mprread.UnitsDecoded()
			var buf bytes.Buffer
			exec.output = &buf
			if err := agreeExec(t, exec, "describe "+k+";"); err != nil {
				t.Fatalf("describe %s: %v", k, err)
			}
			decoded[k] = mprread.UnitsDecoded() - before
			if !strings.Contains(buf.String(), strings.Fields(k)[1]) {
				t.Fatalf("control: describe %s printed %q", k, buf.String())
			}
		}
		exec.output = out
		return decoded
	}
	base := measure()

	const extra = 15
	var more strings.Builder
	more.WriteString("mdl 1;\n")
	for i := range extra {
		fmt.Fprintf(&more, "create snippet MyFirstModule.DescFiller%d { dynamictext t (Content: 'x') };\n", i)
		fmt.Fprintf(&more, "create page MyFirstModule.DescFillerPage%d (Title: 'P', Layout: Atlas_Core.Atlas_Default) { dynamictext t (Content: 'x') };\n", i)
		fmt.Fprintf(&more, "create microflow MyFirstModule.DescFillerFlow%d () begin end;\n", i)
		fmt.Fprintf(&more, "create nanoflow MyFirstModule.DescFillerNano%d () begin end;\n", i)
	}
	if err := agreeExec(t, exec, more.String()); err != nil {
		t.Fatalf("add documents: %v\n%s", err, out.String())
	}

	after := measure()
	for _, k := range kinds {
		if after[k] > base[k] {
			t.Errorf("describe %s decoded %d units, and %d after adding %d documents of each kind it does not print",
				k, base[k], after[k], extra)
		}
	}
}
