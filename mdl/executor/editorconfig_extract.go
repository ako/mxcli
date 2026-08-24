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
	SkippedNested  int // object-list-nested hide whose guard could not be lifted
	SkippedComplex int // ternary/compound guard, or an alias we couldn't resolve
}

// hideCallRE locates a hidePropertyIn / hidePropertiesIn / hideNestedPropertiesIn
// call and captures its (balanced-enough) argument list. All three are matched:
// the nested forms target object-list items, which is where the Accordion hides
// the group properties that make an authored widget fail CE0463 (upstream #931).
var hideCallRE = regexp.MustCompile(`hide(?:Property|Properties|NestedProperties)In\(`)

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

// WidgetVisibilityRules returns the editorConfig-derived property-visibility rules
// for a widget installed in the given project (nil when the .mpk or its editor
// config can't be found). Exported for the `mxcli widget describe` command, which
// surfaces the dynamic property rules mxcli discovered for a project's widget.
func WidgetVisibilityRules(projectPath, widgetID string) []types.WidgetVisibilityRule {
	return resolveWidgetVisibilityRules(projectPath, widgetID)
}

// ExtractWidgetVisibilityStats lifts a widget's editorConfig visibility rules from
// a specific .mpk path and also returns the extractor's coverage stats (how many
// hide-calls were recognized vs skipped). Exported for `mxcli widget describe` to
// report extraction coverage.
func ExtractWidgetVisibilityStats(mpkPath, widgetID string) ([]types.WidgetVisibilityRule, int, int) {
	js, err := mpk.ReadEditorConfig(mpkPath, widgetID)
	if err != nil || js == "" {
		return nil, 0, 0
	}
	rules, stats := extractVisibilityRulesFromJS(js)
	return rules, stats.Recognized, stats.TotalHideCalls
}

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
		rules = extractVisibilityRulesFromMPK(mpkPath, widgetID)
	}

	visibilityCacheMu.Lock()
	visibilityCache[key] = rules
	visibilityCacheMu.Unlock()
	return rules
}

