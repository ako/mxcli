// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func (pb *pageBuilder) buildDataViewV3(w *ast.WidgetV3) (*pages.DataView, error) {
	dv := &pages.DataView{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DataView",
			},
			Name: w.Name,
		},
	}

	if v := w.GetStringProp("FormOrientation"); v != "" {
		switch strings.ToLower(v) {
		case "vertical":
			dv.FormOrientation = pages.FormOrientationVertical
		case "horizontal":
			dv.FormOrientation = pages.FormOrientationHorizontal
		default:
			return nil, mdlerrors.NewBackend("dataview FormOrientation", fmt.Errorf("invalid value %q (expected Horizontal or Vertical)", v))
		}
	}
	if _, ok := w.Properties["LabelWidth"]; ok {
		lw := w.GetIntProp("LabelWidth")
		if lw < 0 || lw > 12 {
			return nil, mdlerrors.NewBackend("dataview LabelWidth", fmt.Errorf("value %d out of range (expected 0..12)", lw))
		}
		dv.LabelWidth = &lw
	}

	// ShowFooter was previously only ever set implicitly, by the presence of a
	// `footer { … }` block — so `showFooter: true` parsed, passed `check`, and was
	// silently discarded (mendixlabs/mxcli#813). An explicit value is the author's
	// statement and wins over the footer block, in both directions: it can show an
	// empty footer, or hide one whose widgets are still declared.
	showFooterSet := false
	if raw, ok := lookupPropCI(w, "ShowFooter"); ok {
		v, err := propBool(raw)
		if err != nil {
			return nil, mdlerrors.NewBackend("dataview ShowFooter", err)
		}
		dv.ShowFooter = v
		showFooterSet = true
	}

	// Handle DataSource
	if ds := w.GetDataSource(); ds != nil {
		// A DataView shows a single object; Mendix offers only Context / Microflow /
		// Nanoflow / Listen sources — never Database (that belongs to list widgets).
		// The legacy writer emits a best-effort Forms$DataViewSource that MxBuild
		// rejects with CE7007; modelsdk refuses the DatabaseSource downstream. Refuse
		// early with an actionable message so neither engine produces a broken page,
		// and mirror the check-time MDL-WIDGET09.
		// (An *association* DataView source IS valid — "data from context over an
		// association" — and is handled downstream, so it is not refused here.)
		if ds.Type == "database" {
			return nil, mdlerrors.NewValidationf(
				"dataview %q cannot use a database data source (from %s) — a data view shows one object; use a microflow/nanoflow source (or a page parameter), or a list widget (listview/datagrid/gallery) for a collection [MDL-WIDGET09]",
				w.Name, ds.Reference)
		}
		dataSource, entityName, err := pb.buildDataSourceV3(ds)
		if err != nil {
			return nil, mdlerrors.NewBackend("build datasource", err)
		}
		dv.DataSource = dataSource

		// Save and restore entity context so nested DataViews work correctly
		oldContext := pb.entityContext
		oldContextVar := pb.contextVarName
		oldContextKnown := pb.contextKnown
		pb.entityContext = entityName
		pb.contextVarName = contextVarFor(ds)
		pb.contextKnown = true
		defer func() {
			pb.entityContext = oldContext
			pb.contextVarName = oldContextVar
			pb.contextKnown = oldContextKnown
		}()

		// Register the widget name with its entity so template params like $dvOrder.Attr
		// can be resolved to Entity.Attr
		if w.Name != "" && entityName != "" {
			pb.paramEntityNames[w.Name] = entityName
		}
	}

	// Build child widgets, separating FOOTER widgets into FooterWidgets
	for _, child := range w.Children {
		// Check if this is a FOOTER widget - its children go to FooterWidgets
		if child.Type == "footer" {
			if !showFooterSet {
				dv.ShowFooter = true
			}
			for _, fw := range child.Children {
				widget, err := pb.buildWidgetV3(fw)
				if err != nil {
					return nil, err
				}
				dv.FooterWidgets = append(dv.FooterWidgets, widget)
			}
			continue
		}
		childWidget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		dv.Widgets = append(dv.Widgets, childWidget)
	}

	// Also build footer widgets from Properties (legacy support)
	if footerWidgets, ok := w.Properties["Footer"].([]*ast.WidgetV3); ok {
		if !showFooterSet {
			dv.ShowFooter = true
		}
		for _, fw := range footerWidgets {
			widget, err := pb.buildWidgetV3(fw)
			if err != nil {
				return nil, err
			}
			dv.FooterWidgets = append(dv.FooterWidgets, widget)
		}
	}

	if err := pb.registerWidgetName(w.Name, dv.ID); err != nil {
		return nil, err
	}

	return dv, nil
}

