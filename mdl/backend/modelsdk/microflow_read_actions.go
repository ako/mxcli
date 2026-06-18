// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// actionFromGen reconstructs the semantic microflow action from an ActionActivity's
// gen Action child, so DESCRIBE/SHOW can render the activity body (the inverse of
// microflowActionToGen). Returns nil for action types not yet reconstructed — the
// activity then renders as an empty action, which is the prior behaviour, so this
// grows incrementally batch-by-batch without regressing already-handled types.
//
// Actions written via gen setters (this batch) read back through the gen
// accessors; the raw-built actions (list-ops, REST, …) will read their explicit
// keys in a later batch.
func actionFromGen(el element.Element) microflows.MicroflowAction {
	switch a := el.(type) {
	case *genMf.LogMessageAction:
		out := &microflows.LogMessageAction{
			ErrorHandlingType:     microflows.ErrorHandlingType(a.ErrorHandlingType()),
			LogLevel:              microflows.LogLevel(a.Level()),
			LogNodeName:           a.Node(),
			IncludeLastStackTrace: a.IncludeLatestStackTrace(),
		}
		out.ID = model.ID(a.ID())
		// MessageTemplate is a Microflows$StringTemplate (scalar Text + Arguments).
		if st, ok := a.MessageTemplate().(*genMf.StringTemplate); ok && st != nil {
			out.MessageTemplate = &model.Text{Translations: map[string]string{"en_US": st.Text()}}
			for _, argEl := range st.ArgumentsItems() {
				if arg, ok := argEl.(*genMf.TemplateArgument); ok {
					out.TemplateParameters = append(out.TemplateParameters, arg.Expression())
				}
			}
		}
		return out

	case *genMf.CreateVariableAction:
		out := &microflows.CreateVariableAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			VariableName:      a.VariableName(),
			InitialValue:      a.InitialValue(),
			DataType:          dataTypeFromGen(a.VariableType()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ChangeVariableAction:
		out := &microflows.ChangeVariableAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			VariableName:      a.ChangeVariableName(),
			Value:             a.Value(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CreateObjectAction:
		out := &microflows.CreateObjectAction{
			ErrorHandlingType:   microflows.ErrorHandlingType(a.ErrorHandlingType()),
			EntityQualifiedName: a.EntityQualifiedName(),
			OutputVariable:      a.OutputVariableName(),
			Commit:              microflows.CommitType(a.Commit()),
			InitialMembers:      memberChangesFromGen(a.ItemsItems()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ChangeObjectAction:
		out := &microflows.ChangeObjectAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			ChangeVariable:    a.ChangeVariableName(),
			Commit:            microflows.CommitType(a.Commit()),
			RefreshInClient:   a.RefreshInClient(),
			Changes:           memberChangesFromGen(a.ItemsItems()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CommitAction:
		out := &microflows.CommitObjectsAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			CommitVariable:    a.CommitVariableName(),
			WithEvents:        a.WithEvents(),
			RefreshInClient:   a.RefreshInClient(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.DeleteAction:
		out := &microflows.DeleteObjectAction{
			DeleteVariable:  a.DeleteVariableName(),
			RefreshInClient: a.RefreshInClient(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.RollbackAction:
		out := &microflows.RollbackObjectAction{
			RollbackVariable: a.RollbackVariableName(),
			RefreshInClient:  a.RefreshInClient(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.RetrieveAction:
		out := &microflows.RetrieveAction{
			OutputVariable: a.OutputVariableName(),
			Source:         retrieveSourceFromGen(a.RetrieveSource()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.MicroflowCallAction:
		out := &microflows.MicroflowCallAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(a.ErrorHandlingType()),
			ResultVariableName: a.OutputVariableName(),
			UseReturnVariable:  a.UseReturnVariable(),
		}
		out.ID = model.ID(a.ID())
		if mc, ok := a.MicroflowCall().(*genMf.MicroflowCall); ok && mc != nil {
			call := &microflows.MicroflowCall{Microflow: mc.MicroflowQualifiedName()}
			call.ID = model.ID(mc.ID())
			for _, pmEl := range mc.ParameterMappingsItems() {
				if pm, ok := pmEl.(*genMf.MicroflowCallParameterMapping); ok {
					m := &microflows.MicroflowCallParameterMapping{Parameter: pm.ParameterQualifiedName(), Argument: pm.Argument()}
					m.ID = model.ID(pm.ID())
					call.ParameterMappings = append(call.ParameterMappings, m)
				}
			}
			out.MicroflowCall = call
		}
		return out

	case *genMf.NanoflowCallAction:
		out := &microflows.NanoflowCallAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(a.ErrorHandlingType()),
			OutputVariableName: a.OutputVariableName(),
			UseReturnVariable:  a.UseReturnVariable(),
		}
		out.ID = model.ID(a.ID())
		if nc, ok := a.NanoflowCall().(*genMf.NanoflowCall); ok && nc != nil {
			call := &microflows.NanoflowCall{Nanoflow: nc.NanoflowQualifiedName()}
			call.ID = model.ID(nc.ID())
			for _, pmEl := range nc.ParameterMappingsItems() {
				if pm, ok := pmEl.(*genMf.NanoflowCallParameterMapping); ok {
					m := &microflows.NanoflowCallParameterMapping{Parameter: pm.ParameterQualifiedName(), Argument: pm.Argument()}
					m.ID = model.ID(pm.ID())
					call.ParameterMappings = append(call.ParameterMappings, m)
				}
			}
			out.NanoflowCall = call
		}
		return out

	case *genMf.CreateListAction:
		out := &microflows.CreateListAction{
			EntityQualifiedName: a.EntityQualifiedName(),
			OutputVariable:      a.OutputVariableName(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ChangeListAction:
		out := &microflows.ChangeListAction{
			ChangeVariable: a.ChangeVariableName(),
			Type:           microflows.ChangeListType(a.Type()),
			Value:          a.Value(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.AggregateListAction:
		out := &microflows.AggregateListAction{
			InputVariable:          a.InputListVariableName(),
			OutputVariable:         a.OutputVariableName(),
			Function:               microflows.AggregateFunction(a.AggregateFunction()),
			AttributeQualifiedName: a.AttributeQualifiedName(),
			UseExpression:          a.UseExpression(),
			Expression:             a.Expression(),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CastAction:
		// ObjectVariable (the cast input) is not stored via a gen setter, so it is
		// not reconstructable here; OutputVariable is.
		out := &microflows.CastAction{OutputVariable: a.OutputVariableName()}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.CloseFormAction:
		out := &microflows.ClosePageAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			NumberOfPages:     int(a.NumberOfPages()),
		}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ValidationFeedbackAction:
		out := &microflows.ValidationFeedbackAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			ObjectVariable:    a.ObjectVariableName(),
			AttributeName:     a.AttributeQualifiedName(),
			AssociationName:   a.AssociationQualifiedName(),
		}
		out.ID = model.ID(a.ID())
		out.Template, out.TemplateParameters = textTemplateFromGen(a.FeedbackTemplate())
		return out

	case *genMf.ShowHomePageAction:
		out := &microflows.ShowHomePageAction{}
		out.ID = model.ID(a.ID())
		return out

	case *genMf.ShowMessageAction:
		out := &microflows.ShowMessageAction{
			ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType()),
			Type:              microflows.MessageType(a.Type()),
			Blocking:          a.Blocking(),
		}
		out.ID = model.ID(a.ID())
		out.Template, out.TemplateParameters = textTemplateFromGen(a.Template())
		return out

	case *genMf.ShowPageAction:
		// Storage $Type Microflows$ShowFormAction. The FormSettings tree is written
		// raw (Form / ParameterMappings with legacy keys), so read it from raw.
		out := &microflows.ShowPageAction{ErrorHandlingType: microflows.ErrorHandlingType(a.ErrorHandlingType())}
		out.ID = model.ID(a.ID())
		if fs, ok := a.Raw().Lookup("FormSettings").DocumentOK(); ok {
			out.FormSettingsID = model.ID(rawStr(fs, "$ID"))
			out.PageName = rawStr(fs, "Form")
			if arr, ok := fs.Lookup("ParameterMappings").ArrayOK(); ok {
				vals, _ := arr.Values()
				for _, v := range vals {
					md, ok := v.DocumentOK()
					if !ok {
						continue
					}
					pm := &microflows.PageParameterMapping{Parameter: rawStr(md, "Parameter"), Argument: rawStr(md, "Argument")}
					pm.ID = model.ID(rawStr(md, "$ID"))
					out.PageParameterMappings = append(out.PageParameterMappings, pm)
				}
			}
		}
		return out

	case *genMf.JavaActionCallAction:
		// Storage keys JavaAction / ResultVariableName diverge from the gen
		// accessors, so read from raw — the inverse of the direct write.
		raw := a.Raw()
		out := &microflows.JavaActionCallAction{
			ErrorHandlingType:  microflows.ErrorHandlingType(rawStr(raw, "ErrorHandlingType")),
			JavaAction:         rawStr(raw, "JavaAction"),
			ResultVariableName: rawStr(raw, "ResultVariableName"),
		}
		out.ID = model.ID(a.ID())
		if b, ok := raw.Lookup("UseReturnVariable").BooleanOK(); ok {
			out.UseReturnVariable = b
		}
		if arr, ok := raw.Lookup("ParameterMappings").ArrayOK(); ok {
			vals, _ := arr.Values()
			for _, v := range vals {
				md, ok := v.DocumentOK()
				if !ok {
					continue
				}
				pm := &microflows.JavaActionParameterMapping{Parameter: rawStr(md, "Parameter")}
				pm.ID = model.ID(rawStr(md, "$ID"))
				if vd, ok := md.Lookup("Value").DocumentOK(); ok {
					pm.Value = codeActionParameterValueFromRaw(vd)
				}
				out.ParameterMappings = append(out.ParameterMappings, pm)
			}
		}
		return out

	case *genMf.ListOperationAction:
		// Storage $Type Microflows$ListOperationsAction. The write binds the output
		// to "ResultVariableName" and the operation to "NewOperation" (not the gen
		// keys), so read both from the raw BSON — the inverse of the write's
		// listOperationToGen.
		raw := a.Raw()
		out := &microflows.ListOperationAction{OutputVariable: rawStr(raw, "ResultVariableName")}
		out.ID = model.ID(a.ID())
		if opDoc, ok := raw.Lookup("NewOperation").DocumentOK(); ok {
			out.Operation = listOperationFromRaw(opDoc)
		}
		return out

	default:
		return nil
	}
}

// textTemplateFromGen reconstructs a Microflows$TextTemplate's translations and
// template arguments (the {1},{2},… expressions). Inverse of textTemplateToGen.
func textTemplateFromGen(el element.Element) (*model.Text, []string) {
	tt, ok := el.(*genMf.TextTemplate)
	if !ok || tt == nil {
		return nil, nil
	}
	var text *model.Text
	if txt, ok := tt.Text().(*genTexts.Text); ok && txt != nil {
		trans := map[string]string{}
		for _, trEl := range txt.TranslationsItems() {
			if tr, ok := trEl.(*genTexts.Translation); ok {
				trans[tr.LanguageCode()] = tr.Text()
			}
		}
		if len(trans) > 0 {
			text = &model.Text{Translations: trans}
		}
	}
	var params []string
	for _, argEl := range tt.ArgumentsItems() {
		if arg, ok := argEl.(*genMf.TemplateArgument); ok {
			params = append(params, arg.Expression())
		}
	}
	return text, params
}

// codeActionParameterValueFromRaw reconstructs a java-action parameter value from
// its raw Value sub-document. Inverse of codeActionParameterValueToGen.
func codeActionParameterValueFromRaw(doc bson.Raw) microflows.CodeActionParameterValue {
	id := model.ID(rawStr(doc, "$ID"))
	switch rawStr(doc, "$Type") {
	case "Microflows$StringTemplateParameterValue":
		v := &microflows.StringTemplateParameterValue{}
		v.ID = id
		if tt, ok := doc.Lookup("TypedTemplate").DocumentOK(); ok {
			t := &microflows.TypedTemplate{Text: rawStr(tt, "Text")}
			t.ID = model.ID(rawStr(tt, "$ID"))
			v.TypedTemplate = t
		}
		return v
	case "Microflows$ExpressionBasedCodeActionParameterValue":
		v := &microflows.ExpressionBasedCodeActionParameterValue{Expression: rawStr(doc, "Expression")}
		v.ID = id
		return v
	case "Microflows$BasicCodeActionParameterValue":
		v := &microflows.BasicCodeActionParameterValue{Argument: rawStr(doc, "Argument")}
		v.ID = id
		return v
	case "Microflows$MicroflowParameterValue":
		v := &microflows.MicroflowParameterValue{Microflow: rawStr(doc, "Microflow")}
		v.ID = id
		return v
	case "Microflows$EntityTypeCodeActionParameterValue":
		v := &microflows.EntityTypeCodeActionParameterValue{Entity: rawStr(doc, "Entity")}
		v.ID = id
		return v
	default:
		return nil
	}
}

// rawStr reads a string field from a raw BSON document, returning "" if the field
// is absent or not a string.
func rawStr(doc bson.Raw, key string) string {
	if doc == nil {
		return ""
	}
	v, _ := doc.Lookup(key).StringValueOK()
	return v
}

// listOperationFromRaw reconstructs a list operation from its NewOperation BSON
// sub-document, the inverse of listOperationToGen. Each operation carries the
// verified legacy storage keys (ListName / SecondListOrObjectName / …).
func listOperationFromRaw(doc bson.Raw) microflows.ListOperation {
	id := model.ID(rawStr(doc, "$ID"))
	list := rawStr(doc, "ListName")
	expr := rawStr(doc, "Expression")
	second := rawStr(doc, "SecondListOrObjectName")
	switch rawStr(doc, "$Type") {
	case "Microflows$Head":
		o := &microflows.HeadOperation{ListVariable: list}
		o.ID = id
		return o
	case "Microflows$Tail":
		o := &microflows.TailOperation{ListVariable: list}
		o.ID = id
		return o
	case "Microflows$FindByExpression":
		o := &microflows.FindOperation{ListVariable: list, Expression: expr}
		o.ID = id
		return o
	case "Microflows$FilterByExpression":
		o := &microflows.FilterOperation{ListVariable: list, Expression: expr}
		o.ID = id
		return o
	case "Microflows$Find":
		o := &microflows.FindByAttributeOperation{ListVariable: list, Association: rawStr(doc, "Association"), Attribute: rawStr(doc, "Attribute"), Expression: expr}
		o.ID = id
		return o
	case "Microflows$Filter":
		o := &microflows.FilterByAttributeOperation{ListVariable: list, Association: rawStr(doc, "Association"), Attribute: rawStr(doc, "Attribute"), Expression: expr}
		o.ID = id
		return o
	case "Microflows$Sort":
		o := &microflows.SortOperation{ListVariable: list, Sorting: sortItemsFromRaw(doc)}
		o.ID = id
		return o
	case "Microflows$Union":
		o := &microflows.UnionOperation{ListVariable1: list, ListVariable2: second}
		o.ID = id
		return o
	case "Microflows$Intersect":
		o := &microflows.IntersectOperation{ListVariable1: list, ListVariable2: second}
		o.ID = id
		return o
	case "Microflows$Subtract":
		o := &microflows.SubtractOperation{ListVariable1: list, ListVariable2: second}
		o.ID = id
		return o
	case "Microflows$Contains":
		o := &microflows.ContainsOperation{ListVariable: list, ObjectVariable: second}
		o.ID = id
		return o
	case "Microflows$Equals":
		o := &microflows.EqualsOperation{ListVariable1: list, ListVariable2: second}
		o.ID = id
		return o
	case "Microflows$ListRange":
		o := &microflows.ListRangeOperation{ListVariable: list, LimitExpression: rawStr(doc, "LimitExpression"), OffsetExpression: rawStr(doc, "OffsetExpression")}
		o.ID = id
		return o
	default:
		return nil
	}
}

// sortItemsFromRaw reconstructs a Sort operation's sort columns from its nested
// SortingsList → Sortings array. The first array element is the typed-array marker
// (an int, not a document) and is skipped by the DocumentOK guard.
func sortItemsFromRaw(doc bson.Raw) []*microflows.SortItem {
	slDoc, ok := doc.Lookup("Sortings").DocumentOK()
	if !ok {
		return nil
	}
	arr, ok := slDoc.Lookup("Sortings").ArrayOK()
	if !ok {
		return nil
	}
	vals, err := arr.Values()
	if err != nil {
		return nil
	}
	var out []*microflows.SortItem
	for _, v := range vals {
		sd, ok := v.DocumentOK()
		if !ok {
			continue
		}
		it := &microflows.SortItem{Direction: microflows.SortDirection(rawStr(sd, "SortOrder"))}
		it.ID = model.ID(rawStr(sd, "$ID"))
		if ref, ok := sd.Lookup("AttributeRef").DocumentOK(); ok {
			// The AttributeRef stores its by-name reference under "Attribute".
			it.AttributeQualifiedName = rawStr(ref, "Attribute")
		}
		out = append(out, it)
	}
	return out
}

// memberChangesFromGen reconstructs the attribute/association assignments of a
// create/change-object action (the inverse of memberChangeToGen).
func memberChangesFromGen(items []element.Element) []*microflows.MemberChange {
	var out []*microflows.MemberChange
	for _, el := range items {
		g, ok := el.(*genMf.MemberChange)
		if !ok {
			continue
		}
		m := &microflows.MemberChange{
			AttributeQualifiedName:   g.AttributeQualifiedName(),
			AssociationQualifiedName: g.AssociationQualifiedName(),
			Type:                     microflows.MemberChangeType(g.Type()),
			Value:                    g.Value(),
		}
		m.ID = model.ID(g.ID())
		out = append(out, m)
	}
	return out
}

// retrieveSourceFromGen reconstructs a retrieve's source (database with XPath/
// range, or association navigation). Inverse of retrieveSourceToGen.
func retrieveSourceFromGen(el element.Element) microflows.RetrieveSource {
	switch g := el.(type) {
	case *genMf.DatabaseRetrieveSource:
		s := &microflows.DatabaseRetrieveSource{
			EntityQualifiedName: g.EntityQualifiedName(),
			XPathConstraint:     g.XPathConstraint(),
			Range:               rangeFromGen(g.Range()),
		}
		s.ID = model.ID(g.ID())
		return s
	case *genMf.AssociationRetrieveSource:
		s := &microflows.AssociationRetrieveSource{
			StartVariable:            g.StartVariableName(),
			AssociationQualifiedName: g.AssociationQualifiedName(),
		}
		s.ID = model.ID(g.ID())
		return s
	default:
		return nil
	}
}

// rangeFromGen maps a gen range element to the model Range. A ConstantRange with
// SingleObject means "first" (limit 1); SingleObject=false has no range. A
// CustomRange carries limit/offset expressions.
func rangeFromGen(el element.Element) *microflows.Range {
	switch g := el.(type) {
	case *genMf.ConstantRange:
		if g.SingleObject() {
			return &microflows.Range{RangeType: microflows.RangeTypeFirst}
		}
		return nil
	case *genMf.CustomRange:
		return &microflows.Range{RangeType: microflows.RangeTypeCustom, Limit: g.LimitExpression(), Offset: g.OffsetExpression()}
	default:
		return nil
	}
}
