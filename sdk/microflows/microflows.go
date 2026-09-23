// SPDX-License-Identifier: Apache-2.0

// Package microflows provides types for Mendix microflows and nanoflows.
package microflows

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// Microflow represents a microflow in the Mendix model.
type Microflow struct {
	model.BaseElement
	ContainerID              model.ID `json:"containerId"`
	Name                     string   `json:"name"`
	Documentation            string   `json:"documentation,omitempty"`
	AllowConcurrentExecution bool     `json:"allowConcurrentExecution"`
	MarkAsUsed               bool     `json:"markAsUsed"`
	Excluded                 bool     `json:"excluded"`
	// ApplyEntityAccess makes the microflow run under the current user's entity
	// access rules instead of with full access — Studio Pro's "Apply entity
	// access" checkbox.
	//
	// It is a SECURITY setting and it is only ever narrowing, so losing it
	// widens what the microflow may read and write with nothing to show for it:
	// the model stays valid, the app builds, and only a constrained user
	// behaves differently. Both writers used to hardcode false and this struct
	// had no field at all, so every rewrite cleared it — the third property in
	// this struct to go that way, after AllowConcurrentExecution and
	// MarkAsUsed (#723 §A).
	ApplyEntityAccess bool `json:"applyEntityAccess"`

	// ConcurrencyErrorMessage and ConcurrencyErrorMicroflow are what Mendix does
	// when a second invocation arrives while one is already running and
	// AllowConcurrentExecution is false — show this (translatable) message, or
	// run this microflow. Mendix requires one of them in that case (CE4899).
	//
	// Neither has MDL syntax, and neither did AllowConcurrentExecution or
	// MarkAsUsed, so the executor's rebuild wrote its own defaults over all
	// four. The direction matters: the rebuild hardcoded `true`, so a microflow
	// that DISALLOWED concurrent execution came back allowing it — the app's
	// concurrency protection removed — and because "allow" needs no error
	// message, CE4899 does not fire and nothing reports it. The error message
	// and microflow went with it, translations included.
	ConcurrencyErrorMessage   *model.Text `json:"concurrencyErrorMessage,omitempty"`
	ConcurrencyErrorMicroflow string      `json:"concurrencyErrorMicroflow,omitempty"`

	// ExportLevel is Studio Pro's "Export level" — `Hidden` or `API`, the two
	// members MicroflowsExportLevel declares. It decides whether the microflow
	// is part of the module's public surface when the module is exported as a
	// package, so losing it makes a protected module's API silently smaller.
	//
	// Carried, not authored: MDL has no syntax for it. Empty means "the stored
	// document said nothing", and the writer defaults that to `Hidden` — never
	// to the empty string, which is not a member of the enum.
	//
	// Measured across three real marketplace modules (Business Events 3.12.0,
	// External Database Connector 6.2.3 and 6.3.0): every document of every
	// type stores `Hidden`, because all three export at module level `Source`.
	// So `Hidden` is the overwhelmingly common value and the right default —
	// but it is a default, not the only value, and hardcoding it is what made
	// this a drop rather than a no-op (#1120 follow-up).
	ExportLevel string `json:"exportLevel,omitempty"`

	// URL is the microflow's deep link (Mendix 10.6+) — Studio Pro's "URL"
	// field, e.g. `item/{Key}`. MDL has no syntax for it, so it is carried
	// across a rewrite rather than authored.
	//
	// The fourth property in this struct to be lost the way #723 §A describes,
	// after AllowConcurrentExecution, MarkAsUsed and ApplyEntityAccess: the
	// writer hardcoded "" and this struct had no field, so every rewrite
	// deleted the deep link. Nothing reports it — `mxcli check` and `mx check`
	// both pass, because a microflow without a URL is perfectly valid; the loss
	// is only visible in Studio Pro, which is how it reached a user (#1120).
	URL string `json:"url,omitempty"`
	// URLSearchParameters names the microflow parameters supplied as query-string
	// arguments of the deep link, as qualified names. Stored beside URL and lost
	// with it.
	URLSearchParameters []string `json:"urlSearchParameters,omitempty"`

	// Return type
	ReturnType         DataType `json:"returnType,omitempty"`
	ReturnVariableName string   `json:"returnVariableName,omitempty"` // Variable name for return value (e.g., "$Result")

	// Parameters
	Parameters []*MicroflowParameter `json:"parameters,omitempty"`

	// Flow elements
	ObjectCollection *MicroflowObjectCollection `json:"objectCollection,omitempty"`

	// Allowed module roles for execution
	AllowedModuleRoles []model.ID `json:"allowedModuleRoles,omitempty"`

	// Deprecated: never read and never written, and it does not describe what
	// Mendix stores — there is no thread count in the model. The real
	// concurrency state is AllowConcurrentExecution plus the two
	// ConcurrencyError fields above. Kept only because the type is exported.
	ConcurrentExecutionSettings *ConcurrentExecutionSettings `json:"concurrentExecutionSettings,omitempty"`

	// Toolbox entries. A microflow can be exposed twice — once for the microflow
	// editor's toolbox and once for the workflow editor's — and Mendix stores the
	// two under different keys with the same element type. Whoever drags the
	// result in does not need to know it is a microflow, which is the point.
	MicroflowActionInfo *types.MicroflowActionInfo `json:"microflowActionInfo,omitempty"`
	WorkflowActionInfo  *types.MicroflowActionInfo `json:"workflowActionInfo,omitempty"`
}

