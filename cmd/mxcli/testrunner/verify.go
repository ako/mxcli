// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// Verify is one @verify assertion: an OQL query, a comparison operator, and the
// value the query's single result must satisfy.
//
//	@verify select count(*) as n from Sudoku.Cell = 81
//
// @verify exists for the half of a Mendix app @expect cannot reach. Most
// microflows are side effects — they write rows and return nothing useful — so
// asserting on what the database holds afterwards is the only way to test them.
// That is exactly why it must be able to fail: it covers the least testable
// surface in the app, and until now no query, true or false, well-formed or
// not, could make it do so.
type Verify struct {
	// Raw is the annotation body as written, for messages.
	Raw string
	// Query is the OQL sent to the running app.
	Query string
	// Operator is the comparison, normalised (`<>` becomes `!=`).
	Operator string
	// Expected is the literal the result is compared against, as written.
	Expected string
}

// ParseVerify parses one @verify annotation body.
//
// The shape is `<oql> <op> <literal>`, and the split is the **last** comparison
// operator outside quotes and parentheses: the expectation is a literal at the
// very end, so anything earlier belongs to the query — a WHERE clause's own `=`,
// or one inside a subquery.
//
// Both halves are then checked, because the split alone is not enough to know it
// was right. A right-hand side that is not a literal means the operator found
// was part of the query, and continuing would send the runtime a truncated
// query — silently asserting on something other than what was written.
func ParseVerify(raw string) (Verify, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Verify{}, fmt.Errorf("@verify needs an OQL query and an expected value")
	}

	pos, op := lastTopLevelComparison(raw)
	if pos < 0 {
		return Verify{}, fmt.Errorf(
			"@verify %s: no comparison found — a @verify is an OQL query followed by "+
				"an expected value, e.g. `@verify select count(*) as n from Mod.Entity = 1`", raw)
	}

	query := strings.TrimSpace(raw[:pos])
	expected := strings.TrimSpace(raw[pos+len(op):])

	if !isOQLLiteral(expected) {
		return Verify{}, fmt.Errorf(
			"@verify %s: %q is not a value to compare against — the expected value must be "+
				"a number, a quoted string, or true/false", raw, expected)
	}
	if err := looksLikeOQL(query); err != nil {
		return Verify{}, fmt.Errorf("@verify %s: %w", raw, err)
	}

	if op == "<>" {
		op = "!="
	}
	return Verify{Raw: raw, Query: query, Operator: op, Expected: expected}, nil
}

// looksLikeOQL rejects an obviously non-query left-hand side before it is ever
// sent to the runtime.
//
// This is a cheap sanity check, not a parser — the runtime is the authority on
// whether OQL is valid, and an error from it is reported as an error. What this
// catches is the annotation that was never a query at all (`this is not a
// query = 1`), where the runtime's message would be less use than saying so
// here.
func looksLikeOQL(query string) error {
	if query == "" {
		return fmt.Errorf("the query is empty")
	}
	fields := strings.Fields(strings.ToLower(query))
	if len(fields) == 0 || fields[0] != "select" {
		return fmt.Errorf("the query must start with SELECT")
	}
	for _, f := range fields {
		if f == "from" || strings.HasPrefix(f, "from(") {
			return nil
		}
	}
	return fmt.Errorf("the query has no FROM clause")
}

// comparisonTokens are matched longest-first so `<=` is not read as `<`.
var comparisonTokens = []string{"!=", "<>", "<=", ">=", "=", "<", ">"}

// lastTopLevelComparison returns the offset and text of the last comparison
// operator that is not inside a string literal or parentheses, or (-1, "").
func lastTopLevelComparison(s string) (int, string) {
	bestPos, bestOp := -1, ""
	depth := 0
	inString := false

	for i := 0; i < len(s); {
		c := s[i]
		if inString {
			if c == '\'' {
				// '' is an escaped quote inside an OQL string literal.
				if i+1 < len(s) && s[i+1] == '\'' {
					i += 2
					continue
				}
				inString = false
			}
			i++
			continue
		}
		switch {
		case c == '\'':
			inString = true
			i++
			continue
		case c == '(':
			depth++
			i++
			continue
		case c == ')':
			if depth > 0 {
				depth--
			}
			i++
			continue
		}
		if depth == 0 {
			if op := matchComparison(s[i:]); op != "" {
				bestPos, bestOp = i, op
				i += len(op)
				continue
			}
		}
		i++
	}
	return bestPos, bestOp
}

func matchComparison(s string) string {
	for _, op := range comparisonTokens {
		if strings.HasPrefix(s, op) {
			return op
		}
	}
	return ""
}

// isOQLLiteral reports whether the expectation is something that can be compared
// against — a number, a quoted string, or a boolean.
func isOQLLiteral(s string) bool {
	if s == "" {
		return false
	}
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return true
	}
	switch strings.ToLower(s) {
	case "true", "false", "empty", "null":
		return true
	}
	_, err := strconv.ParseFloat(s, 64)
	return err == nil
}

// scalarOf reduces an OQL result to the single value the assertion compares
// against, or explains why it cannot.
//
// A @verify compares one value with one literal, so anything other than exactly
// one row and one column is an error. Picking a cell out of a table would be a
// guess, and a guess here is the same silent wrong answer the annotation is
// being fixed for.
func (v Verify) scalarOf(columns []string, rows [][]any) (any, error) {
	switch {
	case len(rows) == 0:
		return nil, fmt.Errorf("the query returned no rows; @verify needs exactly one row and one column")
	case len(rows) > 1:
		return nil, fmt.Errorf("the query returned %d rows; @verify needs exactly one row and one column "+
			"(aggregate it, e.g. select count(*))", len(rows))
	case len(columns) != 1:
		return nil, fmt.Errorf("the query returned %d columns; @verify needs exactly one row and one column",
			len(columns))
	case len(rows[0]) != 1:
		return nil, fmt.Errorf("the query returned a row of %d values; @verify needs exactly one row and one column",
			len(rows[0]))
	}
	return rows[0][0], nil
}

