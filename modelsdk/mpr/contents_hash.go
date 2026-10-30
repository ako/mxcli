// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ContentsHash verification (ako/mxcli#972, #1159).
//
// In MPR v2 every Unit row carries ContentsHash = base64(SHA-256(<unit>.mxunit)).
// The writer keeps the two in step (WriteTransaction.WriteUnit, updateUnit), but
// anything that changes a .mxunit behind its back does not: restoring one with
// `git checkout` leaves the row describing bytes that are no longer on disk, and
// `mx check` does not notice. VerifyContentsHashes finds the drift and
// RepairContentsHashes rewrites the index from the files.
//
// MPR v1 keeps the contents in the row, and Studio Pro fills ContentsHash there
// too, with the same formula over the Contents column (all 308 units of a fresh
// 9.24.30 app). Nothing outside the row can make the two drift, but a writer
// that forgets the column does: every mxcli before #1159 replaced Contents and
// left the hash describing the old bytes, and stored '' for an inserted unit.

// ContentsHashIssue is one unit whose stored hash does not describe its file
// (v2) or its Contents column (v1).
type ContentsHashIssue struct {
	UnitID string // Mendix UUID form, as everywhere else in this package
	Path   string // the .mxunit the row points at ("" on MPR v1, which has none)
	Stored string // ContentsHash in the .mpr ("" when the row has none)
	Actual string // hash of the file on disk ("" when the file is missing), or of Contents on v1
	Unit   string // "<$Type> <Name>" of the unit on MPR v1, where no file names it

	blob []byte // the row's UnitID, for the repair's UPDATE
}

// ContentsHashReport is the result of VerifyContentsHashes.
type ContentsHashReport struct {
	Version      MPRVersion          // format checked: v1 compares against Contents, v2 against files
	Units        int                 // rows in the Unit table
	Files        int                 // .mxunit files under mprcontents (0 on v1)
	Mismatches   []ContentsHashIssue // file present, hash differs
	MissingFiles []ContentsHashIssue // row present, file absent (v2 only)
	OrphanFiles  []string            // file present, no row (v2 only)
}

// Clean reports whether the index and the files agree completely.
func (r *ContentsHashReport) Clean() bool {
	return len(r.Mismatches) == 0 && len(r.MissingFiles) == 0 && len(r.OrphanFiles) == 0
}

// contentsHashOf is the hash the writer stores for contents.
func contentsHashOf(contents []byte) string {
	sum := sha256.Sum256(contents)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// unitFilePath is where the writer puts a unit's file.
func unitFilePath(contentsDir string, unitIDBlob []byte) string {
	swapped := blobToUUIDSwapped(unitIDBlob)
	return filepath.Join(contentsDir, swapped[0:2], swapped[2:4], swapped+".mxunit")
}

// VerifyContentsHashes compares every Unit.ContentsHash with the SHA-256 of the
// .mxunit file it indexes, and lists .mxunit files no row indexes. On an MPR
// v1 project it compares each row's hash with its own Contents column instead.
// Read-only.
func (r *Reader) VerifyContentsHashes() (*ContentsHashReport, error) {
	if r.version != MPRVersionV2 {
		return r.verifyContentsHashesV1()
	}
	if r.contentsDir == "" {
		return nil, fmt.Errorf("MPR v2 project %s has no mprcontents folder", r.path)
	}
	rows, err := r.db.Query(`SELECT UnitID, COALESCE(ContentsHash, '') FROM Unit`)
	if err != nil {
		return nil, fmt.Errorf("query unit hashes: %w", err)
	}
	type row struct {
		blob []byte
		hash string
	}
	var units []row
	for rows.Next() {
		var rw row
		if err := rows.Scan(&rw.blob, &rw.hash); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan unit row: %w", err)
		}
		units = append(units, rw)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query unit hashes: %w", err)
	}

	rep := &ContentsHashReport{Version: MPRVersionV2, Units: len(units)}
	indexed := make(map[string]bool, len(units))
	for _, u := range units {
		if len(u.blob) != 16 {
			continue
		}
		path := unitFilePath(r.contentsDir, u.blob)
		indexed[filepath.Clean(path)] = true
		issue := ContentsHashIssue{UnitID: blobToUUID(u.blob), Path: path, Stored: u.hash, blob: u.blob}
		contents, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				rep.MissingFiles = append(rep.MissingFiles, issue)
				continue
			}
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		issue.Actual = contentsHashOf(contents)
		if issue.Actual != u.hash {
			rep.Mismatches = append(rep.Mismatches, issue)
		}
	}

	files, err := filepath.Glob(filepath.Join(r.contentsDir, "*", "*", "*.mxunit"))
	if err != nil {
		return nil, fmt.Errorf("scan mprcontents: %w", err)
	}
	rep.Files = len(files)
	for _, f := range files {
		if !indexed[filepath.Clean(f)] {
			rep.OrphanFiles = append(rep.OrphanFiles, f)
		}
	}

	sortIssues(rep.Mismatches)
	sortIssues(rep.MissingFiles)
	sort.Strings(rep.OrphanFiles)
	return rep, nil
}

