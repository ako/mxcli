// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// editorConfigExtractStats reports how much of a widget's editorConfig.js the
// extractor could lift into declarative WidgetVisibilityRules — the coverage
// number that tells `check`/serialization how far to trust the rules and when
// to fall back to mxbuild's update-widgets.
type editorConfigExtractStats struct {
	TotalHideCalls int // hidePropertyIn + hidePropertiesIn call sites seen
	Recognized     int // lifted into a top-level WidgetVisibilityRule
	SkippedNested  int // object-list-nested (e.g. hidePropertyIn(...,"columns",n,"key"))
	SkippedComplex int // ternary/compound guard, or an alias we couldn't resolve
}

// hideCallRE locates a hidePropertyIn / hidePropertiesIn call and captures its
// (balanced-enough) argument list. hideNestedPropertiesIn is deliberately not
// matched — it only ever targets object-list items (Phase 2).
var hideCallRE = regexp.MustCompile(`hidePropert(?:y|ies)In\(`)

// aliasAssignRE finds `IDENT=OBJ.PROP` (a `var x=e.selection`-style alias).
// Resolution is scoped to the enclosing function body (see enclosingAliases),
// because minified editorConfig reuses single-letter identifiers across scopes.
var aliasAssignRE = regexp.MustCompile(`([A-Za-z_$][\w$]*)=([A-Za-z_$][\w$]*)\.([A-Za-z_$][\w$]*)`)

// stringLitRE matches a double-quoted JS string literal (no escapes in the
// property/enum keys we care about).
var stringLitRE = regexp.MustCompile(`"([^"\\]*)"`)

// visibilityCache memoizes extracted rules per (projectPath, widgetID) so the
// editorConfig.js is parsed once per build session, not per widget instance.
var (
	visibilityCache   = map[string][]types.WidgetVisibilityRule{}
	visibilityCacheMu sync.Mutex
)

// resolveWidgetVisibilityRules returns the property-visibility rules for a
// widget, lifted from its installed .mpk's editorConfig.js. Used to enrich
// built-in widget definitions (DataGrid2, Gallery, …) — which the .def.json
// generator skips — with the version-specific applicability logic of the Data
// Widgets package actually installed in the project. Returns nil when the .mpk
// or its editor config can't be found (degrades to "no rules" → template
// default, exactly today's behaviour). Best-effort: see extractVisibilityRules-
// FromJS for coverage limits.
func resolveWidgetVisibilityRules(projectPath, widgetID string) []types.WidgetVisibilityRule {
	if projectPath == "" || widgetID == "" {
		return nil
	}
	key := projectPath + "\x00" + widgetID
	visibilityCacheMu.Lock()
	if r, ok := visibilityCache[key]; ok {
		visibilityCacheMu.Unlock()
		return r
	}
	visibilityCacheMu.Unlock()

	// getProjectPath() yields the .mpr file path; FindMPK wants the directory
	// that contains widgets/.
	projectDir := projectPath
	if strings.EqualFold(filepath.Ext(projectDir), ".mpr") {
		projectDir = filepath.Dir(projectDir)
	}

	var rules []types.WidgetVisibilityRule
	if mpkPath, err := mpk.FindMPK(projectDir, widgetID); err == nil && mpkPath != "" {
		if js, err := mpk.ReadEditorConfig(mpkPath, widgetID); err == nil && js != "" {
			rules, _ = extractVisibilityRulesFromJS(js)
		}
	}

	visibilityCacheMu.Lock()
	visibilityCache[key] = rules
	visibilityCacheMu.Unlock()
	return rules
}

// extractVisibilityRulesFromJS lifts top-level property-hide rules from a
// widget's compiled editorConfig.js into declarative WidgetVisibilityRules.
//
// It recognizes the dominant `getProperties` idioms — `"V"===ref && hide(...)`,
// `"V"!==ref && hide(...)`, `ref && hide(...)`, `ref || hide(...)`, and
// `ref ? hide(...) : …` — where `ref` is `obj.prop` or a locally-aliased
// identifier resolved within the enclosing function scope. Everything it cannot
// lift (object-list-nested hides, compound/computed guards, unresolved aliases)
// is counted in the returned stats so callers can gauge coverage; unrecognized
// hides simply produce no rule, which degrades safely to "not hidden".
func extractVisibilityRulesFromJS(js string) ([]types.WidgetVisibilityRule, editorConfigExtractStats) {
	var rules []types.WidgetVisibilityRule
	var stats editorConfigExtractStats
	seen := map[string]bool{} // dedupe propertyKey+condition

	for _, loc := range hideCallRE.FindAllStringIndex(js, -1) {
		stats.TotalHideCalls++
		callStart, argsOpen := loc[0], loc[1] // argsOpen points just past '('
		args, ok := balancedArgs(js, argsOpen)
		if !ok {
			stats.SkippedComplex++
			continue
		}
		keys, nested := hideTargetKeys(args)
		if nested {
			stats.SkippedNested++
			continue
		}
		if len(keys) == 0 {
			stats.SkippedComplex++
			continue
		}
		cond, ok := parseGuard(js, callStart)
		if !ok {
			stats.SkippedComplex++
			continue
		}
		stats.Recognized++
		for _, key := range keys {
			sig := key + "\x00" + cond.PropertyKey + cond.Operator + cond.Value
			if seen[sig] {
				continue
			}
			seen[sig] = true
			c := cond // copy per rule
			rules = append(rules, types.WidgetVisibilityRule{PropertyKey: key, HiddenWhen: &c})
		}
	}
	return rules, stats
}

