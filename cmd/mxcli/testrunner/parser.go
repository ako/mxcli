// SPDX-License-Identifier: Apache-2.0

// Package testrunner implements the MDL test framework for executing and
// validating microflow tests against a running Mendix runtime.
package testrunner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TestCase represents a single test extracted from a test file.
type TestCase struct {
	ID      string   // Generated test ID (test_1, test_2, ...)
	Name    string   // From @test annotation
	MDL     string   // Raw MDL statements for this test block
	Expects []Expect // @expect assertions
	// AssertionErrors holds one message per annotation that claims to assert
	// something and cannot. A test carrying any of these is reported as an ERROR
	// and never run: an assertion that cannot be evaluated must not be able to
	// report a pass.
	AssertionErrors []string
	Verify          []Verify // @verify OQL post-conditions
	Setup           string   // @setup block reference
	Cleanup         string   // @cleanup strategy ("rollback" or "none")
	Throws          string   // @throws expected error message
	SourceFile      string   // Original file path
	Line            int      // Line number in source file
}

// AssertionCount reports how many assertions the test actually makes.
//
// @expect, @verify and @throws each assert something a runner evaluates. An
// annotation that is parsed and never executed must not be counted here — that
// is what made @verify look like an assertion while asserting nothing.
func (tc TestCase) AssertionCount() int {
	n := len(tc.Expects) + len(tc.Verify)
	if tc.Throws != "" {
		n++
	}
	return n
}

// TestSuite represents a collection of tests from one or more files.
type TestSuite struct {
	Name  string     // Suite name (derived from file name)
	Tests []TestCase // Test cases
}

// ParseTestFile parses a test file (.test.mdl or .test.md) and extracts test cases.
func ParseTestFile(path string) (*TestSuite, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading test file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	base := filepath.Base(path)

	// Derive suite name from filename
	suiteName := strings.TrimSuffix(base, filepath.Ext(base))
	if strings.HasSuffix(suiteName, ".test") {
		suiteName = strings.TrimSuffix(suiteName, ".test")
	}

	var tests []TestCase
	switch ext {
	case ".md":
		tests, err = parseMarkdownTests(string(content), path)
	default:
		// .mdl or .test.mdl
		tests, err = parseMDLTests(string(content), path)
	}
	if err != nil {
		return nil, err
	}

	return &TestSuite{
		Name:  suiteName,
		Tests: tests,
	}, nil
}

// ParseTestDir parses all test files in a directory.
func ParseTestDir(dir string) (*TestSuite, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading test directory: %w", err)
	}

	suite := &TestSuite{
		Name: filepath.Base(dir),
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if isTestFile(name) {
			sub, err := ParseTestFile(filepath.Join(dir, name))
			if err != nil {
				return nil, fmt.Errorf("parsing %s: %w", name, err)
			}
			suite.Tests = append(suite.Tests, sub.Tests...)
		}
	}

	return suite, nil
}

// isTestFile returns true if the filename matches a test file pattern.
func isTestFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".test.mdl") ||
		strings.HasSuffix(lower, ".test.md")
}

// parseMDLTests parses test blocks from a .test.mdl file.
// Each test block is a javadoc comment followed by MDL statements, separated by '/'.
func parseMDLTests(content string, sourcePath string) ([]TestCase, error) {
	blocks := splitTestBlocks(content)
	var tests []TestCase

	for i, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		// Extract javadoc comment and MDL body
		doc, body, line := extractDocAndBody(block, content)
		if doc == "" {
			// No javadoc — skip this block (it's not a test)
			continue
		}

		annotations := parseAnnotations(doc)
		if annotations.Test == "" {
			// Has javadoc but no @test — not a test block
			continue
		}

		if err := validateCleanup(annotations.Cleanup); err != nil {
			return nil, fmt.Errorf("%s: test %q: %w", sourcePath, annotations.Test, err)
		}

		testID := fmt.Sprintf("test_%d", i+1)
		tests = append(tests, TestCase{
			ID:              testID,
			Name:            annotations.Test,
			MDL:             strings.TrimSpace(body),
			Expects:         annotations.Expects,
			AssertionErrors: annotations.AssertionErrors,
			Verify:          annotations.Verify,
			Setup:           annotations.Setup,
			Cleanup:         annotations.Cleanup,
			Throws:          annotations.Throws,
			SourceFile:      sourcePath,
			Line:            line,
		})
	}

	return tests, nil
}

// parseMarkdownTests extracts test blocks from ```mdl-test fenced code blocks.
func parseMarkdownTests(content string, sourcePath string) ([]TestCase, error) {
	var tests []TestCase
	testNum := 0

	scanner := bufio.NewScanner(strings.NewReader(content))
	lineNum := 0
	inCodeBlock := false
	var blockLines []string
	blockStart := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if !inCodeBlock {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```mdl-test") {
				inCodeBlock = true
				blockLines = nil
				blockStart = lineNum
			}
		} else {
			trimmed := strings.TrimSpace(line)
			if trimmed == "```" {
				// End of code block — parse it
				inCodeBlock = false
				blockContent := strings.Join(blockLines, "\n")

				// Parse the block as a single test
				doc, body, _ := extractDocAndBody(blockContent, blockContent)
				annotations := parseAnnotations(doc)

				if err := validateCleanup(annotations.Cleanup); err != nil {
					return nil, fmt.Errorf("%s: test at line %d: %w", sourcePath, blockStart, err)
				}

				testNum++
				testID := fmt.Sprintf("test_%d", testNum)

				name := annotations.Test
				if name == "" {
					name = fmt.Sprintf("test at line %d", blockStart)
				}

				tests = append(tests, TestCase{
					ID:              testID,
					Name:            name,
					MDL:             strings.TrimSpace(body),
					Expects:         annotations.Expects,
					AssertionErrors: annotations.AssertionErrors,
					Verify:          annotations.Verify,
					Setup:           annotations.Setup,
					Cleanup:         annotations.Cleanup,
					Throws:          annotations.Throws,
					SourceFile:      sourcePath,
					Line:            blockStart,
				})
			} else {
				blockLines = append(blockLines, line)
			}
		}
	}

	return tests, nil
}

