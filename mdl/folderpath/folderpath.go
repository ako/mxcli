// SPDX-License-Identifier: Apache-2.0

// Package folderpath writes and reads the folder path of an MDL FOLDER clause.
//
// A folder path names a chain of nested folders, "Private/Apis", with '/' as
// the separator. Studio Pro does not reserve '/' in a folder *name*, so a
// folder called "Private - String en/de-cryption" is one folder, and joining
// names with a bare '/' made it indistinguishable from two. DESCRIBE wrote the
// path that way and `exec` split it, so a describe → exec round trip filed the
// document two folders deeper than it was, with no warning (mendixlabs/mxcli#1367).
//
// Inside a segment a '/' is written as `\/` and a '\' as `\\`. Reading is
// lenient about the backslash: one followed by anything other than '/' or '\'
// is kept as written, so a hand-written `folder 'A\B'` still names the folder
// `A\B` exactly as it did before escapes existed, and only `\/` and `\\`
// change meaning.
//
// This escape is the path's, not the string literal's: the clause is still
// quoted with the describer's string rule (mdlQuote) on top of it. `\/` reads
// the same under both language versions' string rules, which is why the
// separator escape is a backslash and not, say, a doubled slash — `a///b`
// would not say which side the literal slash belongs to.
package folderpath

import "strings"

// Join renders folder names, outermost first, as one folder path.
func Join(names []string) string {
	escaped := make([]string, len(names))
	for i, n := range names {
		escaped[i] = Escape(n)
	}
	return strings.Join(escaped, "/")
}

// Escape renders one folder name as a path segment.
func Escape(name string) string {
	if !strings.ContainsAny(name, `/\`) {
		return name
	}
	var b strings.Builder
	b.Grow(len(name) + 2)
	for i := 0; i < len(name); i++ {
		if c := name[i]; c == '/' || c == '\\' {
			b.WriteByte('\\')
		}
		b.WriteByte(name[i])
	}
	return b.String()
}

// Split reads a folder path back into its folder names, outermost first.
// Empty segments ("A//B", a leading or trailing '/') are dropped, as the path
// walkers always did.
func Split(path string) []string {
	var parts []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(path); i++ {
		c := path[i]
		switch {
		case c == '\\' && i+1 < len(path) && (path[i+1] == '/' || path[i+1] == '\\'):
			cur.WriteByte(path[i+1])
			i++
		case c == '/':
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return parts
}
