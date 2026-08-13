// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"go.mongodb.org/mongo-driver/bson"

	regex "github.com/mendixlabs/mxcli/mdl/regularexpressions"
	"github.com/mendixlabs/mxcli/model"
)

// buildRegularExpressions catalogs RegularExpressions$RegularExpression units.
//
// It decodes with the same codec the writers use, so the catalog cannot drift
// from what is stored — notably the pattern's BSON key, which modelsdk/gen gets
// wrong (see mdl/regularexpressions).
func (b *Builder) buildRegularExpressions() error {
	units, err := b.reader.ListRawUnitsByType(regex.TypeName)
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(
		`INSERT INTO regular_expressions_data
		 (Id, Name, QualifiedName, ModuleName, Folder, Description, Expression,
		  ExportLevel, ProjectId, SnapshotId)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, u := range units {
		var doc bson.M
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			continue
		}
		re := regex.Parse(doc, model.ID(u.ID), model.ID(u.ContainerID))
		if re.Name == "" {
			continue
		}
		moduleID := b.hierarchy.findModuleID(u.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		if _, err := stmt.Exec(
			string(u.ID), re.Name, moduleName+"."+re.Name, moduleName,
			b.hierarchy.buildFolderPath(u.ContainerID), re.Documentation,
			re.Expression, re.ExportLevel, projectID, snapshotID,
		); err != nil {
			return err
		}
		count++
	}

	b.report("Regular Expressions", count)
	return b.collectRegexRuleRefs()
}

// regexRuleRef is one entity → regular-expression edge, held until
// buildReferences.
type regexRuleRef struct{ entityQualifiedName, moduleName, regexQualifiedName string }

// collectRegexRuleRefs walks stored domain models for attribute validation
// rules of type DomainModels$RegExRuleInfo and records the regex each one uses.
//
// It reads raw BSON because the shared domain-model parser normalises the rule
// to its type name ("RegEx") and drops RegExIdentifier — the qualified name that
// makes the edge.
//
// The edge is what lets `show references to <regex>` answer "which entities
// validate against this", which is the question you ask before changing a
// shared pattern. Note it does NOT affect GRAPH_DEAD_ASSETS: that view is
// restricted to ENTITY/MICROFLOW/NANOFLOW/PAGE/SNIPPET, so a regex was never
// reported dead in the first place.
func (b *Builder) collectRegexRuleRefs() error {
	units, err := b.reader.ListRawUnitsByType("DomainModels$DomainModel")
	if err != nil {
		return nil // domain models are catalogued elsewhere; a read failure is reported there
	}
	for _, u := range units {
		var doc bson.M
		if bson.Unmarshal(u.Contents, &doc) != nil {
			continue
		}
		moduleName := b.hierarchy.getModuleName(b.hierarchy.findModuleID(u.ContainerID))
		entities, _ := doc["Entities"].(bson.A)
		for _, e := range entities {
			em, ok := e.(bson.M)
			if !ok {
				continue
			}
			entityName, _ := em["Name"].(string)
			if entityName == "" {
				continue
			}
			for _, target := range regexRuleTargets(em["ValidationRules"]) {
				b.regexRuleRefs = append(b.regexRuleRefs, regexRuleRef{
					entityQualifiedName: moduleName + "." + entityName,
					moduleName:          moduleName,
					regexQualifiedName:  target,
				})
			}
		}
	}
	return nil
}

// regexRuleTargets returns the qualified name of every regex referenced by a
// validation-rule collection.
func regexRuleTargets(v any) []string {
	var out []string
	rules, _ := v.(bson.A)
	for _, r := range rules {
		rm, ok := r.(bson.M)
		if !ok {
			continue
		}
		info, ok := rm["RuleInfo"].(bson.M)
		if !ok {
			continue
		}
		if t, _ := info["$Type"].(string); t != "DomainModels$RegExRuleInfo" {
			continue
		}
		// The reference is a QUALIFIED NAME, not an element ID — confirmed
		// against Mendix Email Connector 6.4.2.
		if name, _ := info["RegExIdentifier"].(string); name != "" {
			out = append(out, name)
		}
	}
	return out
}
