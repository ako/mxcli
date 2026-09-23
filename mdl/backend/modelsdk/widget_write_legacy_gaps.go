// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// The last widgets that sent a user to the legacy engine, now deleted
// (docs/plans/2026-09-14-retire-legacy-engine.md). Closing these was one of its
// preconditions, which is why the file is named for them.
//
// Every "not yet supported by the modelsdk engine" message was a reason the
// legacy engine had to stay shipped and tested. Measured against sdk/pages, the
// reachable set was five, not the twenty-three a name-based scan suggests:
// nineteen widget structs are never constructed by the executor at all, and two
// of the three remaining data-source / client-action gaps (EntityPathSource,
// ShowHomePageClientAction) are written by NEITHER engine and built by nothing,
// so the fallback message named a path legacy could not take either.
//
// The five that were real, each confirmed by running `mxcli exec` on a page that
// uses them:
//
//	statictext   -> pages.Text          Forms$Text
//	dropdown     -> pages.DropDown      Forms$DropDown
//	staticimage  -> pages.StaticImage   Forms$StaticImageViewer
//	dynamicimage -> pages.DynamicImage  Forms$ImageViewer
//	             -> pages.NanoflowSource Forms$NanoflowSource   (list/grid data source)
//
// None was covered by the doctype gate: no example in mdl-examples/ or in any
// skill uses those keywords, which is why the suite ran green on modelsdk while
// four plain widget keywords failed.
//
// The first of the five turned out not to be a gap at all. Closing it and
// running the result through `mx check` showed that BOTH engines wrote a project
// that could not be LOADED — Mendix has no Forms$Text — so `statictext` is now
// refused at build and check time (MDL-WIDGET29,
// mdl/executor/validate_widget_retired.go). Nothing constructs pages.Text any
// more, so there is no writer for it here either — an old project that carries
// one keeps it because ALTER PAGE mutates the stored gen document rather than
// rebuilding from the semantic model.
//
// The bar here is the Studio Pro reference, NOT parity with sdk/mpr. That was
// the starting assumption and measuring it overturned it: ako/TestApp carries
// three Studio-Pro-authored Forms$StaticImageViewer widgets (FeedbackModule),
// and legacy disagrees with all three in two ways — it omits AlternativeText
// (which generated/metamodel declares non-optional on both image types) and it
// writes BSON null for the unset Image. Across the whole corpus a by-name
// reference is written as an EMPTY STRING and never as null: 0 nulls against
// 4,400+ empty strings over 40 (type, property) pairs. Legacy's dynamic image is
// worse still — its hand-rolled AlternativeText carries a "FallbackValue" string
// that Forms$ClientTemplate does not have (metamodel: Fallback / Parameters /
// Template), which is the invent-a-key defect from CLAUDE.md, invisible to
// mxbuild. sdk/mpr was corrected to match rather than pinned as ground truth, so
// the two test files can assert one shape for both engines
// (widget_write_legacy_gaps_test.go and sdk/mpr/writer_widgets_image_test.go).
//
// Two divergences are deliberately left alone, because measuring them showed
// they are project-wide and predate this file — fixing them here would hide
// them: modelsdk writes TabIndex/Width/Height as int32 where Studio Pro and
// legacy write int64 (measured on 265 Forms$DivContainer widgets, not just
// these), and modelsdk omits an empty Widgets list where legacy writes [3].

// dropDownToGen builds a Forms$DropDown (an enumeration/association selector).
func dropDownToGen(dd *pages.DropDown) (element.Element, error) {
	g := genPg.NewDropDown()
	applyWidgetBase(g, &dd.BaseWidget)
	g.SetAriaRequired(false)
	if ref := inputAttributeRefToGen(dd.AttributePath, dd.AttributeRefSteps); ref != nil {
		g.SetAttributeRef(ref)
	}
	g.SetEditable(pages.WidgetEditability(&dd.BaseWidget))
	// An empty Texts$Text, not a null: the property is the caption of the blank
	// option and Studio Pro always writes the holder.
	g.SetEmptyOptionCaption(emptyTranslatedText())
	if dd.Label != "" {
		g.SetLabelTemplate(textAsClientTemplate(textFromString(dd.Label)))
	}
	onChange, err := clientActionToGen(dd.OnChangeAction)
	if err != nil {
		return nil, err
	}
	g.SetOnChangeAction(onChange)
	g.SetOnEnterAction(noActionGen())
	g.SetOnLeaveAction(noActionGen())
	g.SetReadOnlyStyle("Inherit")
	g.SetValidation(widgetValidationToGen())
	return g, nil
}