// hideTargetKeys returns the property key(s) a hide call targets and whether the
// call is object-list-nested (which we skip). A top-level hidePropertyIn has a
// single string arg (the key); a top-level hidePropertiesIn has one array of
// string keys. A `"columns"`/`"..."`-prefixed string arg followed by more args
// marks the nested form.
func hideTargetKeys(args string) (keys []string, nested bool) {
	parts := splitTopLevelCommas(args)
	// Collect string-literal positional args and any array literal.
	var stringArgs []string
	var arrayKeys []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "[") {
			for _, m := range stringLitRE.FindAllStringSubmatch(p, -1) {
				arrayKeys = append(arrayKeys, m[1])
			}
			continue
		}
		if m := stringLitRE.FindStringSubmatch(p); m != nil && strings.HasPrefix(p, `"`) {
			stringArgs = append(stringArgs, m[1])
		}
	}
	if len(arrayKeys) > 0 {
		// hidePropertiesIn(obj, obj, [keys]) — nested if a leading string arg
		// (e.g. "columns") also appears.
		if len(stringArgs) > 0 {
			return nil, true
		}
		return arrayKeys, false
	}
	switch len(stringArgs) {
	case 1:
		return stringArgs, false // hidePropertyIn(obj, obj, "key")
	case 0:
		return nil, false
	default:
		return nil, true // "columns","key" etc. — nested
	}
}

// parseGuard reads the guard expression immediately preceding a hide call and
// converts it to a WidgetVisibilityCondition. callStart points at the hide
// function name; the connector just before it is `&&`, `||`, or `?`.
func parseGuard(js string, callStart int) (types.WidgetVisibilityCondition, bool) {
	// Strip an optional `_.` namespace prefix before the function name.
	end := callStart
	pre := strings.TrimRight(js[:end], " ")
	if strings.HasSuffix(pre, "_.") {
		pre = pre[:len(pre)-2]
	}
	// Identify the connector.
	var falsy bool // || connector or !prefix ⇒ hide when guard is falsy
	switch {
	case strings.HasSuffix(pre, "&&"):
		pre = pre[:len(pre)-2]
	case strings.HasSuffix(pre, "?"):
		pre = pre[:len(pre)-1]
	case strings.HasSuffix(pre, "||"):
		pre = pre[:len(pre)-2]
		falsy = true
	default:
		return types.WidgetVisibilityCondition{}, false
	}
	guard, boundary := lastGuardExpr(pre)
	if guard == "" {
		return types.WidgetVisibilityCondition{}, false
	}
	// Skip guards nested inside a larger expression. A clean statement-level guard
	// is bounded by a statement separator (`,`, `;`, `{`, or start-of-input); a
	// boundary of `(`/`?`/`:`/`&`/`|` means the guard is one operand of a compound
	// or ternary condition (e.g. `"web"===r ? (e.advanced || hide(...))`), of which
	// we'd capture only a fragment — producing a WRONG rule that over-fires. Better
	// to emit no rule (→ "not hidden" → template default), which is safe.
	switch boundary {
	case 0, ',', ';', '{':
		// clean
	default:
		return types.WidgetVisibilityCondition{}, false
	}
	aliases := enclosingAliases(js, callStart)
	return guardToCondition(guard, falsy, aliases)
}

// lastGuardExpr returns the balanced expression ending at the end of `pre`,
// bounded by the previous top-level separator, and the boundary byte that
// terminated it (0 for start-of-input). The boundary lets the caller reject
// guards nested inside a compound/ternary expression.
func lastGuardExpr(pre string) (string, byte) {
	depth := 0
	var boundary byte
	i := len(pre) - 1
	for ; i >= 0; i-- {
		c := pre[i]
		switch c {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			if depth == 0 {
				boundary = c
				goto done
			}
			depth--
		case ',', ';', ':', '?', '&', '|':
			if depth == 0 {
				boundary = c
				goto done
			}
		}
	}
done:
	return strings.TrimSpace(pre[i+1:]), boundary
}

