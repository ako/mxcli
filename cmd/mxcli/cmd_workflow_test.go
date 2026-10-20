// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/workflow"
)

// Every workflow mxcli ships loads, has a skill beside it, and its gates call
// mxcli commands and flags that exist: a renamed flag would otherwise turn a
// gate red for every user, with nothing in mxcli's own build to say so.
func TestShippedWorkflows_GatesCallRealCommands(t *testing.T) {
	defs, err := loadWorkflows("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := workflow.Find(defs, "vibe"); !ok {
		t.Fatal("the vibe workflow is not embedded: run `make sync-skills` (or `make build`) before go test")
	}
	for _, d := range defs {
		if _, err := skillsFS.ReadFile("skills/" + d.Skill + "/SKILL.md"); err != nil {
			t.Errorf("%s: its skill %q has no SKILL.md", d.ID, d.Skill)
		}
		for _, g := range d.Gates {
			if len(g.Args) == 0 {
				continue
			}
			cmd, rest, err := rootCmd.Find(g.Args)
			if err != nil || cmd == rootCmd {
				t.Errorf("%s/%s: %v is not an mxcli command", d.ID, g.ID, g.Args)
				continue
			}
			for _, a := range rest {
				if !strings.HasPrefix(a, "-") {
					continue
				}
				name := strings.TrimLeft(a, "-")
				known := cmd.Flags().Lookup(name) != nil || cmd.InheritedFlags().Lookup(name) != nil
				if len(name) == 1 {
					known = known || cmd.Flags().ShorthandLookup(name) != nil || cmd.InheritedFlags().ShorthandLookup(name) != nil
				}
				if !known {
					t.Errorf("%s/%s: %q has no flag %s", d.ID, g.ID, cmd.CommandPath(), a)
				}
			}
		}
	}
}