// staticImageToGen builds a Forms$StaticImageViewer.
//
// Not supported by the React client — which Mendix added in 10.7 and which is the
// only client on 11 — so mxbuild reports CE0582 wherever that client is enabled,
// and `image` routes to the pluggable widget instead.
//
// `staticimage` is still a keyword the executor dispatches, so the writer has to
// answer for it. Unlike `statictext` the TYPE exists: the project loads, and
// CE0582 is Mendix's own advice rather than a defect, so refusing it would be
// over-reach. `mxcli lint` reports the widget as MPR012 instead, which is where
// a deprecation belongs — a `check` warning would fire on every legitimate
// describe -> exec of a legacy page.
func staticImageToGen(img *pages.StaticImage) (element.Element, error) {
	g := genPg.NewStaticImageViewer()
	applyWidgetBase(g, &img.BaseWidget)
	g.SetAlternativeText(emptyClientTemplate())
	click, err := clientActionToGen(img.OnClickAction)
	if err != nil {
		return nil, err
	}
	g.SetClickAction(click)
	// Which image is shown, as the qualified name of an image-collection entry
	// (Module.Collection.Image). Unset is "", not null; see the header. MDL had
	// no spelling for this at all until mendixlabs/mxcli#1057, so a
	// describe -> exec of a page carrying one silently emptied the widget.
	g.SetImageQualifiedName(img.ImageName)
	g.SetHeight(int32(img.Height))
	// The units were hardcoded to "Auto", so a pixel-sized image came back
	// auto-sized on any rewrite. Auto is Studio Pro's default and stays the
	// value for an unset field, so nothing an existing script writes changes.
	g.SetHeightUnit(imageSizeUnit(img.HeightUnit))
	g.SetResponsive(img.Responsive)
	g.SetWidth(int32(img.Width))
	g.SetWidthUnit(imageSizeUnit(img.WidthUnit))
	return g, nil
}

// imageSizeUnit maps a semantic width/height unit onto the Pages$WidthUnit /
// Pages$HeightUnit member Mendix stores, defaulting to Studio Pro's "Auto".
// Validating rather than passing the string through: an unknown member is the
// enum trap CLAUDE.md names ("SettingsDatabaseType is Hsqldb, never HSQLDB").
func imageSizeUnit(u pages.WidthUnit) string {
	switch strings.ToLower(string(u)) {
	case "pixels":
		return "Pixels"
	case "percentage":
		return "Percentage"
	default:
		return "Auto"
	}
}

// dynamicImageToGen builds a Forms$ImageViewer — gen calls the type
// DynamicImageViewer, and its storage name is the one that matters.
func dynamicImageToGen(img *pages.DynamicImage) (element.Element, error) {
	g := genPg.NewDynamicImageViewer()
	applyWidgetBase(g, &img.BaseWidget)
	g.SetAlternativeText(emptyClientTemplate())
	click, err := clientActionToGen(img.OnClickAction)
	if err != nil {
		return nil, err
	}
	g.SetClickAction(click)
	// The entity holding the image. Bound to nothing, mxbuild refuses the widget
	// with CE0489 "Select an entity for the data source of this dynamic image",
	// so this is the difference between a widget that builds and one that does
	// not — not a fidelity nicety.
	source, err := imageViewerSourceToGen(img.DataSource)
	if err != nil {
		return nil, err
	}
	g.SetDataSource(source)
	// The fallback image, as the qualified name of an image-collection entry.
	// Unset is "", not null; see the header.
	g.SetDefaultImageQualifiedName(img.DefaultImageName)
	g.SetHeight(int32(img.Height))
	g.SetHeightUnit(imageSizeUnit(img.HeightUnit))
	g.SetOnClickEnlarge(img.OnClickEnlarge)
	g.SetResponsive(img.Responsive)
	g.SetShowAsThumbnail(img.ShowAsThumbnail)
	g.SetWidth(int32(img.Width))
	g.SetWidthUnit(imageSizeUnit(img.WidthUnit))
	return g, nil
}

