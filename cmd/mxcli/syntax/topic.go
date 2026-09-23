// SPDX-License-Identifier: Apache-2.0

package syntax

import "strings"

// Match is the outcome of looking a topic up in the registry.
type Match struct {
	// Path is the registry path the words resolved to.
	Path string
	// Features are the topics to show. Empty when nothing matched.
	Features []SyntaxFeature
	// Exact reports whether Path named a topic (or the prefix of one)
	// directly. When it is false and Features is non-empty, the features came
	// from a segment match on Fallback rather than from Path.
	Exact bool
	// Fallback is the word the segment match ran on, set only when Exact is
	// false and Features is non-empty.
	Fallback string
}

// Lookup resolves the words a caller typed to a set of topics.
//
// It is the ONE answer to "which topic is this?", shared by `mxcli syntax` and
// the REPL's `help`. They used to resolve separately and disagree:
// `help workflow user task` found the page, `mxcli syntax "workflow user-task"`
// did not, because the CLI joined its arguments on "." and never split them.
// A topic handed over as a single string — a quoted copy-paste from the
// command's own help, a tool wrapper, `sh -c` — came out as the path
// "workflow user-task", which matches nothing, and the answer to a topic that
// exists was "Unknown topic: workflow user-task"
// (mendixlabs/mxcli#1025, and #955 before it).
func Lookup(args []string) Match {
	words := topicWords(args)
	if len(words) == 0 {
		return Match{}
	}

	path := ResolveAlias(resolvePath(words))
	if HasPrefix(path) {
		return Match{Path: path, Features: ByPrefix(path), Exact: true}
	}

	// Not a path from the left. Before giving up, match against any SEGMENT of
	// a path: `rule` names a real topic (two, in fact), it just is not the
	// first segment of either (#955).
	//
	// The segment match runs on the LAST word, not on the dotted path: no
	// segment contains a ".", so passing the whole path could only ever match
	// when the query was a single word — which made this fallback silently
	// dead for every multi-word query.
	last := words[len(words)-1]
	if features := BySegmentMatch(last); len(features) > 0 {
		return Match{Path: path, Features: features, Fallback: last}
	}
	return Match{Path: path}
}

// topicWords flattens the arguments into lower-case words, splitting on
// whitespace and on ".". That is what makes these one query:
//
//	mxcli syntax workflow user-task targeting
//	mxcli syntax "workflow user-task targeting"
//	mxcli syntax workflow.user-task.targeting
//	help workflow user task            (in the REPL)
func topicWords(args []string) []string {
	var words []string
	for _, arg := range args {
		for _, field := range strings.Fields(strings.ToLower(arg)) {
			for _, seg := range strings.Split(field, ".") {
				if seg != "" {
					words = append(words, seg)
				}
			}
		}
	}
	return words
}

// resolvePath converts words like ["workflow", "user", "task"] into a registry
// path like "workflow.user-task", greedily merging adjacent words with hyphens
// to find the longest matching prefix at each level.
func resolvePath(words []string) string {
	var segments []string
	i := 0
	for i < len(words) {
		matched := false
		for j := len(words); j > i; j-- {
			candidate := strings.Join(words[i:j], "-")
			testPath := strings.Join(append(segments, candidate), ".")
			if HasPrefix(testPath) {
				segments = append(segments, candidate)
				i = j
				matched = true
				break
			}
		}
		if !matched {
			segments = append(segments, words[i])
			i++
		}
	}
	return strings.Join(segments, ".")
}
