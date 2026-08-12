// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/cmd/mxcli/marketplace"
	"github.com/mendixlabs/mxcli/internal/auth"
	"github.com/spf13/cobra"
)

var marketplaceUpdateCmd = &cobra.Command{
	Use:   "update <content-id> -p <project.mpr> --to <version>",
	Short: "Replace an installed marketplace module with a newer version",
	Long: `Replace an installed marketplace module with another published version,
preserving the two things a plain replace destroys.

The first is element identity. The runtime keys entities and their attributes on
the model's GUID, so a module whose documents are replaced without carrying the
old GUIDs is a different module as far as the database is concerned, and its
tables are dropped on the next deploy. Studio Pro's own update transplants them;
so does this.

The second is access. A user role's grant of a module role lives in the project's
security document rather than in the module, so removing the module takes the
grants with it and putting the module back does not return them.

Local edits are NOT preserved, and by default the update refuses when it finds
any. Use --save-edits to write them out as re-executable MDL first, and --force
to proceed. Studio Pro discards them without asking; this at least tells you what
they were.

This does not roll back. Work on a copy or have the project in version control:
if a step fails partway, the module has already been removed.`,
	Example: `  # What would this update touch, and have I edited any of it?
  mxcli marketplace diff 23513 -p app.mpr --to 4.5.0

  # Park local edits, then update over them
  mxcli marketplace update 23513 -p app.mpr --to 4.5.0 --save-edits ./local-edits
  mxcli marketplace update 23513 -p app.mpr --to 4.5.0 --force
  mxcli exec ./local-edits/entity-Account.mdl -p app.mpr`,
	Args: cobra.ExactArgs(1),
	RunE: runMarketplaceUpdate,
}

func runMarketplaceUpdate(cmd *cobra.Command, args []string) error {
	contentID, err := parseContentID(args[0])
	if err != nil {
		return err
	}
	mprPath, _ := cmd.Flags().GetString("project")
	if mprPath == "" {
		return fmt.Errorf("-p/--project is required")
	}
	target, _ := cmd.Flags().GetString("to")
	if target == "" {
		return fmt.Errorf("--to <version> is required: an update needs a version to move to")
	}
	moduleName, _ := cmd.Flags().GetString("module")
	saveEdits, _ := cmd.Flags().GetString("save-edits")
	force, _ := cmd.Flags().GetBool("force")

	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	client, err := newMarketplaceClient(ctx, cmd)
	if err != nil {
		return err
	}
	versions, err := client.Versions(ctx, contentID)
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "mxupdate")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	var installedVersionID string
	if moduleName == "" {
		moduleName, installedVersionID, err = marketplace.ModuleForVersionIDs(mprPath, versionIDs(versions.Items))
		if err != nil {
			return fmt.Errorf("%w\nhint: pass --module with the module's name in the project", err)
		}
	}
	installedVersion, mendixVersion, err := marketplace.InstalledModule(mprPath, moduleName)
	if err != nil {
		return err
	}
	if installedVersion == target {
		fmt.Fprintf(out, "%s is already at %s; nothing to do.\n", moduleName, target)
		return nil
	}

	// Has anyone edited this module? Answering needs the version it was installed
	// from, built as a reference exactly as `marketplace diff` does.
	base, err := pickVersion(versions.Items, installedVersionID, installedVersion)
	if err != nil {
		return err
	}
	baseRef, basePkgModule, err := referenceFor(ctx, client, base, mendixVersion, work, "base")
	if err != nil {
		return err
	}
	installed, err := marketplace.SnapshotModule(mprPath, moduleName, newBackendFactory())
	if err != nil {
		return fmt.Errorf("read %s from the project: %w", moduleName, err)
	}
	published, err := marketplace.SnapshotModule(baseRef, basePkgModule, newBackendFactory())
	if err != nil {
		return fmt.Errorf("read %s from its published package: %w", basePkgModule, err)
	}
	drift := marketplace.Compare(installed, published)

	if saveEdits != "" {
		written, unsaved, serr := marketplace.SaveEdits(saveEdits, drift)
		if serr != nil {
			return serr
		}
		reportSavedEdits(out, saveEdits, written, unsaved)
	}

	if err := gateOnLocalEdits(out, drift, force, saveEdits); err != nil {
		return err
	}

	// Build the version being moved to, and replace.
	targetVersion, err := pickVersion(versions.Items, "", target)
	if err != nil {
		return err
	}
	targetRef, _, err := referenceFor(ctx, client, targetVersion, mendixVersion, work, "target")
	if err != nil {
		return err
	}
	// referenceFor downloaded the package into the work directory under this
	// slot; the widgets are taken from it rather than from the reference project,
	// whose widgets/ also holds the blank template's.
	targetMpk := filepath.Join(work, "target.mpk")

	fmt.Fprintf(out, "\nUpdating %s %s → %s...\n", moduleName, installedVersion, target)
	res, err := marketplace.PerformUpdate(mprPath, targetRef, targetMpk, moduleName, installedVersion, target,
		targetVersion.VersionID, newBackendFactory())
	if err != nil {
		return fmt.Errorf("%w\n\nThe project may be mid-update — %s could be missing. Restore from version control",
			err, moduleName)
	}
	reportUpdate(out, res)
	return nil
}

