// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"regexp"
	"strings"
)

// GenerateTestRunner generates the MDL for a TestRunner microflow from parsed test cases.
// The generated microflow:
// - Wraps each test in LOG+CALL+assertion pattern
// - Outputs structured MXTEST: log lines for result parsing
// - Returns Boolean (required by Mendix for after-startup)
func GenerateTestRunner(suite *TestSuite) string {
	var b strings.Builder

	// Module creation (idempotent — only if MxTest module doesn't exist)
	b.WriteString("CREATE MODULE MxTest;\n\n")

	b.WriteString("CREATE OR REPLACE MICROFLOW MxTest.TestRunner ()\n")
	b.WriteString("RETURNS Boolean AS $AllPassed\n")
	b.WriteString("BEGIN\n")
	b.WriteString("  DECLARE $AllPassed Boolean = true;\n")
	b.WriteString("  DECLARE $TestFailed Boolean = false;\n")
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  LOG INFO NODE 'MXTEST' 'MXTEST:START:%s';\n", escapeMDLString(suite.Name)))
	b.WriteString("\n")

	for i, tc := range suite.Tests {
		// A test whose @expect did not compile gets no block. The runner reports
		// it as an ERROR from the parse message; running it would report a pass
		// for an assertion that was never made.
		if len(tc.ExpectErrors) > 0 {
			continue
		}
		writeTestBlock(&b, tc, i)
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("  LOG INFO NODE 'MXTEST' 'MXTEST:END:%s';\n", escapeMDLString(suite.Name)))
	b.WriteString("  RETURN $AllPassed;\n")
	b.WriteString("END;\n")
	b.WriteString("/\n")

	return b.String()
}

