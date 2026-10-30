// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	mpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/spf13/cobra"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// cmd_fix_hashes.go verifies and repairs the .mpr's ContentsHash index
// (ako/mxcli#972; MPR v1 since ako/mxcli#1159). Unlike its siblings in cmd_fix.go it needs no mx: the index
// is mxcli's own to read and write.

var fixHashesCmd = &cobra.Command{
	Use:   "hashes",
	Short: "Verify (and with --repair, rewrite) the .mpr's ContentsHash index",
	Long: `Verify that every unit's ContentsHash in the .mpr matches its contents.

In an MPR v2 project the .mpr indexes each mprcontents/**/*.mxunit file by
base64(SHA-256(file)). mxcli and Studio Pro keep the two in step, but anything
that changes a .mxunit behind their back does not: restoring a unit with
'git checkout', 'git restore' or a merge leaves the index describing bytes that
are no longer on disk. 'mx check' does not notice; Studio Pro uses the index to
decide what changed.

Reported:
  MISMATCH  the file's hash differs from the stored one (repairable)
  MISSING   the .mpr indexes a unit whose file does not exist
  ORPHAN    a .mxunit file that no unit in the .mpr indexes

--repair rewrites every mismatched hash from the file on disk, in one
transaction, and leaves the files untouched — the files are the source of
truth. MISSING and ORPHAN have no correct hash to write and are only reported
('mxcli diag --check-units --fix' removes orphan files). Like every mxcli
write, the repair is refused while Studio Pro has the project open.

An MPR v1 project (Mendix below 10.18) keeps each unit's contents inside the
.mpr, in the same row as its ContentsHash, so there are no files to drift. The
hash is still Studio Pro's to read, and earlier mxcli versions rewrote contents
without it (ako/mxcli#1159): on v1 the command compares each hash with the
contents stored beside it, and --repair rewrites it from them.

Exits 1 when an issue remains after the command (so the verify can gate CI).`,
	Example: `  mxcli fix hashes -p app.mpr
  mxcli fix hashes -p app.mpr --repair`,
	Args:          cobra.NoArgs,
	RunE:          runFixHashes,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	fixHashesCmd.Flags().StringP("project", "p", "", "path to the Mendix project (.mpr)")
	_ = fixHashesCmd.MarkFlagRequired("project")
	fixHashesCmd.Flags().Bool("repair", false, "rewrite mismatched ContentsHash values from the files on disk")
	fixCmd.AddCommand(fixHashesCmd)
}

// errHashIssuesRemain makes the command exit 1 after it printed the report.
var errHashIssuesRemain = errors.New("ContentsHash issues remain")

func runFixHashes(cmd *cobra.Command, _ []string) error {
	mprPath, _ := cmd.Flags().GetString("project")
	repair, _ := cmd.Flags().GetBool("repair")
	out := cmd.OutOrStdout()
	if _, err := os.Stat(mprPath); err != nil {
		return fmt.Errorf("project not found: %s", mprPath)
	}
	err := fixHashes(mprPath, repair, out)
	if errors.Is(err, errHashIssuesRemain) {
		cmd.SilenceErrors = true
		os.Exit(1)
	}
	return err
}

// fixHashes is the command without its exit: it prints the report and returns
// errHashIssuesRemain when the index and the files still disagree.
func fixHashes(mprPath string, repair bool, out io.Writer) error {
	var (
		rep      *mpr.ContentsHashReport
		repaired int
		err      error
	)
	if repair {
		w, werr := mpr.NewWriter(mprPath)
		if werr != nil {
			return fmt.Errorf("open project: %w", werr)
		}
		rep, repaired, err = w.RepairContentsHashes()
		_ = w.Close()
	} else {
		r, rerr := mpr.Open(mprPath)
		if rerr != nil {
			return fmt.Errorf("open project: %w", rerr)
		}
		rep, err = r.VerifyContentsHashes()
		_ = r.Close()
	}
	if rep == nil {
		return err
	}
	if rep.Version == mpr.MPRVersionV1 {
		return reportV1Hashes(mprPath, rep, repair, repaired, err, out)
	}

	fmt.Fprintf(out, "Checked %d unit(s) in %s against %d .mxunit file(s).\n", rep.Units, filepath.Base(mprPath), rep.Files)
	contentsDir := filepath.Join(filepath.Dir(mprPath), "mprcontents")
	rel := func(p string) string {
		if r, e := filepath.Rel(contentsDir, p); e == nil {
			return filepath.Join("mprcontents", r)
		}
		return p
	}
	for _, m := range rep.Mismatches {
		fmt.Fprintf(out, "  MISMATCH  %s  %s%s\n            stored %s, file %s\n", m.UnitID, rel(m.Path), unitLabel(m.Path), orNone(m.Stored), m.Actual)
	}
	for _, m := range rep.MissingFiles {
		fmt.Fprintf(out, "  MISSING   %s  %s (indexed, no file)\n", m.UnitID, rel(m.Path))
	}
	for _, f := range rep.OrphanFiles {
		fmt.Fprintf(out, "  ORPHAN    %s%s (file, not indexed)\n", rel(f), unitLabel(f))
	}
	fmt.Fprintf(out, "\n%d mismatch(es), %d missing file(s), %d orphan file(s).\n",
		len(rep.Mismatches), len(rep.MissingFiles), len(rep.OrphanFiles))

	if err != nil { // the repair itself failed (Studio Pro guard, SQLite)
		return err
	}
	remaining := len(rep.MissingFiles) + len(rep.OrphanFiles)
	switch {
	case repair && repaired > 0:
		fmt.Fprintf(out, "Repaired %d ContentsHash value(s) from the files on disk.\n", repaired)
	case !repair && len(rep.Mismatches) > 0:
		remaining += len(rep.Mismatches)
		fmt.Fprintf(out, "Run 'mxcli fix hashes -p %s --repair' to rewrite the mismatched hashes from the files.\n", mprPath)
	case rep.Clean():
		fmt.Fprintln(out, "The ContentsHash index matches every unit file.")
	}
	if len(rep.OrphanFiles) > 0 {
		fmt.Fprintf(out, "Orphan files are not indexed by the .mpr; 'mxcli diag --check-units -p %s --fix' removes them.\n", mprPath)
	}
	if len(rep.MissingFiles) > 0 {
		fmt.Fprintln(out, "Missing files cannot be repaired from here: restore them from version control.")
	}
	if remaining > 0 {
		return errHashIssuesRemain
	}
	return nil
}

