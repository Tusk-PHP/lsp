package resolve

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// instanceof narrowing: `$x instanceof Foo` in an if, an early-return guard,
// assert(), or before `&&`, `?` or `||` on the same line says what $x is
// where that holds, following PHPStan.

// instanceofTest returns the class `varName instanceof Name` tests in an
// expression that is exactly that test. ok is false for anything else,
// including `instanceof $other` and `instanceof static`.
func instanceofTest(expr, varName string) (name string, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(expr), varName)
	if !found || rest == "" || rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	rest = strings.TrimLeft(rest, " \t")
	if len(rest) < len("instanceof")+1 || !strings.EqualFold(rest[:len("instanceof")], "instanceof") {
		return "", false
	}
	rest = rest[len("instanceof"):]
	if rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	name = strings.TrimSpace(rest)
	if name == "" || name[0] >= '0' && name[0] <= '9' {
		return "", false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; !isIdentByte(c) && !(c >= '0' && c <= '9') && c != '\\' {
			return "", false
		}
	}
	return name, true
}

// narrowInstanceof is what t becomes where `$x instanceof name` is value.
func (r *Resolver) narrowInstanceof(t ResolvedType, name string, value bool, file *parser.FileNode) ResolvedType {
	switch strings.ToLower(name) {
	case "self", "static", "parent":
		if value {
			return ResolvedType{} // which class that is isn't worked out here
		}
		return t
	}
	fqn := r.ResolveClassName(name, file)
	if fqn == "" || r.Index == nil || r.Index.Lookup(fqn) == nil {
		// A class that isn't indexed could have any member.
		if value {
			return ResolvedType{}
		}
		return t
	}
	cls := ResolvedType{FQN: fqn}
	t = splitWrittenUnion(t)
	if !value {
		return r.removeType(t, cls)
	}
	parts := t.Parts()
	if len(t.Intersection) > 0 {
		parts = t.Intersection
	}
	if t.IsEmpty() || len(parts) == 0 {
		return cls
	}
	var kept []ResolvedType
	narrower := false
	for _, p := range parts {
		switch {
		case p.FQN == "" || strings.EqualFold(p.FQN, "mixed") || strings.EqualFold(p.FQN, "object"):
			narrower = true
		case r.typeMatches(p, cls) == matchYes:
			kept = append(kept, p)
		case !IsBuiltinTypeName(p.FQN) && r.Index.IsSubtypeOf(fqn, p.FQN):
			narrower = true
		}
	}
	switch {
	case len(kept) > 0 && len(t.Intersection) == 0:
		return MakeUnion(kept, false)
	case len(kept) > 0:
		return t
	case narrower:
		return cls
	}
	// A class and an interface it doesn't implement, or two unrelated
	// classes: an object that is both, as a subclass may be.
	var objects []ResolvedType
	for _, p := range parts {
		if !IsBuiltinTypeName(p.FQN) {
			objects = append(objects, p)
		}
	}
	if len(objects) == 0 || len(objects) > 1 && len(t.Intersection) == 0 {
		return cls
	}
	return makeIntersection(append(objects, cls), false)
}

// statementInstanceof applies an instanceof test written earlier in the
// read's own statement, before `&&`, `and`, `?` or `=>` (when true) or `||`,
// `or` (when false): `$x instanceof Foo && $x->bar()`,
// `!$x instanceof Foo || $x->bar()`, `$x instanceof Foo ? $x->bar() : null`,
// `match (true) { $x instanceof Foo => $x->bar() }`. It holds only while the
// read is still under that operator: not past a `,` or the `)` that closes
// around it, nor in the other branch of a ternary.
func (r *Resolver) statementInstanceof(varName string, t ResolvedType, prefix string, file *parser.FileNode) ResolvedType {
	if !strings.Contains(prefix, "instanceof") {
		return t
	}
	for from := 0; ; {
		k := strings.Index(prefix[from:], varName)
		if k < 0 {
			return t
		}
		start := from + k
		end := start + len(varName)
		from = end
		if end < len(prefix) && (isIdentByte(prefix[end]) || prefix[end] >= '0' && prefix[end] <= '9') {
			continue
		}
		if start > 0 && (prefix[start-1] == '>' || prefix[start-1] == ':') {
			continue // ->$x, ::$x
		}
		rest := prefix[end:]
		name, nameEnd := instanceofAt(rest)
		if name == "" {
			continue
		}
		value := true
		before := strings.TrimRight(prefix[:start], " \t(")
		if strings.HasSuffix(before, "!") && !strings.HasSuffix(before, "!=") {
			value = false
		}
		after := rest[nameEnd:]
		closes := 0
		for closes < len(after) && (after[closes] == ' ' || after[closes] == '\t' || after[closes] == ')') {
			closes++
		}
		join := after[closes:]
		op := ""
		for _, candidate := range []string{"&&", "||", "=>", "?"} {
			if strings.HasPrefix(join, candidate) {
				op = candidate
				break
			}
		}
		if op == "" && hasWordPrefix(join, "and") {
			op = "&&"
			join = join[1:]
		} else if op == "" && hasWordPrefix(join, "or") {
			op = "||"
		}
		if op == "" || op == "?" && (strings.HasPrefix(join, "??") || strings.HasPrefix(join, "?->") || strings.HasPrefix(join, "?:")) {
			continue
		}
		if !underOperator(join[len(op):], op) {
			continue
		}
		if (op == "?" || op == "=>") && orBefore(prefix[:start]) {
			continue // a || $x instanceof Foo ? … — the test alone isn't the condition
		}
		switch op {
		case "&&", "?", "=>":
			t = r.narrowInstanceof(t, name, value, file)
		case "||":
			t = r.narrowInstanceof(t, name, !value, file)
		}
	}
}

