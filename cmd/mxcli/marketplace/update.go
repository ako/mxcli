// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"go.mongodb.org/mongo-driver/bson"
)

// SaveEdits writes each locally modified element's current definition to dir as
// re-executable MDL, and returns the files written.
//
// This is what makes `--force` recoverable: the update discards local edits, so
// parking them first turns "your work is gone" into "replay these and review the
// result". The text is the same DESCRIBE output the comparison ran on, which
// re-executes — an entity comes back as `create or modify persistent entity ...`
// with its access rules, and `mxcli check` accepts it.
//
// Two limits are inherent and are the caller's to communicate:
//
//   - `create or modify` MERGES. An edit that *removed* something — a deleted
//     attribute, a tightened grant — is not in this text, because the text is
//     the resulting state rather than a diff. Additions and changes replay;
//     removals do not.
//   - An element that could not be described has nothing to save. Those are
//     reported rather than skipped, because "no file written" and "nothing was
//     changed here" must not look the same.
func SaveEdits(dir string, rep *Report) (written []string, unsaved []string, err error) {
	var wanted []Finding
	for _, f := range rep.Findings {
		switch f.Verdict {
		case Modified, OnlyInstalled:
			wanted = append(wanted, f)
		case Unknown:
			unsaved = append(unsaved, fmt.Sprintf("%s (%s)", f.Key, f.Reason))
		}
	}
	if len(wanted) == 0 {
		return nil, unsaved, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, unsaved, fmt.Errorf("create %s: %w", dir, err)
	}

	for _, f := range wanted {
		body := replayable(f.InstalledMDL)
		if strings.TrimSpace(body) == "" {
			unsaved = append(unsaved, f.Key.String()+" (no re-executable definition)")
			continue
		}
		name := filepath.Join(dir, safeFileName(f.Key)+".mdl")
		header := fmt.Sprintf("-- %s, as it stands in your project before the update.\n"+
			"-- Replay with: mxcli exec %s -p <project.mpr>\n"+
			"-- Note: this is the element's resulting state, not a diff — an edit that\n"+
			"-- REMOVED something will not be restored by replaying it.\n\n", f.Key, name)
		if err := os.WriteFile(name, []byte(header+body+"\n"), 0o644); err != nil {
			return written, unsaved, fmt.Errorf("write %s: %w", name, err)
		}
		written = append(written, name)
	}
	return written, unsaved, nil
}

// trailingSeparator matches the lone "/" the executor prints after DESCRIBE
// output. It is harmless in a comparison, where both sides carry it, but it is
// noise in a file a user is meant to replay.
var trailingSeparator = regexp.MustCompile(`(?m)^\s*/\s*$`)

func replayable(mdl string) string {
	return strings.TrimSpace(trailingSeparator.ReplaceAllString(mdl, ""))
}

// safeFileName turns an element key into something usable on disk.
func safeFileName(k ElementKey) string {
	s := strings.ToLower(k.Type) + "-" + k.Name
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, s)
}

// UpdateResult is what an update did.
type UpdateResult struct {
	Module          string
	FromVersion     string
	ToVersion       string
	UnitsCopied     int
	IdentitiesKept  int
	IdentitiesLost  []string
	GrantsRestored  int
	GrantsDropped   []string
	FilesInstalled  []string
	ForcedOverEdits []string
}

// PerformUpdate replaces an installed module with the copy in referenceMpr,
// preserving the two things that do not survive a plain replace: the `GUID`s the
// database keys on (§8) and the user-role grants of the module's roles.
//
// The order is not arbitrary. Identities and grants are read *before* the module
// is removed, because removing it destroys both. The module is then dropped
// through mxcli's own DROP MODULE so project references are unpicked cleanly,
// the replacement is copied in with mxcli's writer (never `mx module-import`,
// which would rewrite an MPR v2 project as v1), and only then are identity and
// access put back.
//
// A failure partway leaves the project in a broken state. The caller must work
// on a copy, or hold a backup: this does not roll back.
func PerformUpdate(mprPath, referenceMpr, targetMpk, moduleName, fromVersion, toVersion, toVersionID string,
	newBackend func() backend.FullBackend) (*UpdateResult, error) {

	ids, err := CaptureIdentities(mprPath, moduleName)
	if err != nil {
		return nil, fmt.Errorf("record the identities the database keys on: %w", err)
	}
	grants, err := CaptureRoleGrants(mprPath, moduleName)
	if err != nil {
		return nil, fmt.Errorf("record role grants: %w", err)
	}

	if err := execStatements(mprPath, "drop module "+moduleName+";", newBackend); err != nil {
		return nil, fmt.Errorf("remove the installed module: %w", err)
	}

	copied, err := TransplantModule(referenceMpr, mprPath, moduleName)
	if err != nil {
		return nil, fmt.Errorf("copy in the new version (the project no longer has the module): %w", err)
	}

	applied, missing, err := ApplyIdentities(mprPath, moduleName, ids)
	if err != nil {
		return nil, fmt.Errorf("restore identities (the project's data mapping is at risk): %w", err)
	}
	restored, dropped, err := RestoreRoleGrants(mprPath, moduleName, grants, newBackend)
	if err != nil {
		return nil, fmt.Errorf("restore role grants: %w", err)
	}
	if err := StampMarketplaceVersion(mprPath, moduleName, toVersion, toVersionID); err != nil {
		return nil, fmt.Errorf("record the installed version: %w", err)
	}
	files, err := InstallPackageFiles(targetMpk, filepath.Dir(mprPath))
	if err != nil {
		return nil, fmt.Errorf("install the new version's bundled files: %w", err)
	}

	return &UpdateResult{
		Module:         moduleName,
		FromVersion:    fromVersion,
		ToVersion:      toVersion,
		UnitsCopied:    copied,
		IdentitiesKept: applied,
		IdentitiesLost: missing,
		GrantsRestored: restored,
		GrantsDropped:  dropped,
		FilesInstalled: files,
	}, nil
}

