package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

// extensionGuards holds what a file checks for before using it. Code guarded
// that way runs only where the extension or symbol exists, so a name it
// covers isn't reported as unknown even when this project's profile lacks it.
type extensionGuards struct {
	extensions []string        // lowercase extension names from extension_loaded()
	names      map[string]bool // lowercase class and function names from *_exists()
}

// extensionGuards reads the file's guards once. A guard anywhere in the file
// counts for all of it: WordPress checks extension_loaded('imagick') in one
// method and uses Imagick in every other method of the class.
func (f *fileSyntax) extensionGuards() extensionGuards {
	var g extensionGuards
	for _, call := range f.references().calls {
		fn := strings.ToLower(strings.TrimPrefix(f.text(call.name), "\\"))
		if !extensionGuardFuncs[fn] || len(call.args.Args) == 0 {
			continue
		}
		name, ok := f.guardedName(call.args.Args[0].Value, fn != "extension_loaded")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimPrefix(name, "\\"))
		if fn == "extension_loaded" {
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

var extensionGuardFuncs = map[string]bool{
	"extension_loaded": true, "class_exists": true, "interface_exists": true,
	"trait_exists": true, "enum_exists": true, "function_exists": true,
}

func (f *fileSyntax) guardedName(arg ast.Expr, classConst bool) (string, bool) {
	if name, ok := f.stringLiteral(arg); ok {
		return name, name != ""
	}
	fetch, ok := arg.(*ast.ClassConstFetch)
	if !classConst || !ok || !strings.EqualFold(f.text(fetch.Name), "class") {
		return "", false
	}
	class, ok := fetch.Class.(*ast.ConstFetch)
	if !ok || class.Name == nil {
		return "", false
	}
	return f.text(class.Name), true
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
