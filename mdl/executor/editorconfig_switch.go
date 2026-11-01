// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// A widget's getProperties can choose what to hide with a `switch` on one of its
// enumerations. Timeline 3.2 does, and three of its six hide rules were unread
// (ako/mxcli#1762, mendixlabs/mxcli#1668):
//
//	switch (…, e.groupByKey) {
//	case "day":   hidePropertyIn(t, e, "groupByMonthOptions"); break;
//	case "month": hidePropertyIn(t, e, "groupByDayOptions");   break;
//	default:      hidePropertiesIn(t, e, ["groupByDayOptions", "groupByMonthOptions"]);
//	}
//
// A hide in an arm fires on the values that enter the arm: its own labels, the
// labels of every arm that falls through into it, and — when a default arm is
// among those — every value no case names. That is one `eq`/`in` condition, or
// one `ne`/`notin` over the cases the switch does name.
//
// Exact or nothing, like the rest of the extractor: a hide carrying its own
// guard inside the arm, a label that is not a string literal, a switch that is
// not a statement of a function body, or a discriminant that is not one of the
// widget's properties yields no rule — which reads as "not hidden", the state
// every one of these was in before.

// switchArm is one `case`/`default` arm of a switch body.
type switchArm struct {
	labels     []string // the case values; empty for a lone default
	isDefault  bool
	start, end int     // the arm's statements, as offsets into the switch body
	exit       armExit // whether control falls into the next arm
}

// armExit is how control leaves a switch arm.
type armExit int

const (
	armFallsThrough armExit = iota // runs on into the next arm
	armTerminates                  // ends in break/return/throw/continue
	armUnknown                     // cannot be told from its text (a block, a conditional break)
)

// switchArmCondition reports whether the hide at callStart sits in a switch arm
// (inArm) and, when it does, the condition under which that arm runs. ok is
// false for an arm whose condition cannot be stated exactly; the caller then
// emits no rule and does NOT fall back to the guard parser, whose reading of
// the arm's `case "v":` label as a ternary's else could only be wrong.
func switchArmCondition(js string, callStart int, itemIdent string) (cond types.WidgetVisibilityCondition, inArm, ok bool) {
	none := types.WidgetVisibilityCondition{}
	open := enclosingOpenerQuoted(js, callStart)
	if open < 0 || js[open] != '{' {
		return none, false, false
	}
	head := strings.TrimRight(js[:open], " ")
	if !strings.HasSuffix(head, ")") {
		return none, false, false
	}
	parenOpen := matchingOpenParen(head)
	if parenOpen < 0 {
		return none, false, false
	}
	kwHead := strings.TrimRight(head[:parenOpen], " ")
	if !hasKeywordSuffix(kwHead, "switch") {
		return none, false, false
	}

	// From here on the hide is in a switch arm.
	kwStart := len(kwHead) - len("switch")
	if !atFunctionStatementLevel(js, kwStart) {
		return none, true, false // the switch runs only under some outer condition
	}
	// terser folds the preceding statements into the head — `switch(A, B, e.k)` —
	// and the switch is on the LAST of them.
	parts := splitTopLevelCommas(head[parenOpen+1 : len(head)-1])
	m := refRE.FindStringSubmatch(strings.TrimSpace(parts[len(parts)-1]))
	if m == nil || m[1] == "!" {
		return none, true, false
	}
	key, scope, resolved := resolveRef(m[2], enclosingAliases(js, kwStart), itemIdent)
	if !resolved {
		return none, true, false // the platform argument, or a computed local
	}
	body, balanced := balancedArgs(js, open+1)
	if !balanced {
		return none, true, false
	}
	arms, parsed := switchArms(body)
	if !parsed {
		return none, true, false
	}
	at := callStart - (open + 1)
	idx := -1
	for i, a := range arms {
		if at >= a.start && at < a.end {
			idx = i
		}
	}
	if idx < 0 || !onlyHideStatementsBefore(body[arms[idx].start:at]) {
		return none, true, false
	}

	first := idx
walk:
	for first > 0 {
		switch arms[first-1].exit {
		case armTerminates:
			break walk
		case armUnknown:
			// Treating it either way could be wrong: as falling through, this
			// rule claims its values; as terminating, it misses them.
			return none, true, false
		}
		first-- // that arm falls through into this one
	}
	var entered []string
	hasDefault := false
	for _, a := range arms[first : idx+1] {
		entered = append(entered, a.labels...)
		hasDefault = hasDefault || a.isDefault
	}
	op, vals := "in", uniqueStrings(entered)
	if hasDefault {
		// Entered by every value no case names, plus this run's own cases: the
		// complement of the OTHER arms' cases.
		var all []string
		for _, a := range arms {
			all = append(all, a.labels...)
		}
		op, vals = "notin", withoutStrings(uniqueStrings(all), vals)
	}
	switch {
	case len(vals) == 0:
		return none, true, false // every value enters: unconditional, not a condition
	case len(vals) == 1 && op == "in":
		return types.WidgetVisibilityCondition{PropertyKey: key, Operator: "eq", Value: vals[0], Scope: scope}, true, true
	case len(vals) == 1:
		return types.WidgetVisibilityCondition{PropertyKey: key, Operator: "ne", Value: vals[0], Scope: scope}, true, true
	}
	for _, v := range vals {
		if strings.ContainsRune(v, ',') {
			return none, true, false // the joined set would decode wrong
		}
	}
	return types.WidgetVisibilityCondition{PropertyKey: key, Operator: op, Value: strings.Join(vals, ","), Scope: scope}, true, true
}

