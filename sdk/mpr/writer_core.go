// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// idToBsonBinary converts a UUID string to BSON Binary format.
// For invalid or empty UUIDs (e.g. test placeholders), generates a random ID
// to maintain backward compatibility with existing serialization paths.
//
// WARNING: an empty id here is almost always a bug (e.g. an unset pointer on a
// SequenceFlow) and produces a random UUID that references nothing — which
// Studio Pro surfaces as "KeyNotFoundException". Fix callers to pass a real ID.
func idToBsonBinary(id string) primitive.Binary {
	blob := types.UUIDToBlob(id)
	if blob == nil || len(blob) != 16 {
		blob = types.UUIDToBlob(types.GenerateID())
	}
	return primitive.Binary{
		Subtype: 0x00,
		Data:    blob,
	}
}

// Writer provides methods to write Mendix project files.
type Writer struct {
	reader *Reader

	// writesOffered / writesLanded count what this session tried to persist and
	// how much of it was not skipped as a no-op (ADR-0008). Both unit writes and
	// generated source files count: a code action's body lives in
	// javascriptsource/ rather than in its unit, so counting units alone would
	// call a body-only edit unchanged. The executor reads these to tell
	// "Modified X" from "X was already in sync" — without them, re-running a
	// script that changes nothing still announces a write for every statement,
	// which is how the churn in #910 was misdiagnosed.
	writesOffered int
	writesLanded  int
}

// WriteStats reports how many writes this session offered to storage and how
// many of them actually changed something.
func (w *Writer) WriteStats() (offered, written int) {
	return w.writesOffered, w.writesLanded
}

// NewWriter creates a new writer from a reader opened in read-write mode.
func NewWriter(path string) (*Writer, error) {
	reader, err := OpenWithOptions(path, OpenOptions{ReadOnly: false})
	if err != nil {
		return nil, err
	}
	return &Writer{reader: reader}, nil
}

// Close closes the writer.
func (w *Writer) Close() error {
	return w.reader.Close()
}

// Reader returns the underlying reader.
func (w *Writer) Reader() *Reader {
	return w.reader
}

// Transaction support

// Transaction represents a database transaction.
type Transaction struct {
	tx     *sql.Tx
	writer *Writer
}

// BeginTransaction starts a new transaction.
func (w *Writer) BeginTransaction() (*Transaction, error) {
	tx, err := w.reader.db.Begin()
	if err != nil {
		return nil, err
	}
	return &Transaction{tx: tx, writer: w}, nil
}