// writeTestBlock writes the MDL for a single test case within the TestRunner.
func writeTestBlock(b *strings.Builder, tc TestCase, index int) {
	suffix := fmt.Sprintf("_%d", index+1)

	b.WriteString(fmt.Sprintf("  -- Test %d: %s\n", index+1, tc.Name))
	b.WriteString(fmt.Sprintf("  LOG INFO NODE 'MXTEST' 'MXTEST:RUN:%s:%s';\n",
		escapeMDLString(tc.ID), escapeMDLString(tc.Name)))
	b.WriteString("  SET $TestFailed = false;\n")

	if tc.Throws != "" {
		writeThrowsTestBlock(b, tc, suffix)
		return
	}

	// Collect variable names from the MDL body and rename them with suffix
	varNames := extractVariableNames(tc.MDL)
	renamedMDL := renameVariables(tc.MDL, varNames, suffix)

	// Rewrite the MDL body: indent it and wrap CALL MICROFLOW with error handling
	lines := strings.Split(renamedMDL, "\n")
	rewritten := rewriteWithErrorHandling(lines, tc.ID)
	for _, line := range rewritten {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Generate assertion checks for @expect (with renamed variables)
	// Use flat IF blocks (no nesting) to avoid Mendix end-event issues
	if len(tc.Expects) > 0 {
		for _, exp := range tc.Expects {
			renamedExp := renameExpect(exp, varNames, suffix)
			writeExpectAssertion(b, tc.ID, renamedExp)
		}
	} else if tc.Throws == "" {
		// No expectations — just check it didn't throw
		b.WriteString("  IF $TestFailed = false THEN\n")
		b.WriteString(fmt.Sprintf("    LOG INFO NODE 'MXTEST' 'MXTEST:PASS:%s';\n", escapeMDLString(tc.ID)))
		b.WriteString("  END IF;\n")
	}
}

// writeThrowsTestBlock generates code for a test that expects an error.
func writeThrowsTestBlock(b *strings.Builder, tc TestCase, suffix string) {
	didThrowVar := "$DidThrow" + suffix
	b.WriteString(fmt.Sprintf("  DECLARE %s Boolean = false;\n", didThrowVar))

	// Rename variables in the body
	varNames := extractVariableNames(tc.MDL)
	renamedMDL := renameVariables(tc.MDL, varNames, suffix)

	lines := strings.Split(renamedMDL, "\n")
	rewritten := rewriteForThrowsTest(lines, tc.ID, didThrowVar)
	for _, line := range rewritten {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}

	// Check that it did throw
	b.WriteString(fmt.Sprintf("  IF %s THEN\n", didThrowVar))
	b.WriteString(fmt.Sprintf("    LOG INFO NODE 'MXTEST' 'MXTEST:PASS:%s';\n", escapeMDLString(tc.ID)))
	b.WriteString("  ELSE\n")
	b.WriteString(fmt.Sprintf("    LOG ERROR NODE 'MXTEST' 'MXTEST:FAIL:%s:Expected exception but none was thrown';\n",
		escapeMDLString(tc.ID)))
	b.WriteString("    SET $AllPassed = false;\n")
	b.WriteString("  END IF;\n")
}

// writeExpectAssertion generates an IF/ELSE check for a single @expect assertion.
//
// The condition is the author's own expression, guarded by $TestFailed so an
// assertion is not evaluated after the body already threw. `<>` never reaches
// the model: ParseExpect rewrites it to `!=`, which is the spelling Mendix
// accepts — the branch-swapping this function used to do was a workaround for
// emitting `<>` verbatim.
//
// Unlike the endpoint generator this path reports failures through the log
// protocol, whose message is a single log line, so the observed value is
// concatenated into that line rather than into a returned verdict.
func writeExpectAssertion(b *strings.Builder, testID string, exp Expect) {
	passCondition := fmt.Sprintf("$TestFailed = false and (%s)", exp.Condition)
	failMsg := "MXTEST:FAIL:" + testID + ":Expected " + exp.Raw

	b.WriteString(fmt.Sprintf("  IF %s THEN\n", passCondition))
	b.WriteString(fmt.Sprintf("    LOG INFO NODE 'MXTEST' 'MXTEST:PASS:%s';\n", escapeMDLString(testID)))
	b.WriteString("  ELSE\n")
	if exp.Actual == "" {
		b.WriteString(fmt.Sprintf("    LOG ERROR NODE 'MXTEST' '%s';\n", escapeMDLString(failMsg)))
	} else {
		b.WriteString(fmt.Sprintf("    LOG ERROR NODE 'MXTEST' '%s' + %s;\n",
			escapeMDLString(failMsg+", actual: "), exp.Actual))
	}
	b.WriteString("    SET $AllPassed = false;\n")
	b.WriteString("  END IF;\n")
}

// varPattern matches $VariableName in MDL ($ followed by word characters).
var varPattern = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// assignPattern matches "$var = CALL" or "$var = CREATE" at the start of any
// statement line inside the test body.
var assignPattern = regexp.MustCompile(`(?m)^\s*\$([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:CALL|CREATE)`)

// extractVariableNames finds all user-defined variable names in the MDL body.
// Returns the set of variable names (without $ prefix).
func extractVariableNames(mdl string) map[string]bool {
	names := make(map[string]bool)

	// Find assignment targets: $var = CALL MICROFLOW ... or $var = CREATE ...
	for _, m := range assignPattern.FindAllStringSubmatch(mdl, -1) {
		names[m[1]] = true
	}

	return names
}

// renameVariables replaces all occurrences of $varName with $varName_suffix
// for each variable in the names set.
func renameVariables(mdl string, names map[string]bool, suffix string) string {
	if len(names) == 0 {
		return mdl
	}

	return varPattern.ReplaceAllStringFunc(mdl, func(match string) string {
		// match is like "$result" — extract the name part
		name := match[1:] // strip $
		if names[name] {
			return "$" + name + suffix
		}
		return match
	})
}

// renameExpect applies variable renaming to an Expect assertion.
//
// The monolithic runner compiles every test into one microflow, so `$result` in
// test 1 and `$result` in test 2 have to be told apart. Renaming runs over the
// rendered condition and the actual-value expression, which is why both are kept
// as text rather than as a tree.
func renameExpect(exp Expect, names map[string]bool, suffix string) Expect {
	rename := func(src string) string {
		return varPattern.ReplaceAllStringFunc(src, func(match string) string {
			if names[match[1:]] {
				return match + suffix
			}
			return match
		})
	}
	renamed := exp
	renamed.Condition = rename(exp.Condition)
	renamed.Actual = rename(exp.Actual)
	return renamed
}

// rewriteWithErrorHandling adds ON ERROR clauses to CALL MICROFLOW statements.
// The error handler ends with RETURN $AllPassed to properly terminate the flow path
// with a return value (required when the microflow returns a type).
func rewriteWithErrorHandling(lines []string, testID string) []string {
	var result []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if containsCallMicroflow(trimmed) && strings.HasSuffix(trimmed, ";") {
			withoutSemicolon := strings.TrimSuffix(trimmed, ";")
			result = append(result, withoutSemicolon+" ON ERROR {")
			result = append(result, fmt.Sprintf(
				"  LOG ERROR NODE 'MXTEST' 'MXTEST:FAIL:%s:Exception during execution';",
				escapeMDLString(testID)))
			result = append(result, "  SET $TestFailed = true;")
			result = append(result, "  SET $AllPassed = false;")
			result = append(result, "  RETURN $AllPassed;")
			result = append(result, "};")
		} else if containsCallMicroflow(trimmed) && !strings.HasSuffix(trimmed, ";") {
			var callLines []string
			callLines = append(callLines, line)
			for i+1 < len(lines) {
				i++
				nextLine := lines[i]
				callLines = append(callLines, nextLine)
				if strings.HasSuffix(strings.TrimSpace(nextLine), ";") {
					break
				}
			}
			joined := strings.Join(callLines, "\n")
			joined = strings.TrimSpace(joined)
			joined = strings.TrimSuffix(joined, ";")
			result = append(result, joined+" ON ERROR {")
			result = append(result, fmt.Sprintf(
				"  LOG ERROR NODE 'MXTEST' 'MXTEST:FAIL:%s:Exception during execution';",
				escapeMDLString(testID)))
			result = append(result, "  SET $TestFailed = true;")
			result = append(result, "  SET $AllPassed = false;")
			result = append(result, "  RETURN $AllPassed;")
			result = append(result, "};")
		} else {
			result = append(result, line)
		}
	}

	return result
}

