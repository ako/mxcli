// SPDX-License-Identifier: Apache-2.0

// Package executor - where a flow's layout style comes from.
//
// The project's committed layout-config.yaml, looked up the way lint-config.yaml
// is; no file is the default style. A canonical DESCRIBE judges a position
// "derived" against that same style, because it is the style `create or modify`
// rebuilds the description in — so a describe → exec round trip is exact.
//
// Nothing about the style is stored in the flow (ako/mxcli#2635). Until then a
// flow laid out in a style other than the default carried an `@mxcli-layout:`
// line in its Documentation, which is the modeler's text: Studio Pro shows it and
// generated documentation copies it. A flow that still carries one is read as
// before — DESCRIBE and the next write lay it out in the style it records — and
// that write removes the line, so a project sheds them as its flows are
// rewritten. A flow drawn in a style other than the project's now describes with
// the positions that style put where the project's would not; the round trip
// stays exact, only louder.
package executor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// minRowWidth and maxRowWidthLimit bound max-row-width: narrower than a few
// activities every statement starts a row of its own, and past the limit a
// typo of an extra digit would quietly mean "never wrap" — which is `wrap: off`.
const (
	minRowWidth      = 4 * HorizontalSpacing
	maxRowWidthLimit = 20000
)

// flowLayoutMarker starts the documentation line older releases wrote to record a
// flow's style. It is plain documentation text: the visitor keeps it inside
// `/** … */` verbatim and never reads it as an annotation. mxcli reads it and
// removes it, and never writes it (ako/mxcli#2635).
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
	"true-right-false-down": {IfElse: IfElseColumn, CaseSplit: CaseSplitColumns},
}

// flowLayoutOption is one option key: its values and how each sets a flowLayout.
// An option with a number for a value has parse instead of values.
type flowLayoutOption struct {
	values map[string]func(*flowLayout)
	parse  func(*flowLayout, string) error
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
	"case-split": {
		values: map[string]func(*flowLayout){
			"lanes":                    func(l *flowLayout) { l.CaseSplit = CaseSplitLanes },
			"first-right-rest-columns": func(l *flowLayout) { l.CaseSplit = CaseSplitColumns },
		},
		value: func(l flowLayout) string {
			if l.CaseSplit == CaseSplitColumns {
				return "first-right-rest-columns"
			}
			return "lanes"
		},
		defaultValue: "lanes",
	},
	"wrap": {
		values: map[string]func(*flowLayout){
			"rows": func(l *flowLayout) { l.Wrap = WrapRows },
			"off":  func(l *flowLayout) { l.Wrap = WrapOff },
		},
		value: func(l flowLayout) string {
			if l.Wrap == WrapOff {
				return "off"
			}
			return "rows"
		},
		defaultValue: "rows",
	},
	"max-row-width": {
		parse: func(l *flowLayout, v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || n < minRowWidth || n > maxRowWidthLimit {
				return fmt.Errorf("%q is not a width in px from %d to %d", v, minRowWidth, maxRowWidthLimit)
			}
			if n == MaxRowWidth {
				n = 0
			}
			l.MaxRowWidth = n
			return nil
		},
		value:        func(l flowLayout) string { return strconv.Itoa(l.rowWidth()) },
		defaultValue: strconv.Itoa(MaxRowWidth),
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
	if opt.parse != nil {
		if err := opt.parse(l, value); err != nil {
			return fmt.Errorf("layout option %s: %w", key, err)
		}
		return nil
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

// withoutFlowLayoutMarker returns doc without its marker line. Documentation
// with none is returned byte for byte; otherwise only the marker line goes, with
// the blank lines it leaves at the end.
func withoutFlowLayoutMarker(doc string) string {
	if _, found, _ := flowLayoutFromDocumentation(doc); !found {
		return doc
	}
	var kept []string
	for line := range strings.SplitSeq(doc, "\n") {
		if !isFlowLayoutMarkerLine(line) {
			kept = append(kept, line)
		}
	}
	return strings.TrimRight(strings.Join(kept, "\n"), "\r\n\t ")
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
// documentation, and returns the documentation without a marker line. A flow
// that still carries one (written by an older release) is built in the style it
// records, once — the line goes with this write; any other gets the project's.
// A marker that does not parse is warned about and treated as absent.
//
// keepRecorded false ignores the marker: `mxcli layout flows` re-lays a flow out
// in the project style (or --style), which is the point of running it.
func resolveFlowLayout(ctx *ExecContext, doc string, keepRecorded bool) (*flowLayout, string, error) {
	if keepRecorded {
		l, found, err := flowLayoutFromDocumentation(doc)
		if found && err == nil {
			return &l, withoutFlowLayoutMarker(doc), nil
		}
		if found && err != nil && ctx != nil && ctx.Output != nil {
			fmt.Fprintf(ctx.Output, "Warning: ignoring the flow's %s line: %v\n", flowLayoutMarker, err)
		}
	}
	l, err := ctx.ProjectFlowLayout()
	if err != nil {
		return nil, doc, fmt.Errorf("layout config: %w", err)
	}
	return l, withoutFlowLayoutMarker(doc), nil
}

// describingFlowLayout records, for the description of one flow, the style it
// is read in, and returns the function that restores what was recorded before:
// the style a marker line records, else the project's — the one `create or
// modify` rebuilds the description in. The describer reads a shape only that
// style draws as the statement that drew it (flowLooksLikeGuardContinuation).
// A project config that does not load reads as the default; the build reports
// it.
func (ctx *ExecContext) describingFlowLayout(doc string) func() {
	if ctx == nil {
		return func() {}
	}
	prev := ctx.describedFlowLayout
	ctx.describedFlowLayout = nil
	if l, found, err := flowLayoutFromDocumentation(doc); found && err == nil {
		ctx.describedFlowLayout = &l
	} else if l, err := ctx.ProjectFlowLayout(); err == nil && l != nil {
		ctx.describedFlowLayout = l
	}
	return func() { ctx.describedFlowLayout = prev }
}
