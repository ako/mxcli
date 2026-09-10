// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// webclient_legacy_paths.go recognises one mxbuild failure that only ever
// appears under --watch, and only on Mendix 11.14+.
//
// 11.14 moved the browser client to a pre-bundled shape: the serve Deploy
// target writes web/dist/ and nothing else. Measured on a blank 11.14.0 app,
// clean deployment, immediately after the cold build:
//
//	web/dist      PRESENT
//	web/pages     ABSENT
//	web/layouts   ABSENT
//	web/widgets   ABSENT
//
// Every *subsequent* Deploy build against the same serve process still writes
// the legacy per-document client — one .js per page and per layout — into
// directories the cold build never created, and fails on all of them at once:
//
//	Could not find a part of the path '…/deployment/web/pages/MyFirstModule.Home_Web.js'
//	Could not find a part of the path '…/deployment/web/layouts/Atlas_Core.Atlas_TopBar.js'
//
// So on 11.14 the first build succeeds and every rebuild fails, which is only
// reachable through --watch because nothing else asks for a second build.
//
// mxcli cannot fix this from the outside. Creating the directories is not a
// workaround — measured, it advances the same build to "Deployment failed
// during export of pluggable widgets", because the legacy export path wants
// widget packages the 11.14 client no longer ships in that form. The build is
// on the legacy client path end to end.
//
// What mxcli can do is not let the failure read as the user's model being
// broken. The raw message names four absolute paths inside deployment/ and
// nothing about why they are missing, so the natural reading is a corrupt
// deployment — and `rm -rf deployment/` makes it worse by costing a cold build
// and changing nothing.

// legacyClientDirs are the client output directories a pre-11.14 serve Deploy
// build creates and 11.14+ does not.
var legacyClientDirs = []string{"pages", "layouts"}

// legacyClientBuildHint explains an incremental build that failed because
// mxbuild wrote the legacy per-document client into directories this Mendix
// version's cold build never created. It returns "" for every other failure.
//
// The check is on the failure's own shape rather than on the Mendix version:
// the message must name a missing path under one of those directories AND that
// directory must actually be absent from the deployment. A build that fails for
// any other reason, or on a version whose cold build does create them, gets the
// ordinary error and no speculation.
func legacyClientBuildHint(deployDir, message string) string {
	if !strings.Contains(message, "Could not find a part of the path") {
		return ""
	}
	var named []string
	for _, dir := range legacyClientDirs {
		if !strings.Contains(message, filepath.Join("web", dir)+string(filepath.Separator)) {
			continue
		}
		if _, err := os.Stat(filepath.Join(deployDir, "web", dir)); os.IsNotExist(err) {
			named = append(named, "web/"+dir+"/")
		}
	}
	if len(named) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"  This is not a problem with your model, and deployment/ is not corrupt.\n"+
			"  This Mendix version's build writes the browser client pre-bundled into web/dist/\n"+
			"  and never creates %s, but the incremental build asks for the older\n"+
			"  one-file-per-page client and fails on every one of them.\n"+
			"  Creating those directories does not help — the same build then fails exporting\n"+
			"  pluggable widgets, because the whole path is the older client.\n"+
			"  Until mxbuild closes this, drop --watch and restart per change:\n"+
			"    mxcli run --local --screenshot\n",
		strings.Join(named, " or "))
}