// gateOnLocalEdits refuses an update that would destroy local work, unless the
// user has said to proceed. The refusal names the elements rather than just
// their count: "3 elements were modified" is not enough to decide with.
func gateOnLocalEdits(out io.Writer, drift *marketplace.Report, force bool, saveEdits string) error {
	result := marketplace.NewDiffResult("", "", "", drift)
	edited := append(append([]string{}, result.Modified...), result.OnlyInstalled...)

	if len(edited) == 0 {
		if !result.Verified {
			// Unknown elements are not proof of safety. Say so, and let --force
			// carry the decision, rather than reporting a clean bill of health.
			if !force {
				return fmt.Errorf("%d element(s) could not be read, so it cannot be shown that the module is unedited.\n"+
					"Re-run with --force to update anyway", len(result.Unknown))
			}
			fmt.Fprintf(out, "Note: %d element(s) could not be read; proceeding under --force.\n", len(result.Unknown))
		}
		return nil
	}

	if !force {
		msg := fmt.Sprintf("refusing to update: %d element(s) have been changed locally and the update would discard them:\n", len(edited))
		for _, e := range edited {
			msg += "    " + e + "\n"
		}
		if saveEdits == "" {
			msg += "\n  - Save them first:  --save-edits <dir>\n  - Then update:      --force"
		} else {
			msg += "\n  Saved above. Re-run with --force to proceed."
		}
		return fmt.Errorf("%s", msg)
	}

	fmt.Fprintf(out, "Proceeding under --force; %d locally changed element(s) will be replaced:\n", len(edited))
	for _, e := range edited {
		fmt.Fprintf(out, "    %s\n", e)
	}
	return nil
}

func reportSavedEdits(out io.Writer, dir string, written, unsaved []string) {
	if len(written) > 0 {
		fmt.Fprintf(out, "Saved %d locally changed element(s) to %s:\n", len(written), dir)
		for _, f := range written {
			fmt.Fprintf(out, "    %s\n", f)
		}
		fmt.Fprintln(out, "  Replay after the update with 'mxcli exec <file> -p <project.mpr>'.")
		fmt.Fprintln(out, "  These are resulting states, not diffs: an edit that REMOVED something")
		fmt.Fprintln(out, "  will not be restored by replaying them.")
	}
	if len(unsaved) > 0 {
		fmt.Fprintf(out, "\n  Could not be saved (%d) — these edits are not recoverable this way:\n", len(unsaved))
		for _, u := range unsaved {
			fmt.Fprintf(out, "    %s\n", u)
		}
	}
}

func reportUpdate(out io.Writer, r *marketplace.UpdateResult) {
	fmt.Fprintf(out, "\n%s updated %s → %s\n", r.Module, r.FromVersion, r.ToVersion)
	fmt.Fprintf(out, "  %d units copied, %d element identities preserved, %d role grant(s) restored.\n",
		r.UnitsCopied, r.IdentitiesKept, r.GrantsRestored)
	if len(r.WidgetsInstalled) > 0 {
		fmt.Fprintf(out, "  %d widget package(s) replaced in widgets/.\n", len(r.WidgetsInstalled))
	}

	if len(r.IdentitiesLost) > 0 {
		fmt.Fprintf(out, "\n  Removed in %s (%d) — their database columns or tables will go on the next deploy:\n",
			r.ToVersion, len(r.IdentitiesLost))
		for _, e := range r.IdentitiesLost {
			fmt.Fprintf(out, "    %s\n", e)
		}
	}
	if len(r.GrantsDropped) > 0 {
		fmt.Fprintf(out, "\n  Role grants that could not be restored (%d) — those users lose that access:\n",
			len(r.GrantsDropped))
		for _, g := range r.GrantsDropped {
			fmt.Fprintf(out, "    %s\n", g)
		}
	}
	// A newer module's pages reference widget definitions the project has not
	// resynced, so `mx check` reports CE0463 until it is told to. Measured on
	// Administration 4.3.2 → 4.5.0: 11 CE0463 errors, and 0 after update-widgets.
	// Saying so here is the difference between a two-command fix and a day in
	// diagnose-ce0463.md, which is where that error normally leads.
	fmt.Fprintln(out, "\n  Next: resync widget definitions, or 'mx check' will report CE0463 on the")
	fmt.Fprintln(out, "  new version's pages (this is expected after any headless module install):")
	fmt.Fprintln(out, "      mx update-widgets <project.mpr>")
	fmt.Fprintln(out, "\n  Then check the app. A newer module can need newer companions — measured on")
	fmt.Fprintln(out, "  DataWidgets 3.11.3, whose widgets want design properties an older Atlas does")
	fmt.Fprintln(out, "  not define (29 × CE6083). That is a dependency to resolve, not something")
	fmt.Fprintln(out, "  this update can fix:")
	fmt.Fprintln(out, "      mxcli docker check -p <project.mpr>")
	fmt.Fprintln(out, "\n  Review the change with 'mxcli diff-local'.")
}

func init() {
	marketplaceUpdateCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	marketplaceUpdateCmd.Flags().String("to", "", "version to update to (required)")
	marketplaceUpdateCmd.Flags().String("module", "", "module name in the project, when it cannot be identified automatically")
	marketplaceUpdateCmd.Flags().String("save-edits", "", "write locally changed elements to this directory as re-executable MDL")
	marketplaceUpdateCmd.Flags().Bool("force", false, "update even though local edits will be discarded")
	marketplaceUpdateCmd.Flags().String("profile", auth.ProfileDefault, "credential profile")

	marketplaceCmd.AddCommand(marketplaceUpdateCmd)
}
