package symbols

import (
	"strings"
)

// validLocales is the set of locales the PHP manual actually serves.
var validLocales = map[string]bool{
	"en":    true,
	"de":    true,
	"es":    true,
	"fr":    true,
	"it":    true,
	"ja":    true,
	"pt_BR": true,
	"ru":    true,
	"tr":    true,
	"zh":    true,
}

const phpManualBase = "https://www.php.net/manual/"

// PHPManualURL returns a php.net URL for the given builtin symbol, or "" when
// the symbol is not a builtin or no confident URL pattern exists.
//
// owner is the resolved class symbol for member symbols (KindMethod,
// KindProperty, and class-constant KindConstant). Pass nil for functions,
// classes, and predefined constants.
//
// locale selects the manual locale segment. Empty string or unknown locale
// falls back to "en". Accepted: en, de, es, fr, it, ja, pt_BR, ru, tr, zh.
//
// The URL is deterministic from the symbol; we do not validate against the
// live manual.
func PHPManualURL(sym *Symbol, owner *Symbol, locale string) string {
	if sym == nil || sym.Name == "" {
		return ""
	}
	if !IsBuiltin(sym) {
		return ""
	}

	loc := "en"
	if validLocales[locale] {
		loc = locale
	}

	base := phpManualBase + loc + "/"

	switch sym.Kind {
	case KindFunction:
		slug := strings.ReplaceAll(strings.ToLower(sym.Name), "_", "-")
		return base + "function." + slug + ".php"

	case KindClass, KindInterface, KindTrait, KindEnum:
		name := classSlug(fqnOrName(sym))
		return base + "class." + name + ".php"

	case KindMethod:
		if owner == nil {
			return ""
		}
		ownerSlug := classSlug(fqnOrName(owner))
		method := strings.ToLower(sym.Name)
		method = strings.TrimPrefix(method, "__")
		return base + ownerSlug + "." + method + ".php"

	case KindProperty:
		if owner == nil {
			return ""
		}
		ownerSlug := classSlug(fqnOrName(owner))
		prop := strings.ToLower(sym.Name)
		return base + "class." + ownerSlug + ".php#" + ownerSlug + ".props." + prop

	case KindConstant:
		if owner != nil {
			// Class constant
			ownerSlug := classSlug(fqnOrName(owner))
			constName := strings.ToLower(sym.Name)
			return base + "class." + ownerSlug + ".php#" + ownerSlug + ".constants." + constName
		}
		// Predefined constant — derive extension from URI
		ext := extensionFromURI(sym.URI)
		nameSlug := strings.ReplaceAll(strings.ToLower(sym.Name), "_", "-")
		if ext != "" {
			return base + ext + ".constants.php#constant." + nameSlug
		}
		return base + "reserved.constants.php#constant." + nameSlug
	}

	return ""
}

// fqnOrName returns the FQN if non-empty, else the Name.
func fqnOrName(sym *Symbol) string {
	if sym.FQN != "" {
		return sym.FQN
	}
	return sym.Name
}

// classSlug converts a class FQN to the php.net manual slug:
// leading backslashes (global-namespace marker) are stripped first,
// then remaining backslashes are replaced with hyphens and the result is lowercased.
func classSlug(fqn string) string {
	fqn = strings.TrimLeft(fqn, "\\")
	return strings.ReplaceAll(strings.ToLower(fqn), "\\", "-")
}

// extensionFromURI extracts the extension name from a stub URI of the form
// builtin://php/extensions/<ext>/..., returning "" for core or plain "builtin" URIs.
func extensionFromURI(uri string) string {
	const prefix = "builtin://php/extensions/"
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	rest := uri[len(prefix):]
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return rest
	}
	return rest[:idx]
}
