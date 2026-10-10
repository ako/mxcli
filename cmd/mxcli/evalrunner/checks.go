// SPDX-License-Identifier: Apache-2.0

package evalrunner

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// CheckOptions configures how checks are executed.
type CheckOptions struct {
	// ProjectPath is the path to the .mpr file.
	ProjectPath string

	// MxCliPath is the path to the mxcli binary. If empty, uses "mxcli" from PATH.
	MxCliPath string

	// MxPath is the path to the mx binary. If empty, attempts to find it.
	MxPath string

	// SkipMxCheck skips the mx check validation (expensive).
	SkipMxCheck bool

	// SkipTests skips tests_pass checks (they boot the app).
	SkipTests bool
}

// RunChecks executes all checks for a test phase and returns results.
func RunChecks(checks []Check, opts CheckOptions) []CheckResult {
	// Pre-fetch entities and pages lists to avoid repeated calls
	entityList := runMxCli(opts, "SHOW ENTITIES")
	pageList := runMxCli(opts, "SHOW PAGES")
	microflowList := runMxCli(opts, "SHOW MICROFLOWS")
	navMenu := runMxCli(opts, "LIST NAVIGATION MENU")

	// Cache for DESCRIBE results (avoid re-describing the same entity/page),
	// and for listings only some checks need.
	describeCache := make(map[string]string)

	var results []CheckResult
	for _, check := range checks {
		result := runCheck(check, opts, entityList, pageList, microflowList, navMenu, describeCache)
		results = append(results, result)
	}
	return results
}

// runCheck executes a single check and returns the result.
func runCheck(check Check, opts CheckOptions, entityList, pageList, microflowList, navMenu string, describeCache map[string]string) CheckResult {
	switch check.Type {
	case "entity_exists":
		return checkEntityExists(check, entityList)
	case "entity_has_attribute":
		return checkEntityHasAttribute(check, opts, entityList, describeCache)
	case "entity_has_type":
		return checkEntityHasType(check, entityList)
	case "page_exists":
		return checkPageExists(check, pageList)
	case "page_has_widget":
		return checkPageHasWidget(check, opts, pageList, describeCache)
	case "microflow_exists":
		return checkMicroflowExists(check, microflowList)
	case "navigation_has_item":
		return checkNavigationHasItem(check, navMenu)
	case "mx_check_passes":
		if opts.SkipMxCheck {
			return CheckResult{Check: check, Passed: true, Detail: "skipped"}
		}
		return checkMxCheck(check, opts)
	case "lint_passes":
		return checkLint(check, opts)
	case "association_exists":
		return checkListed(check, cachedList(opts, describeCache, "list associations"), "association")
	case "module_role_exists":
		return checkListed(check, cachedList(opts, describeCache, "list module roles"), "module role")
	case "workflow_exists":
		return checkListed(check, cachedList(opts, describeCache, "list workflows"), "workflow")
	case "file_exists":
		return checkFileExists(check, opts)
	case "tests_pass":
		if opts.SkipTests {
			return CheckResult{Check: check, Passed: true, Detail: "skipped"}
		}
		return checkTestsPass(check, opts)
	default:
		return CheckResult{Check: check, Passed: false, Detail: fmt.Sprintf("unknown check type: %s", check.Type)}
	}
}

// checkEntityExists verifies that an entity matching the pattern exists.
func checkEntityExists(check Check, entityList string) CheckResult {
	pattern := check.Args
	match := findMatch(entityList, pattern)
	if match != "" {
		return CheckResult{Check: check, Passed: true, Detail: fmt.Sprintf("found: %s", match)}
	}
	return CheckResult{Check: check, Passed: false, Detail: "entity not found"}
}

