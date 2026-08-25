// SPDX-License-Identifier: Apache-2.0

// expose_action_info.go — folding an EXPOSED AS clause onto a stored toolbox entry.
//
// "Expose as microflow action" is what makes a Java action, a JavaScript action
// or a microflow appear in Studio Pro's toolbox: the point of it is that whoever
// drags it in does not need to know which of the three it is. The stored
// sub-document (CodeActions$MicroflowActionInfo) carries six fields — Caption,
// Category, and four PNG bitmaps: an icon and an image, each with a dark-mode
// variant.
//
// MDL's clause names the caption and the category. Rebuilding the sub-document
// from just those two silently discarded the bitmaps, and omitting the clause
// deleted the entry outright — caption, category, icon and image at once, so the
// action vanished from the toolbox. `mx check` reports 0 errors either way,
// because an unexposed action is perfectly valid; only Studio Pro can see it.
// That is the guard-don't-drop rule of ADR-0005.
package executor

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png" // registers the PNG decoder used to sanity-check a toolbox bitmap
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// mergeMicroflowActionInfo folds an EXPOSED AS clause onto the entry already
// stored on the document.
//
// An empty caption means the statement said nothing about the exposure, which is
// not the same as asking for it to be removed — the stored entry is carried
// through untouched. Removal is explicit, via NOT EXPOSED.
//
// When the clause is present, the caption and category come from MDL and
// everything MDL cannot express is carried from what was stored. The element's
// own $ID is carried too: minting a fresh one on every write would make an
// otherwise-unchanged document differ (ADR-0008).
func mergeMicroflowActionInfo(ctx *ExecContext, stored *javaactions.MicroflowActionInfo, caption, category string, remove bool, bitmaps []ast.ExposeBitmap, warn func(string)) (*javaactions.MicroflowActionInfo, error) {
	if remove {
		return nil, nil
	}
	if caption == "" {
		return stored, nil
	}
	out := &javaactions.MicroflowActionInfo{
		Caption:  caption,
		Category: category,
	}
	if stored != nil {
		out.ID = stored.ID
		out.IconData = stored.IconData
		out.IconDataDark = stored.IconDataDark
		out.ImageData = stored.ImageData
		out.ImageDataDark = stored.ImageDataDark
	}
	if out.ID == "" {
		out.ID = model.ID(types.GenerateID())
	}
	if err := applyExposeBitmaps(ctx, out, bitmaps, warn); err != nil {
		return nil, err
	}
	return out, nil
}

// bitmapSpec is what Studio Pro asks for. It is a "should", not a "must" —
// Mendix stores whatever bytes it is given — so a mismatch is reported and
// written rather than refused. A file that is not a PNG at all is refused: that
// is certainly a mistake, and nothing headless would ever show it.
var bitmapSpec = map[bool]struct {
	label         string
	width, height int
}{
	false: {"icon", 64, 64},
	true:  {"image", 256, 192},
}

// applyExposeBitmaps folds the ICON/IMAGE clauses onto a toolbox entry, reading
// each named file from disk.
//
// Relative paths resolve against the working directory, as CREATE IMAGE
// COLLECTION's do.
func applyExposeBitmaps(ctx *ExecContext, out *javaactions.MicroflowActionInfo, bitmaps []ast.ExposeBitmap, warn func(string)) error {
	for _, b := range bitmaps {
		target := &out.IconData
		switch {
		case b.Image && b.Dark:
			target = &out.ImageDataDark
		case b.Image:
			target = &out.ImageData
		case b.Dark:
			target = &out.IconDataDark
		}
		if b.Clear {
			*target = nil
			continue
		}
		data, err := readToolboxBitmap(ctx, b, warn)
		if err != nil {
			return err
		}
		*target = data
	}
	return nil
}

