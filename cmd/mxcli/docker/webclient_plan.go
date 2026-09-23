// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// webClientPlan says what, if anything, mxcli must do to give a deployment a
// browser client.
//
// It exists because the question was answered in two places with the same
// hand-written pair of file checks — BuildWebClient and StartWebClientWatch —
// and they drifted: the Mendix 11.14 fix (ako/mxcli-ledger #146) landed on the
// first, so `run --local` started working on 11.14 while `run --local --watch`
// kept failing. Adding the classic-client case to both would have made three
// copies of a gate that has already drifted once, so the decision is made here
// and the call sites switch on it.
type webClientPlan int

// Measured on blank apps at the same build target:
//
//	11.13.0, OptimizedClient=Yes   web/rollup.config.mjs PRESENT   web/dist/index.js ABSENT
//	11.14.0, OptimizedClient=Yes   web/rollup.config.mjs ABSENT    web/dist/index.js PRESENT
//	11.12.2, OptimizedClient=No    web/rollup.config.mjs ABSENT    web/dist/index.js ABSENT
//	                               web/index.html loads mxclientsystem/mxui/mxui.js
//
// The third row is why absence cannot be the test for either of the other two:
// it looks identical to a build that produced nothing.
const (
	// webClientRollup: mxbuild wrote the client source and a rollup config but
	// not the bundle. Mendix 11.13 and earlier. mxcli runs the rollup step.
	webClientRollup webClientPlan = iota
	// webClientPrebuilt: mxbuild wrote web/dist itself. Mendix 11.14+, which
	// emits no rollup config because there is nothing left to configure.
	webClientPrebuilt
	// webClientClassic: web/ holds the classic (Dojo) client, which has no
	// bundling step in any Mendix version. Settings > Web UI > OptimizedClient
	// = No.
	webClientClassic
	// webClientMissing: none of the above — the build genuinely produced no
	// client, which is the case this gate was written for.
	webClientMissing
)

func (p webClientPlan) String() string {
	switch p {
	case webClientRollup:
		return "rollup"
	case webClientPrebuilt:
		return "prebuilt"
	case webClientClassic:
		return "classic"
	default:
		return "missing"
	}
}

// classicClientEntryMarker is what the classic client's entry point loads. The
// React client's loads dist/index.js instead, so the file names which client the
// deployment will actually serve.
const classicClientEntryMarker = "mxclientsystem/mxui/mxui.js"

// maxEntryPointRead bounds the read of web/index.html. Real ones are ~2KB; the
// cap is only so a wrong path cannot pull a large file into memory.
const maxEntryPointRead = 1 << 20

// planWebClient decides from the DEPLOYMENT, not from the model's
// OptimizedClient setting.
//
// The deployment is what gets served, and the two disagree exactly when it
// matters: right after the setting is changed, the old client is still on disk.
// Reading it here also needs no plumbing through the five call sites that ask
// this question, and it covers MigrationMode without having to predict which
// client that mode leaves in web/ — whichever one is there is the one to serve.
//
// mxbuild swaps the two clients in and out of web/ and parks the other beside it
// (dojo-web/ or react-web/), so only web/ is consulted; the parked directory has
// its own rollup config and is not what the app loads.
func planWebClient(deployDir string) webClientPlan {
	webDir := filepath.Join(deployDir, "web")

	// The classic client is checked FIRST and on positive evidence. Inferring it
	// from the absence of the React shapes would make every genuinely broken
	// deployment look classic, and silently skipping the bundle for those is the
	// black screen this whole file exists to prevent.
	if isClassicWebClient(webDir) {
		return webClientClassic
	}
	if fi, err := os.Stat(filepath.Join(webDir, "rollup.config.mjs")); err == nil && !fi.IsDir() {
		return webClientRollup
	}
	if WebClientBundled(deployDir) {
		return webClientPrebuilt
	}
	return webClientMissing
}

// isClassicWebClient reports whether web/index.html loads the Dojo client.
func isClassicWebClient(webDir string) bool {
	f, err := os.Open(filepath.Join(webDir, "index.html"))
	if err != nil {
		return false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxEntryPointRead))
	if err != nil {
		return false
	}
	return bytes.Contains(b, []byte(classicClientEntryMarker))
}

// noWebClientError is the verdict for a deployment that really has no client.
func noWebClientError(deployDir string) error {
	return fmt.Errorf("no rollup.config.mjs and no bundle at %s\n"+
		"  Mendix 11.13 and earlier emit a rollup config for mxcli to run; 11.14+ writes\n"+
		"  the bundle itself; a classic-client app (Web UI Settings > OptimizedClient = No)\n"+
		"  needs neither. None of the three is present, so the build did not produce a client:\n"+
		"  run a serve Deploy build first (or delete deployment/ if it was built by an\n"+
		"  older Mendix version).",
		webClientBundlePath(deployDir))
}