// underOperator reports whether the end of rest, the text from just after a
// join operator to the read, is still governed by it.
func underOperator(rest, op string) bool {
	var quote byte
	level := 0
	ternary := false
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '(', '[', '{':
			level++
		case ')', ']', '}':
			level--
			if level < 0 {
				return false
			}
		case ';':
			return false
		case ',':
			if level <= 0 {
				return false
			}
		case '?':
			if level != 0 || i+1 < len(rest) && (rest[i+1] == '?' || rest[i+1] == '-' || rest[i+1] == ':') {
				if i+1 < len(rest) && rest[i+1] == '?' {
					i++
				}
				continue
			}
			if op == "||" {
				return false // a || b ? … : … — either branch may not hold
			}
			ternary = true
		case ':':
			if i+1 < len(rest) && rest[i+1] == ':' {
				i++
				continue
			}
			if level == 0 && (op == "?" || ternary) {
				return false // the else branch
			}
		}
	}
	return true
}

// orBefore reports whether the expression that ends at the end of s has a
// `||` or `or` at its own level, as `$a || ` does before a test.
func orBefore(s string) bool {
	level := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch c := s[i]; c {
		case ')', ']', '}':
			level++
		case '(', '[', '{':
			if level == 0 {
				return false
			}
			level--
		case ',', ';':
			if level == 0 {
				return false
			}
		case '|':
			if level == 0 && i > 0 && s[i-1] == '|' {
				return true
			}
		case '=':
			if level == 0 && i > 0 && !strings.ContainsRune("=!<>", rune(s[i-1])) && (i+1 >= len(s) || s[i+1] != '=' && s[i+1] != '>') {
				return false // an assignment: what's left of it isn't the condition
			}
		case 'r', 'R':
			if level == 0 && i > 0 && (s[i-1] == 'o' || s[i-1] == 'O') && i >= 2 && (s[i-2] == ' ' || s[i-2] == ')') && i+1 < len(s) && s[i+1] == ' ' {
				return true
			}
		}
	}
	return false
}

// statementPrefix is the text of the statement the read at pos is in, up to
// pos: the read's line, and the lines before it that the statement continues
// from, joined by spaces.
func statementPrefix(lines []string, pos protocol.Position) string {
	line := lines[pos.Line]
	if pos.Character < len(line) {
		line = line[:pos.Character]
	}
	parts := []string{line}
	for i := pos.Line - 1; i >= 0 && i >= pos.Line-maxStatementLines; i-- {
		prev := strings.TrimSpace(lines[i])
		if prev == "" || strings.HasPrefix(prev, "//") || strings.HasPrefix(prev, "#") || strings.HasPrefix(prev, "*") || strings.HasPrefix(prev, "/*") {
			break
		}
		if last := prev[len(prev)-1]; last == ';' || last == '{' || last == '}' || last == ',' || last == ':' {
			break
		}
		parts = append(parts, prev)
	}
	if len(parts) == 1 {
		return line
	}
	slices.Reverse(parts)
	return strings.Join(parts, " ")
}

// maxStatementLines bounds how far back a statement is read.
const maxStatementLines = 6

