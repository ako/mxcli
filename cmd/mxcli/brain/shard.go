// SPDX-License-Identifier: Apache-2.0

// shard.go - rendering a shard to Markdown and reading it back.
//
// The format is chosen for the reader, not the parser: a Mendix developer meets
// these files in a pull request diff, so the anchors are visible prose rather
// than metadata hidden in an HTML comment. There is exactly one copy of each
// fact in the file — a visible line that is also the parsed one — because a
// human-readable copy beside a machine-readable copy is two things that drift.
package brain

import (
	"fmt"
	"regexp"
	"strings"
)

// shardMarker identifies a file mxcli wrote. `brain init` refuses a docs/brain/
// whose README does not carry it, so an existing folder of someone else's notes
// is never adopted by accident.
const shardMarker = "<!-- mxcli-brain -->"

// metaLine matches an entry's one metadata line. The separator is a middle dot
// so that a title or body containing a hyphen cannot be mistaken for it.
var metaLine = regexp.MustCompile("^Anchors: (.*?) · id `([0-9a-f]{6})` · (\\d{4}-\\d{2}-\\d{2})\\s*$")

// anchorRef matches one backticked anchor inside the metadata line.
var anchorRef = regexp.MustCompile("`(@[A-Za-z_][A-Za-z0-9_.]*)`")

// RenderShard writes a whole shard. Entries are emitted in the order given;
// promotion appends, so that is chronological, and a diff shows one added block.
func RenderShard(shard string, entries []Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", shardTitle(shard))
	b.WriteString(shardMarker)
	b.WriteString("\n\n")
	b.WriteString(shardPreamble(shard))
	for _, e := range entries {
		b.WriteString("\n")
		b.WriteString(renderEntry(e))
	}
	return b.String()
}

func renderEntry(e Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s\n\n", e.Title)
	fmt.Fprintf(&b, "Anchors: %s · id `%s` · %s\n", renderAnchors(e.Anchors), e.ID, e.Date)
	if e.Body != "" {
		fmt.Fprintf(&b, "\n%s\n", e.Body)
	}
	return b.String()
}

func renderAnchors(anchors []string) string {
	if len(anchors) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(anchors))
	for _, a := range anchors {
		if !strings.HasPrefix(a, "@") {
			a = "@" + a
		}
		parts = append(parts, "`"+a+"`")
	}
	return strings.Join(parts, ", ")
}

// ParseShard reads entries back out of a rendered shard. A block that does not
// carry a well-formed metadata line is *reported*, not skipped: silently
// dropping it would make `check` claim a clean shard while an entry sits in it
// unchecked.
//
// The kind is taken from the shard, never from the entry's own text. A
// requirement is a requirement because it lives under plan/, so there is no
// second copy of that fact in the file to drift from the first.
func ParseShard(shard, content string) (entries []Entry, malformed []string, err error) {
	for _, blk := range splitEntries(content) {
		e, ok := parseEntry(blk)
		if !ok {
			malformed = append(malformed, firstLine(blk))
			continue
		}
		if IsPlanShard(shard) {
			e.Kind, e.Slice = KindRequirement, SliceOf(shard)
		}
		entries = append(entries, e)
	}
	return entries, malformed, nil
}

func splitEntries(content string) []string {
	lines := strings.Split(content, "\n")
	var blocks []string
	var cur []string
	in := false
	for _, ln := range lines {
		if strings.HasPrefix(ln, "## ") {
			if in {
				blocks = append(blocks, strings.Join(cur, "\n"))
			}
			in, cur = true, []string{ln}
			continue
		}
		if in {
			cur = append(cur, ln)
		}
	}
	if in {
		blocks = append(blocks, strings.Join(cur, "\n"))
	}
	return blocks
}

func parseEntry(block string) (Entry, bool) {
	lines := strings.Split(block, "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "## ") {
		return Entry{}, false
	}
	e := Entry{Title: strings.TrimSpace(strings.TrimPrefix(lines[0], "## "))}
	metaAt := -1
	for i := 1; i < len(lines); i++ {
		if m := metaLine.FindStringSubmatch(strings.TrimSpace(lines[i])); m != nil {
			for _, a := range anchorRef.FindAllStringSubmatch(m[1], -1) {
				e.Anchors = append(e.Anchors, a[1])
			}
			e.ID, e.Date = m[2], m[3]
			metaAt = i
			break
		}
	}
	if metaAt < 0 {
		return Entry{}, false
	}
	e.Body = strings.TrimSpace(strings.Join(lines[metaAt+1:], "\n"))
	return e, true
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

func shardTitle(shard string) string {
	switch {
	case shard == ProjectShard:
		return "Project"
	case IsPlanShard(shard):
		return "Slice: " + SliceOf(shard)
	default:
		return shard
	}
}

func shardPreamble(shard string) string {
	if IsPlanShard(shard) {
		return "Requirements for this slice. Anchors point FORWARD, at what the slice\n" +
			"will build — an anchor that does not resolve yet means not built, not\n" +
			"stale. `mxcli brain plan` counts them against the model.\n"
	}
	if shard == ProjectShard {
		return "Decisions that are not about one module. This file is loaded every\n" +
			"session, so it carries the tightest cap — see `mxcli brain show`.\n"
	}
	return fmt.Sprintf("Decisions anchored to the %s module. Loaded when %s is in play,\n"+
		"not otherwise.\n", shard, shard)
}

// HasMarker reports whether a file was written by mxcli.
func HasMarker(content string) bool { return strings.Contains(content, shardMarker) }

// CountLines is the size measure caps are expressed in, because lines are what
// an agent pays for when the shard is loaded into context.
func CountLines(content string) int {
	content = strings.TrimRight(content, "\n")
	if content == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}