// switchArms splits a switch body into its arms. ok is false for anything but
// `case "literal":` and `default:` labels, or for text before the first label.
func switchArms(body string) ([]switchArm, bool) {
	var arms []switchArm
	depth := 0
	inStr := byte(0)
	for i := 0; i < len(body); i++ {
		c := body[i]
		if inStr != 0 {
			if c == '\\' {
				i++
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
			continue
		case '(', '[', '{':
			depth++
			continue
		case ')', ']', '}':
			depth--
			continue
		}
		if depth != 0 || (i > 0 && (isIdentByte(body[i-1]) || body[i-1] == '.')) {
			continue
		}
		var arm switchArm
		var next int
		switch {
		case hasKeywordAt(body, i, "case"):
			label, end, ok := switchCaseLabel(body, i+len("case"))
			if !ok {
				return nil, false
			}
			arm.labels, next = []string{label}, end
		case hasKeywordAt(body, i, "default"):
			j := skipSpaces(body, i+len("default"))
			if j >= len(body) || body[j] != ':' {
				return nil, false
			}
			arm.isDefault, next = true, j+1
		default:
			continue
		}
		if len(arms) == 0 {
			if strings.TrimSpace(body[:i]) != "" {
				return nil, false
			}
		} else {
			arms[len(arms)-1].end = i
		}
		arm.start, arm.end = next, len(body)
		arms = append(arms, arm)
		i = next - 1
	}
	if len(arms) == 0 {
		return nil, false
	}
	for i := range arms {
		arms[i].exit = exitOf(body[arms[i].start:arms[i].end])
	}
	return arms, true
}

// switchCaseLabel reads the `"value":` after a `case` keyword, returning the value and
// the offset just past the colon.
func switchCaseLabel(body string, from int) (string, int, bool) {
	j := skipSpaces(body, from)
	if j >= len(body) || (body[j] != '"' && body[j] != '\'') {
		return "", 0, false
	}
	q := body[j]
	k := strings.IndexByte(body[j+1:], q)
	if k < 0 {
		return "", 0, false
	}
	value := body[j+1 : j+1+k]
	if strings.ContainsRune(value, '\\') {
		return "", 0, false
	}
	end := skipSpaces(body, j+1+k+1)
	if end >= len(body) || body[end] != ':' {
		return "", 0, false
	}
	return value, end + 1, true
}

// abruptKeywords leave a switch arm.
var abruptKeywords = []string{"break", "return", "throw", "continue"}

// exitOf reports how control leaves an arm. Only two readings are certain: the
// last statement is itself a break/return/throw/continue, or the arm holds none
// of them and no block that could hide one. Anything else — `{ …; break }`,
// `if (c) break; …` — is unknown.
func exitOf(arm string) armExit {
	stmts := splitTopLevel(arm, ';')
	for i := len(stmts) - 1; i >= 0; i-- {
		s := strings.TrimSpace(stmts[i])
		if s == "" {
			continue
		}
		for _, kw := range abruptKeywords {
			if hasKeywordAt(s, 0, kw) {
				return armTerminates
			}
		}
		if mayLeaveEarly(arm) {
			return armUnknown
		}
		return armFallsThrough
	}
	return armFallsThrough // an empty arm (a grouped label) falls through
}

// mayLeaveEarly reports whether s, outside its string literals, holds a block or
// an abrupt keyword anywhere.
func mayLeaveEarly(s string) bool {
	inStr := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if c == '\\' {
				i++
			} else if c == inStr {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = c
			continue
		case '{':
			return true
		}
		if i > 0 && (isIdentByte(s[i-1]) || s[i-1] == '.') {
			continue
		}
		for _, kw := range abruptKeywords {
			if hasKeywordAt(s, i, kw) {
				return true
			}
		}
	}
	return false
}