// splitTestBlocks splits MDL content on '/' delimiters (the microflow block terminator).
func splitTestBlocks(content string) []string {
	// Split on lines that are just '/' (the MDL block separator)
	var blocks []string
	var current strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "/" {
			blocks = append(blocks, current.String())
			current.Reset()
		} else {
			if current.Len() > 0 {
				current.WriteString("\n")
			}
			current.WriteString(line)
		}
	}

	// Don't forget the last block (after the last '/' or if no '/' found)
	if current.Len() > 0 {
		blocks = append(blocks, current.String())
	}

	return blocks
}

// extractDocAndBody separates the javadoc comment from the MDL body.
// Returns (docComment, body, lineNumber).
func extractDocAndBody(block string, fullContent string) (string, string, int) {
	block = strings.TrimSpace(block)

	// Find /** ... */ pattern
	docStart := strings.Index(block, "/**")
	if docStart == -1 {
		return "", block, 1
	}

	docEnd := strings.Index(block[docStart:], "*/")
	if docEnd == -1 {
		return "", block, 1
	}
	docEnd += docStart + 2 // include the */

	doc := block[docStart:docEnd]
	body := strings.TrimSpace(block[docEnd:])

	// Estimate line number
	line := 1 + strings.Count(fullContent[:strings.Index(fullContent, block[:20])], "\n")

	return doc, body, line
}

// annotations holds parsed javadoc annotations for a test block.
type annotations struct {
	Test            string
	Expects         []Expect
	AssertionErrors []string
	Verify          []Verify
	Setup           string
	Cleanup         string
	Throws          string
}

var (
	// expectPattern captures the whole annotation body rather than a fixed
	// operand/operator/operand shape. Matching a shape is what made this silent:
	// a line the pattern did not fit produced no assertion at all, and a test
	// with no assertions passes. Everything after @expect is now handed to
	// ParseExpect, which either compiles it or reports why it could not.
	expectPattern  = regexp.MustCompile(`@expect\s+(.+)`)
	verifyPattern  = regexp.MustCompile(`@verify\s+(.+)`)
	testPattern    = regexp.MustCompile(`@test\s+(.+)`)
	setupPattern   = regexp.MustCompile(`@setup\s+(\S+)`)
	cleanupPattern = regexp.MustCompile(`@cleanup\s+(\S+)`)
	throwsPattern  = regexp.MustCompile(`@throws\s+'([^']*)'`)
)

// parseAnnotations extracts test annotations from a javadoc comment.
func parseAnnotations(doc string) annotations {
	var a annotations
	a.Cleanup = "rollback" // default

	// Strip /** and */
	doc = strings.TrimPrefix(doc, "/**")
	doc = strings.TrimSuffix(doc, "*/")

	// Process line by line
	scanner := bufio.NewScanner(strings.NewReader(doc))
	for scanner.Scan() {
		line := scanner.Text()
		// Strip leading * and whitespace
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		if m := testPattern.FindStringSubmatch(line); m != nil {
			a.Test = strings.TrimSpace(m[1])
		}
		if m := expectPattern.FindStringSubmatch(line); m != nil {
			exp, err := ParseExpect(m[1])
			if err != nil {
				a.AssertionErrors = append(a.AssertionErrors, err.Error())
			} else {
				a.Expects = append(a.Expects, exp)
			}
		}
		if m := verifyPattern.FindStringSubmatch(line); m != nil {
			v, err := ParseVerify(m[1])
			if err != nil {
				a.AssertionErrors = append(a.AssertionErrors, err.Error())
			} else {
				a.Verify = append(a.Verify, v)
			}
		}
		if m := setupPattern.FindStringSubmatch(line); m != nil {
			a.Setup = strings.TrimSpace(m[1])
		}
		if m := cleanupPattern.FindStringSubmatch(line); m != nil {
			a.Cleanup = strings.TrimSpace(m[1])
		}
		if m := throwsPattern.FindStringSubmatch(line); m != nil {
			a.Throws = m[1]
		}
	}

	checkVerifyCleanup(&a)
	return a
}

// checkVerifyCleanup refuses a @verify on a test whose writes are rolled back.
//
// @verify asserts on rows the microflow wrote, and @cleanup rollback — the
// default — undoes them when the call returns, before anything can look. The
// query would run against the pre-test state and report a confident wrong
// answer, which is the failure mode this annotation is being fixed for. So it is
// refused, with the one-line change that makes it work.
func checkVerifyCleanup(a *annotations) {
	if len(a.Verify) == 0 || a.Cleanup != CleanupRollback {
		return
	}
	for _, v := range a.Verify {
		a.AssertionErrors = append(a.AssertionErrors, fmt.Sprintf(
			"@verify %s: this test uses @cleanup rollback (the default), so its writes are "+
				"undone before the query runs and it would assert against the pre-test "+
				"state. Add @cleanup none to the test", v.Raw))
	}
	a.Verify = nil
}
