package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// DeprecatedUsageRule flags uses of what's marked with PHP 8.4's #[\Deprecated].
type DeprecatedUsageRule struct {
	TypeResolver func(expr string, source string, line, col int, file *parser.FileNode) string
	// Uses, when set, shares the attribute uses found in a file with the
	// other rules run over it.
	Uses *MemberUseCache
}

func (r *DeprecatedUsageRule) Code() string { return "deprecated" }

func (r *DeprecatedUsageRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	var findings []Finding
	for _, use := range r.Uses.get(file, source, index, r.TypeResolver) {
		attr := use.symbol.Attribute("Deprecated")
		if attr == nil {
			continue
		}
		message := fmt.Sprintf("'%s' is deprecated", use.symbol.Name)
		if reason := attributeMessage(attr); reason != "" {
			message += ": " + reason
		}
		findings = append(findings, use.finding("deprecated", message, []Tag{TagDeprecated}))
	}
	return findings
}

// UnusedResultRule flags calls to what's marked with PHP 8.5's #[\NoDiscard]
// whose result is thrown away. A `(void)` cast is the way to do that on purpose.
type UnusedResultRule struct {
	TypeResolver func(expr string, source string, line, col int, file *parser.FileNode) string
	// Uses, when set, shares the attribute uses found in a file with the
	// other rules run over it.
	Uses *MemberUseCache
}

func (r *UnusedResultRule) Code() string { return "unused-result" }

func (r *UnusedResultRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	var findings []Finding
	for _, use := range r.Uses.get(file, source, index, r.TypeResolver) {
		attr := use.symbol.Attribute("NoDiscard")
		if attr == nil || !use.isCall || !use.isStatement {
			continue
		}
		message := fmt.Sprintf("The result of '%s' should be used", use.symbol.Name)
		if reason := attributeMessage(attr); reason != "" {
			message += ": " + reason
		}
		findings = append(findings, use.finding("unused-result", message, nil))
	}
	return findings
}

// InvalidOverrideRule flags methods with PHP 8.3's #[\Override] that override
// nothing, which PHP refuses to compile. It stays quiet when an ancestor isn't
// indexed, since the method could be overriding something there.
type InvalidOverrideRule struct{}

func (r *InvalidOverrideRule) Code() string { return "invalid-override" }

func (r *InvalidOverrideRule) Check(file *parser.FileNode, _ string, index *symbols.Index) []Finding {
	if file == nil || index == nil {
		return nil
	}
	var findings []Finding
	check := func(name string, methods []parser.MethodNode) {
		fqn := name
		if file.Namespace != "" {
			fqn = file.Namespace + "\\" + name
		}
		for _, m := range methods {
			if !hasOverrideAttribute(m.Attributes, file) || strings.EqualFold(m.Name, "__toString") {
				continue
			}
			if overrides, known := index.OverridesInherited(fqn, m.Name); known && !overrides {
				findings = append(findings, Finding{
					StartLine: m.StartLine,
					StartCol:  m.StartCol,
					EndLine:   m.StartLine,
					EndCol:    m.StartCol + len(m.Name),
					Severity:  SeverityError,
					Code:      "invalid-override",
					Message:   fmt.Sprintf("'%s' has #[\\Override], but there's no such method to override", m.Name),
				})
			}
		}
	}
	for _, c := range file.Classes {
		check(c.Name, c.Methods)
	}
	for _, e := range file.Enums {
		check(e.Name, e.Methods)
	}
	return findings
}

func hasOverrideAttribute(attrs []parser.AttributeDef, file *parser.FileNode) bool {
	for _, attr := range attrs {
		if attr.Name == "\\Override" || (attr.Name == "Override" && file.Namespace == "") {
			return true
		}
		for _, use := range file.Uses {
			if use.FullName == "Override" && strings.EqualFold(use.Alias, attr.Name) {
				return true
			}
		}
	}
	return false
}

// attributeMessage returns the `message` argument of #[\Deprecated] or
// #[\NoDiscard], which is the first one unless it's passed by name.
func attributeMessage(attr *symbols.Attribute) string {
	for i, arg := range attr.Args {
		if value, ok := strings.CutPrefix(arg, "message:"); ok {
			return strings.Trim(strings.TrimSpace(value), `'"`)
		}
		if i == 0 && !strings.Contains(arg, ":") {
			return strings.Trim(arg, `'"`)
		}
	}
	return ""
}

// memberUse is a function, method or constant used in the source.
type memberUse struct {
	symbol      *symbols.Symbol
	token       parser.Token
	isCall      bool
	isStatement bool // nothing takes its result
}

// MemberUseCache holds the attribute uses found in one file, so the rules
// that read them (deprecated, unused-result) find them once. A nil cache
// finds them every time.
type MemberUseCache struct {
	// Shared, when set, shares the file's parse with the other rules.
	Shared *Shared
	source string
	file   *parser.FileNode
	uses   []memberUse
	done   bool
}

func (c *MemberUseCache) get(file *parser.FileNode, source string, index *symbols.Index, typeResolver func(string, string, int, int, *parser.FileNode) string) []memberUse {
	if c == nil {
		return memberUses(file, source, index, typeResolver, nil)
	}
	if !c.done || c.source != source || c.file != file {
		c.source, c.file, c.done = source, file, true
		c.uses = memberUses(file, source, index, typeResolver, c.Shared)
	}
	return c.uses
}

func (u memberUse) finding(code, message string, tags []Tag) Finding {
	return Finding{
		StartLine: u.token.Line,
		StartCol:  u.token.Column,
		EndLine:   u.token.Line,
		EndCol:    u.token.Column + len(u.token.Value),
		Severity:  SeverityWarning,
		Code:      code,
		Message:   message,
		Tags:      tags,
	}
}