// buildClientTemplateParams converts AST template parameters (e.g. from
// CaptionParams / ContentParams) into pages.ClientTemplateParameter values
// with attribute paths resolved against the current entity context.
// Returns nil if the input is empty.
func (pb *pageBuilder) buildClientTemplateParams(astParams []ast.ParamAssignmentV3) []*pages.ClientTemplateParameter {
	if len(astParams) == 0 {
		return nil
	}
	out := make([]*pages.ClientTemplateParameter, 0, len(astParams))
	for _, p := range astParams {
		param := &pages.ClientTemplateParameter{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ClientTemplateParameter",
			},
		}
		// A per-parameter `format (...)` block carries the same FormattingInfo the
		// standalone dynamictext widget honors (buildDynamicTextV3). This shared
		// helper feeds DataGrid2 dynamic-text columns (object-list path) and the
		// ALTER PAGE column path, so a `format` block authored on a column param
		// reaches the runtime instead of being silently dropped (ledger #77).
		param.FormattingInfo = formattingInfoFromParamFormat(p.Format)
		strVal, ok := p.Value.(string)
		if !ok {
			out = append(out, param)
			continue
		}
		if strings.HasPrefix(strVal, "'") || strings.HasPrefix(strVal, "\"") {
			// Already a quoted string literal — use as-is.
			param.Expression = strVal
		} else {
			// Attribute reference (with or without $ prefix) or bare attribute name.
			pb.resolveTemplateAttributePathFull(strVal, param)
		}
		out = append(out, param)
	}
	return out
}

// buildColumnSpecFromAST converts a single AST column widget into a
// DataGridColumnSpec. Filter-type grandchildren are routed to the column
// filter slot; other grandchildren become ChildWidgets (custom content).
func (pb *pageBuilder) buildColumnSpecFromAST(child *ast.WidgetV3) (*backend.DataGridColumnSpec, error) {
	attr := child.GetAttribute()
	if attr == "" && child.Name != "" && len(child.Children) == 0 {
		attr = child.Name
	}
	// An attribute over associations (Assoc/Attr) resolves to a final attribute +
	// association steps (AttributeRef.EntityRef); a flat path is resolved as-is.
	resolvedAttr := pb.resolveAttributePath(attr)
	var attrSteps []pages.AttributeRefStep
	if finalQN, steps, ok := pb.resolveAssociationAttributePath(attr); ok {
		resolvedAttr = finalQN
		attrSteps = steps
	}
	col := backend.DataGridColumnSpec{
		Attribute:         resolvedAttr,
		AttributeRefSteps: attrSteps,
		Caption:           child.GetCaption(),
		CaptionParams:     pb.buildClientTemplateParams(child.GetCaptionParams()),
		ShowContentAs:     child.GetStringProp("ShowContentAs"),
		Content:           child.GetContent(),
		ContentParams:     pb.buildClientTemplateParams(child.GetContentParams()),
		Properties:        child.Properties,
	}
	for _, grandchild := range child.Children {
		if filterWidgetID := dataGridFilterWidgetID(grandchild.Type); filterWidgetID != "" {
			fw, err := pb.widgetBackend.BuildFilterWidget(backend.FilterWidgetSpec{
				WidgetID:        filterWidgetID,
				FilterName:      grandchild.Name,
				VisibilityRules: resolveWidgetVisibilityRules(pb.backend.Path(), filterWidgetID),
			}, pb.backend.Path())
			if err != nil {
				return nil, mdlerrors.NewBackend("build column filter widget", err)
			}
			col.FilterWidget = fw
		} else {
			childWidget, err := pb.buildWidgetV3(grandchild)
			if err != nil {
				return nil, mdlerrors.NewBackend("build column child widget", err)
			}
			if childWidget != nil {
				col.ChildWidgets = append(col.ChildWidgets, childWidget)
			}
		}
	}
	return &col, nil
}

func (pb *pageBuilder) buildDataGridColumnV3(w *ast.WidgetV3) (*pages.DataGridColumn, error) {
	col := &pages.DataGridColumn{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$DataGridColumn",
		},
		Name:     w.Name,
		Editable: true,
	}

	// Get attribute from Attribute property
	if attr := w.GetAttribute(); attr != "" {
		col.AttributePath = pb.resolveAttributePath(attr)
	}

	// Get caption
	if caption := w.GetCaption(); caption != "" {
		col.Caption = &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": caption},
		}
	}

	return col, nil
}

