// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var renameCmd = &cobra.Command{
	Use:   "rename <type> <qualified-name> <new-name>",
	Short: "Rename a project element and update all references",
	Long: `Rename an element and automatically update all cross-references.

Types:
  entity         Rename an entity
  microflow      Rename a microflow
  nanoflow       Rename a nanoflow
  page           Rename a page
  enumeration    Rename an enumeration
  association    Rename an association
  constant       Rename a constant
  module         Rename a module (updates all qualified names)

Use --dry-run to preview changes without modifying.

Anchors in docs/brain/ are updated too, if the project has a brain. They are
references to the same elements, and "mxcli brain check" cannot reliably report
a stale one after the fact: a requirement's anchor points forward, so one that
stops resolving is indistinguishable from something not built yet.

Example:
  mxcli rename -p app.mpr entity MyModule.Customer Client
  mxcli rename -p app.mpr microflow MyModule.ACT_Old ACT_New
  mxcli rename -p app.mpr page MyModule.OldPage NewPage
  mxcli rename -p app.mpr module OldModule NewModule
  mxcli rename -p app.mpr entity MyModule.Customer Client --dry-run
`,
	Args: cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
			os.Exit(1)
		}

		dryRun, _ := cmd.Flags().GetBool("dry-run")

		objectType := strings.ToUpper(args[0])
		qualifiedName := args[1]
		newName := args[2]

		var mdlCmd string
		dryRunSuffix := ""
		if dryRun {
			dryRunSuffix = " DRY RUN"
		}

		switch objectType {
		case "ENTITY":
			mdlCmd = fmt.Sprintf("RENAME ENTITY %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "MICROFLOW":
			mdlCmd = fmt.Sprintf("RENAME MICROFLOW %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "NANOFLOW":
			mdlCmd = fmt.Sprintf("RENAME NANOFLOW %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "PAGE":
			mdlCmd = fmt.Sprintf("RENAME PAGE %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "ENUMERATION":
			mdlCmd = fmt.Sprintf("RENAME ENUMERATION %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "ASSOCIATION":
			mdlCmd = fmt.Sprintf("RENAME ASSOCIATION %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "CONSTANT":
			mdlCmd = fmt.Sprintf("RENAME CONSTANT %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		case "MODULE":
			mdlCmd = fmt.Sprintf("RENAME MODULE %s TO %s%s", qualifiedName, newName, dryRunSuffix)
		default:
			fmt.Fprintf(os.Stderr, "Unknown type: %s\n", args[0])
			fmt.Fprintln(os.Stderr, "Valid types: entity, microflow, nanoflow, page, enumeration, association, constant, module")
			os.Exit(1)
		}

		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		defer exec.Close()

		// Connect
		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s' FOR WRITING", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

		// Execute rename
		renameProg, errs := visitor.Build(mdlCmd)
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
			}
			os.Exit(1)
		}

		for _, stmt := range renameProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}

		renameBrainAnchors(projectPath, objectType, qualifiedName, newName, dryRun)
	},
}

// renameBrainAnchors keeps docs/brain/ pointing at the thing that was renamed.
//
// The model's own cross-references are updated by the RENAME statement above.
// The brain's anchors are references to the same elements and were not, so a
// refactor invalidated them silently — and `brain check` can only report half of
// it, because a requirement's forward anchor failing is indistinguishable from
// "not built yet". Both names are known here and nowhere later, which is why
// this belongs at the rename.
//
// It never fails the command. The rename itself has already been applied and is
// the thing the user asked for; a store that could not be updated is a warning
// to act on, not a reason to leave the project half-renamed.
func renameBrainAnchors(projectPath, objectType, qualifiedName, newName string, dryRun bool) {
	projectDir := filepath.Dir(projectPath)
	store := brain.NewStore(projectDir)
	if !store.Exists() {
		return
	}

	oldName, ok := brainRenameTarget(objectType, qualifiedName)
	if !ok {
		return
	}
	// An element rename gives a bare new name; the module part is unchanged.
	// A module rename gives the whole thing.
	target := newName
	if i := strings.LastIndex(oldName, "."); i >= 0 {
		target = oldName[:i+1] + newName
	}

	if dryRun {
		// Report without writing, matching the statement's own DRY RUN.
		n, err := brain.CountAnchorsNaming(store, oldName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not read the project brain: %v\n", err)
			return
		}
		if n > 0 {
			fmt.Printf("Would update %d brain anchor(s): @%s -> @%s\n", n, oldName, target)
		}
		return
	}

	shardN, err := store.RenameAnchors(oldName, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: the rename succeeded but docs/brain/ was not updated: %v\n", err)
		fmt.Fprintf(os.Stderr, "         Run 'mxcli brain check' — anchors naming %s are now stale.\n", oldName)
		return
	}
	queueN, err := brain.RenameQueueAnchors(brain.NewQueue(projectDir), oldName, target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: the staged brain queue was not updated: %v\n", err)
	}
	if n := shardN + queueN; n > 0 {
		fmt.Printf("Updated %d brain anchor(s): @%s -> @%s\n", n, oldName, target)
	}
}

// brainRenameTarget maps a rename to the qualified name an anchor would use.
//
// Only the types an anchor can actually name are handled. An anchor is
// @Module[.Element[.Member]], so a constant or an association renames like any
// other element, while a type the anchor grammar cannot express is skipped
// rather than guessed at — writing a name no anchor could have held would
// corrupt entries instead of repairing them.
func brainRenameTarget(objectType, qualifiedName string) (string, bool) {
	switch objectType {
	case "ENTITY", "MICROFLOW", "NANOFLOW", "PAGE", "ENUMERATION", "ASSOCIATION", "CONSTANT":
		// These are all Module.Element, which is what an anchor names.
		if !strings.Contains(qualifiedName, ".") {
			return "", false
		}
		return qualifiedName, true
	case "MODULE":
		return qualifiedName, true
	default:
		return "", false
	}
}

func init() {
	renameCmd.Flags().Bool("dry-run", false, "Preview changes without modifying")
	rootCmd.AddCommand(renameCmd)
}
