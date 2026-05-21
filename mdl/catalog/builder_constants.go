// SPDX-License-Identifier: Apache-2.0

package catalog

import "strings"

func (b *Builder) buildConstants() error {
	constants, err := b.reader.ListConstants()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO constants_data (Id, Name, QualifiedName, ModuleName, Folder, Description, DataType,
			DefaultValue, ExposedToClient,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	for _, c := range constants {
		moduleID := b.hierarchy.findModuleID(c.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + c.Name
		folderPath := b.hierarchy.buildFolderPath(c.ContainerID)

		dataType := c.Type.Kind
		if c.Type.Kind == "Enumeration" && c.Type.EnumRef != "" {
			dataType = "Enumeration(" + c.Type.EnumRef + ")"
		}

		exposed := 0
		if c.ExposedToClient {
			exposed = 1
		}

		_, err := stmt.Exec(
			string(c.ID),
			c.Name,
			qualifiedName,
			moduleName,
			folderPath,
			c.Documentation,
			dataType,
			c.DefaultValue,
			exposed,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}
	}

	b.report("Constants", len(constants))
	return nil
}

func (b *Builder) buildConstantValues() error {
	ps, err := b.reader.GetProjectSettings()
	if err != nil {
		// Project settings may not exist — not an error
		return nil
	}

	if ps.Configuration == nil {
		return nil
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO constant_values (ConstantName, ConfigurationName, Value, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID := b.catalog.projectID
	snapshotID := b.snapshot.ID

	count := 0
	for _, cfg := range ps.Configuration.Configurations {
		for _, cv := range cfg.ConstantValues {
			constName := cv.ConstantId
			// Normalize: some MPR files store without module prefix
			if !strings.Contains(constName, ".") {
				continue
			}
			_, err := stmt.Exec(constName, cfg.Name, cv.Value, projectID, snapshotID)
			if err != nil {
				return err
			}
			count++
		}
	}

	b.report("ConstantValues", count)
	return nil
}
