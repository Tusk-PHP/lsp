package workspace

import "strings"

// IsPHPSource reports whether a path or URI is PHP source the server reads:
// `.php` files and `.phtml` templates (Magento, Zend Framework and others keep
// their views in .phtml). Matching ignores case.
func IsPHPSource(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".php") || strings.HasSuffix(lower, ".phtml")
}

// IsTemplate reports whether a path or URI is a template: a .phtml file or a
// Blade view. A template is always rendered by other code, which provides the
// variables it reads.
func IsTemplate(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".phtml") || IsBlade(lower)
}

// IsBlade reports whether a path or URI is a Blade template (.blade.php). Its
// PHP is read through a virtual document, and it declares no symbols.
func IsBlade(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".blade.php")
}