func memberUses(file *parser.FileNode, source string, index *symbols.Index, typeResolver func(string, string, int, int, *parser.FileNode) string, shared *Shared) []memberUse {
	if file == nil || index == nil {
		return nil
	}
	// Most files use no attributes at all, so don't resolve anything for them.
	if !index.HasAttributedSymbols() {
		return nil
	}
	result := shared.parse(source)
	if result == nil {
		return nil
	}
	tokens := make([]parser.Token, 0, len(result.Tokens))
	for _, tok := range result.Tokens {
		switch tok.Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
		default:
			tokens = append(tokens, tok)
		}
	}
	lines := strings.Split(source, "\n")

	var uses []memberUse
	for i, tok := range tokens {
		if tok.Kind != parser.TokenIdentifier {
			continue
		}
		isCall := i+1 < len(tokens) && tokens[i+1].Kind == parser.TokenOpenParen
		var sym *symbols.Symbol
		prev := parser.TokenEOF
		if i > 0 {
			prev = tokens[i-1].Kind
		}
		switch {
		case prev == parser.TokenArrow || prev == parser.TokenDoubleColon:
			if typeResolver == nil || tok.Line >= len(lines) {
				continue
			}
			var mask maskState
			line := maskPHPLine(lines[tok.Line], &mask)
			opStart := tokens[i-1].Column
			if i > 1 && tokens[i-2].Kind == parser.TokenQuestion {
				opStart = tokens[i-2].Column
			}
			if opStart > len(line) {
				continue
			}
			ownerExpr := strings.TrimSpace(line[backwardExprStart(line, opStart):opStart])
			if ownerExpr == "" {
				continue
			}
			ownerFQN := resolveMemberOwner(ownerExpr, tok.Line, backwardExprStart(line, opStart), source, file, index, typeResolver)
			if ownerFQN == "" {
				continue
			}
			for _, member := range index.GetClassMembers(ownerFQN) {
				if member.Name == tok.Value && member.Kind != symbols.KindProperty {
					sym = member
					break
				}
			}
		case isCall && i > 0 && (tokens[i-1].Kind == parser.TokenFunction || strings.EqualFold(tokens[i-1].Value, "new")):
			continue
		case isCall:
			sym = lookupCalledFunction(tok.Value, file, index)
		}
		if sym == nil || len(sym.Attributes) == 0 {
			continue
		}
		uses = append(uses, memberUse{symbol: sym, token: tok, isCall: isCall, isStatement: isCall && isExpressionStatement(tokens, i)})
	}
	return uses
}

// resolveMemberOwner returns the class-like a member is accessed on.
func resolveMemberOwner(ownerExpr string, line, col int, source string, file *parser.FileNode, index *symbols.Index, typeResolver func(string, string, int, int, *parser.FileNode) string) string {
	if !strings.HasPrefix(ownerExpr, "$") && !strings.HasSuffix(ownerExpr, ")") && !strings.HasSuffix(ownerExpr, "]") {
		if fqn, ok := resolveClassLikeName(ownerExpr, line, file); ok && isClassLikeSymbol(index.Lookup(fqn)) {
			return fqn
		}
	}
	if owners := resolvedClassLikes(typeResolver(ownerExpr, source, line, col, file), file, index); len(owners) > 0 {
		return owners[0]
	}
	return ""
}

func lookupCalledFunction(name string, file *parser.FileNode, index *symbols.Index) *symbols.Symbol {
	candidates := []string{strings.TrimPrefix(name, "\\")}
	if !strings.HasPrefix(name, "\\") && file.Namespace != "" {
		candidates = append([]string{file.Namespace + "\\" + name}, candidates...)
	}
	for _, use := range file.Uses {
		if use.Kind == "function" && strings.EqualFold(use.Alias, name) {
			candidates = append([]string{use.FullName}, candidates...)
		}
	}
	for _, fqn := range candidates {
		if sym := index.Lookup(fqn); sym != nil && sym.Kind == symbols.KindFunction {
			return sym
		}
	}
	return nil
}

// isExpressionStatement reports whether the call named at i is a whole statement:
// `foo();`, `$a->b()->foo();`, but not `return foo();` or `(void) foo();`.
func isExpressionStatement(tokens []parser.Token, i int) bool {
	close := matchingParen(tokens, i+1)
	if close < 0 || close+1 >= len(tokens) || tokens[close+1].Kind != parser.TokenSemicolon {
		return false
	}
	start := i
	for start > 1 {
		k := tokens[start-1].Kind
		if k != parser.TokenArrow && k != parser.TokenDoubleColon {
			break
		}
		start -= 2
		if start >= 0 && tokens[start].Kind == parser.TokenQuestion {
			start--
		}
		// Step over `(...)` to the name it follows, as in `new Foo()->bar()`.
		if start >= 0 && tokens[start].Kind == parser.TokenCloseParen {
			depth := 0
			for ; start >= 0; start-- {
				if tokens[start].Kind == parser.TokenCloseParen {
					depth++
				} else if tokens[start].Kind == parser.TokenOpenParen {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			start--
		}
		if start < 0 {
			return false
		}
	}
	if start > 0 && strings.EqualFold(tokens[start-1].Value, "new") {
		start--
	}
	if start == 0 {
		return true
	}
	switch tokens[start-1].Kind {
	case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace:
		return true
	}
	return false
}

func matchingParen(tokens []parser.Token, open int) int {
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen:
			depth++
		case parser.TokenCloseParen:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