func (pb *pageBuilder) buildListViewV3(w *ast.WidgetV3) (*pages.ListView, error) {
	lv := &pages.ListView{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ListView",
			},
			Name: w.Name,
		},
		PageSize: 20,
	}

	// Honor an explicit PageSize (the property was parsed but previously dropped —
	// the writer hardcoded 20 regardless, so the app always paged at 20). (traceops #17)
	if ps := w.GetIntProp("PageSize"); ps > 0 {
		lv.PageSize = ps
	}

	// Handle DataSource
	var listEntity string
	if ds := w.GetDataSource(); ds != nil {
		dataSource, entityName, err := pb.buildDataSourceV3(ds)
		if err != nil {
			return nil, mdlerrors.NewBackend("build datasource", err)
		}
		lv.DataSource = dataSource
		listEntity = entityName

		// Save and restore entity context so nested containers work correctly
		oldContext := pb.entityContext
		oldContextVar := pb.contextVarName
		oldContextKnown := pb.contextKnown
		pb.entityContext = entityName
		pb.contextVarName = contextVarFor(ds)
		pb.contextKnown = true
		defer func() {
			pb.entityContext = oldContext
			pb.contextVarName = oldContextVar
			pb.contextKnown = oldContextKnown
		}()

		// Register widget name with entity for SELECTION datasource lookup
		if w.Name != "" && entityName != "" {
			pb.paramEntityNames[w.Name] = entityName
		}
	}

	// Register widget scope for SELECTION references
	if err := pb.registerWidgetName(w.Name, lv.ID); err != nil {
		return nil, err
	}

	// Split the body: `template for Module.Entity { ... }` blocks become
	// specialization templates, everything else is the list view's own body (the
	// default rendering, which Mendix uses for an object no template matches).
	// The BSON keeps the two in separate arrays and so do we.
	seen := make(map[string]bool, len(w.Children))
	for _, child := range w.Children {
		if child.Specialization == "" {
			widget, err := pb.buildWidgetV3(child)
			if err != nil {
				return nil, err
			}
			lv.Widgets = append(lv.Widgets, widget)
			continue
		}
		tpl, err := pb.buildListViewTemplateV3(child, w.Name, listEntity, seen)
		if err != nil {
			return nil, err
		}
		lv.Templates = append(lv.Templates, tpl)
	}

	return lv, nil
}

// buildListViewTemplateV3 builds one `template for Module.Entity { ... }` block.
//
// Source order is preserved: Mendix stores templates in an ordered array and the
// order is authored, not derived — the four templates on ako/TestApp's
// Vehicle_Overview are Bus, Truck, Car, SUV, which is neither alphabetical nor
// domain-model order.
func (pb *pageBuilder) buildListViewTemplateV3(w *ast.WidgetV3, listViewName, listEntity string, seen map[string]bool) (*pages.ListViewTemplate, error) {
	spec := w.Specialization

	if seen[spec] {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"list view %s has more than one template for %s — a list view renders at most "+
				"one template per specialization", listViewName, spec))
	}
	seen[spec] = true

	// The specialization must actually be one: Mendix matches a template against
	// the object's type, so a template for an unrelated entity can never render.
	// listEntity is empty when the datasource could not be resolved to an entity,
	// and an unresolvable datasource is already reported elsewhere — do not
	// report it a second time as a bogus specialization error.
	if listEntity != "" && !pb.entityIsOrDescendsFrom(spec, listEntity) {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"template for %s in list view %s: %s is not %s or a specialization of it, "+
				"so the template can never match an object the list view shows",
			spec, listViewName, spec, listEntity))
	}

	tpl := &pages.ListViewTemplate{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$ListViewTemplate",
		},
		Specialization: spec,
	}

	// Inside the template the context object IS the specialization, so an
	// attribute the specialization adds resolves here even though it does not
	// exist on the list view's own entity.
	oldContext := pb.entityContext
	pb.entityContext = spec
	defer func() { pb.entityContext = oldContext }()

	for _, child := range w.Children {
		if child.Specialization != "" {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"template for %s in list view %s contains a nested `template for %s` — "+
					"list view templates cannot nest", spec, listViewName, child.Specialization))
		}
		widget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		tpl.Widgets = append(tpl.Widgets, widget)
	}
	return tpl, nil
}

// applyOnChangeV3 resolves a widget's `OnChange:` client action into dst.
//
// Every input widget carrying an OnChangeAction must call this. Before ledger
// #14 only `buildTextBoxV3` read GetOnChange(), so an OnChange authored on a
// checkbox / radiobuttons / dropdown / textarea / datepicker parsed, checked and
// executed clean while the property was dropped between the AST and the model —
// the control rendered correctly and produced no server round-trip at all.
func (pb *pageBuilder) applyOnChangeV3(w *ast.WidgetV3, dst *pages.ClientAction) error {
	action := w.GetOnChange()
	if action == nil {
		return nil
	}
	act, err := pb.buildClientActionV3(action)
	if err != nil {
		return err
	}
	*dst = act
	return nil
}

func (pb *pageBuilder) buildTextBoxV3(w *ast.WidgetV3) (*pages.TextBox, error) {
	tb := &pages.TextBox{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$TextBox",
			},
			Name: w.Name,
		},
	}

	// Handle Attribute (attribute path)
	if attr := w.GetAttribute(); attr != "" {
		tb.AttributePath = pb.resolveAttributePath(attr)
	}

	// Handle Label
	if label := w.GetLabel(); label != "" {
		tb.Label = label
	}

	// Handle Placeholder (input hint text — a ClientTemplate, like the label)
	if ph := w.GetPlaceholder(); ph != "" {
		tb.Placeholder = &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": ph},
		}
	}

	// Handle OnChange (the "On change" client action)
	if err := pb.applyOnChangeV3(w, &tb.OnChangeAction); err != nil {
		return nil, err
	}

	if err := pb.registerWidgetName(w.Name, tb.ID); err != nil {
		return nil, err
	}

	return tb, nil
}

