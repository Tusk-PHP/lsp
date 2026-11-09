package resolve

import "strings"

// The providers pass a file around both as its source and as its lines, and
// resolving one expression can go back and forth between the two many times.
// Over a long file, joining and splitting it each time costs more than the
// resolution itself. The resolver keeps the last file it saw in both forms,
// with its scopes, and only rebuilds them when the file changes.

// sameLines reports whether a and b hold the same lines. Lines split from the
// same source share their bytes, so this is usually a pointer comparison per
// line rather than a comparison of their text.
func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 || &a[0] == &b[0] {
		return true // the same slice, as Lines hands out
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// JoinLines returns lines joined into the source they were split from,
// reusing the last result while the lines don't change.
func (r *Resolver) JoinLines(lines []string) string {
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	if r.cachedLines != nil && sameLines(r.cachedLines, lines) {
		return r.scopeSource
	}
	r.setSourceLocked(strings.Join(lines, "\n"), lines)
	return r.scopeSource
}

// Lines returns the source split into lines, reusing the last result while
// the source doesn't change. Treat the result as read-only.
func (r *Resolver) Lines(source string) []string {
	r.scopeMu.Lock()
	defer r.scopeMu.Unlock()
	if r.cachedLines != nil && r.scopeSource == source {
		return r.cachedLines
	}
	r.setSourceLocked(source, SplitLines(source))
	return r.cachedLines
}

// setSourceLocked switches the cache to another file; its scopes are worked
// out again when they're next asked for.
func (r *Resolver) setSourceLocked(source string, lines []string) {
	if r.scopeSource == source && r.cachedLines != nil {
		r.cachedLines = lines
		return
	}
	r.scopeSource, r.cachedLines, r.scopeDoc = source, lines, nil
}
