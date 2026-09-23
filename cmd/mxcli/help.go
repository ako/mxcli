// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	"github.com/mendixlabs/mxcli/cmd/mxcli/syntax"
	"github.com/spf13/cobra"
)

var syntaxCmd = &cobra.Command{
	Use:   "syntax [topic [subtopic...]]",
	Short: "Show MDL syntax reference",
	Long: `Show MDL syntax reference from the feature registry.

Use --json for machine-readable output (optimized for LLM consumption).
Drill down with multiple arguments: mxcli syntax workflow user-task targeting
The topic may be given as separate words, as one quoted string, or dotted —
all three reach the same page, as does the plain-word spelling ("user task").

Top-level topics:
  domain-model    - Entities, associations, enumerations, constants, keywords, types
  microflow       - Microflow/nanoflow creation and activities
  page            - Pages, snippets, fragments, widgets
  layout          - Layouts: regions, navigation, placeholders, repointing pages
  security        - Roles, access control, demo users
  workflow        - Workflows, user tasks, decisions, parallel splits
  navigation      - Navigation profiles, menus, home pages
  settings        - Project settings
  integration     - OData, REST, SQL, OQL, XPath, Java actions, business events
  agents          - AI agent documents (Model, KB, Consumed MCP Service, Agent)
  errors          - Common validation errors and fixes
  structure       - SHOW STRUCTURE command
  move            - MOVE command for relocating documents
  search          - Full-text SEARCH command

Examples:
  mxcli syntax --json                          # Full index (LLM: cache this)
  mxcli syntax workflow --json                 # All workflow features
  mxcli syntax workflow user-task targeting     # Drill down to targeting
  mxcli syntax security entity-access           # Entity access rules
  mxcli syntax workflow user task               # Plain words resolve too
  mxcli syntax entity                           # Legacy alias → domain-model.entity
`,
	Run: func(cmd *cobra.Command, args []string) {
		jsonFlag, _ := cmd.Flags().GetBool("json")
		out := cmd.OutOrStdout()

		// No args: show full index (JSON) or help text
		if len(args) == 0 {
			if jsonFlag {
				syntax.WriteJSON(out, syntax.All())
				return
			}
			cmd.Help()
			return
		}

		// One resolver for both surfaces — see syntax.Lookup. The topic may
		// arrive as separate words, as one quoted string, or dotted.
		m := syntax.Lookup(args)
		if len(m.Features) > 0 {
			if jsonFlag {
				syntax.WriteJSON(out, m.Features)
				return
			}
			if !m.Exact {
				fmt.Fprintf(out, "No topic %q. Showing %d topic(s) matching %q:\n\n",
					m.Path, len(m.Features), m.Fallback)
			}
			syntax.WriteText(out, m.Features)
			return
		}

		fmt.Fprintf(out, "Unknown topic: %s\n\n", m.Path)
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(syntaxCmd)
}
