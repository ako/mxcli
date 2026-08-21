// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"strings"
)

// Verdict protocol. A test microflow returns one string: either verdictPass, or
// verdictFailPrefix followed by the reason. The endpoint hands that string back
// as the HTTP response's "result" field, so a result is a returned value rather
// than something recovered from the runtime log.
const (
	verdictPass       = "PASS"
	verdictFailPrefix = "FAIL:"
	// verdictSetupPrefix is followed by the setup microflow that threw. It is a
	// third outcome on purpose: the test never ran, so it neither passed nor
	// failed, and reporting a broken fixture as a FAIL blames the code under
	// test for it.
	verdictSetupPrefix = "SETUP:"
)

// GenerateTestFlows returns the MDL declaring one microflow per test case.
//
// This is the endpoint path's counterpart to GenerateTestRunner, which compiles
// the whole suite into a single after-startup microflow. One microflow per test
// buys three things that the monolith cannot give:
//
//   - Each test can be invoked, re-invoked, or skipped on its own, so --filter
//     and single-test runs are a matter of which URL is called.
//   - A test that throws fails only itself. In the monolith an uncaught error
//     ends the whole flow, and because that flow is the after-startup action it
//     also fails the boot.
//   - Every test gets its own variable scope, so the suffix-renaming the
//     monolith needs to keep `$result` in test 1 from colliding with `$result`
//     in test 2 is simply not required here.
func GenerateTestFlows(suite *TestSuite) string {
	var b strings.Builder
	b.WriteString("CREATE MODULE " + mxTestModule + ";\n\n")
	for _, tc := range suite.Tests {
		// A test with an uncompilable @expect gets no microflow. The runner
		// reports it as an ERROR from the parse message, which is more useful
		// than a microflow that runs and cannot assert anything.
		if len(tc.AssertionErrors) > 0 {
			continue
		}
		writeTestFlow(&b, tc)
		b.WriteString("\n")
	}
	return b.String()
}

// writeTestFlow writes one test's microflow.
func writeTestFlow(b *strings.Builder, tc TestCase) {
	fmt.Fprintf(b, "/** %s */\n", escapeMDLComment(tc.Name))
	fmt.Fprintf(b, "CREATE OR REPLACE MICROFLOW %s ()\n", testFlowName(tc))
	b.WriteString("RETURNS String AS $Verdict\n")
	b.WriteString("BEGIN\n")
	fmt.Fprintf(b, "  DECLARE $Verdict String = '%s';\n", verdictPass)

	// Before the body, and before a @throws test pre-sets its failing verdict:
	// the fixture is not the thing expected to throw.
	writeSetupCalls(b, tc)

	if tc.Throws != "" {
		writeThrowsFlowBody(b, tc)
	} else {
		writeExpectFlowBody(b, tc)
	}

	b.WriteString("  RETURN $Verdict;\n")
	b.WriteString("END;\n")
	b.WriteString("/\n")
}

// writeSetupCalls writes the @setup microflow calls that precede a test's body.
//
// Each is a plain call — a fixture is a microflow, so there is nothing to
// resolve and nothing to declare — with a handler that returns the SETUP verdict
// and stops. Continuing into a test whose preconditions were not established
// produces an assertion failure that says nothing about the code under test.
func writeSetupCalls(b *strings.Builder, tc TestCase) {
	for _, flow := range tc.Setups {
		fmt.Fprintf(b, "  CALL MICROFLOW %s() ON ERROR {\n", flow)
		fmt.Fprintf(b, "    SET $Verdict = '%s';\n",
			escapeMDLString(verdictSetupPrefix+flow))
		b.WriteString("    RETURN $Verdict;\n")
		b.WriteString("  };\n")
	}
}

