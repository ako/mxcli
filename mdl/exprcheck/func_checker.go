// SPDX-License-Identifier: Apache-2.0

package exprcheck

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

type funcSig struct {
	args    []TypeKind // full parameter list (max arity)
	minArgs int        // minimum required args; 0 means all args required (= len(args))
	ret     TypeKind
}

// min returns the effective minimum argument count.
func (s funcSig) min() int {
	if s.minArgs > 0 {
		return s.minArgs
	}
	return len(s.args)
}

// funcTable lists every Mendix built-in function and its signature.
// Extracted from Mendix 10/11 expression documentation.
// When Mendix adds new built-ins, add them here and update the corresponding
// test in func_checker_test.go. Source: reference/mendixmodellib/reflection-data/.
var funcTable = map[string]funcSig{
	// Boolean
	"not": {args: []TypeKind{KindBoolean}, ret: KindBoolean},

	// String — predicates
	"contains":   {args: []TypeKind{KindString, KindString}, ret: KindBoolean},
	"startsWith": {args: []TypeKind{KindString, KindString}, ret: KindBoolean},
	"endsWith":   {args: []TypeKind{KindString, KindString}, ret: KindBoolean},
	"isMatch":    {args: []TypeKind{KindString, KindString}, ret: KindBoolean},

	// String — transforms
	"length":       {args: []TypeKind{KindString}, ret: KindInteger},
	"find":         {args: []TypeKind{KindString, KindString}, ret: KindInteger},
	"findLast":     {args: []TypeKind{KindString, KindString}, ret: KindInteger},
	"substring":    {args: []TypeKind{KindString, KindInteger, KindInteger}, minArgs: 2, ret: KindString},
	"trim":         {args: []TypeKind{KindString}, ret: KindString},
	"toUpperCase":  {args: []TypeKind{KindString}, ret: KindString},
	"toLowerCase":  {args: []TypeKind{KindString}, ret: KindString},
	"replaceAll":   {args: []TypeKind{KindString, KindString, KindString}, ret: KindString},
	"replaceFirst": {args: []TypeKind{KindString, KindString, KindString}, ret: KindString},
	"replace":      {args: []TypeKind{KindString, KindString, KindString}, ret: KindString},
	"urlEncode":    {args: []TypeKind{KindString}, ret: KindString},
	"urlDecode":    {args: []TypeKind{KindString}, ret: KindString},

	// Type conversion
	"toString":     {args: []TypeKind{KindAny}, ret: KindString},
	"parseInteger": {args: []TypeKind{KindString}, ret: KindInteger},
	"parseDecimal": {args: []TypeKind{KindString}, ret: KindDecimal},
	"parseBoolean": {args: []TypeKind{KindString}, ret: KindBoolean},
	// formatDecimal(value [, format [, languageTag]])  — format is optional
	"formatDecimal": {args: []TypeKind{KindDecimal, KindString, KindString}, minArgs: 1, ret: KindString},

	// Math
	"abs":    {args: []TypeKind{KindDecimal}, ret: KindDecimal},
	"round":  {args: []TypeKind{KindDecimal, KindInteger}, minArgs: 1, ret: KindDecimal},
	"floor":  {args: []TypeKind{KindDecimal}, ret: KindDecimal},
	"ceil":   {args: []TypeKind{KindDecimal}, ret: KindDecimal},
	"pow":    {args: []TypeKind{KindDecimal, KindDecimal}, ret: KindDecimal},
	"sqrt":   {args: []TypeKind{KindDecimal}, ret: KindDecimal},
	"max":    {args: []TypeKind{KindDecimal, KindDecimal}, ret: KindDecimal},
	"min":    {args: []TypeKind{KindDecimal, KindDecimal}, ret: KindDecimal},
	"random": {args: []TypeKind{}, ret: KindDecimal},

	// Enumerations
	"getCaption": {args: []TypeKind{KindAny}, ret: KindString},
	"getKey":     {args: []TypeKind{KindAny}, ret: KindString},

	// Special checks — the object-state predicates. Each takes an object and
	// returns Boolean; `isSynced`/`isSyncing` are offline-sync predicates and are
	// nanoflow-only, but that is a context restriction, not an unknown name.
	// All three were missing from this table, so MDL044 flagged them as
	// hallucinated; each was built against mxbuild 11.13.0 at 0 errors before
	// being added here (the table is the sole allow-list MDL044 consults, and
	// MDL044 is now enforced on the exec path).
	"isNew":     {args: []TypeKind{KindAny}, ret: KindBoolean},
	"isSynced":  {args: []TypeKind{KindAny}, ret: KindBoolean},
	"isSyncing": {args: []TypeKind{KindAny}, ret: KindBoolean},

	// DateTime — construction
	"currentDateTime": {args: []TypeKind{}, ret: KindDateTime},
	// dateTime/dateTimeUTC(year, month, day [, hour, minute, second]) — 3 or 6 args
	"dateTime":    {args: []TypeKind{KindInteger, KindInteger, KindInteger, KindInteger, KindInteger, KindInteger}, minArgs: 3, ret: KindDateTime},
	"dateTimeUTC": {args: []TypeKind{KindInteger, KindInteger, KindInteger, KindInteger, KindInteger, KindInteger}, minArgs: 3, ret: KindDateTime},

	// DateTime — add (local calendar)
	"addMilliseconds": {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addSeconds":      {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addMinutes":      {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addHours":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addDays":         {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addWeeks":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addMonths":       {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addQuarters":     {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addYears":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},

	// DateTime — add (UTC calendar)
	"addDaysUTC":     {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addWeeksUTC":    {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addMonthsUTC":   {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addQuartersUTC": {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"addYearsUTC":    {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},

	// DateTime — subtract (local calendar)
	"subtractMilliseconds": {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractSeconds":      {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractMinutes":      {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractHours":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractDays":         {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractWeeks":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractMonths":       {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractQuarters":     {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractYears":        {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},

	// DateTime — subtract (UTC calendar)
	"subtractDaysUTC":     {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractWeeksUTC":    {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractMonthsUTC":   {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractQuartersUTC": {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},
	"subtractYearsUTC":    {args: []TypeKind{KindDateTime, KindInteger}, ret: KindDateTime},

	// DateTime — between
	// The "between dates" duration functions return Decimal in Mendix (e.g.
	// daysBetween can be 1.5), so assigning one to an Integer/Long variable fails
	// the build with CE0117 (findings #2). The "calendar" variants return whole
	// units (Integer); millisecondsBetween returns a whole count (Long).
	"millisecondsBetween":   {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindLong},
	"secondsBetween":        {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"minutesBetween":        {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"hoursBetween":          {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"daysBetween":           {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"weeksBetween":          {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindDecimal},
	"calendarMonthsBetween": {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindInteger},
	"calendarYearsBetween":  {args: []TypeKind{KindDateTime, KindDateTime}, ret: KindInteger},

	// DateTime — begin-of
	"beginOfDay":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"beginOfWeek":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"beginOfMonth": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"beginOfYear":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},

	// DateTime — end-of
	"endOfDay":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"endOfWeek":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"endOfMonth": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"endOfYear":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},

	// DateTime — trim-to (local calendar)
	"trimToSeconds": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToMinutes": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToHours":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToDays":    {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToMonths":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToYears":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},

	// DateTime — trim-to (UTC calendar)
	"trimToHoursUTC":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToDaysUTC":   {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToMonthsUTC": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
	"trimToYearsUTC":  {args: []TypeKind{KindDateTime}, ret: KindDateTime},

	// DateTime — formatting / parsing (local calendar)
	"formatDateTime": {args: []TypeKind{KindDateTime, KindString}, ret: KindString},
	"formatTime":     {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	"formatDate":     {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	"parseDateTime":  {args: []TypeKind{KindString, KindString}, ret: KindDateTime},

	// DateTime — formatting / parsing (UTC calendar)
	"formatDateTimeUTC": {args: []TypeKind{KindDateTime, KindString}, ret: KindString},
	"formatTimeUTC":     {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	"formatDateUTC":     {args: []TypeKind{KindDateTime, KindString}, minArgs: 1, ret: KindString},
	"parseDateTimeUTC":  {args: []TypeKind{KindString, KindString}, ret: KindDateTime},

	// DateTime — epoch conversion
	"dateTimeToEpoch": {args: []TypeKind{KindDateTime}, ret: KindLong},
	"epochToDateTime": {args: []TypeKind{KindLong}, ret: KindDateTime},

	// DateTime — extraction → Integer.
	// NOTE: `year(...)` and `month(...)` were previously listed here but Mendix
	// has no such built-ins — `mxcli check` passed and the build then failed with
	// CE0117 (ledger finding #16). The correct approach is date formatting/parsing
	// (`parseInteger(formatDateTime($d, 'yyyy'))`). The sibling extraction names
	// below are UNVERIFIED against a real Mendix build and are likely invalid too;
	// remove any that a build rejects (do not add more without confirming against
	// Studio Pro — this whole family looks speculative).
	"dayOfYear":   {args: []TypeKind{KindDateTime}, ret: KindInteger},
	"dayOfMonth":  {args: []TypeKind{KindDateTime}, ret: KindInteger},
	"weekOfYear":  {args: []TypeKind{KindDateTime}, ret: KindInteger},
	"dayOfWeek":   {args: []TypeKind{KindDateTime}, ret: KindInteger},
	"hour":        {args: []TypeKind{KindDateTime}, ret: KindInteger},
	"minute":      {args: []TypeKind{KindDateTime}, ret: KindInteger},
	"second":      {args: []TypeKind{KindDateTime}, ret: KindInteger},
	"millisecond": {args: []TypeKind{KindDateTime}, ret: KindInteger},

	// DateTime — extraction (legacy alias)
	"dateTimeToDate": {args: []TypeKind{KindDateTime}, ret: KindDateTime},
}

// checkCallExpr returns hints for arity / arg-type mismatches against
// the built-in function signature table. User-defined microflow calls
// (CallExpr.Name containing a dot) are not checked here — they require
// catalog lookup.
func checkCallExpr(c *CallExpr, ctx Context) []Hint {
	sig, ok := funcTable[c.Name]
	if !ok {
		return nil
	}
	var out []Hint
	got := len(c.Args)
	minA, maxA := sig.min(), len(sig.args)
	if got < minA || got > maxA {
		var expectStr, fixStr string
		if minA == maxA {
			expectStr = fmt.Sprintf("%d", maxA)
			fixStr = fmt.Sprintf("Provide %d argument(s) for %s().", maxA, c.Name)
		} else {
			expectStr = fmt.Sprintf("%d to %d", minA, maxA)
			fixStr = fmt.Sprintf("Provide %d to %d argument(s) for %s().", minA, maxA, c.Name)
		}
		out = append(out, Hint{
			Code: "E006", Slug: "func-arg-arity",
			Severity: hints.SeverityError,
			Where: hints.Location{
				File: ctx.File, Line: c.Pos().Line, Column: c.Pos().Column,
				Microflow: ctx.Microflow,
				Context:   fmt.Sprintf("call to %s()", c.Name),
			},
			YouWrote: fmt.Sprintf("%s(...) with %d argument(s)", c.Name, got),
			Problem:  fmt.Sprintf("%s() expects %s argument(s), got %d.", c.Name, expectStr, got),
			Fix:      fixStr,
			Reference: &hints.Reference{
				FunctionName:    c.Name,
				FunctionReturns: typeKindName(sig.ret),
				FunctionArgs:    typeKindNames(sig.args),
			},
		})
	}
	return out
}

func typeKindName(k TypeKind) string {
	switch k {
	case KindBoolean:
		return "Boolean"
	case KindString:
		return "String"
	case KindInteger:
		return "Integer"
	case KindLong:
		return "Long"
	case KindDecimal:
		return "Decimal"
	case KindDateTime:
		return "DateTime"
	case KindBinary:
		return "Binary"
	case KindObject:
		return "Object"
	case KindList:
		return "List"
	case KindEnumeration:
		return "Enumeration"
	case KindAny:
		return "Any"
	case KindEmpty:
		return "Empty"
	}
	return "Unknown"
}

func typeKindNames(ks []TypeKind) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = typeKindName(k)
	}
	return out
}

// KindName is the public accessor for the human-readable name of a TypeKind
// (e.g. KindBoolean → "Boolean"). Used by cmd/mxcli for help/explain output.
func KindName(k TypeKind) string { return typeKindName(k) }

// PublicFuncSig is a JSON/CLI-friendly view of a built-in function signature.
type PublicFuncSig struct {
	Args    []string
	MinArgs int // 0 means all Args are required
	Returns string
}

// PublicFuncTable returns a JSON-friendly view of the built-in function
// signatures used by the checker.
func PublicFuncTable() map[string]PublicFuncSig {
	out := make(map[string]PublicFuncSig, len(funcTable))
	for k, v := range funcTable {
		out[k] = PublicFuncSig{
			Args:    typeKindNames(v.args),
			MinArgs: v.minArgs,
			Returns: typeKindName(v.ret),
		}
	}
	return out
}

// FuncReturnKind returns the TypeKind of the return value of a named built-in
// function. Returns (KindUnknown, false) for unknown functions.
func FuncReturnKind(name string) (TypeKind, bool) {
	sig, ok := funcTable[name]
	if !ok {
		return KindUnknown, false
	}
	return sig.ret, true
}