func (pb *pageBuilder) buildTextAreaV3(w *ast.WidgetV3) (*pages.TextArea, error) {
	ta := &pages.TextArea{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$TextArea",
			},
			Name: w.Name,
		},
	}

	// Handle Attribute
	if attr := w.GetAttribute(); attr != "" {
		ta.AttributePath = pb.resolveAttributePath(attr)
	}

	// Handle Label
	if label := w.GetLabel(); label != "" {
		ta.Label = label
	}

	// Handle OnChange (the "On change" client action)
	if err := pb.applyOnChangeV3(w, &ta.OnChangeAction); err != nil {
		return nil, err
	}

	if err := pb.registerWidgetName(w.Name, ta.ID); err != nil {
		return nil, err
	}

	return ta, nil
}

func (pb *pageBuilder) buildDatePickerV3(w *ast.WidgetV3) (*pages.DatePicker, error) {
	dp := &pages.DatePicker{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DatePicker",
			},
			Name: w.Name,
		},
	}

	// Handle Attribute
	if attr := w.GetAttribute(); attr != "" {
		dp.AttributePath = pb.resolveAttributePath(attr)
	}

	// Handle Label
	if label := w.GetLabel(); label != "" {
		dp.Label = label
	}

	// Handle OnChange (the "On change" client action)
	if err := pb.applyOnChangeV3(w, &dp.OnChangeAction); err != nil {
		return nil, err
	}

	if err := pb.registerWidgetName(w.Name, dp.ID); err != nil {
		return nil, err
	}

	return dp, nil
}

func (pb *pageBuilder) buildDropdownV3(w *ast.WidgetV3) (*pages.DropDown, error) {
	dd := &pages.DropDown{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DropDown",
			},
			Name: w.Name,
		},
	}

	// Handle Attribute
	if attr := w.GetAttribute(); attr != "" {
		dd.AttributePath = pb.resolveAttributePath(attr)
	}

	// Handle Label
	if label := w.GetLabel(); label != "" {
		dd.Label = label
	}

	// Handle OnChange (the "On change" client action)
	if err := pb.applyOnChangeV3(w, &dd.OnChangeAction); err != nil {
		return nil, err
	}

	if err := pb.registerWidgetName(w.Name, dd.ID); err != nil {
		return nil, err
	}

	return dd, nil
}

func (pb *pageBuilder) buildCheckBoxV3(w *ast.WidgetV3) (*pages.CheckBox, error) {
	cb := &pages.CheckBox{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$CheckBox",
			},
			Name: w.Name,
		},
	}

	// Handle Attribute
	if attr := w.GetAttribute(); attr != "" {
		cb.AttributePath = pb.resolveAttributePath(attr)
	}

	// Handle Label
	if label := w.GetLabel(); label != "" {
		cb.Label = label
	}

	// Handle OnChange (the "On change" client action)
	if err := pb.applyOnChangeV3(w, &cb.OnChangeAction); err != nil {
		return nil, err
	}

	if err := pb.registerWidgetName(w.Name, cb.ID); err != nil {
		return nil, err
	}

	return cb, nil
}

// buildRadioButtonsV3 creates RadioButtons from V3 syntax.
func (pb *pageBuilder) buildRadioButtonsV3(w *ast.WidgetV3) (*pages.RadioButtons, error) {
	rb := &pages.RadioButtons{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$RadioButtonGroup",
			},
			Name: w.Name,
		},
		Label: w.GetLabel(),
	}

	// Get attribute path from Attribute property
	if attr := w.GetAttribute(); attr != "" {
		rb.AttributePath = pb.resolveAttributePath(attr)
	}

	// Handle OnChange (the "On change" client action)
	if err := pb.applyOnChangeV3(w, &rb.OnChangeAction); err != nil {
		return nil, err
	}

	if err := pb.registerWidgetName(w.Name, rb.ID); err != nil {
		return nil, err
	}

	return rb, nil
}

func (pb *pageBuilder) buildTextWidgetV3(w *ast.WidgetV3) (*pages.Text, error) {
	st := &pages.Text{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$Text",
			},
			Name: w.Name,
		},
		RenderMode: pages.TextRenderModeText,
	}

	// Handle Content
	if content := w.GetContent(); content != "" {
		st.Caption = &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": content},
		}
	}

	// Handle RenderMode
	if rm := w.GetRenderMode(); rm != "" {
		st.RenderMode = pages.TextRenderMode(rm)
	}

	if err := pb.registerWidgetName(w.Name, st.ID); err != nil {
		return nil, err
	}

	return st, nil
}

// dynamicTextVariableRe matches a DYNAMICTEXT Content value that is a variable
// reference: a `$` followed by a valid identifier start (letter or underscore),
// e.g. `$var`, `$widget.Attr`, `$currentObject/Assoc/Attr`. It deliberately does
// NOT match `$` followed by a digit (`$318`) — that is not a valid Mendix
// variable and must be treated as literal content. (traceops #10)
var dynamicTextVariableRe = regexp.MustCompile(`^\$[A-Za-z_]`)

// isDynamicTextVariableRef reports whether a DYNAMICTEXT Content value should be
// auto-bound as a variable parameter rather than emitted as literal text.
func isDynamicTextVariableRef(content string) bool {
	return dynamicTextVariableRe.MatchString(content)
}