// extractVisibilityRulesFromMPK reads a widget's editorConfig.js from its .mpk
// and lifts its property-visibility rules. Returns nil when the widget ships no
// editor config. Used both at build time (via resolveWidgetVisibilityRules) and
// at .def.json generation time, so generated definitions carry the rules and the
// hand-transcribed table can retire.
func extractVisibilityRulesFromMPK(mpkPath, widgetID string) []types.WidgetVisibilityRule {
	js, err := mpk.ReadEditorConfig(mpkPath, widgetID)
	if err != nil || js == "" {
		return nil
	}
	rules, _ := extractVisibilityRulesFromJS(js)
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
	var pending []ternaryThenCandidate // resolved after the loop
	seen := map[string]bool{}          // dedupe propertyKey+condition

	for _, loc := range hideCallRE.FindAllStringIndex(js, -1) {
		stats.TotalHideCalls++
		callStart, argsOpen := loc[0], loc[1] // argsOpen points just past '('
		args, ok := balancedArgs(js, argsOpen)
		if !ok {
			stats.SkippedComplex++
			continue
		}
		listKey, keys, condKeys, ok := hideTargetKeys(args)
		if !ok || (len(keys) == 0 && len(condKeys) == 0) {
			stats.SkippedComplex++
			continue
		}
		// A nested hide sits inside the list's `forEach(function(item, i){…})`, so
		// a guard reading `item.<prop>` is about the ITEM, not the widget.
		itemIdent := ""
		if listKey != "" {
			itemIdent = enclosingForEachParam(js, callStart)
		}
		cond, guardText, ok := parseGuard(js, callStart, itemIdent)
		if !ok {
			// `<outer> ? <inner> && hide(...)` — a ternary THEN branch carrying a
			// second condition. Held back and resolved after the loop, once the
			// else branch's rules are known (see ternaryThenCandidate).
			//
			// ProgressCircle's equivalent parses today only because it wraps the
			// branch in parentheses, which the grouping-paren strip above removes;
			// Slider writes it without them, so the guard arrives as
			// `e.showTooltip?"value"===e.tooltipType` and no rule was produced.
			if outer, inner, split := splitTernaryThenGuard(guardText); split {
				aliases := enclosingAliases(js, callStart)
				ic, iok := guardToCondition(inner, false, aliases, itemIdent)
				oc, ook := guardToCondition(outer, false, aliases, itemIdent)
				if iok && ook {
					for _, key := range keys {
						pending = append(pending, ternaryThenCandidate{
							listKey: listKey, key: key, inner: ic, outer: oc,
						})
					}
				}
			}
			if listKey != "" {
				stats.SkippedNested++
			} else {
				stats.SkippedComplex++
			}
			continue
		}
		// A conditional array element carries its OWN condition, which supersedes
		// the enclosing guard: the branch is what selects the property, so the
		// guard alone would mark both branches hidden together (#956).
		if len(condKeys) > 0 {
			aliases := enclosingAliases(js, callStart)
			for _, tk := range condKeys {
				c, ok := guardToCondition(tk.guard, tk.falsy, aliases, itemIdent)
				if !ok {
					stats.SkippedComplex++
					continue
				}
				sig := listKey + "\x00" + tk.key + "\x00" + c.PropertyKey + c.Operator + c.Value + c.Scope
				if seen[sig] {
					continue
				}
				seen[sig] = true
				cc := c
				rules = append(rules, types.WidgetVisibilityRule{
					PropertyKey:     tk.key,
					ListPropertyKey: listKey,
					HiddenWhen:      &cc,
				})
			}
		}
		stats.Recognized++
		for _, key := range keys {
			sig := listKey + "\x00" + key + "\x00" + cond.PropertyKey + cond.Operator + cond.Value + cond.Scope
			if seen[sig] {
				continue
			}
			seen[sig] = true
			c := cond // copy per rule
			rules = append(rules, types.WidgetVisibilityRule{
				PropertyKey:     key,
				ListPropertyKey: listKey,
				HiddenWhen:      &c,
			})
		}
	}

	// Resolve the held-back ternary-then candidates. The inner condition is
	// emitted only when an existing rule hides the SAME property under the
	// negation of the outer one — the else branch — so the pair reads as a
	// correct disjunction rather than a conjunction it cannot express (#238).
	for _, c := range pending {
		comp, ok := negate(c.outer)
		if !ok {
			continue
		}
		covered := false
		for _, r := range rules {
			if r.PropertyKey != c.key || r.ListPropertyKey != c.listKey || r.HiddenWhen == nil {
				continue
			}
			if *r.HiddenWhen == comp {
				covered = true
				break
			}
		}
		if !covered {
			continue
		}
		sig := c.listKey + "\x00" + c.key + "\x00" + c.inner.PropertyKey + c.inner.Operator + c.inner.Value + c.inner.Scope
		if seen[sig] {
			continue
		}
		seen[sig] = true
		ic := c.inner
		rules = append(rules, types.WidgetVisibilityRule{
			PropertyKey:     c.key,
			ListPropertyKey: c.listKey,
			HiddenWhen:      &ic,
		})
		stats.Recognized++
	}
	return rules, stats
}

// ternaryThenCandidate is a hide whose guard is the THEN branch of a ternary AND
// carries a second condition:
//
//	e.showTooltip ? "value" === e.tooltipType && hidePropertyIn(t, e, "tooltip")
//	              : hidePropertiesIn(t, e, ["tooltip", "tooltipType"])
//
// The property is hidden when `outer AND inner`, which one WidgetVisibilityCondition
// cannot express — so the inner condition is emitted only when another rule already
// hides the same property under the NEGATION of the outer one, i.e. the else branch
// covers the rest. The two rules are then a correct disjunction:
//
//	hidden  <=>  !showTooltip  ||  tooltipType == "value"
//
// Without that cover the conjunction is not representable and the hide is skipped,
// which degrades to "not hidden" and is safe. Emitting the inner condition alone
// would prune the property whenever it holds, including where the widget shows it —
// CE0463's mirror image (#238).
type ternaryThenCandidate struct {
	listKey, key string
	inner, outer types.WidgetVisibilityCondition
}

// splitTernaryThenGuard recognises `<outer> ? <inner>` in a guard parseGuard could
// not read, returning the two halves. The split is on the LAST `?`, so a nested
// ternary yields the innermost then-branch, which is the one guarding this call.
func splitTernaryThenGuard(guard string) (outer, inner string, ok bool) {
	i := strings.LastIndex(guard, "?")
	if i <= 0 || i == len(guard)-1 {
		return "", "", false
	}
	// The outer half still carries whatever preceded it (`exports.getProperties=
	// function(e,t){return …`), so reduce it to the trailing expression the same
	// way the main path does.
	outerExpr, _ := lastGuardExpr(strings.TrimSpace(guard[:i]))
	outer = stripReturnPrefix(outerExpr)
	inner = strings.TrimSpace(guard[i+1:])
	if outer == "" || inner == "" {
		return "", "", false
	}
	return outer, inner, true
}