// reportV1Hashes is fixHashes' report for an MPR v1 project, where each hash
// is compared with the contents in its own row: nothing can be missing or
// orphaned, and the contents are what a repair writes the hash from.
func reportV1Hashes(mprPath string, rep *mpr.ContentsHashReport, repair bool, repaired int, err error, out io.Writer) error {
	fmt.Fprintf(out, "Checked %d unit(s) in %s (MPR v1) against the contents stored beside them.\n", rep.Units, filepath.Base(mprPath))
	for _, m := range rep.Mismatches {
		label := ""
		if m.Unit != "" {
			label = " [" + m.Unit + "]"
		}
		fmt.Fprintf(out, "  MISMATCH  %s%s\n            stored %s, contents %s\n", m.UnitID, label, orNone(m.Stored), m.Actual)
	}
	fmt.Fprintf(out, "\n%d mismatch(es).\n", len(rep.Mismatches))
	if err != nil { // the repair itself failed (Studio Pro guard, SQLite)
		return err
	}
	switch {
	case repair && repaired > 0:
		fmt.Fprintf(out, "Repaired %d ContentsHash value(s) from the stored contents.\n", repaired)
	case len(rep.Mismatches) > 0:
		fmt.Fprintf(out, "Run 'mxcli fix hashes -p %s --repair' to rewrite the mismatched hashes from the stored contents.\n", mprPath)
		return errHashIssuesRemain
	default:
		fmt.Fprintln(out, "Every unit's ContentsHash matches its stored contents.")
	}
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// unitLabel names the unit a .mxunit holds (" [Microflows$Microflow Name]"),
// or "" when the file does not read as BSON.
func unitLabel(path string) string {
	b, err := os.ReadFile(path)
	if err != nil || bson.Raw(b).Validate() != nil {
		return ""
	}
	raw := bson.Raw(b)
	typ, _ := raw.Lookup("$Type").StringValueOK()
	name, _ := raw.Lookup("Name").StringValueOK()
	switch {
	case typ == "":
		return ""
	case name == "":
		return " [" + typ + "]"
	}
	return " [" + typ + " " + name + "]"
}

// contentsHashDriftWarning is the one-line warning exec and docker check print
// when the ContentsHash index disagrees with the unit files, or "" when it
// agrees. Measured on testapp (900 units): the whole `mxcli fix hashes` run,
// process start included, takes ~0.2 s, so it is cheap enough to run on every
// exec and check. It never fails the caller: any error opening or reading the
// project yields "".
//
// MPR v1 is skipped. Its hashes live in the rows with the contents, so a
// restored file cannot make them drift, and verifying them reads every blob in
// the .mpr: far more than 0.2 s on a large v1 app, on every command. `fix
// hashes` checks them when asked (ako/mxcli#1159).
func contentsHashDriftWarning(mprPath string) string {
	if mprPath == "" {
		return ""
	}
	r, err := mpr.Open(mprPath)
	if err != nil {
		return ""
	}
	defer r.Close()
	if r.Version() != mpr.MPRVersionV2 {
		return ""
	}
	rep, err := r.VerifyContentsHashes()
	if err != nil {
		return ""
	}
	n := len(rep.Mismatches) + len(rep.MissingFiles)
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("Warning: %d unit(s) in %s do not match the .mpr's ContentsHash index "+
		"(typically .mxunit files restored with git outside Studio Pro; mx check does not notice). "+
		"Run 'mxcli fix hashes -p %s' for the list, and --repair to fix it.\n",
		n, filepath.Base(mprPath), mprPath)
}
