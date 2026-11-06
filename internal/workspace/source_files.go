package workspace

import "strings"

// IsPHPSource reports whether a path or URI is PHP source the server reads:
// `.php` files and `.phtml` templates (Magento, Zend Framework and others keep
// their views in .phtml). Matching ignores case.
func IsPHPSource(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".php") || strings.HasSuffix(lower, ".phtml")
}

// IsTemplate reports whether a path or URI is a .phtml template. A template is
// always included by other code, which provides the variables it reads.
func IsTemplate(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".phtml")
}
