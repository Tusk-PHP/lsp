package checks

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// UnusedPrivateRule detects private methods and properties that are never
// referenced within their declaring class or the traits it uses, which can
// call the class's private members as their own.
type UnusedPrivateRule struct{}

func (r *UnusedPrivateRule) Code() string { return "unused-private" }

func (r *UnusedPrivateRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding

	for _, cls := range file.Classes {
		var traits []string
		if len(cls.Traits) > 0 {
			var ok bool
			if traits, ok = traitSources(file.Namespace, cls.Name, index); !ok {
				// A trait the server can't read might use any of them.
				continue
			}
		}
		findings = append(findings, checkUnusedPrivateMethods(cls, lines, traits)...)
		findings = append(findings, checkUnusedPrivateProperties(cls, lines, traits)...)
	}
	return findings
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

func anySource(sources []string, name string, uses func(src, name string) bool) bool {
	for _, src := range sources {
		if uses(src, name) {
			return true
		}
	}
	return false
}

// sourceUsesMethod reports whether a trait calls or declares the method; an
// abstract private method in a trait is the class's to implement.
func sourceUsesMethod(src, name string) bool {
	return strings.Contains(src, "->"+name) || strings.Contains(src, "::"+name) ||
		strings.Contains(src, "function "+name) ||
		strings.Contains(src, "'"+name+"'") || strings.Contains(src, "\""+name+"\"")
}

// sourceUsesProperty reports whether a trait reads or writes the property.
func sourceUsesProperty(src, name string) bool {
	if strings.Contains(src, "::$"+name) {
		return true
	}
	pattern := "->" + name
	for rest := src; ; {
		i := strings.Index(rest, pattern)
		if i < 0 {
			return false
		}
		after := i + len(pattern)
		if after >= len(rest) || !isIdentChar(rest[after]) {
			return true
		}
		rest = rest[after:]
	}
}

// magicMethods are methods that PHP calls implicitly — never flag these as unused.
var magicMethods = map[string]bool{
	"__construct": true, "__destruct": true, "__get": true, "__set": true,
	"__call": true, "__callStatic": true, "__isset": true, "__unset": true,
	"__toString": true, "__invoke": true, "__clone": true, "__debugInfo": true,
	"__serialize": true, "__unserialize": true, "__sleep": true, "__wakeup": true,
	"__set_state": true,
}

// traits are the sources of the traits the class uses.
func checkUnusedPrivateMethods(cls parser.ClassNode, lines, traits []string) []Finding {
	var findings []Finding
	for _, m := range cls.Methods {
		if m.Visibility != "private" {
			continue
		}
		if magicMethods[m.Name] {
			continue
		}
		if isMethodReferenced(m.Name, m.StartLine, cls, lines) || anySource(traits, m.Name, sourceUsesMethod) {
			continue
		}
		endCol := 0
		if m.StartLine >= 0 && m.StartLine < len(lines) {
			endCol = len(lines[m.StartLine])
		}
		findings = append(findings, Finding{
			StartLine: m.StartLine,
			StartCol:  m.StartCol,
			EndLine:   m.StartLine,
			EndCol:    endCol,
			Severity:  SeverityInfo,
			Code:      "unused-private-method",
			Message:   fmt.Sprintf("Private method '%s::%s' is never used", cls.Name, m.Name),
			Tags:      []Tag{TagUnnecessary},
		})
	}
	return findings
}

func checkUnusedPrivateProperties(cls parser.ClassNode, lines, traits []string) []Finding {
	var findings []Finding
	for _, p := range cls.Properties {
		if p.Visibility != "private" {
			continue
		}
		propName := strings.TrimPrefix(p.Name, "$")
		if isPropertyReferenced(propName, p.StartLine, cls, lines) || anySource(traits, propName, sourceUsesProperty) {
			continue
		}
		endCol := 0
		if p.StartLine >= 0 && p.StartLine < len(lines) {
			endCol = len(lines[p.StartLine])
		}
		findings = append(findings, Finding{
			StartLine: p.StartLine,
			StartCol:  p.StartCol,
			EndLine:   p.StartLine,
			EndCol:    endCol,
			Severity:  SeverityHint,
			Code:      "unused-private-property",
			Message:   fmt.Sprintf("Private property '%s::$%s' is never used", cls.Name, propName),
			Tags:      []Tag{TagUnnecessary},
		})
	}
	return findings
}

// isMethodReferenced checks if a private method name is referenced anywhere
// in the class body outside its declaration line.
func isMethodReferenced(name string, declLine int, cls parser.ClassNode, lines []string) bool {
	start := cls.StartLine
	end := endLineOf(cls, lines)

	// Patterns: ->methodName(  self::methodName(  static::methodName(
	// Also callable array: [$this, 'methodName']  [self::class, 'methodName']
	for i := start; i <= end && i < len(lines); i++ {
		if i == declLine {
			continue
		}
		line := lines[i]
		if strings.Contains(line, "->"+name) || strings.Contains(line, "::"+name) {
			return true
		}
		// Callable string: 'methodName' or "methodName" as array element
		if strings.Contains(line, "'"+name+"'") || strings.Contains(line, "\""+name+"\"") {
			return true
		}
	}
	return false
}

// isPropertyReferenced checks if a private property (without $ prefix) is
// referenced anywhere in the class body outside its declaration line.
func isPropertyReferenced(name string, declLine int, cls parser.ClassNode, lines []string) bool {
	start := cls.StartLine
	end := endLineOf(cls, lines)

	// Patterns: $this->name  self::$name  static::$name
	accessPattern := "->" + name
	staticPattern := "::$" + name

	for i := start; i <= end && i < len(lines); i++ {
		if i == declLine {
			continue
		}
		line := lines[i]
		// Instance access: ->name (check word boundary after)
		if idx := strings.Index(line, accessPattern); idx >= 0 {
			after := idx + len(accessPattern)
			if after >= len(line) || !isIdentChar(line[after]) {
				return true
			}
		}
		// Static access: ::$name
		if strings.Contains(line, staticPattern) {
			return true
		}
	}
	return false
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
