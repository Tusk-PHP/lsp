package symbols

import "strings"

// SetOptionalNamespaces records the namespaces of optional packages the
// project doesn't have installed (from Composer's "suggest"), as lowercase
// prefixes ending in `\`. Classes under them are missing by design, so the
// unknown-class check stays quiet about them. The slice is kept as given.
func (idx *Index) SetOptionalNamespaces(prefixes []string) {
	idx.mu.Lock()
	idx.optionalNamespaces = prefixes
	idx.mu.Unlock()
}

// IsOptionalClass reports whether a fully qualified class name falls under
// one of the optional namespaces. The list holds one or two entries per
// suggested package, so this is a short loop, not a scan of the index.
func (idx *Index) IsOptionalClass(fqn string) bool {
	idx.mu.RLock()
	prefixes := idx.optionalNamespaces
	idx.mu.RUnlock()
	if len(prefixes) == 0 {
		return false
	}
	lower := strings.ToLower(strings.TrimPrefix(fqn, `\`))
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}