// negate returns the condition that holds exactly when c does not, and whether it
// is expressible.
func negate(c types.WidgetVisibilityCondition) (types.WidgetVisibilityCondition, bool) {
	out := c
	switch c.Operator {
	case "truthy":
		out.Operator = "falsy"
	case "falsy":
		out.Operator = "truthy"
	case "eq":
		out.Operator = "ne"
	case "ne":
		out.Operator = "eq"
	default:
		return out, false
	}
	return out, true
}

// hideTargetKeys returns the object-list property a hide call targets (empty for
// a top-level hide) and the property key(s) it hides.
//
//	hidePropertyIn(t, e, "key")                        → "",       ["key"]
//	hidePropertiesIn(t, e, ["a","b"])                  → "",       ["a","b"]
//	hidePropertyIn(t, e, "groups", i, "key")           → "groups", ["key"]
//	hideNestedPropertiesIn(t, e, "groups", i, ["a"])   → "groups", ["a"]
//
// ok is false for a shape it does not recognize; the caller counts it as skipped
// and emits no rule, which degrades to "not hidden".
func hideTargetKeys(args string) (listKey string, keys []string, condKeys []ternaryKey, ok bool) {
	parts := splitTopLevelCommas(args)
	// Collect string-literal positional args and any array literal.
	var stringArgs []string
	var arrayKeys []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "[") {
			for _, el := range splitTopLevelCommas(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(p), "["), "]")) {
				if tk, ok := ternaryElement(el); ok {
					// `cond ? "a" : "b"` as an array ELEMENT. Harvesting its string
					// literals would invent a key from the comparison value and mark
					// BOTH branches hidden under the outer guard — see #956.
					condKeys = append(condKeys, tk...)
					continue
				}
				for _, m := range stringLitRE.FindAllStringSubmatch(el, -1) {
					arrayKeys = append(arrayKeys, m[1])
				}
			}
			continue
		}
		if m := stringLitRE.FindStringSubmatch(p); m != nil && strings.HasPrefix(p, `"`) {
			stringArgs = append(stringArgs, m[1])
		}
	}
	if len(arrayKeys) > 0 {
		switch len(stringArgs) {
		case 0:
			return "", arrayKeys, condKeys, true // hidePropertiesIn(obj, obj, [keys])
		case 1:
			return stringArgs[0], arrayKeys, condKeys, true // hideNestedPropertiesIn(…, "groups", i, [keys])
		default:
			return "", nil, condKeys, false
		}
	}
	switch len(stringArgs) {
	case 1:
		return "", stringArgs, condKeys, true // hidePropertyIn(obj, obj, "key")
	case 2:
		return stringArgs[0], stringArgs[1:], condKeys, true // hidePropertyIn(…, "groups", i, "key")
	default:
		return "", nil, condKeys, false
	}
}

// ternaryKey is an array element of the form `cond ? "a" : "b"` — a hide whose
// TARGET depends on a condition, rather than the whole call being guarded. Each
// branch becomes its own rule carrying that condition (the then-branch when the
// comparison holds, the else-branch when it does not).
//
// Harvesting the element's string literals instead — which is what the extractor
// did — invents a key from the comparison VALUE and marks both branches hidden
// under the enclosing guard, losing the condition that distinguishes them.
// Measured on File Uploader 2.5.0: `associatedImages` was left ungated, so
// authoring the widget's datasource wrote it alongside `associatedFiles` and the
// build failed CE0463 (#956).
type ternaryKey struct {
	key   string
	guard string
	falsy bool // else-branch: the hide fires when the comparison is FALSE
}

// ternaryElementRE splits `<guard>?"then":"else"` (minified: no spaces).
var ternaryElementRE = regexp.MustCompile(`^(.+?)\?\s*"([^"]*)"\s*:\s*"([^"]*)"$`)

