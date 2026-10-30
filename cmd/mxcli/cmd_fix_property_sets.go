// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	mpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/spf13/cobra"
)

// cmd_fix_property_sets.go completes the property sets of units already stored
// (ako/mxcli#1097). Like `fix hashes` it needs no mx: the normalization is
// mxcli's own, the same one every write applies.

var fixPropertySetsCmd = &cobra.Command{
	Use:   "property-sets",
	Short: "Give every stored unit the property names Studio Pro writes (unblocks mx diff / merge)",
	Long: `Normalize the property names of every unit already in the project.

Mendix's merge and diff engine compares two revisions of an element by its
property NAMES and throws when they differ:

  Objects with ID … of type Microflows$RestOperationCallAction do not have
  the same properties.

The model is valid — mx check passes and Studio Pro opens it — but every diff,
merge and the Version Control panel fail once Studio Pro has saved the
document with its full property set.

Every mxcli write already gives a unit that set for the project's Mendix
version: missing keys are added with the value Mendix gives an absent one, and
empty keys the version does not declare yet are dropped. A unit written by an
older mxcli and never rewritten since keeps its gaps. This command applies the
same normalization to every stored unit, and writes the units whose property
names change. It adds and removes keys only: no element $ID moves and no
existing GUID changes. The one GUID it adds is on a project user role that has
none (older mxcli wrote them without): the role's own $ID, the value a role
mxcli creates gets — without it the build invents a random GUID each time and
the runtime can only find the role's row by name. A second run writes nothing.

The repair works from its own commit on: a merge whose base is an older
revision still fails on every document that was incomplete there and that both
branches changed. Run it once on the main branch, in a commit of its own, and
branch from there. To get an older branch past such a merge: abort it (git
merge --abort); list the documents with --dry-run on a checkout of the merge
base; put the ones your branch changed back to their merge-base version (git
checkout <base> -- <unit file>, then mxcli fix hashes --repair) and commit;
merge again in Studio Pro; redo those edits. Never delete and re-create an
entity, attribute or association to get past it: its GUID is the database's
identity for it.

A unit holding a value in a key the project's version does not declare is
reported and left alone — dropping the value would change the model. Like every
mxcli write, the repair is refused while Studio Pro has the project open.

The keys it adds come from values measured on Mendix versions. A key no
measurement covers on the project's version is not filled, but it is found:
Studio Pro writes the same property names on every element of a type, so a
name some elements of a type carry and others lack is listed as MIXED, with the
units on the smaller side, and left as it is. Report those with your Mendix
version so the value can be measured.

Exits 1 when a unit could not be repaired, when a MIXED name remains, or, with
--dry-run, when a unit needs the repair.`,
	Example: `  mxcli fix property-sets -p app.mpr --dry-run
  mxcli fix property-sets -p app.mpr`,
	Args:          cobra.NoArgs,
	RunE:          runFixPropertySets,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	fixPropertySetsCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	_ = fixPropertySetsCmd.MarkFlagRequired("project")
	fixPropertySetsCmd.Flags().Bool("dry-run", false, "report the units that would change, write nothing")
	fixCmd.AddCommand(fixPropertySetsCmd)
}

// errPropertySetsRemain makes the command exit 1 after it printed the report.
var errPropertySetsRemain = errors.New("property-set gaps remain")

func runFixPropertySets(cmd *cobra.Command, _ []string) error {
	mprPath, _ := cmd.Flags().GetString("project")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if _, err := os.Stat(mprPath); err != nil {
		return fmt.Errorf("project not found: %s", mprPath)
	}
	err := fixPropertySets(mprPath, dryRun, cmd.OutOrStdout())
	if errors.Is(err, errPropertySetsRemain) {
		os.Exit(1)
	}
	return err
}