// StampMarketplaceVersion records which published version a module now is.
//
// Necessary because the reference project is built with `mx module-import`,
// which stamps the module with the *author's internal* version rather than the
// marketplace release number: importing Administration 4.5.0 leaves
// AppStoreVersion "2.0.1" and an unrelated AppStoreGuid. Transplanting carries
// that stamp across, so without this the project claims a version it does not
// have.
//
// It is not cosmetic. `marketplace diff` and `marketplace update` identify a
// module by its AppStoreGuid — the marketplace *version* UUID — so a wrong stamp
// makes the module unrecognisable to the next update, which then reports that no
// module in the project came from this content.
func StampMarketplaceVersion(mprPath, moduleName, versionNumber, versionID string) error {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", mprPath, err)
	}
	units, err := reader.ListRawUnitsByType("Projects$ModuleImpl")
	if err != nil {
		reader.Close()
		return fmt.Errorf("read module documents: %w", err)
	}

	var unitID string
	var contents []byte
	for _, u := range units {
		var doc bson.D
		if bson.Unmarshal(u.Contents, &doc) != nil {
			continue
		}
		name, _, _ := nameAndGUID(doc)
		if !strings.EqualFold(name, moduleName) {
			continue
		}
		setStringField(doc, "AppStoreVersion", versionNumber)
		setStringField(doc, "AppStoreGuid", versionID)
		setBoolField(doc, "FromAppStore", true)
		enc, merr := bson.Marshal(doc)
		if merr != nil {
			reader.Close()
			return fmt.Errorf("re-encode module document: %w", merr)
		}
		unitID, contents = string(u.ID), enc
		break
	}
	reader.Close()

	if unitID == "" {
		return fmt.Errorf("module %q not found when stamping its version", moduleName)
	}
	writer, err := modelsdk.OpenForWriting(mprPath)
	if err != nil {
		return fmt.Errorf("open %s for writing: %w", mprPath, err)
	}
	defer writer.Close()
	return writer.UpdateRawUnit(unitID, contents)
}

// setStringField assigns an existing key, and only an existing key. Inventing a
// property Mendix does not store for this version is the failure mode ADR-0005
// warns about: mxbuild tolerates it and Studio Pro refuses to open the document.
func setStringField(doc bson.D, key, value string) {
	for i, e := range doc {
		if e.Key == key {
			doc[i].Value = value
			return
		}
	}
}

func setBoolField(doc bson.D, key string, value bool) {
	for i, e := range doc {
		if e.Key == key {
			doc[i].Value = value
			return
		}
	}
}

// InstallPackageFiles copies every non-model file a package ships into the
// project, and reports what it wrote.
//
// A module is not only its model. The .mpk carries widget binaries under
// widgets/, styling and design-property declarations under themesource/, and
// whatever else the module needs; only project.mpr and package.xml are
// manifest rather than payload. An update that moves the model alone leaves all
// of it at the old version.
//
// Both halves of that were measured on DataWidgets 3.5.0 → 3.11.3, one after the
// other:
//
//   - all ten widget binaries stayed at 3.5.0 while the model said 3.11.3, so
//     the app ran old widget code with new definitions;
//   - and 29 × CE6083 ("design property not supported by your theme") persisted
//     through `mx update-widgets` AND `mx rename-design-properties`, because the
//     properties Gallery wants are declared in the module's *own*
//     themesource/datawidgets/web/design-properties.json, which was still the
//     3.5.0 copy.
//
// The second looked like a cross-module dependency on a newer Atlas. It was not.
// Copying everything the package ships, rather than enumerating the directories
// that seem to matter, is what stops there being a third instance.
func InstallPackageFiles(mpkPath, projectDir string) (written []string, err error) {
	zr, err := zip.OpenReader(mpkPath)
	if err != nil {
		return nil, fmt.Errorf("open package %s: %w", filepath.Base(mpkPath), err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		switch f.Name {
		case packageProjectEntry, "package.xml":
			continue // the model and its manifest, handled by the transplant
		}
		// Refuse a path that escapes the project. Nothing in a Mendix package
		// should contain "..", and honouring one would let a package write
		// anywhere on disk.
		clean := filepath.Clean(f.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return written, fmt.Errorf("package entry %q would write outside the project", f.Name)
		}

		dst := filepath.Join(projectDir, clean)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return written, fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
		}
		rc, oerr := f.Open()
		if oerr != nil {
			return written, fmt.Errorf("read %s from the package: %w", f.Name, oerr)
		}
		body, rerr := io.ReadAll(rc)
		_ = rc.Close()
		if rerr != nil {
			return written, fmt.Errorf("read %s: %w", f.Name, rerr)
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return written, fmt.Errorf("write %s: %w", dst, err)
		}
		written = append(written, clean)
	}
	sort.Strings(written)
	return written, nil
}