// ternaryElement recognizes a conditional array element and returns one entry
// per branch. ok is false for anything else, so a plain element is unaffected.
func ternaryElement(el string) ([]ternaryKey, bool) {
	m := ternaryElementRE.FindStringSubmatch(strings.TrimSpace(el))
	if m == nil {
		return nil, false
	}
	guard := strings.TrimSpace(m[1])
	return []ternaryKey{
		{key: m[2], guard: guard, falsy: false},
		{key: m[3], guard: guard, falsy: true},
	}, true
}

// forEachParamRE matches the callback parameter list of a `.forEach(function(a,b){`
// immediately before an enclosing block brace. The extra `\(*` is load-bearing:
// the Accordion Mendix ships is compiled as `forEach((function(n,o){…}))`, and
// without it the regex reads `function` itself as the parameter name — which
// scoped every nested condition to the widget and dropped the group's own
// `initialCollapsedState` guard on the floor.
var forEachParamRE = regexp.MustCompile(`forEach\(\s*\(*\s*(?:function\b\s*)?\(\s*([A-Za-z_$][\w$]*)`)

// enclosingForEachParam returns the name of the first parameter of the
// `.forEach(function(item, index){…})` callback whose body encloses pos, or ""
// when pos is not inside one.
//
// It matters because a nested hide's guard can read either object: the Accordion
// writes `"dynamic" !== item.initialCollapsedState && hide(…, "initiallyCollapsed")`
// (an ITEM property) in the same callback as
// `(…) && widget.collapsible || hideNested(…)` (a WIDGET property). Without the
// distinction the two conditions are indistinguishable once resolveRef has
// dropped the base identifier.
func enclosingForEachParam(js string, pos int) string {
	for i := 0; i < 8; i++ { // bounded: a hide is never nested this deep
		open := enclosingOpener(js, pos)
		if open < 0 {
			return ""
		}
		if js[open] == '{' {
			// Look just before the brace for the callback's parameter list.
			start := open - 80
			if start < 0 {
				start = 0
			}
			if m := forEachParamRE.FindStringSubmatch(js[start:open]); m != nil && m[1] != "function" {
				return m[1]
			}
		}
		pos = open
	}
	return ""
}

