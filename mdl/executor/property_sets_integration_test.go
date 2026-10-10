// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mendixlabs/mxcli/internal/propertysets"
)

// propertySetLedger lists the (type, key) property-set differences the writer is
// KNOWN to still have, each with its reason. It is a ledger, not an allowlist to
// grow quietly: an entry that stops reproducing must be struck off, and a new
// difference fails the gate until it is fixed or entered here with a reason.
//
// Why the gate exists (mendixlabs/mxcli#1373): Mendix's merge and diff engine
// compares an element's property NAMES between two revisions and throws when
// they differ. A key mxcli leaves out — or writes that Mendix does not have —
// makes the document unmergeable the first time Studio Pro saves it, while
// `mx check`, the build and the runtime all stay green. `mx convert -p` is the
// reference: it re-serializes exactly the units that are not canonical, the way
// Studio Pro saves them.
var propertySetLedger = map[string]string{}

// propertySetExtraLedger is the same for keys mxcli writes that the project's
// version does not declare. Each entry here is a key introduced in a newer
// Mendix version that the writer emits unconditionally: measured extra on
// 10.24.24 and/or 11.6.8, declared on 11.12.2 and later. That is the
// version-floor defect (Studio Pro resolves every stored property against the
// type, see codec.Encoder.OmitKeys), pre-existing and tracked on its own — not a
// gap this gate closes. Strike an entry when its writer is version-gated.
var propertySetExtraLedger = map[string]string{
	"CustomWidgets$WidgetValueType.AllowUpload":          versionFloorExtra,
	"DomainModels$IndexedAttribute.AssociationPointer":   versionFloorExtra,
	"Forms$MicroflowSettings.OutputMappings":             versionFloorExtra,
	"Forms$PageVariable.SubKey":                          versionFloorExtra,
	"Forms$SnippetParameterMapping.Argument":             versionFloorExtra,
	"Navigation$NavigationProfile.ThrowPartialSyncError": versionFloorExtra,
	"Navigation$OfflineEntityConfig.CompatibilityMode":   versionFloorExtra,
	"ODataPublish$PublishedAssociationEnd.IsMany":        versionFloorExtra,
	"ODataPublish$PublishedAttribute.EdmType":            versionFloorExtra,
	"Projects$ModuleImpl.AppStorePackageIdString":        versionFloorExtra,
	"Rest$ODataEntityTypeSource.IsOpen":                  versionFloorExtra,
	"Settings$WorkflowsProjectSettingsPart.Groups":       versionFloorExtra,
}

const versionFloorExtra = "written to projects older than the key (10.24 / 11.6); version-floor defect, tracked separately"

// propertySetConvertKnownFailures lists `mx convert` refusals the gate does not
// own, keyed by a substring of convert's output, each with its reason. The
// project cannot be measured, so the script is skipped for property sets only;
// mx check above still judged it.
var propertySetConvertKnownFailures = map[string]string{
	"type with qualified name Rest$JsonBody": "mxcli writes Rest$JsonBody into projects older than the type " +
		"(10.24), so Mendix cannot load the project at all — a version-floor defect tracked on its own, not a " +
		"property-set gap",
}

// propertySetsBefore fingerprints the project's units before a script runs, so
// checkPropertySets judges only what mxcli wrote. Nil for an MPR v1 project.
func propertySetsBefore(t *testing.T, projectPath string) map[string][32]byte {
	t.Helper()
	dir := filepath.Join(filepath.Dir(projectPath), "mprcontents")
	if _, err := os.Stat(dir); err != nil {
		return nil
	}
	snap, err := propertysets.Snapshot(dir)
	if err != nil {
		t.Fatalf("snapshot units: %v", err)
	}
	return snap
}

// propertySetsRecordEnv names a file the gate appends every measurement to, for
// internal/propertysets/cmd/propdefaults to fold into the defaults table.
const propertySetsRecordEnv = "MXCLI_PROPERTY_SETS_RECORD"

var propertySetsRecordMu sync.Mutex

// checkPropertySets converts a copy of the project with `mx convert -p` and
// fails on every property Mendix adds to, or removes from, an element mxcli
// wrote. The project must be disconnected.
func checkPropertySets(t *testing.T, mxPath, projectPath, mendixVersion string, before map[string][32]byte) {
	t.Helper()
	if before == nil {
		t.Log("property sets: MPR v1 project, nothing to compare per unit")
		return
	}
	if mxPath == "" {
		t.Fatal("property sets: mx not found, but the mx check gate ran")
	}
	src := filepath.Dir(projectPath)
	dst := t.TempDir()
	t.Cleanup(func() { robustRemoveAll(dst) })
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("property sets: copy project: %v", err)
	}
	out, err := exec.Command(mxPath, "convert", "-p", "-s", dst).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "convert finished") {
		for marker, why := range propertySetConvertKnownFailures {
			if strings.Contains(string(out), marker) {
				t.Logf("property sets: not measured, mx convert refuses the project (known: %s)", why)
				return
			}
		}
		t.Fatalf("property sets: mx convert failed (%v):\n%s", err, out)
	}
	res, err := propertysets.MeasureUnits(
		filepath.Join(src, "mprcontents"), filepath.Join(dst, "mprcontents"), before)
	if err != nil {
		t.Fatalf("property sets: measure: %v", err)
	}
	if path := os.Getenv(propertySetsRecordEnv); path != "" {
		recordPropertySets(t, path, mendixVersion, res)
	}
	for _, g := range res.Gaps {
		key := g.Type + "." + g.Key
		if _, known := propertySetLedger[key]; known {
			continue
		}
		t.Errorf("property sets: mxcli wrote %s without %q (%d×, e.g. %s); Studio Pro writes %s — "+
			"Mendix's merge engine cannot compare the two revisions. Fix the writer, or record the measurement into "+
			"modelsdk/canon/studiopro_property_defaults.json (internal/propertysets/cmd/propdefaults)",
			g.Type, g.Key, g.Count, g.Example, strings.Join(g.Values, " | "))
	}
	for _, e := range res.Extras {
		key := e.Type + "." + e.Key
		if _, known := propertySetExtraLedger[key]; known {
			continue
		}
		t.Errorf("property sets: mxcli wrote %q on %s (%d×, e.g. %s), a key this Mendix version does not declare",
			e.Key, e.Type, e.Count, e.Example)
	}
}

func recordPropertySets(t *testing.T, path, mendixVersion string, res *propertysets.Result) {
	t.Helper()
	line, err := json.Marshal(propertysets.Recording{Version: mendixVersion, Result: res})
	if err != nil {
		t.Fatalf("property sets: record: %v", err)
	}
	propertySetsRecordMu.Lock()
	defer propertySetsRecordMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("property sets: record: %v", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		t.Fatalf("property sets: record: %v", err)
	}
}