// writeExpectFlowBody writes the body of a normal test: run the MDL, then check
// each @expect. An error during the body short-circuits to a FAIL verdict.
func writeExpectFlowBody(b *strings.Builder, tc TestCase) {
	for _, line := range rewriteBodyForVerdict(strings.Split(tc.MDL, "\n"), tc) {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	writeExpectAggregates(b, "  ", tc.Expects)
	for _, exp := range tc.Expects {
		writeExpectCheck(b, exp)
	}
}

// writeExpectAggregates emits the Aggregate list activities the assertions need,
// after the body has produced the lists and before the first decision reads
// them. One activity per variable, however many assertions refer to it.
func writeExpectAggregates(b *strings.Builder, indent string, expects []Expect) {
	seen := map[string]bool{}
	for _, exp := range expects {
		for _, agg := range exp.Aggregates {
			if seen[agg.Var] {
				continue
			}
			seen[agg.Var] = true
			fmt.Fprintf(b, "%s%s = %s(%s);\n", indent, agg.Var, agg.Op, agg.List)
		}
	}
}

// writeThrowsFlowBody writes the body of an @throws test: the verdict starts as
// a failure and only the error handler can clear it, so a body that completes
// without throwing fails — which is the point of the annotation.
func writeThrowsFlowBody(b *strings.Builder, tc TestCase) {
	fmt.Fprintf(b, "  SET $Verdict = '%s';\n",
		escapeMDLString(verdictFailPrefix+"expected an exception but none was thrown"))
	for _, line := range rewriteBodyForThrows(strings.Split(tc.MDL, "\n")) {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteString("\n")
	}
}

// writeExpectCheck writes one @expect assertion.
//
// The assertion is emitted as the expression the author wrote, so whatever
// Mendix can evaluate is evaluated. `<>` never reaches the model — ParseExpect
// rewrites it to `!=`, the spelling Mendix's expression engine accepts — which
// is what the old branch-swapping workaround was for.
func writeExpectCheck(b *strings.Builder, exp Expect) {
	// An earlier statement may already have failed the test; never overwrite an
	// existing failure with a later assertion's result.
	fmt.Fprintf(b, "  IF $Verdict = '%s' THEN\n", verdictPass)
	fmt.Fprintf(b, "    IF %s THEN\n", exp.Condition)
	b.WriteString("    ELSE\n")
	fmt.Fprintf(b, "      SET $Verdict = %s;\n", failVerdictExpr(exp))
	b.WriteString("    END IF;\n")
	b.WriteString("  END IF;\n")
}

// failVerdictExpr builds the MDL expression assigned to $Verdict when an
// assertion fails.
//
// When the observed value can be rendered as a String without guessing its type,
// it is concatenated onto the message. A failure that says only what was expected
// tells you nothing about what came back, which is half the value of a failing
// test.
func failVerdictExpr(exp Expect) string {
	msg := verdictFailPrefix + "expected " + exp.Raw
	if exp.Actual == "" {
		return "'" + escapeMDLString(msg) + "'"
	}
	return "'" + escapeMDLString(msg+", actual: ") + "' + " + exp.Actual
}

// rewriteBodyForVerdict attaches an ON ERROR handler to every CALL in the test
// body, turning a thrown error into a FAIL verdict and an early return.
func rewriteBodyForVerdict(lines []string, tc TestCase) []string {
	handler := []string{
		fmt.Sprintf("  SET $Verdict = '%s';",
			escapeMDLString(verdictFailPrefix+"exception during execution")),
		"  RETURN $Verdict;",
	}
	return attachOnError(lines, handler)
}

// rewriteBodyForThrows attaches an ON ERROR handler that clears the pre-set
// failure verdict — the error is the expected outcome.
func rewriteBodyForThrows(lines []string) []string {
	handler := []string{fmt.Sprintf("  SET $Verdict = '%s';", verdictPass)}
	return attachOnError(lines, handler)
}

// attachOnError appends `ON ERROR { ... }` to each CALL statement in the body,
// joining a statement that spans several lines first.
func attachOnError(lines, handler []string) []string {
	var out []string
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !containsCallMicroflow(trimmed) {
			out = append(out, lines[i])
			continue
		}

		stmt := lines[i]
		for !strings.HasSuffix(strings.TrimSpace(stmt), ";") && i+1 < len(lines) {
			i++
			stmt += "\n" + lines[i]
		}
		stmt = strings.TrimSuffix(strings.TrimSpace(stmt), ";")

		out = append(out, stmt+" ON ERROR {")
		out = append(out, handler...)
		out = append(out, "};")
	}
	return out
}

// escapeMDLComment keeps a test name from closing the javadoc block it sits in.
func escapeMDLComment(s string) string {
	return strings.ReplaceAll(s, "*/", "* /")
}
