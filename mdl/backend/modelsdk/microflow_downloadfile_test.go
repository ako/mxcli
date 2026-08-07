// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestActionFromGen_DownloadFile guards DOWNLOAD FILE rendering. Without the
// DownloadFileAction case it renders "-- Empty action". An empty error-handling
// type defaults to Rollback, matching legacy parseDownloadFileAction.
func TestActionFromGen_DownloadFile(t *testing.T) {
	act := decodeAction(t, bson.D{
		{Key: "$ID", Value: "df-1"},
		{Key: "$Type", Value: "Microflows$DownloadFileAction"},
		{Key: "FileDocumentVariableName", Value: "nodePermission"},
		// Storage key is ShowFileInBrowser (legacy's parseDownloadFileAction reads
		// the wrong "ShowInBrowser" key — a latent legacy bug; gen reads it right).
		{Key: "ShowFileInBrowser", Value: true},
	})
	df, ok := act.(*microflows.DownloadFileAction)
	if !ok {
		t.Fatalf("actionFromGen → %T, want *microflows.DownloadFileAction", act)
	}
	if df.FileDocument != "nodePermission" {
		t.Errorf("FileDocument = %q, want nodePermission", df.FileDocument)
	}
	if !df.ShowInBrowser {
		t.Error("ShowInBrowser = false, want true")
	}
	if df.ErrorHandlingType != microflows.ErrorHandlingTypeRollback {
		t.Errorf("ErrorHandlingType = %q, want Rollback default", df.ErrorHandlingType)
	}
}

// TestMicroflowRoundTrip_DownloadFile is the write-side counterpart of
// TestActionFromGen_DownloadFile, and the test that would have caught issue #850.
//
// microflowActionToGen had no *microflows.DownloadFileAction case, so the action
// fell through to `default: return nil` and the enclosing ActionActivity was
// written with no Action at all. `download file $Doc;` was accepted by the
// grammar, the visitor, the flow builder and the DESCRIBE formatter, and only
// vanished in the one stage that reports nothing — `mxcli exec` printed "Created
// microflow" and `mx check` then failed with CE0008 "No action defined."
//
// A reader-only test cannot catch this class: it starts from BSON the writer
// never had to produce. The round trip closes that gap.
func TestMicroflowRoundTrip_DownloadFile(t *testing.T) {
	for _, showInBrowser := range []bool{true, false} {
		name := "ShowInBrowser=false"
		if showInBrowser {
			name = "ShowInBrowser=true"
		}
		t.Run(name, func(t *testing.T) {
			act := &microflows.DownloadFileAction{
				FileDocument:      "Doc",
				ShowInBrowser:     showInBrowser,
				ErrorHandlingType: microflows.ErrorHandlingTypeRollback,
			}
			act.ID = model.ID("df-1")
			activity := &microflows.ActionActivity{Action: act}
			activity.ID = model.ID("act-1")

			mf := &microflows.Microflow{
				Name: "ACT_Download",
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{activity},
				},
			}
			mf.ID = model.ID("mf-1")

			got := roundTripMicroflow(t, mf)

			var found *microflows.DownloadFileAction
			for _, obj := range got.ObjectCollection.Objects {
				aa, ok := obj.(*microflows.ActionActivity)
				if !ok {
					continue
				}
				if aa.Action == nil {
					t.Fatal("ActionActivity round-tripped with a nil Action — " +
						"this is the CE0008 \"No action defined.\" shape from #850")
				}
				if df, ok := aa.Action.(*microflows.DownloadFileAction); ok {
					found = df
				}
			}
			if found == nil {
				t.Fatal("no DownloadFileAction survived the round trip")
			}
			if found.FileDocument != "Doc" {
				t.Errorf("FileDocument = %q, want Doc", found.FileDocument)
			}
			if found.ShowInBrowser != showInBrowser {
				t.Errorf("ShowInBrowser = %v, want %v", found.ShowInBrowser, showInBrowser)
			}
			if found.ErrorHandlingType != microflows.ErrorHandlingTypeRollback {
				t.Errorf("ErrorHandlingType = %q, want Rollback", found.ErrorHandlingType)
			}
		})
	}
}
