// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	_ "github.com/mendixlabs/mxcli/modelsdk/gen/texts" // register Texts$Text so page titles decode concretely
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// gen→pages read adapter. SHOW PAGES needs only the page header — name, module,
// excluded, title, URL, parameter count — not the widget tree, so this stays
// shallow. Widget-tree conversion (DESCRIBE PAGE / ALTER) is a later phase.

func (b *Backend) ListPages() ([]*pages.Page, error) {
	units, err := mprread.ListUnitsWithContainer[*genPg.Page](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*pages.Page, 0, len(units))
	for _, u := range units {
		out = append(out, pageFromGen(u.Element, u.ContainerID))
	}
	// Legacy ListPages calls listUnitsByType("Forms$Page"), which is
	// prefix-matched and therefore also sweeps in Forms$PageTemplate units.
	// Replicate that here so SHOW PAGES matches legacy. (The modelsdk reader
	// is strict-typed, so we add templates explicitly.)
	tmpls, err := mprread.ListUnitsWithContainer[*genPg.PageTemplate](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range tmpls {
		out = append(out, pageTemplateAsPage(u.Element, u.ContainerID))
	}
	return out, nil
}

// pageTemplateAsPage adapts a page template into the Page shape SHOW PAGES
// expects. Templates carry only name + excluded; title/url/params stay zero,
// matching legacy's prefix-match behaviour.
func pageTemplateAsPage(pt *genPg.PageTemplate, containerID model.ID) *pages.Page {
	out := &pages.Page{ContainerID: containerID, Name: pt.Name(), Excluded: pt.Excluded()}
	out.ID = model.ID(pt.ID())
	return out
}

func (b *Backend) GetPage(id model.ID) (*pages.Page, error) {
	units, err := mprread.ListUnitsWithContainer[*genPg.Page](b.reader)
	if err != nil {
		return nil, err
	}
	for _, u := range units {
		if model.ID(u.Element.ID()) == id {
			return pageFromGen(u.Element, u.ContainerID), nil
		}
	}
	return nil, nil
}

func pageFromGen(p *genPg.Page, containerID model.ID) *pages.Page {
	out := &pages.Page{
		ContainerID: containerID,
		Name:        p.Name(),
		Excluded:    p.Excluded(),
		URL:         p.Url(),
		Title:       textElementToModel(p.Title()),
	}
	out.ID = model.ID(p.ID())
	for _, el := range p.ParametersItems() {
		pp := &pages.PageParameter{}
		pp.ID = model.ID(el.ID())
		out.Parameters = append(out.Parameters, pp)
	}
	return out
}

// textElementToModel converts a gen Texts$Text element to *model.Text via its
// translation accessors. Returns nil when el is nil or not a text element, so
// callers can keep the renderer's `if Title != nil` guards. Ported from
// engalar's convert_reader.go.
func textElementToModel(el element.Element) *model.Text {
	type translationsAccessor interface {
		TranslationsItems() []element.Element
	}
	type translationAccessor interface {
		LanguageCode() string
		Text() string
	}
	ta, ok := el.(translationsAccessor)
	if !ok {
		return nil
	}
	out := &model.Text{Translations: map[string]string{}}
	for _, item := range ta.TranslationsItems() {
		tr, ok := item.(translationAccessor)
		if !ok {
			continue
		}
		if code := tr.LanguageCode(); code != "" {
			out.Translations[code] = tr.Text()
		}
	}
	return out
}