// enclosingOpener returns the index of the nearest unbalanced opening bracket
// before pos, or -1 when there is none.
func enclosingOpener(js string, pos int) int {
	depth := 0
	for i := pos - 1; i >= 0; i-- {
		switch js[i] {
		case '}', ')', ']':
			depth++
		case '{', '(', '[':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

// nsPrefixRE matches the widget-editor namespace prefix before a hide call — the
// minifier names it per widget (`_.`, `D.`, `M.`, `j.`, `A.`, …), so it must be
// matched generically rather than hard-coded to `_.`.
var nsPrefixRE = regexp.MustCompile(`[A-Za-z_$][\w$]*\.$`)

// parseGuard reads the guard expression immediately preceding a hide call and
// converts it to a WidgetVisibilityCondition. callStart points at the hide
// function name; the connector just before it is `&&`, `||`, or `?`.
func parseGuard(js string, callStart int, itemIdent string) (types.WidgetVisibilityCondition, string, bool) {
	pre := strings.TrimRight(js[:callStart], " ")
	// Strip the widget-editor namespace prefix (any `<ident>.`, not just `_.`).
	if loc := nsPrefixRE.FindStringIndex(pre); loc != nil {
		pre = pre[:loc[0]]
	}
	pre = strings.TrimRight(pre, " ")
	// Strip an optional grouping paren: `cond && ( hide(...), … )` groups several
	// hides under one condition; the first hide sits right after the `(`.
	if strings.HasSuffix(pre, "(") {
		pre = strings.TrimRight(pre[:len(pre)-1], " ")
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
	case strings.HasSuffix(pre, ":"):
		// The ELSE branch of a ternary: `cond ? (…hides…) : hide(…)`. The hide
		// fires when cond is falsy, and cond is not the text next to the `:` —
		// it sits before the matching `?`, past the whole then-branch.
		//
		// ProgressCircle 3.3.2 is the case that made this matter:
		//
		//	return e.showLabel
		//	  ? ("text" !== e.labelType && hidePropertyIn(t,e,"labelText"))
		//	  : hidePropertiesIn(t,e,["customLabel","labelText","labelType"])
		//
		// Without this branch only the inner `!==` rule was seen, so labelText
		// read as VISIBLE whenever labelType was "text" — its default — even
		// with showLabel false. See the CE0463 that produced (ledger #104).
		cond, ok := ternaryCondition(pre[:len(pre)-1])
		if !ok {
			return types.WidgetVisibilityCondition{}, pre, false
		}
		pre = cond
		falsy = true
	default:
		return types.WidgetVisibilityCondition{}, pre, false
	}
	guard, boundary := lastGuardExpr(pre)
	guard = stripReturnPrefix(guard) // getProperties' first statement is `return <guard> && hide…`
	if guard == "" {
		return types.WidgetVisibilityCondition{}, guard, false
	}
	// Skip guards nested inside a larger expression. A clean statement-level guard
	// is bounded by a statement separator (`,`, `;`, `{`, or start-of-input); a
	// boundary of `(`/`?`/`:`/`|` means the guard is one operand of a compound
	// or ternary condition (e.g. `"web"===r ? (e.advanced || hide(...))`), of which
	// we'd capture only a fragment — producing a WRONG rule that over-fires. Better
	// to emit no rule (→ "not hidden" → template default), which is safe.
	//
	// `&` is the one compound boundary that is safe, and only for the `||`
	// connector: in `X && Y || hide`, the hide fires when `X && Y` is falsy, and
	// `Y` falsy is sufficient for that WHATEVER X is. So "hide when Y falsy" is
	// implied by the code rather than guessed — it may miss the case where X
	// alone is falsy, never fire where the widget would not hide. This is the
	// Accordion's shape, and the reason its group properties went unflagged:
	//
	//	(e.advancedMode || "web" !== platform) && e.collapsible
	//	  || hideNestedPropertiesIn(t, e, "groups", i, ["initialCollapsedState", …])
	//
	// The `&&` connector gets no such rule: there, hiding needs BOTH operands
	// truthy, which a single condition cannot express.
	switch boundary {
	case 0, ',', ';', '{':
		// clean
	case '?':
		// The guard is the THEN branch of a ternary and carries its own condition
		// (`outer ? inner && hide(...)`). Hand back the WHOLE expression, `?`
		// included, so the caller can split it and decide whether the else branch
		// makes the pair expressible — see ternaryThenCandidate (#238).
		return types.WidgetVisibilityCondition{}, pre, false
	case '&':
		if !falsy {
			return types.WidgetVisibilityCondition{}, guard, false
		}
	default:
		return types.WidgetVisibilityCondition{}, guard, false
	}
	aliases := enclosingAliases(js, callStart)
	c, ok := guardToCondition(guard, falsy, aliases, itemIdent)
	return c, guard, ok
}

// ternaryCondition returns the text preceding the `?` that matches a trailing
// `:`, i.e. the condition of the ternary whose else-branch is about to start.
//
// It walks backwards balancing brackets, and counts nested ternaries: every
// further `:` seen at depth 0 needs its own `?` before the one we want. A `?`
// that never arrives means this `:` was not a ternary at all (a label, or an
// object literal key), and the caller emits no rule — which is the safe
// direction, since a wrong rule hides a property the user set.
func ternaryCondition(pre string) (string, bool) {
	depth, pending := 0, 0
	for i := len(pre) - 1; i >= 0; i-- {
		switch pre[i] {
		case ')', ']', '}':
			depth++
		case '(', '[', '{':
			if depth == 0 {
				return "", false // ran out of enclosing expression
			}
			depth--
		case ':':
			if depth == 0 {
				pending++
			}
		case '?':
			if depth == 0 {
				if pending == 0 {
					return trailingExpr(pre[:i]), true
				}
				pending--
			}
		}
	}
	return "", false
}

// trailingExpr returns the last expression in s, bounded by a statement
// separator. The ternary is usually not the first thing in its function —
// ProgressCircle's is preceded by a whole `switch` — and returning everything
// back to the start hands the caller a fragment with an unbalanced `}` that no
// guard parser can read.
func trailingExpr(s string) string {
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')', ']':
			depth++
		case '(', '[':
			if depth == 0 {
				return strings.TrimSpace(s[i+1:])
			}
			depth--
		case '}', '{', ';', ',':
			// A brace at depth 0 closes or opens a preceding BLOCK, so the
			// expression starts after it. Braces are not counted as nesting here
			// for that reason — an object literal inside the condition would be
			// bounded by its own parens.
			if depth == 0 {
				return strings.TrimSpace(s[i+1:])
			}
		}
	}
	return strings.TrimSpace(s)
}

// stripReturnPrefix removes a leading `return` keyword from a guard expression.
// A widget's getProperties body often starts `return <cond> && hide(…), …`, so
// the first guard is prefixed with `return` (minified: `return"none"===…`).
func stripReturnPrefix(g string) string {
	g = strings.TrimSpace(g)
	const kw = "return"
	if strings.HasPrefix(g, kw) {
		rest := g[len(kw):]
		// Only strip when `return` is a standalone keyword, not the start of an
		// identifier like `returnValue`.
		if rest == "" || !isIdentByte(rest[0]) {
			return strings.TrimSpace(rest)
		}
	}
	return g
}

// isIdentByte reports whether b can appear in a JS identifier.
func isIdentByte(b byte) bool {
	return b == '_' || b == '$' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
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
func guardToCondition(guard string, falsy bool, aliases map[string]string, itemIdent string) (types.WidgetVisibilityCondition, bool) {
	// A comparison guard obeys the connector's polarity just as a bare reference
	// does: with `||`, or in a ternary's ELSE branch, the hide fires when the
	// comparison is FALSE, so `===` becomes "not equal" and `!==` becomes
	// "equal". The Accordion is where this shows:
	//
	//	"text" === item.headerRenderMode
	//	  ? (hide(…, "headerContent"), …)
	//	  : (hide(…, "headerText"), hide(…, "headerHeading"))
	//
	// headerContent is hidden when the mode IS "text"; headerText when it is NOT.
	// Reading both as `eq` marks headerText hidden in exactly the configuration
	// where it is the property being used.
	eqOp, neOp := "eq", "ne"
	if falsy {
		eqOp, neOp = "ne", "eq"
	}
	// "V" === ref  /  ref === "V"
	if m := eqCmpRE.FindStringSubmatch(guard); m != nil {
		if key, scope, ok := resolveRef(m[2], aliases, itemIdent); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: eqOp, Value: m[1], Scope: scope}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	if m := eqCmpRE2.FindStringSubmatch(guard); m != nil {
		if key, scope, ok := resolveRef(m[1], aliases, itemIdent); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: eqOp, Value: m[2], Scope: scope}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	if m := neCmpRE.FindStringSubmatch(guard); m != nil {
		if key, scope, ok := resolveRef(m[2], aliases, itemIdent); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: neOp, Value: m[1], Scope: scope}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	if m := neCmpRE2.FindStringSubmatch(guard); m != nil {
		if key, scope, ok := resolveRef(m[1], aliases, itemIdent); ok {
			return types.WidgetVisibilityCondition{PropertyKey: key, Operator: neOp, Value: m[2], Scope: scope}, true
		}
		return types.WidgetVisibilityCondition{}, false
	}
	// bare ref (truthy) or !ref (falsy), combined with the connector polarity:
	//   ref && hide   → hide when ref truthy
	//   ref || hide   → hide when ref falsy   (falsy==true here)
	//   !ref && hide  → hide when ref falsy
	//   ref ? hide:…  → hide when ref truthy
	if m := refRE.FindStringSubmatch(guard); m != nil {
		key, scope, ok := resolveRef(m[2], aliases, itemIdent)
		if !ok {
			return types.WidgetVisibilityCondition{}, false
		}
		neg := (m[1] == "!")
		wantFalsy := falsy != neg // XOR: || or ! flips polarity (both flips cancel)
		op := "truthy"
		if wantFalsy {
			op = "falsy"
		}
		return types.WidgetVisibilityCondition{PropertyKey: key, Operator: op, Scope: scope}, true
	}
	return types.WidgetVisibilityCondition{}, false
}

// resolveRef turns a guard reference into a property key and the scope that key
// belongs to: `obj.prop` yields `prop`; a bare identifier is looked up in the
// scope alias map. A bare identifier with no alias (e.g. a computed local) is
// unresolvable.
//
// When itemIdent is non-empty (the hide is inside that object list's forEach
// callback) a reference based on it is scoped to the list ITEM; everything else
// is a property of the widget.
func resolveRef(ref string, aliases map[string]string, itemIdent string) (key, scope string, ok bool) {
	if i := strings.LastIndexByte(ref, '.'); i >= 0 {
		if itemIdent != "" && ref[:i] == itemIdent {
			return ref[i+1:], types.ConditionScopeItem, true
		}
		return ref[i+1:], "", true
	}
	if key, ok := aliases[ref]; ok {
		return key, "", true
	}
	return "", "", false
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