// readToolboxBitmap loads and sanity-checks one PNG.
func readToolboxBitmap(ctx *ExecContext, b ast.ExposeBitmap, warn func(string)) ([]byte, error) {
	path, err := ctx.ResolveScriptRelative(b.Path)
	if err != nil {
		return nil, mdlerrors.NewBackend("resolve toolbox bitmap path", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, mdlerrors.NewBackend(fmt.Sprintf("read toolbox bitmap %q", b.Path), err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	spec := bitmapSpec[b.Image]
	if err != nil || format != "png" {
		return nil, mdlerrors.NewValidationf(
			"toolbox %s %q is not a PNG: Studio Pro renders the toolbox from PNG bitmaps "+
				"and shows nothing for anything else", spec.label, b.Path)
	}
	if warn != nil && (cfg.Width != spec.width || cfg.Height != spec.height) {
		warn(fmt.Sprintf(
			"toolbox %s %q is %dx%d; Studio Pro expects %dx%d and will scale it",
			spec.label, b.Path, cfg.Width, cfg.Height, spec.width, spec.height))
	}
	return data, nil
}

// applyExposeClauses folds a microflow's EXPOSED AS clauses onto the two stored
// toolbox entries and returns the pair (microflow-editor, workflow-editor).
//
// A microflow can appear in both toolboxes, under two keys holding the same
// element type, so each clause names which one it means. A kind with no clause
// is carried through untouched — the same preserve-on-silence rule the Java and
// JavaScript actions follow.
func applyExposeClauses(ctx *ExecContext, clauses []ast.ExposeActionClause, storedMicroflow, storedWorkflow *javaactions.MicroflowActionInfo, warn func(string)) (mf, wf *javaactions.MicroflowActionInfo, err error) {
	mf, wf = storedMicroflow, storedWorkflow
	for _, c := range clauses {
		target := &mf
		if c.Workflow {
			target = &wf
		}
		if *target, err = mergeMicroflowActionInfo(ctx, *target, c.Caption, c.Category, c.Remove, c.Bitmaps, warn); err != nil {
			return nil, nil, err
		}
	}
	return mf, wf, nil
}

// refuseExposeOnFlavour rejects an EXPOSED AS clause on a nanoflow or a rule.
//
// Only Microflows$Microflow carries the toolbox properties — gen and
// generated/metamodel agree, and Studio Pro offers the tab on nothing else. The
// clause is accepted by the grammar (it is shared with microflows) so that the
// refusal can say why, rather than surfacing as "no viable alternative".
func refuseExposeOnFlavour(clauses []ast.ExposeActionClause, flavour, qualifiedName string) error {
	if len(clauses) == 0 {
		return nil
	}
	return mdlerrors.NewValidationf(
		"%s '%s' cannot be exposed as a toolbox action: only a microflow stores one. "+
			"Studio Pro offers the tab on microflows, Java actions and JavaScript actions — "+
			"move the logic into a microflow, or expose the Java/JavaScript action that calls it",
		flavour, qualifiedName)
}

// exposeWarner returns the sink for "this bitmap is the wrong size" notices.
//
// Nothing downstream catches a mis-sized toolbox icon: mxbuild stores the bytes,
// `mx check` passes, and the only place it shows is Studio Pro's toolbox — which
// is exactly where a headless author never looks.
func exposeWarner(ctx *ExecContext) func(string) {
	return func(msg string) {
		if ctx == nil || ctx.Output == nil {
			return
		}
		fmt.Fprintf(ctx.Output, "-- warning: %s\n", msg)
	}
}

// describeBitmapComments notes which toolbox bitmaps a document carries.
//
// DESCRIBE cannot re-emit them as ICON/IMAGE clauses: the clause names a file on
// disk and the model holds only the bytes, so re-exec would need a file that may
// not exist. Reporting them as comments keeps the round trip honest — a reader
// can see the icon is there, and re-running the output preserves it rather than
// clearing it, because an omitted bitmap is preserved.
func describeBitmapComments(info *javaactions.MicroflowActionInfo) string {
	if info == nil {
		return ""
	}
	var sb strings.Builder
	for _, b := range []struct {
		label string
		data  []byte
	}{
		{"icon", info.IconData},
		{"icon (dark)", info.IconDataDark},
		{"image", info.ImageData},
		{"image (dark)", info.ImageDataDark},
	} {
		if len(b.data) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "\n-- %s: %d bytes (not re-emitted; an omitted ICON/IMAGE clause preserves it)", b.label, len(b.data))
	}
	return sb.String()
}
