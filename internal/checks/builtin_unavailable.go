package checks

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// BuiltinUnavailableRule reports usages of PHP builtins that are not
// available in the project's resolved PHP version / extension profile.
// It is complementary to UnknownFunctionRule and UnknownClassRule: those
// fire when no record exists of the symbol; this fires when the symbol is
// a known PHP builtin but the project's profile cannot use it.
//
// The rule is intentionally NOT soft-moded during indexing warm-up — its
// decisions depend on the static availability table (symbols.LookupAvailability)
// and the resolved PHP profile, both of which are stable from initialize onward.
type BuiltinUnavailableRule struct {
	// PHPVersion is the project's resolved PHP major.minor target, e.g. "7.4".
	PHPVersion string
	// Extensions is the set of project-enabled PHP extensions, lower-cased,
	// without the "ext-" prefix (e.g. {"json", "mbstring"}).
	Extensions []string

	Shared *Shared

	// versionChecked is set, on the copy Check works with, when the file
	// checks PHP's version: such code is written for several versions, so a
	// builtin from a newer one is assumed to be behind that check.
	versionChecked bool
}

// Code returns the machine-readable rule identifier.
func (r *BuiltinUnavailableRule) Codes() []rule.Code { return []rule.Code{rule.BuiltinUnavailable} }

// Check runs the rule against a parsed file and returns any findings.
func (r *BuiltinUnavailableRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || r == nil || r.PHPVersion == "" {
		return nil
	}
	copied := *r
	copied.versionChecked = checksPHPVersion(source)
	r = &copied
	syntax := r.Shared.tree(source, file.Grammar)
	refs := syntax.references()
	var findings []Finding
	for _, call := range refs.calls {
		findings = append(findings, r.functionFinding(syntax, call, file, index)...)
	}
	for _, ref := range refs.classes {
		name, line := syntax.text(ref.name), syntax.line(ref.name)
		if ref.attribute {
			findings = append(findings, r.attributeFinding(syntax, ref.name, name, line, file, index)...)
		} else {
			findings = append(findings, r.classLikeFinding(syntax, ref.name, name, line, file, index)...)
		}
	}
	for _, constant := range refs.constants {
		findings = append(findings, r.constantFinding(syntax, constant)...)
	}
	slices.SortStableFunc(findings, compareFindingStarts)
	// Code that checks for an extension or symbol first runs only where it
	// exists, so it is fine on a profile without it.
	return dropGuarded(findings, strings.Split(source, "\n"), findExtensionGuards(source))
}

func (r *BuiltinUnavailableRule) functionFinding(syntax *fileSyntax, call *ast.Name, file *parser.FileNode, index *symbols.Index) []Finding {
	written := syntax.text(call)
	if functionExists(written, file, index) {
		return nil
	}
	bare, avail, ok := builtinFunctionAvailability(written, file)
	if !ok || r.satisfies(avail) {
		return nil
	}
	return []Finding{syntax.at(call, Finding{
		Severity: rule.SeverityWarning,
		Code:     rule.BuiltinUnavailable,
		Message:  r.functionMessage(bare, avail),
	})}
}

// classLikeFinding reports a class-like name that resolves to a PHP builtin
// the profile can't use. A name the index has, or one that isn't a builtin,
// is left to unknown-class.
func (r *BuiltinUnavailableRule) classLikeFinding(syntax *fileSyntax, node *ast.Name, name string, line int, file *parser.FileNode, index *symbols.Index) []Finding {
	if classLikeExists(name, line, file, index) {
		return nil
	}
	bare, avail, ok := builtinClassAvailability(name, line, file)
	if !ok || r.satisfies(avail) {
		return nil
	}
	return []Finding{syntax.at(node, Finding{
		Severity: rule.SeverityWarning,
		Code:     rule.BuiltinUnavailable,
		Message:  r.classMessage(bare, avail),
	})}
}

func (r *BuiltinUnavailableRule) constantFinding(syntax *fileSyntax, node *ast.Name) []Finding {
	name := strings.TrimPrefix(syntax.text(node), "\\")
	if strings.Contains(name, "\\") {
		return nil
	}
	avail, ok := symbols.LookupAvailability(name)
	if !ok || r.satisfies(avail) {
		return nil
	}
	return []Finding{syntax.at(node, Finding{
		Severity: rule.SeverityWarning,
		Code:     rule.BuiltinUnavailable,
		Message:  r.constantMessage(name, avail),
	})}
}

func (r *BuiltinUnavailableRule) constantMessage(name string, avail symbols.BuiltinAvailability) string {
	if avail.Extension != "" {
		return fmt.Sprintf("Constant '%s' requires ext-%s%s", name, avail.Extension, sinceSuffix(avail))
	}
	return fmt.Sprintf("Constant '%s' requires PHP >= %s", name, avail.Since)
}

// satisfies reports whether the rule's profile satisfies the given availability
// constraint. Returns true if the builtin can be used under the current profile.
func (r *BuiltinUnavailableRule) satisfies(avail symbols.BuiltinAvailability) bool {
	if avail.Since != "" && compareBuiltinVersionsStrings(avail.Since, r.PHPVersion) > 0 {
		return r.versionChecked && avail.Extension == ""
	}
	if avail.Extension != "" {
		for _, e := range r.Extensions {
			if e == avail.Extension {
				return true
			}
		}
		return false
	}
	return true
}

