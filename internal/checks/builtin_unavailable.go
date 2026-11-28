package checks

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
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
	lines := strings.Split(source, "\n")
	var findings []Finding
	mask := newMaskState(source)
	for lineNum, raw := range lines {
		line := maskPHPLine(raw, &mask)
		findings = append(findings, r.checkFunctions(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesNew(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesSingleRef(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesStatic(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesCatch(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesImplements(line, lineNum, file, index)...)
		findings = append(findings, r.checkClassLikesAttribute(line, lineNum, file, index)...)
	}
	findings = append(findings, r.checkConstants(source)...)
	// Code that checks for an extension or symbol first runs only where it
	// exists, so it is fine on a profile without it.
	return dropGuarded(findings, lines, findExtensionGuards(source))
}

func (r *BuiltinUnavailableRule) checkFunctions(line string, lineNum int, file *parser.FileNode, index *symbols.Index) []Finding {
	var findings []Finding
	attrGroups := scanAttributeGroups(line)
	for _, match := range unknownFunctionCallRe.FindAllStringSubmatchIndex(line, -1) {
		start, end := match[2], match[3]
		name := line[start:end]
		// `#[\NoDiscard('…')]` is an attribute, not a call.
		if posInAttributeGroup(start, attrGroups) || shouldSkipFunctionCall(line, start, name) {
			continue
		}

		// If the function resolves to something already in the index, a
		// parallel rule (unknown-function or the project's own definition)
		// handles it. We only fire when the function is NOT in the index but
		// IS a known builtin — which means it was excluded by the profile.
		written := writtenName(line, start, name)
		if functionExists(written, file, index) {
			continue
		}

		bare, avail, ok := builtinFunctionAvailability(written, file)
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
			Severity:  rule.SeverityWarning,
			Code:      rule.BuiltinUnavailable,
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

	bare, avail, ok := builtinClassAvailability(name, lineNum, file)
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
		Severity:  rule.SeverityWarning,
		Code:      rule.BuiltinUnavailable,
		Message:   r.classMessage(bare, avail),
	}}
}

// checkConstants reports the global builtin constants the file uses that
// the project's profile lacks, read from PHP's own tokens. The newest grammar
// reads the constants in any older PHP's code the same way, with each name
// one token.
func (r *BuiltinUnavailableRule) checkConstants(source string) []Finding {
	tokens := withoutTrivia(lexer.Lex(source, token.Latest))
	lineStarts := lineStartOffsets(source)
	var findings []Finding
	for _, at := range globalConstantUses(source, tokens) {
		tok := tokens[at]
		name := strings.TrimPrefix(tok.Text(source), "\\")
		avail, ok := symbols.LookupAvailability(name)
		if !ok || r.satisfies(avail) {
			continue
		}
		line, col := offsetPosition(lineStarts, int(tok.Start))
		findings = append(findings, Finding{
			StartLine: line,
			StartCol:  col,
			EndLine:   line,
			EndCol:    col + int(tok.End-tok.Start),
			Severity:  rule.SeverityWarning,
			Code:      rule.BuiltinUnavailable,
			Message:   r.constantMessage(name, avail),
		})
	}
	return findings
}

func withoutTrivia(tokens []lexer.Token) []lexer.Token {
	out := make([]lexer.Token, 0, len(tokens))
	for _, t := range tokens {
		if !t.Kind.IsTrivia() {
			out = append(out, t)
		}
	}
	return out
}

func lineStartOffsets(source string) []int {
	starts := []int{0}
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

func offsetPosition(lineStarts []int, offset int) (int, int) {
	line := sort.Search(len(lineStarts), func(i int) bool { return lineStarts[i] > offset }) - 1
	return line, offset - lineStarts[line]
}

// globalConstantUses lists the tokens that read a global constant by name,
// skipping the text of strings but not the expressions interpolated with
// {$...} or ${...}.
func globalConstantUses(source string, tokens []lexer.Token) []int {
	var uses []int
	var braces []int
	inString := false
	for i, tok := range tokens {
		switch tok.Kind {
		case token.DoubleQuote, token.Backtick:
			if len(braces) == 0 {
				inString = !inString
			}
			continue
		case token.StartHeredoc:
			inString = true
			continue
		case token.EndHeredoc:
			inString = false
			continue
		case token.CurlyOpen, token.DollarOpenCurlyBraces:
			braces = append(braces, 0)
			continue
		case token.LeftBrace:
			if n := len(braces); n > 0 {
				braces[n-1]++
			}
			continue
		case token.RightBrace:
			if n := len(braces); n > 0 {
				if braces[n-1] == 0 {
					braces = braces[:n-1]
				} else {
					braces[n-1]--
				}
			}
			continue
		}
		if inString && len(braces) == 0 {
			continue
		}
		if isGlobalConstantUse(source, tokens, i) {
			uses = append(uses, i)
		}
	}
	return uses
}

// isGlobalConstantUse reports whether tokens[i] reads a global constant, such
// as `E_ALL` or `\PHP_EOL`: an upper-case name that isn't called, declared,
// reached through a class or an object, namespaced, a type, a label or a
// named argument.
func isGlobalConstantUse(source string, tokens []lexer.Token, i int) bool {
	name := tokens[i].Text(source)
	switch tokens[i].Kind {
	case token.Identifier:
	case token.NameFullyQualified:
		name = name[1:]
	default:
		return false
	}
	if !isConstantName(name) {
		return false
	}
	prev, next := token.Invalid, token.Invalid
	if i > 0 {
		prev = tokens[i-1].Kind
	}
	if i+1 < len(tokens) {
		next = tokens[i+1].Kind
	}
	switch next {
	case token.DoubleColon, token.LeftParen, token.Variable, token.Ellipsis, token.AmpersandFollowedByVarOrVararg:
		return false
	case token.Colon:
		switch prev {
		case token.LeftParen, token.Comma, token.Semicolon, token.LeftBrace, token.RightBrace, token.Invalid, token.OpenTag:
			return false
		}
	}
	switch prev {
	case token.ObjectOperator, token.NullsafeObjectOperator, token.DoubleColon, token.New, token.Class, token.Interface,
		token.Trait, token.Enum, token.Function, token.Const, token.Use, token.Namespace, token.Extends, token.Implements,
		token.InstanceOf, token.InsteadOf, token.As, token.Goto, token.Attribute:
		return false
	case token.Case:
		return next != token.Equals && next != token.Semicolon
	case token.Colon:
		return !(i >= 2 && tokens[i-2].Kind == token.RightParen && (next == token.LeftBrace || next == token.DoubleArrow))
	}
	return true
}

func isConstantName(name string) bool {
	if len(name) < 2 || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
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

// attributeFinding emits a builtin-unavailable finding for an attribute name
// when the name resolves to a known PHP builtin that the project's profile
// cannot use. Returns nil if the name is already resolved or is in-profile.
func (r *BuiltinUnavailableRule) attributeFinding(name string, lineNum, start, end int, file *parser.FileNode, index *symbols.Index) []Finding {
	// Before PHP 8.0, `#[...]` is a comment, so no attribute can fail there.
	// Code that targets PHP 7 adds them on purpose, for when it runs on 8.x.
	if compareBuiltinVersionsStrings(r.PHPVersion, "8.0") < 0 {
		return nil
	}
	// If the class-like is already resolved (locally defined or indexed), skip.
	if classLikeExists(name, lineNum, file, index) {
		return nil
	}

	bare, avail, ok := builtinClassAvailability(name, lineNum, file)
	if !ok {
		// Not a known builtin — UnknownClassRule handles this.
		return nil
	}

	if r.satisfies(avail) {
		return nil
	}

	// PHP's own attributes are forward-compatible: an older PHP ignores one it
	// doesn't know rather than failing. So this is a hint that it does nothing
	// yet, not a warning.
	return []Finding{{
		StartLine: lineNum,
		StartCol:  start,
		EndLine:   lineNum,
		EndCol:    end,
		Severity:  rule.SeverityHint,
		Code:      rule.BuiltinUnavailable,
		Message:   r.attributeMessage(bare, avail),
	}}
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
