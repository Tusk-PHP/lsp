package checks

import (
	"regexp"
	"strings"
)

// extensionGuardRe finds the checks code makes before using something that may
// not be installed: extension_loaded('imagick'), class_exists('Imagick'),
// function_exists('ssh2_connect') and their interface/trait/enum variants.
var extensionGuardRe = regexp.MustCompile(`(?i)\b(extension_loaded|class_exists|interface_exists|trait_exists|enum_exists|function_exists)\s*\(\s*['"]\\?([A-Za-z_][A-Za-z0-9_\\]*)['"]`)

// extensionGuards holds what a file checks for before using it. Code guarded
// that way runs only where the extension or symbol exists, so a name it
// covers isn't reported as unknown even when this project's profile lacks it.
type extensionGuards struct {
	extensions []string        // lowercase extension names from extension_loaded()
	names      map[string]bool // lowercase class and function names from *_exists()
}

// findExtensionGuards scans a whole file once. A guard anywhere in the file
// counts for all of it: WordPress checks extension_loaded('imagick') in one
// method and uses Imagick in every other method of the class.
func findExtensionGuards(source string) extensionGuards {
	var g extensionGuards
	if !strings.Contains(source, "_exists") && !strings.Contains(source, "extension_loaded") {
		return g
	}
	for _, m := range extensionGuardRe.FindAllStringSubmatch(source, -1) {
		name := strings.ToLower(m[2])
		if strings.EqualFold(m[1], "extension_loaded") {
			g.extensions = append(g.extensions, name)
			continue
		}
		if g.names == nil {
			g.names = map[string]bool{}
		}
		g.names[name] = true
	}
	return g
}

// covers reports whether a class or function name is guarded in this file:
// named by a *_exists() check, or belonging to an extension checked with
// extension_loaded(). Extensions name their symbols after themselves, so
// `imagick` covers Imagick and ImagickPixel, and `ssh2` covers ssh2_connect.
func (g extensionGuards) covers(name string) bool {
	name = strings.TrimPrefix(name, "\\")
	if name == "" || (len(g.extensions) == 0 && len(g.names) == 0) {
		return false
	}
	lower := strings.ToLower(name)
	if g.names[lower] {
		return true
	}
	for _, ext := range g.extensions {
		if !strings.HasPrefix(lower, ext) {
			continue
		}
		// The extension's name must end at a word boundary in the symbol:
		// the end, `_` (ssh2_connect) or a capital (ImagickPixel). So `gd`
		// covers gd_info but not a project class named Gdpr.
		if len(name) == len(ext) {
			return true
		}
		next := name[len(ext)]
		if next == '_' || (next >= 'A' && next <= 'Z') {
			return true
		}
	}
	return false
}

// dropGuarded removes findings whose name (the text the finding spans) the
// file guards.
func dropGuarded(findings []Finding, lines []string, g extensionGuards) []Finding {
	if len(g.extensions) == 0 && len(g.names) == 0 {
		return findings
	}
	kept := findings[:0]
	for _, f := range findings {
		if f.StartLine == f.EndLine && f.StartLine < len(lines) {
			line := lines[f.StartLine]
			if f.StartCol >= 0 && f.EndCol <= len(line) && f.StartCol < f.EndCol && g.covers(line[f.StartCol:f.EndCol]) {
				continue
			}
		}
		kept = append(kept, f)
	}
	return kept
}
