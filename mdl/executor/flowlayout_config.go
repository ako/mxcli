// SPDX-License-Identifier: Apache-2.0

// Package executor - where a flow's layout style comes from.
//
// Two places, in this order (ako/mxcli#1832):
//
//  1. The flow's own documentation, when it carries an `@mxcli-layout:` line.
//     That line records the style the flow was last laid out with, so a
//     describe → exec round trip rebuilds the same geometry whatever the project
//     says now, and canonical DESCRIBE judges a position "derived" against the
//     style that actually drew it.
//  2. Otherwise the project's committed layout-config.yaml, looked up the way
//     lint-config.yaml is. No file is the default style.
//
// A flow laid out under a style other than the default gets the line; under the
// default it loses it. So a project with no config file writes exactly what it
// always wrote.
package executor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// flowLayoutMarker starts the documentation line that records a flow's style.
// It is plain documentation text: the visitor keeps it inside `/** … */`
// verbatim and never reads it as an annotation.
const flowLayoutMarker = "@mxcli-layout:"

// flowLayoutConfigName is the project file holding the layout style.
const flowLayoutConfigName = "layout-config.yaml"

// FindFlowLayoutConfig returns the project's layout-config.yaml, or "" when there
// is none. Searched like lint-config.yaml: .claude/ first, then the project
// directory — both committed, unlike .mxcli/, which holds secrets and is
// git-ignored.
func FindFlowLayoutConfig(projectDir string) string {
	for _, p := range []string{
		filepath.Join(projectDir, ".claude", flowLayoutConfigName),
		filepath.Join(projectDir, flowLayoutConfigName),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// flowLayoutPresets are the named styles. A preset only names options; it is
// never what the marker records, so a preset can grow options later without
// changing how an already laid-out flow is read back.
var flowLayoutPresets = map[string]flowLayout{
	"happy-line":            {},
	"true-right-false-down": {IfElse: IfElseColumn},
}

// flowLayoutOption is one option key: its values and how each sets a flowLayout.
type flowLayoutOption struct {
	values map[string]func(*flowLayout)
	// value returns the option's spelling for a layout.
	value func(flowLayout) string
	// defaultValue is the spelling of the default style's value.
	defaultValue string
}

var flowLayoutOptions = map[string]flowLayoutOption{
	"if-else": {
		values: map[string]func(*flowLayout){
			"lanes":                  func(l *flowLayout) { l.IfElse = IfElseLanes },
			"true-right-else-column": func(l *flowLayout) { l.IfElse = IfElseColumn },
		},
		value: func(l flowLayout) string {
			if l.IfElse == IfElseColumn {
				return "true-right-else-column"
			}
			return "lanes"
		},
		defaultValue: "lanes",
	},
	"guard": {
		values: map[string]func(*flowLayout){
			"return-below": func(l *flowLayout) { l.Guard = GuardReturnBelow },
			"true-right":   func(l *flowLayout) { l.Guard = GuardTrueRight },
		},
		value: func(l flowLayout) string {
			if l.Guard == GuardTrueRight {
				return "true-right"
			}
			return "return-below"
		},
		defaultValue: "return-below",
	},
}

func sortedLayoutKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// setFlowLayoutOption applies key=value to l, refusing an unknown key or value:
// a typo silently falling back to the default is exactly what the file exists
// to prevent.
func setFlowLayoutOption(l *flowLayout, key, value string) error {
	opt, ok := flowLayoutOptions[key]
	if !ok {
		return fmt.Errorf("unknown layout option %q (known: %s)", key, strings.Join(sortedLayoutKeys(flowLayoutOptions), ", "))
	}
	set, ok := opt.values[value]
	if !ok {
		return fmt.Errorf("layout option %s: unknown value %q (known: %s)", key, value, strings.Join(sortedLayoutKeys(opt.values), ", "))
	}
	set(l)
	return nil
}

// FlowLayoutPreset resolves a preset name, for `--style`.
func FlowLayoutPreset(name string) (*flowLayout, error) {
	l, ok := flowLayoutPresets[name]
	if !ok {
		return nil, fmt.Errorf("unknown layout preset %q (known: %s)", name, strings.Join(sortedLayoutKeys(flowLayoutPresets), ", "))
	}
	return &l, nil
}

// LoadFlowLayoutConfig reads a layout-config.yaml. An empty path is the default
// style.
func LoadFlowLayoutConfig(path string) (*flowLayout, error) {
	if path == "" {
		return &flowLayout{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file struct {
		Flows map[string]string `yaml:"flows"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	l := flowLayout{}
	if name, ok := file.Flows["preset"]; ok {
		p, err := FlowLayoutPreset(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		l = *p
	}
	for _, key := range sortedLayoutKeys(file.Flows) {
		if key == "preset" {
			continue
		}
		if err := setFlowLayoutOption(&l, key, file.Flows[key]); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	return &l, nil
}

// markerValue is what the marker line records for l: every option that differs
// from the default, `key=value`, in key order — so the same style always gives
// the same line and re-laying out a flow changes nothing. "" for the default.
func (l flowLayout) markerValue() string {
	var parts []string
	for _, key := range sortedLayoutKeys(flowLayoutOptions) {
		opt := flowLayoutOptions[key]
		if v := opt.value(l); v != opt.defaultValue {
			parts = append(parts, key+"="+v)
		}
	}
	return strings.Join(parts, "; ")
}

// String names the style for messages: its marker value, or happy-line.
func (l flowLayout) String() string {
	if v := l.markerValue(); v != "" {
		return v
	}
	return "happy-line"
}

// parseFlowLayoutMarker reads a marker value back.
func parseFlowLayoutMarker(value string) (flowLayout, error) {
	l := flowLayout{}
	for _, part := range strings.Split(value, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, v, ok := strings.Cut(part, "=")
		if !ok {
			return flowLayout{}, fmt.Errorf("layout marker: %q is not key=value", part)
		}
		if err := setFlowLayoutOption(&l, strings.TrimSpace(key), strings.TrimSpace(v)); err != nil {
			return flowLayout{}, err
		}
	}
	return l, nil
}

// isFlowLayoutMarkerLine reports whether a documentation line is the marker.
func isFlowLayoutMarkerLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), flowLayoutMarker)
}

// flowLayoutFromDocumentation returns the style a flow's documentation records.
// found is false when there is no marker line. A marker that does not parse is
// reported as err, with found true; the caller warns and treats the flow as
// unmarked.
func flowLayoutFromDocumentation(doc string) (l flowLayout, found bool, err error) {
	for line := range strings.SplitSeq(doc, "\n") {
		if !isFlowLayoutMarkerLine(line) {
			continue
		}
		value := strings.TrimPrefix(strings.TrimSpace(line), flowLayoutMarker)
		l, err = parseFlowLayoutMarker(value)
		return l, true, err
	}
	return flowLayout{}, false, nil
}

// withFlowLayoutMarker returns doc with its marker line set for l: any marker
// line is removed, and one is appended for a style other than the default. The
// rest of the documentation is kept byte for byte; only blank lines the removed
// marker leaves at the end go with it.
func withFlowLayoutMarker(doc string, l flowLayout) string {
	_, found, _ := flowLayoutFromDocumentation(doc)
	value := l.markerValue()
	if !found && value == "" {
		return doc
	}
	out := doc
	if found {
		var kept []string
		for line := range strings.SplitSeq(doc, "\n") {
			if !isFlowLayoutMarkerLine(line) {
				kept = append(kept, line)
			}
		}
		out = strings.TrimRight(strings.Join(kept, "\n"), "\r\n\t ")
	}
	if value == "" {
		return out
	}
	line := flowLayoutMarker + " " + value
	if out == "" {
		return line
	}
	return out + "\n" + line
}

// ProjectFlowLayout returns the style new flows are laid out with: the
// override if one is set, else the project's layout-config.yaml, else the
// default. A config that does not load is an error for every flow built while it
// stands, never a silent fallback.
func (ctx *ExecContext) ProjectFlowLayout() (*flowLayout, error) {
	if ctx == nil {
		return &flowLayout{}, nil
	}
	if ctx.FlowLayoutOverride != nil {
		return ctx.FlowLayoutOverride, nil
	}
	if !ctx.projectFlowLayoutLoaded {
		ctx.projectFlowLayoutLoaded = true
		path := ""
		if ctx.MprPath != "" {
			path = FindFlowLayoutConfig(filepath.Dir(ctx.MprPath))
		}
		ctx.projectFlowLayout, ctx.projectFlowLayoutErr = LoadFlowLayoutConfig(path)
	}
	return ctx.projectFlowLayout, ctx.projectFlowLayoutErr
}

// resolveFlowLayout decides the style a flow is built with from its final
// documentation, and returns the documentation with the marker set for that
// style. A marked flow keeps its recorded style; an unmarked one gets the
// project's. A marker that does not parse is warned about and treated as absent.
//
// keepRecorded false ignores the marker: `mxcli layout flows` re-lays a flow out
// in the project style (or --style), which is the point of running it.
func resolveFlowLayout(ctx *ExecContext, doc string, keepRecorded bool) (*flowLayout, string, error) {
	if keepRecorded {
		l, found, err := flowLayoutFromDocumentation(doc)
		if found && err == nil {
			return &l, withFlowLayoutMarker(doc, l), nil
		}
		if found && err != nil && ctx != nil && ctx.Output != nil {
			fmt.Fprintf(ctx.Output, "Warning: ignoring the flow's %s line: %v\n", flowLayoutMarker, err)
		}
	}
	l, err := ctx.ProjectFlowLayout()
	if err != nil {
		return nil, doc, fmt.Errorf("layout config: %w", err)
	}
	return l, withFlowLayoutMarker(doc, *l), nil
}

// describingFlowLayout records, for the description of one flow, the style its
// documentation says it was laid out in, and returns the function that restores
// what was recorded before. The describer reads a shape only that style draws
// as the statement that drew it (flowLooksLikeGuardContinuation).
func (ctx *ExecContext) describingFlowLayout(doc string) func() {
	if ctx == nil {
		return func() {}
	}
	prev := ctx.describedFlowLayout
	ctx.describedFlowLayout = nil
	if l, found, err := flowLayoutFromDocumentation(doc); found && err == nil {
		ctx.describedFlowLayout = &l
	}
	return func() { ctx.describedFlowLayout = prev }
}
