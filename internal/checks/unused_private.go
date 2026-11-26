package checks

import (
	"fmt"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// UnusedPrivateRule detects private methods, properties and constants that are
// never referenced within their declaring class or the traits it uses, which
// can reach the class's private members as their own.
type UnusedPrivateRule struct {
	Shared *Shared
}

func (r *UnusedPrivateRule) Codes() []rule.Code {
	return []rule.Code{rule.UnusedPrivateMethod, rule.UnusedPrivateProperty, rule.UnusedPrivateConstant}
}

func (r *UnusedPrivateRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	fileTokens := sync.OnceValue(func() []parser.Token { return r.Shared.parse(source).Tokens })
	var findings []Finding
	for _, cls := range file.Classes {
		findings = append(findings, checkUnusedPrivateMembers(cls, file.Namespace, fileTokens, lines, index)...)
	}
	return findings
}

func checkUnusedPrivateMembers(cls parser.ClassNode, namespace string, fileTokens func() []parser.Token, lines []string, index *symbols.Index) []Finding {
	if !hasPrivateMembers(cls) {
		return nil
	}
	uses := newPrivateMemberUses()
	if len(cls.Traits) > 0 {
		traits, ok := traitSources(namespace, cls.Name, index)
		if !ok {
			return nil
		}
		for _, src := range traits {
			uses.collect(parser.Tokenize(src), declarationsAreUses)
		}
	}
	uses.collect(classTokens(fileTokens(), declarationStartLine(cls), endLineOf(cls, lines)), declarationsAreNotUses)
	findings := checkUnusedPrivateMethods(cls, uses, lines)
	findings = append(findings, checkUnusedPrivateProperties(cls, uses, lines)...)
	return append(findings, checkUnusedPrivateConstants(cls, uses, lines)...)
}

func hasPrivateMembers(cls parser.ClassNode) bool {
	for _, m := range cls.Methods {
		if m.Visibility == "private" {
			return true
		}
	}
	for _, p := range cls.Properties {
		if p.Visibility == "private" {
			return true
		}
	}
	for _, c := range cls.Constants {
		if c.Visibility == "private" {
			return true
		}
	}
	return false
}

// traitSources returns the source of every trait the class uses, directly or
// through other traits. ok is false when one of them isn't indexed.
func traitSources(namespace, class string, index *symbols.Index) (sources []string, ok bool) {
	if index == nil {
		return nil, false
	}
	fqn := class
	if namespace != "" {
		fqn = namespace + "\\" + class
	}
	seen := map[string]bool{}
	pending := index.TraitsOf(fqn)
	if len(pending) == 0 {
		return nil, false
	}
	for len(pending) > 0 {
		tr := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		key := strings.ToLower(tr)
		if seen[key] {
			continue
		}
		seen[key] = true
		sym := index.Lookup(tr)
		if sym == nil || sym.Kind != symbols.KindTrait {
			return nil, false
		}
		src := index.GetFileSource(sym.URI)
		if src == "" {
			return nil, false
		}
		sources = append(sources, src)
		pending = append(pending, index.TraitsOf(sym.FQN)...)
	}
	return sources, true
}

// magicMethods are methods that PHP calls implicitly — never flag these as unused.
var magicMethods = map[string]bool{
	"__construct": true, "__destruct": true, "__get": true, "__set": true,
	"__call": true, "__callStatic": true, "__isset": true, "__unset": true,
	"__toString": true, "__invoke": true, "__clone": true, "__debugInfo": true,
	"__serialize": true, "__unserialize": true, "__sleep": true, "__wakeup": true,
	"__set_state": true,
}

func checkUnusedPrivateMethods(cls parser.ClassNode, uses *privateMemberUses, lines []string) []Finding {
	var findings []Finding
	for _, m := range cls.Methods {
		if m.Visibility != "private" || magicMethods[m.Name] || uses.usesMethod(m.Name) {
			continue
		}
		findings = append(findings, Finding{
			StartLine: m.StartLine,
			StartCol:  m.StartCol,
			EndLine:   m.StartLine,
			EndCol:    len(lines[m.StartLine]),
			Severity:  rule.SeverityInfo,
			Code:      rule.UnusedPrivateMethod,
			Message:   fmt.Sprintf("Private method '%s::%s' is never used", cls.Name, m.Name),
			Tags:      []Tag{TagUnnecessary},
		})
	}
	return findings
}

func checkUnusedPrivateProperties(cls parser.ClassNode, uses *privateMemberUses, lines []string) []Finding {
	var findings []Finding
	for _, p := range cls.Properties {
		propName := strings.TrimPrefix(p.Name, "$")
		if p.Visibility != "private" || uses.usesProperty(propName) {
			continue
		}
		findings = append(findings, Finding{
			StartLine: p.StartLine,
			StartCol:  p.StartCol,
			EndLine:   p.StartLine,
			EndCol:    len(lines[p.StartLine]),
			Severity:  rule.SeverityHint,
			Code:      rule.UnusedPrivateProperty,
			Message:   fmt.Sprintf("Private property '%s::$%s' is never used", cls.Name, propName),
			Tags:      []Tag{TagUnnecessary},
		})
	}
	return findings
}

// endLineOf estimates the end line of a class from the source. It looks for
// the class EndLine if available, otherwise scans for the matching closing brace.
func endLineOf(cls parser.ClassNode, lines []string) int {
	// The parser knows where the class ends; it reads strings and comments as
	// such, so a '{' in a string can't close the class early.
	if cls.EndLine > cls.StartLine {
		if cls.EndLine < len(lines) {
			return cls.EndLine
		}
		return len(lines) - 1
	}
	// Fall back to counting braces from the class start.
	depth := 0
	for i := cls.StartLine; i < len(lines); i++ {
		for _, ch := range lines[i] {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return len(lines) - 1
}