// checkEntityHasType verifies that `SHOW ENTITIES` lists the entity under the
// expected Type column: "Persistent", "View", "Non-Persistent" or "External".
// Args format: "Service.OpenRequestsPerFab View".
//
// entity_exists cannot do this. It matches on the qualified name alone, so a
// brief that asks for a view entity passes when the agent builds an ordinary
// persistent entity of the same name — which is the whole of what makes a view
// entity a view entity. The Type column is the only place the distinction is
// visible in a listing, and a name that merely ends in "View" is not evidence.
func checkEntityHasType(check Check, entityList string) CheckResult {
	parts := strings.Fields(check.Args)
	if len(parts) != 2 {
		return CheckResult{Check: check, Passed: false, Detail: `invalid args: expected "EntityPattern Type"`}
	}
	pattern, want := parts[0], parts[1]

	for _, line := range strings.Split(entityList, "\n") {
		matched := ""
		for _, name := range extractQualifiedNames(line) {
			if matchPattern(name, pattern) {
				matched = name
				break
			}
		}
		if matched == "" {
			continue
		}
		for _, field := range strings.Fields(line) {
			field = strings.Trim(field, "()[]{}:,;|")
			// The qualified name itself is never the type, so an entity called
			// Mod.View does not answer for a view entity.
			if field == matched {
				continue
			}
			if strings.EqualFold(field, want) {
				return CheckResult{Check: check, Passed: true, Detail: fmt.Sprintf("found: %s (%s)", matched, field)}
			}
		}
		// Distinguished on purpose from "not found": the entity was built and
		// is the wrong kind, which every other check kind reads as a pass.
		return CheckResult{Check: check, Passed: false, Detail: fmt.Sprintf("found %s, but not as %s", matched, want)}
	}
	return CheckResult{Check: check, Passed: false, Detail: "entity not found"}
}

// checkEntityHasAttribute verifies that an entity has an attribute with the expected type.
// Args format: "*.Book.Title String" or "*.Book.Title" (type optional)
func checkEntityHasAttribute(check Check, opts CheckOptions, entityList string, describeCache map[string]string) CheckResult {
	parts := strings.Fields(check.Args)
	if len(parts) < 1 {
		return CheckResult{Check: check, Passed: false, Detail: "invalid args: expected 'EntityPattern.Attribute [Type]'"}
	}

	// Split the pattern into entity pattern and attribute name
	attrPath := parts[0]
	expectedType := ""
	if len(parts) >= 2 {
		expectedType = parts[1]
	}

	// Find the last dot to split entity pattern from attribute name
	lastDot := strings.LastIndex(attrPath, ".")
	if lastDot == -1 {
		return CheckResult{Check: check, Passed: false, Detail: "invalid args: expected 'EntityPattern.Attribute'"}
	}

	entityPattern := attrPath[:lastDot]
	attrName := attrPath[lastDot+1:]

	// Resolve entity name
	entityName := findMatch(entityList, entityPattern)
	if entityName == "" {
		return CheckResult{Check: check, Passed: false, Detail: fmt.Sprintf("entity matching %q not found", entityPattern)}
	}

	// Get or cache the DESCRIBE output
	describe, ok := describeCache[entityName]
	if !ok {
		describe = runMxCli(opts, fmt.Sprintf("DESCRIBE ENTITY %s", entityName))
		describeCache[entityName] = describe
	}

	// Look for the attribute in the describe output
	// DESCRIBE ENTITY output looks like:
	//   Title: String(200)
	//   Price: Decimal
	//   StockQuantity: Integer
	found, foundType := findAttributeInDescribe(describe, attrName)
	if !found {
		return CheckResult{Check: check, Passed: false, Detail: fmt.Sprintf("attribute %q not found in %s", attrName, entityName)}
	}

	if expectedType != "" && !strings.EqualFold(foundType, expectedType) {
		// Check if the found type starts with the expected type (e.g., "String(200)" matches "String")
		if !strings.HasPrefix(strings.ToLower(foundType), strings.ToLower(expectedType)) {
			return CheckResult{Check: check, Passed: false, Detail: fmt.Sprintf("attribute %s has type %s, expected %s", attrName, foundType, expectedType)}
		}
	}

	return CheckResult{Check: check, Passed: true, Detail: fmt.Sprintf("found: %s.%s (%s)", entityName, attrName, foundType)}
}