// GetName returns the microflow's name.
func (m *Microflow) GetName() string {
	return m.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (m *Microflow) GetContainerID() model.ID {
	return m.ContainerID
}

// Nanoflow represents a nanoflow in the Mendix model.
// Nanoflows run on the client side and have restrictions on which activities can be used.
type Nanoflow struct {
	model.BaseElement
	ContainerID        model.ID   `json:"containerId"`
	Name               string     `json:"name"`
	Documentation      string     `json:"documentation,omitempty"`
	MarkAsUsed         bool       `json:"markAsUsed"`
	Excluded           bool       `json:"excluded"`
	AllowedModuleRoles []model.ID `json:"allowedModuleRoles,omitempty"`

	// Return type
	ReturnType DataType `json:"returnType,omitempty"`

	// Parameters
	Parameters []*MicroflowParameter `json:"parameters,omitempty"`

	// Flow elements
	ObjectCollection *MicroflowObjectCollection `json:"objectCollection,omitempty"`
}

// GetName returns the nanoflow's name.
func (n *Nanoflow) GetName() string {
	return n.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (n *Nanoflow) GetContainerID() model.ID {
	return n.ContainerID
}

// Rule represents a rule (Microflows$Rule) in the Mendix model — Mendix's own
// reference calls it "a special kind of microflow" that returns a Boolean or an
// enumeration and may only be used from a decision.
//
// The fields are the ten properties a rule document stores, measured against two
// Studio Pro-authored rules (ako/TestApp, Mendix 11.13.0). A rule is a microflow
// minus nine properties, and the nine are the ones a rule has no concept of:
// AllowedModuleRoles (a rule is not independently callable, so there is nothing
// to grant), the concurrency group, Url/UrlSearchParameters, StableId, and the
// two action-info slots.
type Rule struct {
	model.BaseElement
	ContainerID        model.ID `json:"containerId"`
	Name               string   `json:"name"`
	Documentation      string   `json:"documentation,omitempty"`
	Excluded           bool     `json:"excluded"`
	MarkAsUsed         bool     `json:"markAsUsed"`
	ApplyEntityAccess  bool     `json:"applyEntityAccess"`
	ReturnVariableName string   `json:"returnVariableName,omitempty"`

	// Return type — Boolean or an enumeration. Not "always boolean": Rules.Rule2
	// in the reference app returns Rules.RuleResult.
	ReturnType DataType `json:"returnType,omitempty"`

	// Parameters
	Parameters []*MicroflowParameter `json:"parameters,omitempty"`

	// Flow elements
	ObjectCollection *MicroflowObjectCollection `json:"objectCollection,omitempty"`
}

// GetName returns the rule's name.
func (r *Rule) GetName() string {
	return r.Name
}

// GetContainerID returns the ID of the containing folder/module.
func (r *Rule) GetContainerID() model.ID {
	return r.ContainerID
}

// MicroflowParameter represents a parameter of a microflow.
type MicroflowParameter struct {
	model.BaseElement
	ContainerID   model.ID `json:"containerId"`
	Name          string   `json:"name"`
	Documentation string   `json:"documentation,omitempty"`
	Type          DataType `json:"type"`

	// Position is the parameter's place on the canvas, set only when those
	// coordinates say something DerivedParameterPosition would not have
	// produced — see that function for why the distinction is the whole point.
	// nil means "wherever the layout puts it", which is why this is a pointer:
	// 0;0 is a position a person can choose, and two flows in the reference
	// project use it.
	Position *model.Point `json:"position,omitempty"`
}

// DerivedParameterPosition returns where mxcli's own layout puts the parameter
// at index idx: a row of boxes along the top of the canvas, one spacing unit
// apart. Both writers used to compute this inline and unconditionally, which is
// why a hand-placed parameter did not survive a rewrite (#993).
//
// It exists so that both readers can tell an authored position from mxcli's own
// arithmetic handed back. A parameter sitting exactly here carries no intent and
// is re-derived on the next write; one anywhere else was put there by a person
// and is kept. This is the arbitration authoredStartPosition makes for the
// StartEvent, and it is made for the same reason: carrying stored coordinates
// over UNCONDITIONALLY pins the node, so inserting a parameter would leave the
// existing ones stranded on the old grid while the new one lands on top of them
// (the shape of #951, one node family over).
//
// A person who places a parameter exactly where the layout would have is
// indistinguishable from the layout — and re-deriving gives back the same point,
// so the ambiguity costs nothing.
func DerivedParameterPosition(idx int) model.Point {
	return model.Point{X: 200 + idx*100, Y: 53}
}

// AuthoredParameterPosition returns stored, or nil when stored is the point the
// layout would have derived for index idx. Readers call this so that everything
// downstream can treat a non-nil Position as intent.
func AuthoredParameterPosition(stored model.Point, idx int) *model.Point {
	if stored == DerivedParameterPosition(idx) {
		return nil
	}
	p := stored
	return &p
}

// GetName returns the parameter's name.
func (p *MicroflowParameter) GetName() string {
	return p.Name
}

// GetContainerID returns the ID of the containing microflow.
func (p *MicroflowParameter) GetContainerID() model.ID {
	return p.ContainerID
}

// MicroflowObjectCollection contains all objects and flows in a microflow.
type MicroflowObjectCollection struct {
	model.BaseElement
	Objects         []MicroflowObject `json:"objects,omitempty"`
	Flows           []*SequenceFlow   `json:"flows,omitempty"`
	AnnotationFlows []*AnnotationFlow `json:"annotationFlows,omitempty"`
}

// MicroflowObject is the base interface for all microflow objects.
type MicroflowObject interface {
	GetID() model.ID
	GetPosition() model.Point
	SetPosition(p model.Point)
}

// BaseMicroflowObject provides common fields for microflow objects.
type BaseMicroflowObject struct {
	model.BaseElement
	Position       model.Point `json:"position"`
	Size           model.Size  `json:"size,omitempty"`
	RelativeMiddle model.Point `json:"relativeMiddle,omitempty"`
}

// GetSize returns the object's box size.
func (o *BaseMicroflowObject) GetSize() model.Size {
	return o.Size
}

// SetSize sets the object's box size.
func (o *BaseMicroflowObject) SetSize(s model.Size) {
	o.Size = s
}

// GetPosition returns the object's position.
func (o *BaseMicroflowObject) GetPosition() model.Point {
	return o.Position
}

// SetPosition sets the object's position.
func (o *BaseMicroflowObject) SetPosition(p model.Point) {
	o.Position = p
}

// SequenceFlow represents a flow connection between objects.
type SequenceFlow struct {
	model.BaseElement
	OriginID                   model.ID  `json:"originId"`
	DestinationID              model.ID  `json:"destinationId"`
	OriginConnectionIndex      int       `json:"originConnectionIndex"`
	DestinationConnectionIndex int       `json:"destinationConnectionIndex"`
	CaseValue                  CaseValue `json:"caseValue,omitempty"`
	IsErrorHandler             bool      `json:"isErrorHandler,omitempty"`
	OriginControlVector        string    `json:"originControlVector,omitempty"`
	DestinationControlVector   string    `json:"destinationControlVector,omitempty"`
}

// CaseValue represents a case value for a decision flow.
type CaseValue interface {
	isCaseValue()
}

// NoCase represents no case (default flow).
type NoCase struct {
	model.BaseElement
}

func (NoCase) isCaseValue() {}

// EnumerationCase represents an enumeration case value.
type EnumerationCase struct {
	model.BaseElement
	Value string `json:"value"`
}

func (EnumerationCase) isCaseValue() {}

// InheritanceCase represents an inheritance/type case value.
type InheritanceCase struct {
	model.BaseElement
	EntityID            model.ID `json:"entityId"`
	EntityQualifiedName string   `json:"entityQualifiedName,omitempty"`
}

func (InheritanceCase) isCaseValue() {}

// BooleanCase represents a boolean case value.
type BooleanCase struct {
	model.BaseElement
	Value bool `json:"value"`
}

func (BooleanCase) isCaseValue() {}

// ExpressionCase represents an expression-based case value (true/false branches).
type ExpressionCase struct {
	model.BaseElement
	Expression string `json:"expression"`
}

func (ExpressionCase) isCaseValue() {}

// Annotation represents an annotation in a microflow.
type Annotation struct {
	BaseMicroflowObject
	Caption string `json:"caption"`
}

// AnnotationFlow connects an annotation to an object.
type AnnotationFlow struct {
	model.BaseElement
	OriginID      model.ID `json:"originId"`
	DestinationID model.ID `json:"destinationId"`
}

// Events

// StartEvent represents the start of a microflow.
type StartEvent struct {
	BaseMicroflowObject
}

// EndEvent represents the end of a microflow.
type EndEvent struct {
	BaseMicroflowObject
	ReturnValue string `json:"returnValue,omitempty"`
}

// ContinueEvent represents a continue in a loop.
type ContinueEvent struct {
	BaseMicroflowObject
}

// BreakEvent represents a break in a loop.
type BreakEvent struct {
	BaseMicroflowObject
}

// ErrorEvent represents an error throw in a microflow.
type ErrorEvent struct {
	BaseMicroflowObject
}

// Decisions and Control Flow

// ExclusiveSplit represents an exclusive decision (if/else).
type ExclusiveSplit struct {
	BaseMicroflowObject
	Caption           string            `json:"caption,omitempty"`
	Documentation     string            `json:"documentation,omitempty"`
	SplitCondition    SplitCondition    `json:"splitCondition,omitempty"`
	ErrorHandlingType ErrorHandlingType `json:"errorHandlingType,omitempty"`
}

// ExclusiveMerge represents a merge point for exclusive splits.
type ExclusiveMerge struct {
	BaseMicroflowObject
}

// InheritanceSplit represents a type-based decision.
type InheritanceSplit struct {
	BaseMicroflowObject
	Caption           string            `json:"caption,omitempty"`
	Documentation     string            `json:"documentation,omitempty"`
	VariableName      string            `json:"variableName"`
	ErrorHandlingType ErrorHandlingType `json:"errorHandlingType,omitempty"`
}

// SplitCondition represents the condition for a split.
type SplitCondition interface {
	isSplitCondition()
}

// ExpressionSplitCondition represents an expression-based split condition.
type ExpressionSplitCondition struct {
	model.BaseElement
	Expression string `json:"expression"`
}

func (ExpressionSplitCondition) isSplitCondition() {}

// RuleSplitCondition represents a rule-based split condition.
// In Mendix BSON the rule is referenced by qualified name under the
// "RuleCall.Microflow" field (rules and microflows share a namespace).
type RuleSplitCondition struct {
	model.BaseElement
	RuleQualifiedName string                      `json:"ruleQualifiedName,omitempty"`
	ParameterMappings []*RuleCallParameterMapping `json:"parameterMappings,omitempty"`
}

func (RuleSplitCondition) isSplitCondition() {}

// RuleCallParameterMapping maps a parameter to a value.
// ParameterName is the fully qualified parameter name (e.g. "Module.Rule.ParamName")
// as stored in BSON; used when the describer renders the rule call.
type RuleCallParameterMapping struct {
	model.BaseElement
	ParameterID   model.ID `json:"parameterId"`
	ParameterName string   `json:"parameterName,omitempty"`
	Argument      string   `json:"argument"`
}

// LoopSource is the source for a LoopedActivity. Either IterableList (FOR EACH) or WhileLoopCondition (WHILE).
type LoopSource interface {
	isLoopSource()
}

// LoopedActivity represents a loop construct (FOR EACH or WHILE).
type LoopedActivity struct {
	BaseMicroflowObject
	Caption           string                     `json:"caption,omitempty"`
	Documentation     string                     `json:"documentation,omitempty"`
	LoopSource        LoopSource                 `json:"loopSource,omitempty"`
	ObjectCollection  *MicroflowObjectCollection `json:"objectCollection,omitempty"`
	ErrorHandlingType ErrorHandlingType          `json:"errorHandlingType,omitempty"`
}

// IterableList represents the source for a FOR EACH loop iteration.
type IterableList struct {
	model.BaseElement
	ListVariableName string `json:"listVariableName"` // The list to iterate over
	VariableName     string `json:"variableName"`     // The iterator variable name
}

func (*IterableList) isLoopSource() {}

// WhileLoopCondition represents the source for a WHILE loop.
type WhileLoopCondition struct {
	model.BaseElement
	WhileExpression string `json:"whileExpression"` // The condition expression
}

func (*WhileLoopCondition) isLoopSource() {}

// ErrorHandlingType represents how errors are handled.
type ErrorHandlingType string

const (
	ErrorHandlingTypeAbort                 ErrorHandlingType = "Abort"
	ErrorHandlingTypeContinue              ErrorHandlingType = "Continue"
	ErrorHandlingTypeCustom                ErrorHandlingType = "Custom"
	ErrorHandlingTypeCustomWithoutRollback ErrorHandlingType = "CustomWithoutRollBack"
	ErrorHandlingTypeRollback              ErrorHandlingType = "Rollback"
)

// Activities

// Activity is the base interface for all activities.
type Activity interface {
	MicroflowObject
	IsActivity()
}

// BaseActivity provides common fields for activities.
type BaseActivity struct {
	BaseMicroflowObject
	Caption             string            `json:"caption,omitempty"`
	Documentation       string            `json:"documentation,omitempty"`
	ErrorHandlingType   ErrorHandlingType `json:"errorHandlingType,omitempty"`
	AutoGenerateCaption bool              `json:"autoGenerateCaption"`
	BackgroundColor     string            `json:"backgroundColor,omitempty"`
	Disabled            bool              `json:"disabled"` // @excluded in MDL
}

// IsActivity marks this as an activity.
func (a *BaseActivity) IsActivity() {}

// ActionActivity wraps an action.
type ActionActivity struct {
	BaseActivity
	Action MicroflowAction `json:"action,omitempty"`
}

// ConcurrentExecutionSettings represents settings for concurrent execution.
// Deprecated: a fiction — nothing reads or writes it, and Mendix stores no
// thread count. See Microflow.AllowConcurrentExecution and its
// ConcurrencyErrorMessage / ConcurrencyErrorMicroflow siblings.
type ConcurrentExecutionSettings struct {
	model.BaseElement
	Enabled         bool `json:"enabled"`
	NumberOfThreads int  `json:"numberOfThreads,omitempty"`
}