// compare evaluates the assertion against the value the query returned. It also
// returns the value rendered for the failure message, because a @verify that
// says only what was expected leaves you no better off than before.
func (v Verify) compare(actual any) (bool, string, error) {
	shown := renderOQLValue(actual)

	// The runtime is asked for numbers as strings (numberHandling: asString), so
	// a count comes back as "81" and must still compare numerically against 81.
	if want, err := strconv.ParseFloat(strings.TrimSpace(v.Expected), 64); err == nil {
		got, ok := numericValue(actual)
		if !ok {
			return false, shown, fmt.Errorf(
				"the query returned %s, which cannot be compared numerically with %s", shown, v.Expected)
		}
		return compareOrdered(got, want, v.Operator), shown, nil
	}

	switch strings.ToLower(v.Expected) {
	case "true", "false":
		want := strings.EqualFold(v.Expected, "true")
		got, ok := actual.(bool)
		if !ok {
			// A boolean may arrive as the string "true"/"false".
			s := strings.ToLower(strings.TrimSpace(shown))
			if s != "true" && s != "false" {
				return false, shown, fmt.Errorf(
					"the query returned %s, which is not a boolean", shown)
			}
			got = s == "true"
		}
		return compareEquality(got == want, v.Operator, shown)
	case "empty", "null":
		return compareEquality(actual == nil, v.Operator, shown)
	}

	// A quoted string literal.
	want := strings.ReplaceAll(strings.Trim(v.Expected, "'"), "''", "'")
	if actual == nil {
		return compareEquality(false, v.Operator, shown)
	}
	return compareOrderedStrings(shown, want, v.Operator), shown, nil
}

// compareEquality handles the operators that make sense for a value with no
// ordering: a boolean, or a null check.
//
// shown is threaded through rather than dropped — a FAIL that reports the
// expectation with an empty "actual" is the failure message this change exists
// to improve.
func compareEquality(equal bool, op, shown string) (bool, string, error) {
	switch op {
	case "=":
		return equal, shown, nil
	case "!=":
		return !equal, shown, nil
	}
	return false, shown, fmt.Errorf("%s cannot be used with this expected value; use = or !=", op)
}

func compareOrdered(got, want float64, op string) bool {
	switch op {
	case "=":
		return got == want
	case "!=":
		return got != want
	case "<":
		return got < want
	case "<=":
		return got <= want
	case ">":
		return got > want
	case ">=":
		return got >= want
	}
	return false
}

func compareOrderedStrings(got, want, op string) bool {
	switch op {
	case "=":
		return got == want
	case "!=":
		return got != want
	case "<":
		return got < want
	case "<=":
		return got <= want
	case ">":
		return got > want
	case ">=":
		return got >= want
	}
	return false
}

// numericValue coerces an OQL value to a float for comparison. JSON numbers
// arrive as float64 and, under numberHandling asString, as strings.
func numericValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	}
	return 0, false
}

// renderOQLValue formats a returned value for a failure message.
func renderOQLValue(v any) string {
	if v == nil {
		return "NULL"
	}
	if s, ok := v.(string); ok {
		return s
	}
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return fmt.Sprintf("%v", v)
}

// runVerifies evaluates a test's @verify assertions against the running app and
// downgrades the result if any of them does not hold.
//
// The three outcomes are kept apart on purpose, because collapsing them is the
// defect this replaces:
//
//   - the query ran and the comparison held — nothing changes;
//   - the query ran and the comparison did not hold — FAIL, reporting the value
//     that came back alongside the one that was wanted;
//   - the query could not be run or its result cannot be compared — ERROR, which
//     is counted with the failures, never a pass.
//
// The first failing assertion wins; later ones are not run, for the same reason
// @expect stops at the first failure — the first one is the informative one.
func runVerifies(res *TestResult, tc TestCase, admin docker.M2EEOptions, projectPath string) {
	for _, v := range tc.Verify {
		result, err := docker.ExecuteOQL(oqlOptionsFor(admin, projectPath), v.Query)
		if err != nil {
			res.Status = StatusError
			res.Message = fmt.Sprintf("@verify %s: %v", v.Raw, err)
			return
		}
		actual, err := v.scalarOf(result.Columns, result.Rows)
		if err != nil {
			res.Status = StatusError
			res.Message = fmt.Sprintf("@verify %s: %v", v.Raw, err)
			return
		}
		ok, shown, err := v.compare(actual)
		if err != nil {
			res.Status = StatusError
			res.Message = fmt.Sprintf("@verify %s: %v", v.Raw, err)
			return
		}
		if !ok {
			res.Status = StatusFail
			res.Message = fmt.Sprintf("expected %s, actual: %s", v.Raw, shown)
			return
		}
	}
}

// oqlOptionsFor adapts the admin connection the runner already holds into what
// ExecuteOQL wants. Direct is always set: the runner reaches the app over
// loopback in both the booted and attached cases, never through docker exec.
func oqlOptionsFor(admin docker.M2EEOptions, projectPath string) docker.OQLOptions {
	return docker.OQLOptions{
		Host:        admin.Host,
		Port:        admin.Port,
		Token:       admin.Token,
		ProjectPath: projectPath,
		Direct:      true,
		Stdout:      io.Discard,
		Stderr:      io.Discard,
	}
}