// checkPageExists verifies that a page matching the pattern exists.
func checkPageExists(check Check, pageList string) CheckResult {
	pattern := check.Args
	match := findMatch(pageList, pattern)
	if match != "" {
		return CheckResult{Check: check, Passed: true, Detail: fmt.Sprintf("found: %s", match)}
	}
	return CheckResult{Check: check, Passed: false, Detail: "page not found"}
}

// checkPageHasWidget verifies that a page contains a specific widget type.
// Args format: "*Overview* dataGrid" or "*Overview* combobox|dropdown"
func checkPageHasWidget(check Check, opts CheckOptions, pageList string, describeCache map[string]string) CheckResult {
	parts := strings.SplitN(check.Args, " ", 2)
	if len(parts) < 2 {
		return CheckResult{Check: check, Passed: false, Detail: "invalid args: expected 'PagePattern widgetType'"}
	}

	pagePattern := parts[0]
	widgetTypes := strings.Split(parts[1], "|")

	// Resolve page name
	pageName := findMatch(pageList, pagePattern)
	if pageName == "" {
		return CheckResult{Check: check, Passed: false, Detail: fmt.Sprintf("page matching %q not found", pagePattern)}
	}

	// Get or cache the DESCRIBE output
	cacheKey := "page:" + pageName
	describe, ok := describeCache[cacheKey]
	if !ok {
		describe = runMxCli(opts, fmt.Sprintf("DESCRIBE PAGE %s", pageName))
		describeCache[cacheKey] = describe
	}

	// Check if any of the widget types appear in the describe output
	lower := strings.ToLower(describe)
	for _, wt := range widgetTypes {
		if strings.Contains(lower, strings.ToLower(strings.TrimSpace(wt))) {
			return CheckResult{Check: check, Passed: true, Detail: fmt.Sprintf("found %s in %s", wt, pageName)}
		}
	}

	return CheckResult{Check: check, Passed: false, Detail: fmt.Sprintf("no widget of type %s found in %s", parts[1], pageName)}
}

// checkMicroflowExists verifies that a microflow matching the pattern exists.
func checkMicroflowExists(check Check, microflowList string) CheckResult {
	pattern := check.Args
	match := findMatch(microflowList, pattern)
	if match != "" {
		return CheckResult{Check: check, Passed: true, Detail: fmt.Sprintf("found: %s", match)}
	}
	return CheckResult{Check: check, Passed: false, Detail: "microflow not found"}
}

// checkNavigationHasItem verifies that the navigation menu has at least one item.
func checkNavigationHasItem(check Check, navMenu string) CheckResult {
	navMenu = strings.TrimSpace(navMenu)
	if navMenu == "" {
		return CheckResult{Check: check, Passed: false, Detail: "navigation menu is empty"}
	}
	// Check for any MENU ITEM or page references
	lower := strings.ToLower(navMenu)
	if strings.Contains(lower, "menu item") || strings.Contains(lower, "page") || strings.Contains(lower, "├") || strings.Contains(lower, "└") {
		return CheckResult{Check: check, Passed: true, Detail: "navigation menu has items"}
	}
	// If there's any non-trivial content, count it as having items
	lines := strings.Split(navMenu, "\n")
	contentLines := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			contentLines++
		}
	}
	if contentLines > 1 {
		return CheckResult{Check: check, Passed: true, Detail: fmt.Sprintf("navigation has %d lines", contentLines)}
	}
	return CheckResult{Check: check, Passed: false, Detail: "navigation menu appears empty"}
}