// onlyHideStatementsBefore reports whether the text of an arm before a hide is
// nothing but earlier hide calls — `case "v": hide(a), hide(b); hide(c)` — so
// the hide is guarded by the arm alone and not by a condition of its own.
func onlyHideStatementsBefore(prefix string) bool {
	p := trimNamespacePrefix(strings.TrimRight(prefix, " "))
	for {
		if strings.TrimSpace(p) == "" {
			return true
		}
		if !strings.HasSuffix(p, ";") && !strings.HasSuffix(p, ",") {
			return false
		}
		call := strings.TrimRight(p[:len(p)-1], " ")
		if !strings.HasSuffix(call, ")") {
			return false
		}
		open := matchingOpenParen(call)
		if open < 0 {
			return false
		}
		name := strings.TrimRight(call[:open], " ")
		at := hideCallTail(name)
		if at < 0 {
			return false
		}
		p = trimNamespacePrefix(strings.TrimRight(name[:at], " "))
	}
}

func trimNamespacePrefix(p string) string {
	if at := nsPrefix(p); at >= 0 {
		return strings.TrimRight(p[:at], " ")
	}
	return p
}

// statementHeadParen reports whether the '(' at open is the head of a `switch`
// or `if` statement sitting directly in a function body. Its contents then run
// whenever the function does, so a guard bounded by it is statement-level.
func statementHeadParen(js string, open int) bool {
	head := strings.TrimRight(js[:open], " ")
	for _, kw := range []string{"switch", "if"} {
		if hasKeywordSuffix(head, kw) {
			return atFunctionStatementLevel(js, len(head)-len(kw))
		}
	}
	return false
}

// atFunctionStatementLevel reports whether the statement starting at pos is one
// of a function body's own statements — not an `else`'s, not under an `if`
// without braces, not inside a nested block. Anything else runs under a
// condition the rule would not carry.
func atFunctionStatementLevel(js string, pos int) bool {
	before := strings.TrimRight(js[:pos], " ")
	if before != "" {
		switch before[len(before)-1] {
		case '{', ';', '}':
		default:
			return false
		}
	}
	open := enclosingOpenerQuoted(js, pos)
	return open >= 0 && js[open] == '{' && isFunctionBody(js, open)
}

// isFunctionBody reports whether the '{' at open opens a function's body:
// `function(…){`, `function name(…){`, or an arrow's `=>{`.
func isFunctionBody(js string, open int) bool {
	head := strings.TrimRight(js[:open], " ")
	if strings.HasSuffix(head, "=>") {
		return true
	}
	if !strings.HasSuffix(head, ")") {
		return false
	}
	params := matchingOpenParen(head)
	if params < 0 {
		return false
	}
	name := strings.TrimRight(head[:params], " ")
	if hasKeywordSuffix(name, "function") {
		return true
	}
	i := len(name)
	for i > 0 && isIdentByte(name[i-1]) {
		i--
	}
	return i < len(name) && hasKeywordSuffix(strings.TrimRight(name[:i], " "), "function")
}

// enclosingOpenerQuoted is enclosingOpener skipping quoted strings, so a caption
// holding a bracket does not throw the count off.
func enclosingOpenerQuoted(js string, pos int) int {
	depth := 0
	inStr := byte(0)
	for i := pos - 1; i >= 0; i-- {
		c := js[i]
		if inStr != 0 {
			if c == inStr && (i == 0 || js[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr = c
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

// hasKeywordSuffix reports whether s ends with the keyword kw as a whole word.
func hasKeywordSuffix(s, kw string) bool {
	if !strings.HasSuffix(s, kw) {
		return false
	}
	i := len(s) - len(kw)
	return i == 0 || (!isIdentByte(s[i-1]) && s[i-1] != '.')
}

// hasKeywordAt reports whether the keyword kw starts at s[i] as a whole word.
func hasKeywordAt(s string, i int, kw string) bool {
	if !strings.HasPrefix(s[i:], kw) {
		return false
	}
	end := i + len(kw)
	return end == len(s) || !isIdentByte(s[end])
}

func skipSpaces(s string, i int) int {
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return i
}

// splitTopLevel splits s on sep where it is not nested in a bracket or string.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth := 0
	inStr := byte(0)
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if c == '\\' {
				i++
			} else if c == inStr {
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
		case sep:
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	return append(parts, s[start:])
}

func withoutStrings(ss, drop []string) []string {
	out := ss[:0:0]
	for _, s := range ss {
		keep := true
		for _, d := range drop {
			if s == d {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, s)
		}
	}
	return out
}
