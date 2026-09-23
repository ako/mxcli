// SPDX-License-Identifier: Apache-2.0

// Package version provides Mendix project version detection and handling.
package version

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// ProjectVersion is an alias for types.ProjectVersion, the canonical
// declaration. All its methods — IsAtLeast, IsAtLeastFull, String, IsMPRv2 —
// are defined there.
//
// It is an ALIAS and not a struct of its own on purpose. This package used to
// declare a duplicate whose fields matched types.ProjectVersion exactly, which
// made the two unrelated Go types that print under the same name: a value could
// not cross the mdl/ ↔ modelsdk/ boundary without a conversion, and a mismatch
// reported itself as a tautology rather than as a type error anyone could read.
// The sdk/mpr copy deleted in the legacy-engine retirement aliased the canonical
// type; this one did not, and that asymmetry is what CLAUDE.md's shared-types
// rule exists to prevent.
type ProjectVersion = types.ProjectVersion

// DefaultVersion returns the default version (11.6.0) used when detection fails.
func DefaultVersion() *ProjectVersion {
	return &ProjectVersion{
		ProductVersion: "11.6.0",
		BuildVersion:   "11.6.0",
		FormatVersion:  2,
		MajorVersion:   11,
		MinorVersion:   6,
		PatchVersion:   0,
	}
}

// DetectFromDB reads version information from the MPR database.
func DetectFromDB(db *sql.DB) (*ProjectVersion, error) {
	var formatVersion int
	var productVersion, buildVersion, schemaHash string

	// Try the old schema first (with _FormatVersion)
	row := db.QueryRow("SELECT _FormatVersion, _ProductVersion, _BuildVersion, _SchemaHash FROM _MetaData LIMIT 1")
	err := row.Scan(&formatVersion, &productVersion, &buildVersion, &schemaHash)
	if err != nil {
		if err == sql.ErrNoRows {
			// Return default if no metadata found
			return DefaultVersion(), nil
		}
		// Try new schema without _FormatVersion (Mendix 11.6.2+)
		row = db.QueryRow("SELECT _ProductVersion, _BuildVersion, _SchemaHash FROM _MetaData LIMIT 1")
		err = row.Scan(&productVersion, &buildVersion, &schemaHash)
		if err != nil {
			if err == sql.ErrNoRows {
				return DefaultVersion(), nil
			}
			return nil, fmt.Errorf("failed to read version metadata: %w", err)
		}
		// Default format version to 2 for newer schemas
		formatVersion = 2
	}

	pv := &ProjectVersion{
		ProductVersion: productVersion,
		BuildVersion:   buildVersion,
		FormatVersion:  formatVersion,
		SchemaHash:     schemaHash,
	}

	// Parse version components
	pv.MajorVersion, pv.MinorVersion, pv.PatchVersion = parseVersion(productVersion)

	return pv, nil
}

// parseVersion extracts major, minor, patch from a version string like "10.18.0"
func parseVersion(version string) (major, minor, patch int) {
	parts := strings.Split(version, ".")
	if len(parts) >= 1 {
		major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 {
		patch, _ = strconv.Atoi(parts[2])
	}
	return
}

// Removed with the alias: IsSupported, SupportsFeature, Feature and its
// constants, MinVersion, featureVersions and SupportedVersionRange.
//
// They could not survive as methods on an aliased type, and nothing called
// them — measured, zero references outside their own declarations, in this
// package or any other. They were also a second, hand-maintained copy of the
// feature registry: the live one is sdk/versions/mendix-{9,10,11}.yaml, read
// through checkFeature in mdl/executor/cmd_features.go, and featureVersions
// described itself as "the fallback when the YAML registry is unavailable" —
// a fallback with no caller is a list that can only drift.
