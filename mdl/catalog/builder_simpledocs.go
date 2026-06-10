// SPDX-License-Identifier: Apache-2.0

package catalog

import "go.mongodb.org/mongo-driver/bson"

// buildSimpleNamedDocs catalogs documents that are plain named units (a top-level
// BSON `Name`, optionally `Documentation`) into a `<table>_data` table with the
// standard Id/Name/QualifiedName/ModuleName/Folder/Description columns.
//
// It reads through the raw-unit surface (ListRawUnitsByType) rather than a typed
// reader method, so adding a new document type to the catalog needs no change to
// the CatalogReader interface or its backend implementation — only a table, this
// call, and an objects-view union. Used for image collections, JavaScript
// actions, and data transformers, which had no catalog table before.
func (b *Builder) buildSimpleNamedDocs(typePrefix, table, reportLabel string) error {
	units, err := b.reader.ListRawUnitsByType(typePrefix)
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(
		"INSERT INTO " + table + "_data " +
			"(Id, Name, QualifiedName, ModuleName, Folder, Description, ProjectId, SnapshotId) " +
			"VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, u := range units {
		var doc struct {
			Name          string `bson:"Name"`
			Documentation string `bson:"Documentation"`
		}
		if err := bson.Unmarshal(u.Contents, &doc); err != nil || doc.Name == "" {
			continue
		}
		moduleID := b.hierarchy.findModuleID(u.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + doc.Name
		folderPath := b.hierarchy.buildFolderPath(u.ContainerID)

		if _, err := stmt.Exec(
			string(u.ID), doc.Name, qualifiedName, moduleName, folderPath,
			doc.Documentation, projectID, snapshotID,
		); err != nil {
			return err
		}
		count++
	}

	b.report(reportLabel, count)
	return nil
}