// compareBuiltinVersionsStrings compares two "major.minor" version strings.
// Returns -1, 0, or 1 (a < b, a == b, a > b).
func compareBuiltinVersionsStrings(a, b string) int {
	ma, mi := parseMajorMinor(a)
	mb, mibb := parseMajorMinor(b)
	if ma != mb {
		if ma < mb {
			return -1
		}
		return 1
	}
	if mi != mibb {
		if mi < mibb {
			return -1
		}
		return 1
	}
	return 0
}

func parseMajorMinor(v string) (int, int) {
	parts := strings.SplitN(v, ".", 2)
	var maj, min int
	if len(parts) > 0 {
		fmt.Sscanf(parts[0], "%d", &maj)
	}
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &min)
	}
	return maj, min
}

func (r *BuiltinUnavailableRule) functionMessage(name string, avail symbols.BuiltinAvailability) string {
	if avail.Extension != "" {
		return fmt.Sprintf("Function '%s' requires ext-%s%s", name, avail.Extension, sinceSuffix(avail))
	}
	return fmt.Sprintf("Function '%s' requires PHP >= %s", name, avail.Since)
}

func (r *BuiltinUnavailableRule) classMessage(name string, avail symbols.BuiltinAvailability) string {
	if avail.Extension != "" {
		return fmt.Sprintf("Class '%s' requires ext-%s%s", name, avail.Extension, sinceSuffix(avail))
	}
	return fmt.Sprintf("Class '%s' requires PHP >= %s", name, avail.Since)
}

// attributeFinding reports one of PHP's own attributes that has no effect
// on the profile yet. PHP ignores an attribute it doesn't know, so this is a
// hint, not a warning; before PHP 8.0 `#[...]` is a comment, so nothing is.
func (r *BuiltinUnavailableRule) attributeFinding(syntax *fileSyntax, node *ast.Name, name string, line int, file *parser.FileNode, index *symbols.Index) []Finding {
	if compareBuiltinVersionsStrings(r.PHPVersion, "8.0") < 0 || classLikeExists(name, line, file, index) {
		return nil
	}
	bare, avail, ok := builtinClassAvailability(name, line, file)
	if !ok || r.satisfies(avail) {
		return nil
	}
	return []Finding{syntax.at(node, Finding{
		Severity: rule.SeverityHint,
		Code:     rule.BuiltinUnavailable,
		Message:  r.attributeMessage(bare, avail),
	})}
}

func (r *BuiltinUnavailableRule) attributeMessage(name string, avail symbols.BuiltinAvailability) string {
	if avail.Extension != "" {
		return fmt.Sprintf("Attribute '%s' has no effect without ext-%s%s; it is ignored until then", name, avail.Extension, sinceSuffix(avail))
	}
	return fmt.Sprintf("Attribute '%s' has no effect before PHP >= %s; PHP %s ignores it", name, avail.Since, r.PHPVersion)
}

// builtinFunctionAvailability finds the PHP builtin a function call names,
// with its version and extension constraints: the name it resolves to, as
// PHP looks it up (the namespaced name, then the global one). ok is false
// when it isn't a builtin. Builtin function names are matched in lowercase,
// as the registry lists them.
func builtinFunctionAvailability(name string, file *parser.FileNode) (string, symbols.BuiltinAvailability, bool) {
	fqns, ok := resolve.FunctionNames(name, file)
	if !ok {
		return "", symbols.BuiltinAvailability{}, false
	}
	for _, fqn := range fqns {
		if avail, ok := symbols.LookupAvailability(fqn); ok {
			return fqn, avail, true
		}
		if avail, ok := symbols.LookupAvailability(strings.ToLower(fqn)); ok {
			return fqn, avail, true
		}
	}
	return "", symbols.BuiltinAvailability{}, false
}

// builtinClassAvailability is builtinFunctionAvailability for a class-like
// name, resolved through the file's namespace and imports.
func builtinClassAvailability(name string, line int, file *parser.FileNode) (string, symbols.BuiltinAvailability, bool) {
	fqn, ok := resolveClassLikeName(name, line, file)
	if !ok || fqn == "" {
		return "", symbols.BuiltinAvailability{}, false
	}
	avail, ok := symbols.LookupAvailability(fqn)
	return fqn, avail, ok
}

// checksPHPVersion reports whether source checks the running PHP's version,
// as in `if (\PHP_VERSION_ID < 80500) { return …; }`. It reads text, not
// tokens: it only ever makes the rule quieter.
func checksPHPVersion(source string) bool {
	return strings.Contains(source, "PHP_VERSION") || strings.Contains(source, "PHP_MAJOR_VERSION") ||
		strings.Contains(source, "PHP_MINOR_VERSION") || strings.Contains(source, "phpversion(")
}

// sinceSuffix is " (PHP >= X.Y)" for an extension builtin that also needs a
// PHP version, and nothing for one that doesn't.
func sinceSuffix(avail symbols.BuiltinAvailability) string {
	if avail.Since == "" {
		return ""
	}
	return " (PHP >= " + avail.Since + ")"
}