// checkMxCheck runs `mx check` on the project.
func checkMxCheck(check Check, opts CheckOptions) CheckResult {
	mxPath := opts.MxPath
	if mxPath == "" {
		// Try common locations
		candidates := []string{
			"reference/mxbuild/modeler/mx",
			filepath.Join(filepath.Dir(opts.ProjectPath), "..", "reference/mxbuild/modeler/mx"),
		}
		for _, c := range candidates {
			if _, err := exec.LookPath(c); err == nil {
				mxPath = c
				break
			}
		}
		if mxPath == "" {
			// Try PATH
			if p, err := exec.LookPath("mx"); err == nil {
				mxPath = p
			}
		}
	}

	if mxPath == "" {
		return CheckResult{Check: check, Passed: false, Detail: "mx binary not found"}
	}

	// On a temporary copy: mx check writes theme-cache/ and deployment/sass/
	// into the project it checks (ako/mxcli#961).
	var stdout, stderr bytes.Buffer
	if err := docker.MxCheckOnCopy(mxPath, opts.ProjectPath, nil, &stdout, &stderr); err != nil {
		// Parse error output for error count
		output := stdout.String() + stderr.String()
		return CheckResult{Check: check, Passed: false, Detail: fmt.Sprintf("mx check failed: %s", firstLine(output))}
	}

	return CheckResult{Check: check, Passed: true, Detail: "mx check passed"}
}

// checkLint runs mxcli lint on the project.
func checkLint(check Check, opts CheckOptions) CheckResult {
	output := runMxCli(opts, "")
	if output == "" {
		return CheckResult{Check: check, Passed: false, Detail: "lint execution failed"}
	}

	// Run lint via the CLI
	cliPath := opts.MxCliPath
	if cliPath == "" {
		cliPath = "mxcli"
	}

	cmd := exec.Command(cliPath, "lint", "-p", opts.ProjectPath, "--format", "json")
	cmd.Env = append(cmd.Environ(), "MXCLI_QUIET=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return CheckResult{Check: check, Passed: false, Detail: fmt.Sprintf("lint failed: %s", firstLine(stderr.String()))}
	}

	// Check for errors in JSON output
	result := stdout.String()
	if strings.Contains(result, `"severity":"error"`) {
		return CheckResult{Check: check, Passed: false, Detail: "lint found errors"}
	}

	return CheckResult{Check: check, Passed: true, Detail: "lint passed"}
}

// runMxCli executes an mxcli command and returns the stdout output.
func runMxCli(opts CheckOptions, mdlCmd string) string {
	cliPath := opts.MxCliPath
	if cliPath == "" {
		cliPath = "mxcli"
	}

	args := []string{"-p", opts.ProjectPath, "-c", mdlCmd}

	cmd := exec.Command(cliPath, args...)
	cmd.Env = append(cmd.Environ(), "MXCLI_QUIET=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return ""
	}

	return stdout.String()
}

// findMatch looks for a qualified name in the output that matches the given pattern.
// Patterns support * as wildcard:
//   - "*.Book" matches "MyModule.Book"
//   - "*Overview*" matches "MyModule.Book_Overview"
//   - "MyModule.Book" matches exactly
func findMatch(output string, pattern string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Extract qualified names from the line (first word or known patterns)
		names := extractQualifiedNames(line)
		for _, name := range names {
			if matchPattern(name, pattern) {
				return name
			}
		}
	}
	return ""
}

// extractQualifiedNames extracts Module.Element style names from a line.
func extractQualifiedNames(line string) []string {
	var names []string

	// Split by whitespace and common separators
	fields := strings.Fields(line)
	for _, field := range fields {
		// Strip surrounding punctuation
		field = strings.Trim(field, "()[]{}:,;|")

		// Check if it looks like a qualified name (contains a dot, both parts are non-empty)
		if idx := strings.Index(field, "."); idx > 0 && idx < len(field)-1 {
			// Avoid matching URLs, file paths, etc.
			if !strings.Contains(field, "/") && !strings.Contains(field, "://") {
				names = append(names, field)
			}
		}
	}

	return names
}

// matchPattern matches a name against a pattern with * wildcards.
func matchPattern(name, pattern string) bool {
	// Simple wildcard matching
	if pattern == name {
		return true
	}

	// Handle * wildcards
	if !strings.Contains(pattern, "*") {
		return strings.EqualFold(name, pattern)
	}

	// Convert pattern to a simple check
	parts := strings.Split(strings.ToLower(pattern), "*")
	lower := strings.ToLower(name)

	// Check that all parts appear in order
	pos := 0
	for i, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(lower[pos:], part)
		if idx == -1 {
			return false
		}
		// First part must be at start if pattern doesn't start with *
		if i == 0 && !strings.HasPrefix(pattern, "*") && idx != 0 {
			return false
		}
		pos += idx + len(part)
	}

	// Last part must be at end if pattern doesn't end with *
	if !strings.HasSuffix(pattern, "*") && pos != len(lower) {
		return false
	}

	return true
}