// switchCaseInstanceof applies `case $x instanceof Foo:` in a
// `switch (true)` the read is under, with no break between them.
func (r *Resolver) switchCaseInstanceof(varName string, t ResolvedType, lines []string, from int, pos protocol.Position, file *parser.FileNode) ResolvedType {
	depth := 0
	var names []string
	for i := pos.Line - 1; i > from && i >= pos.Line-maxCaseLines; i-- {
		line := strings.TrimSpace(lines[i])
		label, isCase := strings.CutPrefix(line, "case ")
		if depth == 0 && isCase {
			name, ok := caseInstanceof(label, varName)
			if !ok {
				return t
			}
			names = append(names, name)
			// Labels stacked above it share its body.
			for j := i - 1; j > from; j-- {
				above, more := strings.CutPrefix(strings.TrimSpace(lines[j]), "case ")
				if !more {
					break
				}
				name, ok := caseInstanceof(above, varName)
				if !ok {
					return t
				}
				names = append(names, name)
			}
			break
		}
		if depth == 0 && (strings.HasPrefix(line, "default") || strings.HasPrefix(line, "break") || strings.HasPrefix(line, "return")) {
			return t
		}
		for k := len(line) - 1; k >= 0; k-- {
			switch line[k] {
			case '}':
				depth++
			case '{':
				if depth > 0 {
					depth--
					continue
				}
				// A block the read is in: an if inside the case is fine, the
				// switch itself or a function ends the search.
				header := strings.TrimSpace(line[:k])
				if strings.HasPrefix(header, "switch") || strings.Contains(header, "function") || strings.Contains(header, "fn") || strings.HasPrefix(header, "class") {
					return t
				}
			}
		}
	}
	if len(names) == 0 {
		return t
	}
	parts := make([]ResolvedType, 0, len(names))
	for _, name := range names {
		n := r.narrowInstanceof(t, name, true, file)
		if n.IsEmpty() {
			return n
		}
		parts = append(parts, n)
	}
	return MakeUnion(parts, false)
}

// caseInstanceof returns the class a `case` label tests varName against:
// `case $x instanceof Foo:`, or one of the conditions of
// `case $x instanceof Foo && $x->ready():`.
func caseInstanceof(label, varName string) (string, bool) {
	label = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(label), ":"))
	if len(splitTopLevel(label, "||")) > 1 {
		return "", false
	}
	for _, c := range splitTopLevel(label, "&&") {
		c = strings.TrimSpace(c)
		if inner, ok := unwrapParens(c); ok {
			c = inner
		}
		if name, ok := instanceofTest(c, varName); ok {
			return name, true
		}
	}
	return "", false
}

// maxCaseLines bounds how far back a case label is looked for.
const maxCaseLines = 40

// instanceofAt reads ` instanceof Name` at the start of s, returning the
// name and where it ends.
func instanceofAt(s string) (string, int) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if i == 0 || len(s)-i < len("instanceof")+1 || !strings.EqualFold(s[i:i+len("instanceof")], "instanceof") {
		return "", 0
	}
	i += len("instanceof")
	if s[i] != ' ' && s[i] != '\t' {
		return "", 0
	}
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	j := i
	for j < len(s) && (isIdentByte(s[j]) || s[j] >= '0' && s[j] <= '9' || s[j] == '\\') {
		j++
	}
	if j == i || s[i] >= '0' && s[i] <= '9' {
		return "", 0
	}
	return s[i:j], j
}

func hasWordPrefix(s, word string) bool {
	if len(s) <= len(word) || !strings.EqualFold(s[:len(word)], word) {
		return false
	}
	c := s[len(word)]
	return c == ' ' || c == '\t' || c == '('
}

// assertedCondition returns the condition of an `assert(…);` statement.
func assertedCondition(stmt string) (string, bool) {
	rest, ok := strings.CutPrefix(stmt, "assert")
	if !ok {
		rest, ok = strings.CutPrefix(stmt, "\\assert")
	}
	if !ok {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "(") || matchingParen(rest, 0) != len(rest)-1 {
		return "", false
	}
	args := splitTopLevel(rest[1:len(rest)-1], ",")
	return strings.TrimSpace(args[0]), true
}

// splitWrittenUnion makes a union kept as written in its name, as a
// parameter's declared `Foo|Bar` is, into its parts.
func splitWrittenUnion(t ResolvedType) ResolvedType {
	if len(t.Union) > 0 || len(t.Intersection) > 0 || !strings.Contains(t.FQN, "|") || strings.ContainsAny(t.FQN, "<({&") {
		return t
	}
	var parts []ResolvedType
	nullable := t.Nullable
	for _, part := range strings.Split(t.FQN, "|") {
		part = strings.TrimPrefix(strings.TrimSpace(part), "?")
		switch {
		case part == "":
		case strings.EqualFold(part, "null"):
			nullable = true
		default:
			parts = append(parts, ResolvedType{FQN: part})
		}
	}
	if len(parts) == 0 {
		return t
	}
	return MakeUnion(parts, nullable)
}
