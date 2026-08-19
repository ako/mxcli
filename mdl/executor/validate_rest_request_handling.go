// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// unwritableRestRequestHandling maps the RequestHandling types mxcli cannot
// serialize to the Studio Pro feature they represent.
//
// Mendix has six (generated/metamodel: RequestHandling is implemented by
// Advanced, Binary, Custom, FormData, Mapping, Simple). mxcli writes four; these
// two it can only read.
var unwritableRestRequestHandling = map[string]string{
	"Microflows$FormDataRequestHandling": "form data",
	"Microflows$AdvancedRequestHandling": "advanced request handling",
}

// checkNoUnwritableRestBody refuses a microflow rewrite that would silently drop
// a REST call's request body.
//
// A CREATE OR REPLACE/MODIFY rebuilds the microflow from the statement, so a
// body the writer cannot express simply vanishes. The loss is invisible from
// every angle mxcli offers: DESCRIBE renders the call with no body clause at
// all, so a describe → edit → exec cycle drops the payload and the resulting
// script looks like a faithful copy. mxbuild does not object either — a REST
// call with no body is perfectly valid — so the app builds clean and posts
// nothing (guard-don't-drop, ADR-0005).
//
// This is the same failure the binary body had before it became authorable, and
// the reason to refuse rather than warn: the request still succeeds, often with
// a 200, and only the receiving system notices.
func checkNoUnwritableRestBody(ctx *ExecContext, microflowID model.ID, qualifiedName string) error {
	raw, err := ctx.Backend.GetRawUnit(microflowID)
	if err != nil {
		// Unreadable stored unit is not this guard's business; the rewrite path
		// reports its own errors.
		return nil
	}
	found := unwritableRequestHandlings(raw)
	if len(found) == 0 {
		return nil
	}
	sort.Strings(found)
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"microflow %s has a REST call using %s, which MDL cannot express — "+
			"rewriting the microflow would drop the request body and the call would "+
			"post nothing.\n"+
			"  Nothing downstream reports the loss: DESCRIBE omits the clause and the "+
			"app still builds.\n"+
			"  Change the microflow in Studio Pro, or remove the body from the call first.",
		qualifiedName, strings.Join(found, ", ")))
}

// unwritableRequestHandlings walks a stored unit for RequestHandling sub-documents
// whose $Type the writer cannot reproduce.
func unwritableRequestHandlings(v any) []string {
	var out []string
	switch t := v.(type) {
	case map[string]any:
		if rh, ok := t["RequestHandling"].(map[string]any); ok && rh != nil {
			if typeName, _ := rh["$Type"].(string); typeName != "" {
				if label, bad := unwritableRestRequestHandling[typeName]; bad {
					out = append(out, label)
				}
			}
		}
		for _, val := range t {
			out = append(out, unwritableRequestHandlings(val)...)
		}
	case []any:
		for _, el := range t {
			out = append(out, unwritableRequestHandlings(el)...)
		}
	}
	return dedupeStrings(out)
}