var (
	eqCmpRE  = regexp.MustCompile(`^"([^"]*)"===([A-Za-z_$][\w$.]*)$`)
	neCmpRE  = regexp.MustCompile(`^"([^"]*)"!==([A-Za-z_$][\w$.]*)$`)
	eqCmpRE2 = regexp.MustCompile(`^([A-Za-z_$][\w$.]*)==="([^"]*)"$`)
	neCmpRE2 = regexp.MustCompile(`^([A-Za-z_$][\w$.]*)!=="([^"]*)"$`)
	refRE    = regexp.MustCompile(`^(!?)([A-Za-z_$][\w$.]*)$`)
)

// guardToCondition parses a single guard expression into a visibility
// condition, resolving a bare identifier through the scope's alias map.
func guardToCondition(guard string, falsy bool, aliases map[string]string) (types.WidgetVisibilityCondition, bool) {
	// "V" === ref  /  ref === "V"
	if m := eqCmpRE.FindStringSubmatch(guard); m != nil {
		if key, ok := resolveRef(m[2], aliases); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: "eq", Value: m[1]}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	if m := eqCmpRE2.FindStringSubmatch(guard); m != nil {
		if key, ok := resolveRef(m[1], aliases); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: "eq", Value: m[2]}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	if m := neCmpRE.FindStringSubmatch(guard); m != nil {
		if key, ok := resolveRef(m[2], aliases); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: "ne", Value: m[1]}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	if m := neCmpRE2.FindStringSubmatch(guard); m != nil {
		if key, ok := resolveRef(m[1], aliases); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: "ne", Value: m[2]}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	// bare ref (truthy) or !ref (falsy), combined with the connector polarity:
	//   ref && hide   → hide when ref truthy
	//   ref || hide   → hide when ref falsy   (falsy==true here)
	//   !ref && hide  → hide when ref falsy
	//   ref ? hide:…  → hide when ref truthy
	if m := refRE.FindStringSubmatch(guard); m != nil {
		key, ok := resolveRef(m[2], aliases)
		if !ok {
			return types.WidgetVisibilityCondition{}, false
		}
		neg := (m[1] == "!")
		wantFalsy := falsy != neg // XOR: || or ! flips polarity (both flips cancel)
		op := "truthy"
		if wantFalsy {
			op = "falsy"
		}
		return types.WidgetVisibilityCondition{PropertyKey: key, Operator: op}, true
	}
	return types.WidgetVisibilityCondition{}, false
}

// resolveRef turns a guard reference into a widget property key: `obj.prop`
// yields `prop`; a bare identifier is looked up in the scope alias map. A bare
// identifier with no alias (e.g. a computed local) is unresolvable.
func resolveRef(ref string, aliases map[string]string) (string, bool) {
	if i := strings.LastIndexByte(ref, '.'); i >= 0 {
		return ref[i+1:], true
	}
	if key, ok := aliases[ref]; ok {
		return key, true
	}
	return "", false
}

// enclosingAliases returns the `ident → property` aliases declared in the
// function body that encloses the hide call at pos, resolved by scanning back
// to the nearest unbalanced `{`. Scoping matters: minified editorConfig reuses
// identifiers like `r`/`n` across functions, so only same-scope `var r=e.prop`
// assignments are trustworthy.
func enclosingAliases(js string, pos int) map[string]string {
	// Walk back to the enclosing block's opening brace.
	depth := 0
	open := 0
	for i := pos - 1; i >= 0; i-- {
		switch js[i] {
		case '}', ')', ']':
			depth++
		case '{', '(', '[':
			if depth == 0 {
				open = i
				goto found
			}
			depth--
		}
	}
found:
	body := js[open:pos]
	aliases := map[string]string{}
	ambiguous := map[string]bool{}
	for _, m := range aliasAssignRE.FindAllStringSubmatch(body, -1) {
		ident, prop := m[1], m[3]
		if ambiguous[ident] {
			continue
		}
		if existing, ok := aliases[ident]; ok && existing != prop {
			delete(aliases, ident)
			ambiguous[ident] = true
			continue
		}
		aliases[ident] = prop
	}
	return aliases
}

// balancedArgs returns the argument-list text between the '(' just before
// `open` and its matching ')'. Handles nested (), [], {} and string literals.
func balancedArgs(js string, open int) (string, bool) {
	depth := 1
	inStr := byte(0)
	for i := open; i < len(js); i++ {
		c := js[i]
		if inStr != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				return js[open:i], true
			}
		}
	}
	return "", false
}

// splitTopLevelCommas splits an argument list on commas that are not nested
// inside (), [], {}, or string literals.
func splitTopLevelCommas(args string) []string {
	var parts []string
	depth := 0
	inStr := byte(0)
	start := 0
	for i := 0; i < len(args); i++ {
		c := args[i]
		if inStr != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, args[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, args[start:])
	return parts
}