// rewriteForThrowsTest wraps CALL MICROFLOW with ON ERROR that sets the throw flag.
func rewriteForThrowsTest(lines []string, testID string, didThrowVar string) []string {
	var result []string

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if containsCallMicroflow(trimmed) && strings.HasSuffix(trimmed, ";") {
			withoutSemicolon := strings.TrimSuffix(trimmed, ";")
			result = append(result, withoutSemicolon+" ON ERROR {")
			result = append(result, fmt.Sprintf("  SET %s = true;", didThrowVar))
			result = append(result, "};")
		} else if containsCallMicroflow(trimmed) && !strings.HasSuffix(trimmed, ";") {
			var callLines []string
			callLines = append(callLines, line)
			for i+1 < len(lines) {
				i++
				nextLine := lines[i]
				callLines = append(callLines, nextLine)
				if strings.HasSuffix(strings.TrimSpace(nextLine), ";") {
					break
				}
			}
			joined := strings.Join(callLines, "\n")
			joined = strings.TrimSpace(joined)
			joined = strings.TrimSuffix(joined, ";")
			result = append(result, joined+" ON ERROR {")
			result = append(result, fmt.Sprintf("  SET %s = true;", didThrowVar))
			result = append(result, "};")
		} else {
			result = append(result, line)
		}
	}

	return result
}

// containsCallMicroflow checks if a line starts a CALL MICROFLOW statement.
func containsCallMicroflow(s string) bool {
	upper := strings.ToUpper(s)
	return strings.Contains(upper, "CALL MICROFLOW") ||
		strings.Contains(upper, "CALL NANOFLOW")
}

// escapeMDLString escapes single quotes for MDL string literals.
func escapeMDLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
