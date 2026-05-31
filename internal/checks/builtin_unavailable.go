package checks

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// builtinConstantRe matches sequences of uppercase letters, digits, and
// underscores that start with an uppercase letter and are at least two
// characters long (to avoid false positives on single-letter type keywords).
// The surrounding context is checked in the calling loop to filter out class
// constant accesses (:: prefix) and function calls (trailing '(').
var builtinConstantRe = regexp.MustCompile(`\b([A-Z][A-Z0-9_]+)\b`)

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
}

// Code returns the machine-readable rule identifier.
func (r *BuiltinUnavailableRule) Code() string { return "builtin-unavailable" }

// Check runs the rule against a parsed file and returns any findings.
func (r *BuiltinUnavailableRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil || r == nil || r.PHPVersion == "" {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding
	inBlockComment := false
	for lineNum, raw := range lines {
		line := maskPHPLine(raw, &inBlockComment)
		findings = append(findings, r.checkFunctions(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesNew(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesSingleRef(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesStatic(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesCatch(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesImplements(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesAttribute(line, lineNum, file, index)...)
		findings = append(findings, r.checkConstants(line, lineNum)...)
	}
	return findings
}

func (r *BuiltinUnavailableRule) checkFunctions(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	for _, match := range unknownFunctionCallRe.FindAllStringSubmatchIndex(line, -1) {
		start, end := match[2], match[3]
		name := line[start:end]
		if shouldSkipFunctionCall(line, start, name) {
			continue
		}
		// Strip leading backslash for the lookup; partial namespaces aren't
		// builtins. If the call is namespaced (contains "\" after trim) it's
		// not a global builtin — bail.
		bare := strings.TrimPrefix(name, "\\")
		if strings.Contains(bare, "\\") {
			continue
		}

		// If the function resolves to something already in the index, a
		// parallel rule (unknown-function or the project's own definition)
		// handles it. We only fire when the function is NOT in the index but
		// IS a known builtin — which means it was excluded by the profile.
		if functionExists(name, file, index) {
			continue
		}

		avail, ok := symbols.LookupAvailability(bare)
		if !ok {
			// Not a known builtin — UnknownFunctionRule handles this.
			continue
		}

		if r.satisfies(avail) {
			// Builtin is known but within the profile's range; nothing to flag.
			continue
		}

		findings = append(findings, Finding{
			StartLine: lineNum,
			StartCol:  start,
			EndLine:   lineNum,
			EndCol:    end,
			Severity:  SeverityWarning,
			Code:      "builtin-unavailable",
			Message:   r.functionMessage(bare, avail),
		})
	}
	return findings
}

func (r *BuiltinUnavailableRule) checkClassLikesNew(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	return r.checkClassLikeMatches(line, lineNum, file, index, unknownClassNewRe)
}

func (r *BuiltinUnavailableRule) checkClassLikesSingleRef(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	return r.checkClassLikeMatches(line, lineNum, file, index, unknownClassSingleRefRe)
}

func (r *BuiltinUnavailableRule) checkClassLikesStatic(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	for _, match := range unknownClassStaticRe.FindAllStringSubmatchIndex(line, -1) {
		start, end := match[4], match[5]
		name := line[start:end]
		findings = append(findings, r.classLikeFinding(name, lineNum, start, end, file, index)...)
	}
	return findings
}

func (r *BuiltinUnavailableRule) checkClassLikesCatch(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	for _, match := range unknownClassCatchRe.FindAllStringSubmatchIndex(line, -1) {
		listStart, listEnd := match[2], match[3]
		list := line[listStart:listEnd]
		offset := 0
		for _, part := range strings.Split(list, "|") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				offset += len(part) + 1
				continue
			}
			rel := strings.Index(part, trimmed)
			start := listStart + offset + rel
			end := start + len(trimmed)
			findings = append(findings, r.classLikeFinding(trimmed, lineNum, start, end, file, index)...)
			offset += len(part) + 1
		}
	}
	return findings
}

func (r *BuiltinUnavailableRule) checkClassLikesImplements(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	for _, match := range unknownClassImplementsRe.FindAllStringSubmatchIndex(line, -1) {
		listStart, listEnd := match[2], match[3]
		list := line[listStart:listEnd]
		offset := 0
		for _, part := range strings.Split(list, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				offset += len(part) + 1
				continue
			}
			rel := strings.Index(part, trimmed)
			start := listStart + offset + rel
			end := start + len(trimmed)
			findings = append(findings, r.classLikeFinding(trimmed, lineNum, start, end, file, index)...)
			offset += len(part) + 1
		}
	}
	return findings
}

func (r *BuiltinUnavailableRule) checkClassLikesAttribute(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	for _, g := range scanAttributeGroups(line) {
		for _, tok := range g.names {
			findings = append(findings, r.attributeFinding(tok.name, lineNum, tok.start, tok.start+len(tok.name), file, index)...)
		}
	}
	return findings
}

// checkClassLikeMatches handles regexes where group 2/3 is the class name.
func (r *BuiltinUnavailableRule) checkClassLikeMatches(line string, lineNum int, file *parser.FileNode, index *symbols.Index, re *regexp.Regexp) []Finding {
	var findings []Finding
	for _, match := range re.FindAllStringSubmatchIndex(line, -1) {
		start, end := match[2], match[3]
		name := line[start:end]
		findings = append(findings, r.classLikeFinding(name, lineNum, start, end, file, index)...)
	}
	return findings
}

// classLikeFinding emits a builtin-unavailable finding for a class-like name
// when the name resolves to a known PHP builtin that the project's profile
// cannot use. Returns nil if the name is already resolved or is in-profile.
func (r *BuiltinUnavailableRule) classLikeFinding(name string, lineNum, start, end int, file *parser.FileNode, index *symbols.Index) []Finding {
	// If the class-like is already resolved (locally defined or indexed), skip.
	// classLikeExists returns true both for locally-defined types and for
	// types present in the index (which includes builtins that passed the profile).
	if classLikeExists(name, lineNum, file, index) {
		return nil
	}

	bare := strings.TrimPrefix(name, "\\")
	if strings.Contains(bare, "\\") {
		// Namespaced reference — not a global builtin.
		return nil
	}

	avail, ok := symbols.LookupAvailability(bare)
	if !ok {
		// Not a known builtin — UnknownClassRule handles this.
		return nil
	}

	if r.satisfies(avail) {
		return nil
	}

	return []Finding{{
		StartLine: lineNum,
		StartCol:  start,
		EndLine:   lineNum,
		EndCol:    end,
		Severity:  SeverityWarning,
		Code:      "builtin-unavailable",
		Message:   r.classMessage(bare, avail),
	}}
}

// checkConstants scans a masked line for global built-in constant references
// and emits findings for any that require a newer PHP version than the profile.
// It uses builtinConstantRe to find candidate all-uppercase identifiers and
// then applies context checks to exclude:
//   - class constant accesses: Foo::BAR  (preceded by '::')
//   - function calls: NAME(  (followed by '(')
//   - const declarations: `const NAME` or `define(` contexts are already masked
//     by the string/comment masking layer, but leading `const ` is checked too.
func (r *BuiltinUnavailableRule) checkConstants(line string, lineNum int) []Finding {
	var findings []Finding
	for _, match := range builtinConstantRe.FindAllStringSubmatchIndex(line, -1) {
		start, end := match[2], match[3]
		name := line[start:end]

		// Skip if preceded by '::' — this is a class constant (Foo::CONST).
		if start >= 2 && line[start-2:start] == "::" {
			continue
		}
		// A single ':' before means it's part of a ternary or label, not '::'.
		// Skip '->CONST' — a property or method named with all-caps (unusual but valid).
		if start >= 2 && line[start-2:start] == "->" {
			continue
		}

		// Skip if followed by '(' — this is a function call, not a constant.
		after := end
		for after < len(line) && line[after] == ' ' {
			after++
		}
		if after < len(line) && line[after] == '(' {
			continue
		}

		// Skip leading `const NAME` declarations (class/namespace constants).
		prefix := strings.TrimSpace(line[:start])
		if strings.HasSuffix(prefix, "const") {
			continue
		}

		// Strip a fully-qualified leading backslash (\JSON_THROW_ON_ERROR).
		// builtinConstantRe won't include the backslash; the backslash sits one
		// character before start. We look it up bare regardless.
		avail, ok := symbols.LookupAvailability(name)
		if !ok {
			continue
		}

		if r.satisfies(avail) {
			continue
		}

		findings = append(findings, Finding{
			StartLine: lineNum,
			StartCol:  start,
			EndLine:   lineNum,
			EndCol:    end,
			Severity:  SeverityWarning,
			Code:      "builtin-unavailable",
			Message:   r.constantMessage(name, avail),
		})
	}
	return findings
}

func (r *BuiltinUnavailableRule) constantMessage(name string, avail symbols.BuiltinAvailability) string {
	if avail.Extension != "" {
		return fmt.Sprintf("Constant '%s' requires ext-%s (PHP >= %s)", name, avail.Extension, avail.Since)
	}
	return fmt.Sprintf("Constant '%s' requires PHP >= %s", name, avail.Since)
}

// satisfies reports whether the rule's profile satisfies the given availability
// constraint. Returns true if the builtin can be used under the current profile.
func (r *BuiltinUnavailableRule) satisfies(avail symbols.BuiltinAvailability) bool {
	if avail.Since != "" && compareBuiltinVersionsStrings(avail.Since, r.PHPVersion) > 0 {
		return false
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
		return fmt.Sprintf("Function '%s' requires ext-%s (PHP >= %s)", name, avail.Extension, avail.Since)
	}
	return fmt.Sprintf("Function '%s' requires PHP >= %s", name, avail.Since)
}

func (r *BuiltinUnavailableRule) classMessage(name string, avail symbols.BuiltinAvailability) string {
	if avail.Extension != "" {
		return fmt.Sprintf("Class '%s' requires ext-%s (PHP >= %s)", name, avail.Extension, avail.Since)
	}
	return fmt.Sprintf("Class '%s' requires PHP >= %s", name, avail.Since)
}

// attributeFinding emits a builtin-unavailable finding for an attribute name
// when the name resolves to a known PHP builtin that the project's profile
// cannot use. Returns nil if the name is already resolved or is in-profile.
func (r *BuiltinUnavailableRule) attributeFinding(name string, lineNum, start, end int, file *parser.FileNode, index *symbols.Index) []Finding {
	// If the class-like is already resolved (locally defined or indexed), skip.
	if classLikeExists(name, lineNum, file, index) {
		return nil
	}

	bare := strings.TrimPrefix(name, "\\")
	if strings.Contains(bare, "\\") {
		// Namespaced reference — not a global builtin.
		return nil
	}

	avail, ok := symbols.LookupAvailability(bare)
	if !ok {
		// Not a known builtin — UnknownClassRule handles this.
		return nil
	}

	if r.satisfies(avail) {
		return nil
	}

	return []Finding{{
		StartLine: lineNum,
		StartCol:  start,
		EndLine:   lineNum,
		EndCol:    end,
		Severity:  SeverityWarning,
		Code:      "builtin-unavailable",
		Message:   r.attributeMessage(bare, avail),
	}}
}

func (r *BuiltinUnavailableRule) attributeMessage(name string, avail symbols.BuiltinAvailability) string {
	if avail.Extension != "" {
		return fmt.Sprintf("Attribute '%s' requires ext-%s (PHP >= %s)", name, avail.Extension, avail.Since)
	}
	return fmt.Sprintf("Attribute '%s' requires PHP >= %s", name, avail.Since)
}