// Commit commits the transaction.
func (t *Transaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction.
func (t *Transaction) Rollback() error {
	return t.tx.Rollback()
}

// WriteTransaction provides atomic write operations for MPR v2 format.
// It coordinates database and file system changes to ensure consistency.
type WriteTransaction struct {
	tx           *sql.Tx
	writer       *Writer
	pendingFiles []pendingFile
	finalized    []finalizedFile
	committed    bool
}

type pendingFile struct {
	unitID    string
	tempPath  string
	finalPath string
}

// finalizedFile records a rename that has already happened, so it can be undone
// if a later step of the same Commit fails. backupPath is empty when the unit
// had no file on disk to preserve.
type finalizedFile struct {
	pendingFile
	backupPath string
}

// BeginWriteTransaction starts a new write transaction.
// For v2 format, this coordinates both database and file writes.
func (w *Writer) BeginWriteTransaction() (*WriteTransaction, error) {
	tx, err := w.reader.db.Begin()
	if err != nil {
		return nil, err
	}
	return &WriteTransaction{
		tx:           tx,
		writer:       w,
		pendingFiles: make([]pendingFile, 0),
	}, nil
}

// WriteUnit writes a unit within the transaction.
// The actual file write is deferred until Commit.
func (wt *WriteTransaction) WriteUnit(unitID string, contents []byte) error {
	unitIDBlob := uuidToBlob(unitID)

	if wt.writer.reader.version == MPRVersionV2 {
		swappedUUID := blobToUUIDSwapped(unitIDBlob)

		// Create directory if needed
		dir := filepath.Join(wt.writer.reader.contentsDir, swappedUUID[0:2], swappedUUID[2:4])
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Write to temp file first
		finalPath := filepath.Join(dir, swappedUUID+".mxunit")
		tempPath := finalPath + ".tmp"

		if err := os.WriteFile(tempPath, contents, 0644); err != nil {
			return fmt.Errorf("failed to write temp file: %w", err)
		}

		wt.pendingFiles = append(wt.pendingFiles, pendingFile{
			unitID:    unitID,
			tempPath:  tempPath,
			finalPath: finalPath,
		})

		contentsHash := contentHashBase64(contents)
		_, err := wt.tx.Exec(`
			UPDATE Unit SET ContentsHash = ? WHERE UnitID = ?
		`, contentsHash, unitIDBlob)
		return err
	}

	// V1: Update in database directly
	contentsHash := contentHashBase64(contents)
	_, err := wt.tx.Exec(`
		UPDATE Unit SET Contents = ?, ContentsHash = ? WHERE UnitID = ?
	`, contents, contentsHash, unitIDBlob)
	if err != nil && isContentsHashSchemaError(err) {
		// Older v1 schemas do not have ContentsHash; retry without it.
		// Any other error (disk full, invalid UnitID, rolled-back tx) propagates.
		_, err = wt.tx.Exec(`
			UPDATE Unit SET Contents = ? WHERE UnitID = ?
		`, contents, unitIDBlob)
	}
	return err
}

// Commit commits the transaction.
//
// For v2 the unit files are renamed into place FIRST and the database is
// committed only once every rename has succeeded; any failure undoes the
// renames and rolls the transaction back, so the two either move together or
// not at all. The order matters: committing first and then renaming leaves the
// Unit table — ContentsHash included — describing bytes that are not on disk,
// and the old code warned about that on stdout and returned nil (upstream
// #954). A rename fails for ordinary environmental reasons — on Windows a
// .mxunit held open without FILE_SHARE_DELETE by an editor, an indexer or a
// sync client is enough.
//
// Kept in step with modelsdk/mpr's copy, which is the one storage path the
// codec engine reaches.
func (wt *WriteTransaction) Commit() error {
	if wt.committed {
		return fmt.Errorf("transaction already committed")
	}

	if err := wt.finalizeFiles(); err != nil {
		wt.undoFinalizedFiles()
		wt.cleanupTempFiles()
		_ = wt.tx.Rollback()
		return err
	}

	if err := wt.tx.Commit(); err != nil {
		wt.undoFinalizedFiles()
		wt.cleanupTempFiles()
		return err
	}

	wt.discardBackups()
	wt.committed = true
	return nil
}

// finalizeFiles renames each pending temp file into place, first moving the file
// it replaces aside so undoFinalizedFiles can put it back. It stops at the first
// failure, leaving wt.finalized describing exactly what has to be undone.
func (wt *WriteTransaction) finalizeFiles() error {
	seen := make(map[string]bool, len(wt.pendingFiles))
	for _, pf := range wt.pendingFiles {
		// A unit written twice in one transaction shares a temp path, so the
		// first rename already carried the latest bytes; a second would move the
		// file just written aside as if it were the stored one.
		if seen[pf.finalPath] {
			continue
		}
		seen[pf.finalPath] = true

		backupPath := ""
		if _, err := os.Stat(pf.finalPath); err == nil {
			backupPath = pf.finalPath + ".bak"
			if err := os.Rename(pf.finalPath, backupPath); err != nil {
				return fmt.Errorf("finalize unit %s: cannot move %s aside: %w",
					pf.unitID, pf.finalPath, err)
			}
		}
		if err := os.Rename(pf.tempPath, pf.finalPath); err != nil {
			if backupPath != "" {
				_ = os.Rename(backupPath, pf.finalPath)
			}
			return fmt.Errorf("finalize unit %s: cannot write %s: %w",
				pf.unitID, pf.finalPath, err)
		}
		wt.finalized = append(wt.finalized, finalizedFile{pendingFile: pf, backupPath: backupPath})
	}
	return nil
}

// undoFinalizedFiles reverses finalizeFiles, newest first: the new bytes go back
// to their temp path (for cleanupTempFiles to remove) and the file that was
// moved aside returns to its own name. Diagnostics go to stderr — stdout carries
// the CLI's own output and must stay parseable.
func (wt *WriteTransaction) undoFinalizedFiles() {
	for i := len(wt.finalized) - 1; i >= 0; i-- {
		f := wt.finalized[i]
		if err := os.Rename(f.finalPath, f.tempPath); err != nil {
			fmt.Fprintf(os.Stderr, "mpr: could not undo write of %s: %v\n", f.finalPath, err)
		}
		if f.backupPath != "" {
			if err := os.Rename(f.backupPath, f.finalPath); err != nil {
				fmt.Fprintf(os.Stderr, "mpr: could not restore %s: %v\n", f.finalPath, err)
			}
		}
	}
	wt.finalized = nil
}

// discardBackups drops the moved-aside files once the commit has succeeded and
// they can no longer be needed. A leftover is inert — nothing reads a path that
// is not <unit>.mxunit — so a failure here is reported, not returned.
func (wt *WriteTransaction) discardBackups() {
	for _, f := range wt.finalized {
		if f.backupPath == "" {
			continue
		}
		if err := os.Remove(f.backupPath); err != nil {
			fmt.Fprintf(os.Stderr, "mpr: could not remove %s: %v\n", f.backupPath, err)
		}
	}
	wt.finalized = nil
}

// Rollback rolls back the transaction and cleans up temp files.
func (wt *WriteTransaction) Rollback() error {
	if wt.committed {
		return fmt.Errorf("transaction already committed")
	}

	// Clean up temp files
	wt.cleanupTempFiles()

	// Rollback database
	return wt.tx.Rollback()
}

func (wt *WriteTransaction) cleanupTempFiles() {
	for _, pf := range wt.pendingFiles {
		os.Remove(pf.tempPath)
	}
}

// generateUUID delegates to types.GenerateID.
func generateUUID() string {
	return types.GenerateID()
}

// uuidToBlob delegates to types.UUIDToBlob.
func uuidToBlob(uuid string) []byte {
	return types.UUIDToBlob(uuid)
}