// imageViewerSourceToGen builds the Forms$ImageViewerSource a dynamic image
// binds through. The source names an ENTITY and nothing else: unlike its
// list-widget siblings it declares no XPath constraint and no sort bar, so only
// an entity-backed source has anywhere to go here.
//
// Its EntityRef is a DomainModels$DirectEntityRef{Entity: "Module.Entity"} —
// pinned to Studio Pro at 20 of 20 instances in a blank 11.12.1 app — and it is
// the element mxbuild's CE0489 is asking for.
//
// A NIL source yields the bare element. That is what mxcli wrote for every
// dynamic image until now and what mxbuild flags as CE0489, and it stays the
// behaviour on purpose: describe emits no DataSource clause for a stored widget
// that has none, so refusing here would make describe -> exec fail on a model
// that already exists (guard-don't-drop, ADR-0005).
//
// Any OTHER source is refused rather than ignored. Forms$ImageViewerSource has
// no slot for a microflow, a nanoflow or an association, and the metamodel's
// context-path variants (EntityPath / SourceVariable) have no Studio Pro
// reference here to pin them against. Writing the holder without them would
// produce CE0489 — a message that says the author forgot the source when they
// did not — so the gap is named at the point it is hit instead.
func imageViewerSourceToGen(ds pages.DataSource) (element.Element, error) {
	src := genPg.NewImageViewerSource()
	assignID(src)
	src.SetForceFullObjects(false)
	switch d := ds.(type) {
	case nil:
		return src, nil
	case *pages.DatabaseSource:
		if d.EntityName != "" {
			ref := genDm.NewDirectEntityRef()
			assignID(ref)
			ref.SetEntityQualifiedName(d.EntityName)
			src.SetEntityRef(ref)
		}
		return src, nil
	default:
		return nil, fmt.Errorf("dynamicimage: a %T data source cannot be stored on a "+
			"Forms$ImageViewerSource, which holds an entity and nothing else — use "+
			"`DataSource: database from Module.Entity` naming the entity that holds "+
			"the image", ds)
	}
}

// nanoflowSourceToGen builds a Forms$NanoflowSource — a list widget's "nanoflow"
// data source.
//
// Built raw, and this one is a judgement rather than a limitation: gen's
// NanoflowSource offers ForceFullObjects and NanoflowQualifiedName, binding the
// nanoflow name DIRECTLY on the source, while Studio Pro nests it inside a
// Forms$NanoflowSettings child alongside ParameterMappings — which is what
// sdk/mpr writes. Writing gen's shape would put the name in a key Studio Pro
// does not read there, the same class of defect as the storage-name overrides
// (CLAUDE.md). Legacy's shape is the one with a working project behind it.
func nanoflowSourceToGen(d *pages.NanoflowSource) element.Element {
	g := newElem("Forms$NanoflowSource", string(d.ID))
	settings := newElem("Forms$NanoflowSettings", "")
	addStr(settings, "Nanoflow", d.Nanoflow)
	addEmptyTypedList(settings, "ParameterMappings", 3)
	addPart(g, "NanoflowSettings", settings)
	return g
}

// emptyClientTemplate is the Forms$ClientTemplate an image's AlternativeText
// carries when no alt text has been set: an empty Template, an empty Fallback
// and no parameters. Matches the three Studio-Pro-authored StaticImageViewer
// widgets in ako/TestApp element for element.
func emptyClientTemplate() element.Element {
	return textAsClientTemplate(nil)
}

// emptyTranslatedText is a Texts$Text with no translations — the holder Studio
// Pro writes for an unset caption.
func emptyTranslatedText() element.Element {
	holder := newElem("Texts$Text", "")
	addEmptyTypedList(holder, "Items", 3)
	return holder
}

func init() {
	// Every widget here needs its null slots and its list marker registered, or
	// it serializes with keys missing and under the wrong array version. Both
	// were caught by diffing the two engines' output for one page carrying all
	// four widgets — neither shows up as a build error.
	//
	// Marker 2, not the codec's default of 3: a container's Widgets list takes
	// its marker from the CHILD type, so an unregistered widget silently changed
	// the marker of the list it sits in.
	codec.RegisterTypeDefaults("Forms$DropDown", codec.TypeDefaults{
		NullFields: []string{
			"AttributeRef", "ScreenReaderLabel", "SourceVariable", "LabelTemplate",
			"ConditionalVisibilitySettings", "ConditionalEditabilitySettings",
			"NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$DropDown", 2)

	// Image / DefaultImage are by-name references and are set to "" above, not
	// listed here: an unset one is an empty string in every Studio Pro document
	// measured. Only the two child slots are genuinely null.
	codec.RegisterTypeDefaults("Forms$StaticImageViewer", codec.TypeDefaults{
		NullFields: []string{
			"ConditionalVisibilitySettings", "NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$StaticImageViewer", 2)

	codec.RegisterTypeDefaults("Forms$ImageViewer", codec.TypeDefaults{
		NullFields: []string{
			"ConditionalVisibilitySettings", "NativeAccessibilitySettings",
		},
	})
	codec.RegisterListMarker("Forms$ImageViewer", 2)

	// EntityRef is the attribute path the image comes from — null when unbound.
	codec.RegisterTypeDefaults("Forms$ImageViewerSource", codec.TypeDefaults{
		NullFields: []string{"EntityRef"},
	})
}
