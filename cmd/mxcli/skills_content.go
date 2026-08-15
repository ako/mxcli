// SPDX-License-Identifier: Apache-2.0

// skills_content.go - Embedded skill and command content for mxcli init
//
// Skills are synced from reference/mendix-repl/templates/.claude/skills/
// Commands are synced from .claude/commands/mendix/
// Both use go:embed directive to embed at compile time.
//
// To update skills/commands:
//
//	make sync-all   # Sync both
//	make build      # Build (auto-syncs)
package main

import (
	"embed"
)

// Embed all skill files from the synced directory
//
//go:embed skills/*.md
var skillsFS embed.FS

// Embed skill packs from the synced directory — skills that carry assets, not
// just prose (references/, specs/, scripts/, mdl/).
//
// `all:` is load-bearing rather than defensive. A plain go:embed of a directory
// skips `_`- and `.`-prefixed files, and cmd/mxcli/theme/assets.go carries the
// same prefix for exactly this reason: `_partial.scss` is how SCSS spells a
// partial, and the theme package lost them once already. A pack is just as
// likely to ship a `_helper.mjs` or an `.eslintrc`.
//
//go:embed all:skillpacks
var skillPacksFS embed.FS

// Embed all command files from the synced directory
//
//go:embed commands/*.md
var commandsFS embed.FS

// Embed all lint rule files from the synced directory
//
//go:embed lint-rules/*.star
var lintRulesFS embed.FS

// Embed the VS Code extension package (optional — file may not exist during dev builds)
//
//go:embed vscode-mdl.vsix
var vsixData []byte

// settingsJSON is the Claude Code settings for mxcli permissions
const settingsJSON = `{
  "permissions": {
    "allow": [
      "Bash(mxcli:*)",
      "Bash(./mxcli:*)",
      "Bash(./mxcli *)",
      "Bash(playwright-cli:*)",
      "Bash(playwright-cli *)"
    ]
  },
  "env": {
    "MXCLI_QUIET": "1"
  }
}
`