// findAttributeInDescribe looks for an attribute in DESCRIBE ENTITY output.
// Returns (found, typeName).
func findAttributeInDescribe(describe string, attrName string) (bool, string) {
	lines := strings.Split(describe, "\n")
	lowerAttr := strings.ToLower(attrName)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Match patterns like:
		//   Title: String(200)
		//   Price: Decimal
		//   StockQuantity: Integer
		//   Name : String(100)     -- with spaces around colon
		colonIdx := strings.Index(trimmed, ":")
		if colonIdx == -1 {
			continue
		}

		name := strings.TrimSpace(trimmed[:colonIdx])
		typePart := strings.TrimSpace(trimmed[colonIdx+1:])
		// Strip trailing comma/semicolon (DESCRIBE output uses comma-separated attributes)
		typePart = strings.TrimRight(typePart, ",;")
		typePart = strings.TrimSpace(typePart)

		if strings.EqualFold(name, lowerAttr) {
			// Extract just the type name (strip size annotations like "(200)")
			baseType := typePart
			if parenIdx := strings.Index(baseType, "("); parenIdx > 0 {
				baseType = strings.TrimSpace(baseType[:parenIdx])
			}
			return true, baseType
		}
	}

	return false, ""
}

// firstLine returns the first non-empty line of a string.
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return s
}

// cachedList runs an MDL listing once per check run.
func cachedList(opts CheckOptions, cache map[string]string, mdl string) string {
	key := "list:" + mdl
	if out, ok := cache[key]; ok {
		return out
	}
	out := runMxCli(opts, mdl)
	cache[key] = out
	return out
}

// checkListed verifies that a listing names an element matching the pattern.
func checkListed(check Check, list, what string) CheckResult {
	if match := findMatch(list, check.Args); match != "" {
		return CheckResult{Check: check, Passed: true, Detail: "found: " + match}
	}
	return CheckResult{Check: check, Passed: false, Detail: what + " not found"}
}

// checkFileExists verifies that a glob, relative to the project directory,
// matches at least one file — e.g. "tests/*.test.mdl".
func checkFileExists(check Check, opts CheckOptions) CheckResult {
	pattern := filepath.Join(filepath.Dir(opts.ProjectPath), check.Args)
	matches, _ := filepath.Glob(pattern)
	if len(matches) == 0 {
		return CheckResult{Check: check, Passed: false, Detail: "no file matches " + check.Args}
	}
	return CheckResult{Check: check, Passed: true, Detail: fmt.Sprintf("%d file(s), e.g. %s", len(matches), filepath.Base(matches[0]))}
}

// checkTestsPass runs `mxcli test <path> [flags] -p <project>`. Args are the
// test path, relative to the project directory, then any flags: "tests --local".
func checkTestsPass(check Check, opts CheckOptions) CheckResult {
	fields := strings.Fields(check.Args)
	if len(fields) == 0 {
		return CheckResult{Check: check, Passed: false, Detail: "invalid args: expected '<test path> [flags]'"}
	}
	cliPath := opts.MxCliPath
	if cliPath == "" {
		cliPath = "mxcli"
	}
	args := append([]string{"test", fields[0]}, fields[1:]...)
	args = append(args, "-p", opts.ProjectPath)
	cmd := exec.Command(cliPath, args...)
	cmd.Dir = filepath.Dir(opts.ProjectPath)
	cmd.Env = append(cmd.Environ(), "MXCLI_QUIET=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return CheckResult{Check: check, Passed: false, Detail: "tests failed: " + lastLine(out.String())}
	}
	return CheckResult{Check: check, Passed: true, Detail: lastLine(out.String())}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
