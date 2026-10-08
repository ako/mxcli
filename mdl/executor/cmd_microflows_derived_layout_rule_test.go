// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ruleLookupCounter is the real backend with IsRule counted.
type ruleLookupCounter struct {
	backend.FullBackend
	calls *atomic.Int64
}

func (b ruleLookupCounter) IsRule(qualifiedName string) (bool, error) {
	b.calls.Add(1)
	return b.FullBackend.IsRule(qualifiedName)
}

// handLaidRuleSplits is a flow of n decisions on the same rule, every node
// placed by hand so the derivation takes several rounds — the shape of
// AgentCommons.PromptToUse_ApplyVariable, which has two.
func handLaidRuleSplits(name, rule string, n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "create or modify microflow %s ($X: integer)\nbegin\n", name)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "  @position(%d, %d)\n  if %s(Amount = $X + %d) then\n", 200+i*137, 100+(i%3)*53, rule, i)
		fmt.Fprintf(&b, "    @position(%d, %d)\n    log info node 'T' 'a%d';\n  end if;\n", 260+i*137, 300+(i%5)*31, i)
	}
	b.WriteString("  return;\nend;")
	return b.String()
}

// Every `if Module.Name(...)` asks the backend whether Module.Name is a rule,
// on every build, and the derived layout builds the flow once per round. On a
// real project one answer cost 53 s (a full project read per rule — see
// TestIsRule_ResolvesOneModule), so describe of a two-split flow drawn in
// Studio Pro ran past five minutes. Within one derivation nothing is written,
// so the answer is asked once per name.
//
// The rounds are the control: a flow that settles in one round asks once per
// split even without the memo, so the test first requires several.
func TestDescribe_DerivedLayoutAsksIsRuleOncePerName(t *testing.T) {
	var calls atomic.Int64
	var out bytes.Buffer
	exec := New(&out)
	exec.SetQuiet(true)
	exec.SetBackendFactory(func() backend.FullBackend {
		return ruleLookupCounter{FullBackend: modelsdkbackend.New(), calls: &calls}
	})
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(projectFixture(t))+"'")

	const rule, name, splits = "MyFirstModule.IsLarge", "MyFirstModule.HandLaidRuleSplits", 4
	run(t, exec, "create rule "+rule+" (Amount: Integer) returns Boolean\nbegin\n  return $Amount > 10;\nend;")
	run(t, exec, handLaidRuleSplits(name, rule, splits))

	rounds, asked := derivedLayoutRounds.Load(), calls.Load()
	mdl := describeText(t, exec, &out, "describe microflow "+name+";")
	rounds, asked = derivedLayoutRounds.Load()-rounds, calls.Load()-asked

	if rounds < 3 {
		t.Fatalf("control: the derivation took %d round(s), so this flow cannot show a per-round cost", rounds)
	}
	if got := strings.Count(mdl, "if "+rule+"("); got != splits {
		t.Fatalf("control: described %d rule splits, want %d:\n%s", got, splits, mdl)
	}
	if asked > 1 {
		t.Errorf("describe asked whether %s is a rule %d times (%d splits, %d rounds); want once", rule, asked, splits, rounds)
	}
}