// fixPropertySets is the command without its exit: it prints the report and
// returns errPropertySetsRemain when a unit still differs afterwards.
func fixPropertySets(mprPath string, dryRun bool, out io.Writer) error {
	var (
		r *mpr.Reader
		w *mpr.Writer
	)
	if dryRun {
		var err error
		if r, err = mpr.Open(mprPath); err != nil {
			return fmt.Errorf("open project: %w", err)
		}
		defer r.Close()
	} else {
		var err error
		if w, err = mpr.NewWriter(mprPath); err != nil {
			return fmt.Errorf("open project: %w", err)
		}
		defer w.Close()
		r = w.ConcreteReader()
	}

	pv := r.ProjectVersion()
	if pv == nil || pv.MajorVersion == 0 {
		fmt.Fprintf(out, "%s has no readable Mendix version; nothing is normalized without one.\n", filepath.Base(mprPath))
		return nil
	}
	ids, err := r.ListAllUnitIDs()
	if err != nil {
		return fmt.Errorf("list units: %w", err)
	}
	sort.Strings(ids)

	var (
		changed        []mpr.PropertySetRepair
		failed         []string
		added, removed int
		census         = mpr.NewPropertySetCensus()
	)
	for _, id := range ids {
		var rep mpr.PropertySetRepair
		if dryRun {
			rep, err = r.StoredPropertySetGaps(id)
		} else {
			rep, err = w.CompleteStoredPropertySets(id)
		}
		if err != nil {
			failed = append(failed, fmt.Sprintf("  FAILED    %s%s: %v", id, repairLabel(rep), err))
			continue
		}
		census.Count(rep)
		if rep.Changed() {
			changed = append(changed, rep)
			added += rep.Added
			removed += rep.Stripped
		}
	}
	mixed := census.Mixed()

	verb := "Completed"
	if dryRun {
		verb = "Would complete"
	}
	fmt.Fprintf(out, "Checked %d unit(s) in %s against the Mendix %d.%d.%d property sets.\n",
		len(ids), filepath.Base(mprPath), pv.MajorVersion, pv.MinorVersion, pv.PatchVersion)
	for _, rep := range changed {
		fmt.Fprintf(out, "  %-9s %s%s: +%d key(s), -%d key(s)\n", map[bool]string{true: "NEEDS", false: "FIXED"}[dryRun],
			rep.UnitID, repairLabel(rep), rep.Added, rep.Stripped)
	}
	for _, f := range failed {
		fmt.Fprintln(out, f)
	}
	switch {
	case len(changed) == 0 && len(failed) == 0 && len(mixed) == 0:
		fmt.Fprintln(out, "Every unit already has the property names Studio Pro writes.")
	case len(changed) > 0:
		fmt.Fprintf(out, "\n%s %d unit(s): %d key(s) added, %d stripped.\n", verb, len(changed), added, removed)
	}
	if dryRun && len(changed) > 0 {
		fmt.Fprintf(out, "Run 'mxcli fix property-sets -p %s' to write them.\n", mprPath)
	}
	if len(failed) > 0 {
		fmt.Fprintf(out, "%d unit(s) could not be normalized and were left as they are.\n", len(failed))
	}
	reportMixedPropertyNames(out, mixed, pv.MajorVersion, pv.MinorVersion, pv.PatchVersion)
	if len(failed) > 0 || len(mixed) > 0 || (dryRun && len(changed) > 0) {
		return errPropertySetsRemain
	}
	return nil
}

// reportMixedPropertyNames lists the property names the elements of one $Type
// disagree on after the repair (ako/mxcli#1505): Studio Pro writes the same
// names on every element of a type, so each is a pair of revisions Mendix's
// merge engine refuses to compare. The repair fills only keys with a value
// measured on a Mendix version, so these are reported and left alone.
func reportMixedPropertyNames(out io.Writer, mixed []mpr.MixedPropertyName, major, minor, patch int) {
	if len(mixed) == 0 {
		return
	}
	fmt.Fprintf(out, "\nProperty names that differ between elements of one type (no value measured on Mendix %d.%d.%d fills them):\n",
		major, minor, patch)
	for _, m := range mixed {
		side := "only on"
		if m.Without() {
			side = "without it"
		}
		var units []string
		for _, u := range m.Examples {
			units = append(units, u.UnitID+repairLabel(u))
		}
		fmt.Fprintf(out, "  MIXED     %s.%s: on %d of %d element(s); %s: %s\n",
			m.Type, m.Key, m.With, m.Elements, side, strings.Join(units, ", "))
	}
	fmt.Fprintf(out, "%d property name(s) left as they are. Mendix's merge engine cannot compare a revision of these "+
		"elements with one that has the other set of names. mxcli fills a key only with the value Studio Pro was "+
		"measured writing on a Mendix version; report the names above with your Mendix version so it can be measured.\n",
		len(mixed))
}

// repairLabel names the unit (" [Microflows$Microflow Name]"), or "".
func repairLabel(rep mpr.PropertySetRepair) string {
	switch {
	case rep.Type == "":
		return ""
	case rep.Name == "":
		return " [" + rep.Type + "]"
	}
	return " [" + rep.Type + " " + rep.Name + "]"
}