// verifyContentsHashesV1 is VerifyContentsHashes for MPR v1: each row's hash
// against its own Contents. There are no files, so nothing can be missing or
// orphaned. Reads every blob, so it is for an explicit check, not every command.
func (r *Reader) verifyContentsHashesV1() (*ContentsHashReport, error) {
	rows, err := r.db.Query(`SELECT UnitID, COALESCE(ContentsHash, ''), Contents FROM Unit`)
	if err != nil {
		return nil, fmt.Errorf("query unit hashes: %w", err)
	}
	defer rows.Close()
	rep := &ContentsHashReport{Version: MPRVersionV1}
	for rows.Next() {
		var blob, contents []byte
		var hash string
		if err := rows.Scan(&blob, &hash, &contents); err != nil {
			return nil, fmt.Errorf("scan unit row: %w", err)
		}
		rep.Units++
		if len(blob) != 16 {
			continue
		}
		if actual := contentsHashOf(contents); actual != hash {
			rep.Mismatches = append(rep.Mismatches, ContentsHashIssue{
				UnitID: blobToUUID(blob), Stored: hash, Actual: actual,
				Unit: unitTypeAndName(contents), blob: blob,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query unit hashes: %w", err)
	}
	sort.Slice(rep.Mismatches, func(i, j int) bool { return rep.Mismatches[i].UnitID < rep.Mismatches[j].UnitID })
	return rep, nil
}

// unitTypeAndName is "<$Type> <Name>" of a unit document, or as much of it as
// the document has.
func unitTypeAndName(contents []byte) string {
	typ := getTypeFromContents(contents)
	if typ == "" {
		return ""
	}
	if name, ok := bson.Raw(contents).Lookup("Name").StringValueOK(); ok && name != "" {
		return typ + " " + name
	}
	return typ
}

func sortIssues(issues []ContentsHashIssue) {
	sort.Slice(issues, func(i, j int) bool { return issues[i].Path < issues[j].Path })
}

// RepairContentsHashes rewrites every mismatched ContentsHash from the file on
// disk (on MPR v1, from the row's Contents) in one SQLite transaction, and
// bumps _Transaction.LastTransactionID in that transaction as every other write
// does (Studio Pro reads it to detect an external change; v1 has no such
// table). It takes the writer's Studio Pro open-project guard: a running
// Studio Pro would overwrite the index on its next save.
//
// Missing files and orphan files are reported, not repaired: neither has a
// correct hash to write (`mxcli diag --check-units --fix` removes orphans).
// Returns the report the repair was based on and the number of rows rewritten.
func (w *Writer) RepairContentsHashes() (*ContentsHashReport, int, error) {
	rep, err := w.reader.VerifyContentsHashes()
	if err != nil {
		return nil, 0, err
	}
	if len(rep.Mismatches) == 0 {
		return rep, 0, nil
	}
	if err := w.guardWrite(); err != nil {
		return rep, 0, err
	}

	tx, err := w.reader.db.Begin()
	if err != nil {
		return rep, 0, err
	}
	n := 0
	for _, m := range rep.Mismatches {
		res, err := tx.Exec(`UPDATE Unit SET ContentsHash = ? WHERE UnitID = ?`, m.Actual, m.blob)
		if err != nil {
			_ = tx.Rollback()
			return rep, 0, fmt.Errorf("update ContentsHash of %s: %w", m.UnitID, err)
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n += int(c)
		}
	}
	if _, err := tx.Exec(`UPDATE _Transaction SET LastTransactionID = ?`, generateUUID()); err != nil &&
		!strings.Contains(err.Error(), "no such table") {
		_ = tx.Rollback()
		return rep, 0, fmt.Errorf("update _Transaction: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return rep, 0, err
	}
	w.reader.InvalidateCache()
	return rep, n, nil
}