func (pb *pageBuilder) buildDynamicTextV3(w *ast.WidgetV3) (*pages.DynamicText, error) {
	dt := &pages.DynamicText{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DynamicText",
			},
			Name: w.Name,
		},
		RenderMode: pages.TextRenderModeText,
	}

	// Handle RenderMode
	if rm := w.GetRenderMode(); rm != "" {
		dt.RenderMode = pages.TextRenderMode(rm)
	}

	// Handle Content
	content := w.GetContent()
	explicitParams := w.GetContentParams()

	// Check if Content is an attribute reference AND no explicit params provided
	// If so, auto-generate template {1} and add the attribute as a parameter
	// Examples:
	//   Content: $widget.Name            -> auto-generate {1} with $widget.Name as param
	//   Content: Entity.Attribute        -> auto-generate {1} with Entity.Attribute as param
	//   Content: SomeStaticText          -> literal string, no params (no dot, no $)
	//   Content: 'Name: {1}', ContentParams: [Name] -> use explicit template and params
	var autoGeneratedParams []string
	if content != "" && explicitParams == nil {
		// Only auto-generate for:
		// - Variable references: $var or $widget.Attr (starts with $)
		// - Entity paths: Entity.Attribute (identifier.identifier pattern, not version numbers like "1.0")
		// Simple identifiers without dots are treated as static text
		isEntityPath := false
		if strings.Contains(content, ".") && !strings.HasPrefix(content, "$") {
			// Check if it looks like Entity.Attribute (letter followed by word chars, dot, letter followed by word chars)
			// This avoids matching strings like "Version 1.0" or "Dashboard - V2.1"
			isEntityPath = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\.[A-Za-z_][A-Za-z0-9_]*$`).MatchString(content)
		}
		// A `$`-prefixed value is a variable reference ONLY when the `$` is followed
		// by a valid identifier start (letter/underscore): `$var`, `$widget.Attr`.
		// `$318` (dollar + digits) is not a valid Mendix variable, so treat it as
		// LITERAL content — otherwise it became an unbound `{1}` param and the build
		// failed CE0402/CE1613 ("attribute '$318' no longer exists"). (traceops #10)
		if isDynamicTextVariableRef(content) || isEntityPath {
			autoGeneratedParams = append(autoGeneratedParams, content)
			content = "{1}"
		}
	}

	// Attribute: X binds the dynamic text to an attribute (issue #650), equivalent
	// to `ContentParams: [{1} = X]`. Without this the Attribute was dropped, leaving
	// an orphaned `{1}` template with no parameter — which Studio Pro can't open
	// (NullReferenceException in ClientTemplateFormPart.CollectControls).
	if attr := w.GetAttribute(); attr != "" && explicitParams == nil && len(autoGeneratedParams) == 0 {
		autoGeneratedParams = append(autoGeneratedParams, attr)
		if content == "" {
			content = "{1}"
		}
	}

	// Empty content with NO parameters is a literal empty template. Do NOT
	// synthesize an orphaned `{1}` placeholder — Mendix rejects a template whose
	// highest placeholder index exceeds its parameter count with CE0720 ("Place
	// holder index 1 is greater than 0"). Only default to `{1}` when there is a
	// parameter to bind to it. (traceops #9)
	if content == "" && (len(autoGeneratedParams) > 0 || len(explicitParams) > 0) {
		content = "{1}"
	}

	dt.Content = &pages.ClientTemplate{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$ClientTemplate",
		},
		Template: &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": content},
		},
	}

	// Add auto-generated parameters first
	for _, attrRef := range autoGeneratedParams {
		param := &pages.ClientTemplateParameter{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ClientTemplateParameter",
			},
		}
		pb.resolveTemplateAttributePathFull(attrRef, param)
		dt.Content.Parameters = append(dt.Content.Parameters, param)
	}

	// Handle explicit ContentParams
	if explicitParams != nil {
		for _, p := range explicitParams {
			param := &pages.ClientTemplateParameter{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "Forms$ClientTemplateParameter",
				},
			}
			// Check if it's an attribute reference or literal
			if strVal, ok := p.Value.(string); ok {
				if strings.HasPrefix(strVal, "'") || strings.HasPrefix(strVal, "\"") {
					// Already a quoted string literal - use as-is
					param.Expression = strVal
				} else if strings.HasPrefix(strVal, "$") || strings.Contains(strVal, ".") {
					// Attribute reference - resolve widget references to entity paths
					pb.resolveTemplateAttributePathFull(strVal, param)
				} else {
					// Unquoted literal value - assume attribute in current context
					pb.resolveTemplateAttributePathFull(strVal, param)
				}
			}
			param.FormattingInfo = formattingInfoFromParamFormat(p.Format)
			dt.Content.Parameters = append(dt.Content.Parameters, param)
		}
	}

	if err := pb.registerWidgetName(w.Name, dt.ID); err != nil {
		return nil, err
	}

	return dt, nil
}

// formattingInfoFromParamFormat coerces a parsed per-parameter format block into
// an SDK FormattingInfo. Returns nil when there is no block, so the writers keep
// emitting the existing hardcoded defaults for every unformatted parameter. When
// a block IS present it starts from those same defaults and applies the user's
// keys on top, so only the specified fields change. Unknown keys / invalid values
// are ignored here — check-time validation (MDL-WIDGET18) reports them.
func formattingInfoFromParamFormat(f *ast.ParamFormatV3) *pages.FormattingInfo {
	if f == nil || len(f.Props) == 0 {
		return nil
	}
	fi := &pages.FormattingInfo{
		DateFormat:       "Date",
		DecimalPrecision: 2,
		EnumFormat:       "Text",
	}
	for _, p := range f.Props {
		switch p.Key {
		case "decimalprecision":
			if n, err := strconv.Atoi(p.Value); err == nil {
				fi.DecimalPrecision = n
			}
		case "groupdigits":
			fi.GroupDigits = strings.EqualFold(p.Value, "true")
		case "dateformat":
			fi.DateFormat = p.Value
		case "customdateformat":
			fi.CustomDateFormat = p.Value
		case "enumformat":
			fi.EnumFormat = p.Value
		}
	}
	return fi
}

func (pb *pageBuilder) buildTitleV3(w *ast.WidgetV3) (*pages.Title, error) {
	title := &pages.Title{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$Title",
			},
			Name: w.Name,
		},
	}

	// Set caption from Content property
	content := w.GetContent()
	if content != "" {
		title.Caption = &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": content},
		}
	}

	if err := pb.registerWidgetName(w.Name, title.ID); err != nil {
		return nil, err
	}

	return title, nil
}

func (pb *pageBuilder) buildButtonV3(w *ast.WidgetV3) (*pages.ActionButton, error) {
	btn := &pages.ActionButton{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ActionButton",
			},
			Name: w.Name,
		},
		ButtonStyle: pages.ButtonStyleDefault,
		RenderMode:  pages.ButtonRenderModeButton,
	}
	// A `linkbutton` is an action button rendered as a link (Forms$ActionButton
	// with RenderType "Link"), not the legacy address-based Forms$LinkButton.
	if strings.ToLower(w.Type) == "linkbutton" {
		btn.RenderMode = pages.ButtonRenderModeLink
	}

	// Handle Caption
	if caption := w.GetCaption(); caption != "" {
		btn.CaptionTemplate = &pages.ClientTemplate{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ClientTemplate",
			},
			Template: &model.Text{
				BaseElement: model.BaseElement{
					ID:       model.ID(types.GenerateID()),
					TypeName: "Texts$Text",
				},
				Translations: map[string]string{"en_US": caption},
			},
		}

		// Handle CaptionParams (template parameters like {1}, {2})
		if params := w.GetCaptionParams(); params != nil {
			for _, p := range params {
				param := &pages.ClientTemplateParameter{
					BaseElement: model.BaseElement{
						ID:       model.ID(types.GenerateID()),
						TypeName: "Forms$ClientTemplateParameter",
					},
				}
				// Check if it's an attribute reference or literal
				if strVal, ok := p.Value.(string); ok {
					if strings.HasPrefix(strVal, "'") || strings.HasPrefix(strVal, "\"") {
						// Already a quoted string literal - use as-is
						param.Expression = strVal
					} else if strings.HasPrefix(strVal, "$") || strings.Contains(strVal, ".") {
						// Attribute reference - resolve widget references to entity paths
						param.AttributeRef = pb.resolveTemplateAttributePath(strVal)
					} else {
						// Unquoted literal value - wrap in quotes for expression
						param.Expression = "'" + strVal + "'"
					}
				}
				btn.CaptionTemplate.Parameters = append(btn.CaptionTemplate.Parameters, param)
			}
		}
	}

	// Handle ButtonStyle. Normalize case (so `primary` becomes `Primary`) and
	// reject unknown values up front — an unrecognized style is silently
	// degraded to btn-default by MxBuild, which is a quiet authoring footgun.
	if style := w.GetButtonStyle(); style != "" {
		canonical, ok := pages.CanonicalButtonStyle(style)
		if !ok {
			return nil, mdlerrors.NewValidationf(
				"button %q: unknown button style %q — valid styles are %s",
				w.Name, style, strings.Join(pages.ValidButtonStyleList(), ", "),
			)
		}
		btn.ButtonStyle = canonical
	}

	// Handle Action
	if action := w.GetAction(); action != nil {
		act, err := pb.buildClientActionV3(action)
		if err != nil {
			return nil, mdlerrors.NewBackend("build action", err)
		}
		btn.Action = act
	}

	// Handle Icon (issue #602): an icon-collection reference (a modern Atlas
	// icon) — e.g. `icon: 'Atlas_Core.Atlas_Filled.pencil'` — serialized as a
	// Forms$IconCollectionIcon.
	if iconRef := strings.Trim(strings.TrimSpace(w.GetStringProp("icon")), "'\""); iconRef != "" {
		btn.Icon = &pages.Icon{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$IconCollectionIcon",
			},
			Type:  pages.IconTypeIconCollection,
			Image: iconRef,
		}
	}

	if err := pb.registerWidgetName(w.Name, btn.ID); err != nil {
		return nil, err
	}

	return btn, nil
}

// buildNavigationListV3 creates a NavigationList widget from V3 syntax.
func (pb *pageBuilder) buildNavigationListV3(w *ast.WidgetV3) (*pages.NavigationList, error) {
	navList := &pages.NavigationList{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$NavigationList",
			},
			Name: w.Name,
		},
	}

	// Build items from children (ITEM widgets)
	for _, child := range w.Children {
		if strings.ToLower(child.Type) == "item" {
			item, err := pb.buildNavigationListItemV3(child)
			if err != nil {
				return nil, err
			}
			navList.Items = append(navList.Items, item)
		}
	}

	if err := pb.registerWidgetName(w.Name, navList.ID); err != nil {
		return nil, err
	}

	return navList, nil
}

// buildNavigationListItemV3 creates a NavigationListItem from V3 syntax.
func (pb *pageBuilder) buildNavigationListItemV3(w *ast.WidgetV3) (*pages.NavigationListItem, error) {
	if w.Name == "" {
		return nil, mdlerrors.NewValidation("item inside navigationlist requires a name")
	}

	item := &pages.NavigationListItem{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$NavigationListItem",
		},
		Name: w.Name,
	}

	if err := pb.registerWidgetName(w.Name, item.ID); err != nil {
		return nil, err
	}

	// Set caption from Caption property
	if caption := w.GetCaption(); caption != "" {
		item.Caption = &model.Text{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Texts$Text",
			},
			Translations: map[string]string{"en_US": caption},
		}
	}

	// Handle Action property
	if action := w.GetAction(); action != nil {
		clientAction, err := pb.buildClientActionV3(action)
		if err != nil {
			return nil, err
		}
		item.Action = clientAction
	}

	// Build child widgets
	for _, child := range w.Children {
		childWidget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		item.Widgets = append(item.Widgets, childWidget)
	}

	return item, nil
}

// buildSnippetCallV3 creates a SnippetCallWidget from V3 syntax.
func (pb *pageBuilder) buildSnippetCallV3(w *ast.WidgetV3) (*pages.SnippetCallWidget, error) {
	sc := &pages.SnippetCallWidget{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$SnippetCallWidget",
			},
			Name: w.Name,
		},
	}

	// Handle Snippet property - resolve snippet and store both ID and name
	snippetName := w.GetSnippet()
	if snippetName != "" {
		snippetID, err := pb.resolveSnippetRef(snippetName)
		if err != nil {
			return nil, mdlerrors.NewBackend(fmt.Sprintf("resolve snippet %s", snippetName), err)
		}
		sc.SnippetID = snippetID
		sc.SnippetName = snippetName // Store qualified name for BY_NAME_REFERENCE serialization

		// Validate and wire up parameter mappings.
		if err := pb.buildSnippetCallParams(sc, snippetName, w.GetSnippetParams()); err != nil {
			return nil, err
		}
	}

	if err := pb.registerWidgetName(w.Name, sc.ID); err != nil {
		return nil, err
	}

	return sc, nil
}

// buildSnippetCallParams validates the supplied param mappings against the
// snippet's declared parameters and populates sc.ParameterMappings.
func (pb *pageBuilder) buildSnippetCallParams(sc *pages.SnippetCallWidget, snippetQName string, supplied []ast.SnippetCallParam) error {
	snippets, err := pb.backend.ListSnippets()
	if err != nil {
		return err
	}

	// Find the target snippet to read its declared parameters.
	var targetSnippet *pages.Snippet
	for _, s := range snippets {
		if s.Name != "" && (s.Name == snippetQName || strings.HasSuffix(snippetQName, "."+s.Name)) {
			targetSnippet = s
			break
		}
	}
	if targetSnippet == nil || len(targetSnippet.Parameters) == 0 {
		// Snippet has no declared parameters — nothing to validate or map.
		return nil
	}

	// Build a lookup of supplied mappings by parameter name (strip leading $).
	suppliedByName := make(map[string]string, len(supplied))
	for _, p := range supplied {
		name := strings.TrimPrefix(p.ParamName, "$")
		suppliedByName[name] = p.Variable
	}

	// Validate that every declared parameter is satisfied, then build the list.
	//
	// "Satisfied by the enclosing data context" is NOT a variable in Mendix — it
	// is the ABSENCE of a mapping. Writing `$currentObject` as one produced
	// `Forms$PageVariable{PageParameter: "currentObject"}`, a by-name reference
	// to a page parameter that does not exist, and mxbuild rejected the call with
	// CE0115 "…do not match the expected parameters and need to be refreshed."
	// Studio Pro's own output has no mapping here, which is why its "Refresh
	// snippet parameters" deletes what mxcli wrote (#868).
	for _, declared := range targetSnippet.Parameters {
		argument, ok := suppliedByName[declared.Name]
		if ok && isContextArgument(argument) {
			// Explicitly context-bound: emit nothing for this parameter.
			continue
		}
		if !ok {
			// Omitted. Legal exactly when the surrounding data context supplies
			// the parameter's entity; otherwise nothing can satisfy it and the
			// author gets the guidance rather than a build error later.
			if pb.entityContext != "" && declared.EntityName != "" &&
				strings.EqualFold(pb.entityContext, declared.EntityName) {
				continue
			}
			return mdlerrors.NewValidationf(
				"snippet %s requires parameter $%s — add Params: {%s: $<variable>} to the SNIPPETCALL, "+
					"or place the call inside a data context of %s so the parameter is satisfied from it",
				snippetQName, declared.Name, declared.Name, orDefaultStr(declared.EntityName, "the parameter's entity"),
			)
		}
		sc.ParameterMappings = append(sc.ParameterMappings, pages.SnippetParamMapping{
			ParamName: declared.Name,
			Argument:  argument,
		})
	}

	return nil
}

// isContextArgument reports whether a SNIPPETCALL argument names the enclosing
// data context rather than a real variable. Mendix has no variable for it, so
// such a parameter is left unmapped.
func isContextArgument(argument string) bool {
	return strings.EqualFold(strings.TrimPrefix(argument, "$"), "currentObject")
}

// orDefaultStr returns s, or fallback when s is empty.
func orDefaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// buildTemplateV3 creates a Container to hold template content.
func (pb *pageBuilder) buildTemplateV3(w *ast.WidgetV3) (*pages.Container, error) {
	container := &pages.Container{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DivContainer",
			},
			Name: w.Name,
		},
	}

	// Build children
	for _, child := range w.Children {
		childWidget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		container.Widgets = append(container.Widgets, childWidget)
	}

	return container, nil
}

// buildFilterV3 creates a Container to hold filter widgets.
func (pb *pageBuilder) buildFilterV3(w *ast.WidgetV3) (*pages.Container, error) {
	container := &pages.Container{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$DivContainer",
			},
			Name: w.Name,
		},
	}

	// Build children (filter widgets)
	for _, child := range w.Children {
		childWidget, err := pb.buildWidgetV3(child)
		if err != nil {
			return nil, err
		}
		container.Widgets = append(container.Widgets, childWidget)
	}

	return container, nil
}

func (pb *pageBuilder) buildStaticImageV3(w *ast.WidgetV3) (*pages.StaticImage, error) {
	img := &pages.StaticImage{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$StaticImageViewer",
			},
			Name: w.Name,
		},
		Responsive: true,
	}

	if width := w.GetIntProp("Width"); width > 0 {
		img.Width = width
	}
	if height := w.GetIntProp("Height"); height > 0 {
		img.Height = height
	}

	if err := pb.registerWidgetName(w.Name, img.ID); err != nil {
		return nil, err
	}

	return img, nil
}

func (pb *pageBuilder) buildDynamicImageV3(w *ast.WidgetV3) (*pages.DynamicImage, error) {
	img := &pages.DynamicImage{
		BaseWidget: pages.BaseWidget{
			BaseElement: model.BaseElement{
				ID:       model.ID(types.GenerateID()),
				TypeName: "Forms$ImageViewer",
			},
			Name: w.Name,
		},
		Responsive: true,
	}

	if width := w.GetIntProp("Width"); width > 0 {
		img.Width = width
	}
	if height := w.GetIntProp("Height"); height > 0 {
		img.Height = height
	}

	if err := pb.registerWidgetName(w.Name, img.ID); err != nil {
		return nil, err
	}

	return img, nil
}

// dataGridFilterWidgetID maps a MDL filter type keyword to its pluggable widget ID.
// Returns "" for non-filter widget types.
func dataGridFilterWidgetID(widgetType string) string {
	switch strings.ToLower(widgetType) {
	case "textfilter":
		return pages.WidgetIDDataGridTextFilter
	case "numberfilter":
		return pages.WidgetIDDataGridNumberFilter
	case "datefilter":
		return pages.WidgetIDDataGridDateFilter
	case "dropdownfilter":
		return pages.WidgetIDDataGridDropdownFilter
	}
	return ""
}

// lookupPropCI reports whether a property is present, matching the key
// case-insensitively the way GetStringProp/GetBoolProp resolve values. Presence has
// to be tested the same way the value is read, or `showFooter:` would be read as
// absent while `ShowFooter:` was honoured.
func lookupPropCI(w *ast.WidgetV3, key string) (any, bool) {
	if v, ok := w.Properties[key]; ok {
		return v, true
	}
	lower := strings.ToLower(key)
	for k, v := range w.Properties {
		if strings.ToLower(k) == lower {
			return v, true
		}
	}
	return nil, false
}

// propBool coerces a widget property value to a bool. GetBoolProp cannot be used
// here: it is case-SENSITIVE (unlike GetStringProp) and accepts only a real bool, so
// `showFooter: true` silently read as false — the property was found and its value
// discarded, which is the same silent-drop this fix is closing.
func propBool(v any) (bool, error) {
	switch x := v.(type) {
	case bool:
		return x, nil
	case string:
		switch strings.ToLower(x) {
		case "true", "yes":
			return true, nil
		case "false", "no":
			return false, nil
		}
		return false, fmt.Errorf("invalid value %q (expected true or false)", x)
	}
	return false, fmt.Errorf("invalid value %v (expected true or false)", v)
}
