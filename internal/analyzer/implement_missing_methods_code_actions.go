package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (a *Analyzer) implementMissingMethodsCodeActions(uri, source string, result *parser.ParseResult, only []string, pos protocol.Position) []protocol.CodeAction {
	if !codeActionKindAllowed(only, "refactor") {
		return nil
	}
	if result == nil {
		return nil
	}

	target := findImplementMissingMethodsTarget(result, pos.Line)
	if target == nil || target.endLine <= target.startLine {
		return nil
	}

	methods := a.collectImplementMissingMethods(source, target)
	if len(methods) == 0 {
		return nil
	}

	edit := implementMissingMethodsInsertionEdit(uri, source, target, methods)
	if edit == nil {
		return nil
	}

	return []protocol.CodeAction{{
		Title: "Implement missing methods",
		Kind:  "refactor",
		Edit:  edit,
	}}
}

type implementMissingMethodsTarget struct {
	fqn        string
	startLine  int
	endLine    int
	methods    []parser.MethodDef
	extends    string
	implements []string
}

func findImplementMissingMethodsTarget(result *parser.ParseResult, cursorLine int) *implementMissingMethodsTarget {
	for _, cls := range result.Classes {
		if cls.Line != cursorLine {
			continue
		}
		return &implementMissingMethodsTarget{
			fqn:        cls.FullName,
			startLine:  cls.Line,
			endLine:    cls.EndLine,
			methods:    cls.Methods,
			extends:    cls.Extends,
			implements: append([]string(nil), cls.Implements...),
		}
	}
	for _, en := range result.Enums {
		if en.Line != cursorLine {
			continue
		}
		return &implementMissingMethodsTarget{
			fqn:        en.FullName,
			startLine:  en.Line,
			endLine:    en.EndLine,
			methods:    en.Methods,
			implements: append([]string(nil), en.Implements...),
		}
	}
	return nil
}

type implementMissingMethodCandidate struct {
	symbol       *symbols.Symbol
	signatureKey string
}

func (a *Analyzer) collectImplementMissingMethods(source string, target *implementMissingMethodsTarget) []string {
	if target == nil || target.fqn == "" {
		return nil
	}

	methodIndent := implementMissingMethodsIndent(source, target.startLine)
	targetFile := parser.ParseFile(source)
	if targetFile == nil {
		return nil
	}

	implementedConcrete := make(map[string]bool)
	for _, member := range a.index.GetClassMembers(target.fqn) {
		if member == nil || member.Kind != symbols.KindMethod || member.IsAbstract {
			continue
		}
		implementedConcrete[strings.ToLower(member.Name)] = true
	}

	declaredHere := make(map[string]bool)
	for _, method := range target.methods {
		declaredHere[strings.ToLower(method.Name)] = true
	}

	requirements := make(map[string]implementMissingMethodCandidate)
	conflicts := make(map[string]bool)
	addRequirement := func(member *symbols.Symbol) {
		if member == nil || member.Kind != symbols.KindMethod || member.Name == "" {
			return
		}
		nameKey := strings.ToLower(member.Name)
		if implementedConcrete[nameKey] || declaredHere[nameKey] {
			return
		}
		signatureKey := implementMissingMethodSignatureKey(member)
		if existing, ok := requirements[nameKey]; ok {
			if existing.signatureKey != signatureKey {
				conflicts[nameKey] = true
			}
			return
		}
		requirements[nameKey] = implementMissingMethodCandidate{
			symbol:       member,
			signatureKey: signatureKey,
		}
	}

	for _, ifaceName := range target.implements {
		a.collectInterfaceMethodRequirements(a.resolver.ResolveClassName(ifaceName, targetFile), addRequirement, make(map[string]bool))
	}

	parentFQN := ""
	if target.extends != "" {
		parentFQN = a.resolver.ResolveClassName(target.extends, targetFile)
	}
	if parentFQN == "" {
		if sym := a.index.Lookup(target.fqn); sym != nil {
			parentFQN = sym.Extends
		}
	}
	for parentFQN != "" {
		parent := a.index.Lookup(parentFQN)
		if parent == nil {
			break
		}
		for _, child := range parent.Children {
			if child.Kind == symbols.KindMethod && child.IsAbstract {
				addRequirement(child)
			}
		}
		parentFQN = parent.Extends
	}

	var names []string
	for name := range requirements {
		if conflicts[name] {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var rendered []string
	for _, name := range names {
		text, ok := a.renderImplementMissingMethod(requirements[name].symbol, methodIndent)
		if !ok {
			continue
		}
		rendered = append(rendered, text)
	}
	return rendered
}

func (a *Analyzer) collectInterfaceMethodRequirements(ifaceFQN string, add func(*symbols.Symbol), seen map[string]bool) {
	if ifaceFQN == "" || seen[ifaceFQN] {
		return
	}
	seen[ifaceFQN] = true

	iface := a.index.Lookup(ifaceFQN)
	if iface == nil || iface.Kind != symbols.KindInterface {
		return
	}
	for _, child := range iface.Children {
		if child.Kind == symbols.KindMethod {
			add(child)
		}
	}
	for _, parentFQN := range a.interfaceExtendedFQNs(iface) {
		a.collectInterfaceMethodRequirements(parentFQN, add, seen)
	}
}

func (a *Analyzer) interfaceExtendedFQNs(iface *symbols.Symbol) []string {
	if iface == nil || iface.URI == "" {
		return nil
	}
	source := a.index.GetFileSource(iface.URI)
	if source == "" {
		return nil
	}
	file := parser.ParseFile(source)
	if file == nil {
		return nil
	}
	for _, node := range file.Interfaces {
		fqn := node.FullName
		if fqn == "" {
			fqn = implementMissingMethodsBuildFQN(file.Namespace, node.Name)
		}
		if fqn != iface.FQN {
			continue
		}
		parents := make([]string, 0, len(node.Extends))
		for _, parent := range node.Extends {
			parents = append(parents, a.resolver.ResolveClassName(parent, file))
		}
		return parents
	}
	return nil
}

func implementMissingMethodSignatureKey(member *symbols.Symbol) string {
	if member == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%t|%s|%s", strings.ToLower(member.Name), member.IsStatic, member.Visibility, member.ReturnType)
	for _, param := range member.Params {
		fmt.Fprintf(&b, "|%s:%s:%t:%t:%s", param.Name, param.Type, param.IsVariadic, param.IsReference, param.DefaultValue)
	}
	return b.String()
}

func (a *Analyzer) renderImplementMissingMethod(member *symbols.Symbol, indent string) (string, bool) {
	declaration, ok := a.extractImplementMissingMethodDeclaration(member)
	if !ok {
		return "", false
	}

	declaration = strings.TrimSpace(declaration)
	if declaration == "" {
		return "", false
	}
	declaration = strings.TrimSuffix(declaration, ";")
	declaration = removeStandaloneAbstractModifier(declaration)
	declarationLines := dedentLines(strings.Split(declaration, "\n"))
	if len(declarationLines) == 0 {
		return "", false
	}

	var b strings.Builder
	for i, line := range declarationLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(indent)
		b.WriteString(strings.TrimRight(line, " \t"))
	}
	b.WriteString("\n")
	b.WriteString(indent)
	b.WriteString("{\n")
	b.WriteString(indent)
	b.WriteString("    throw new \\BadMethodCallException(__METHOD__ . ' is not implemented.');\n")
	b.WriteString(indent)
	b.WriteString("}")
	return b.String(), true
}

func removeStandaloneAbstractModifier(declaration string) string {
	fields := strings.Fields(declaration)
	if len(fields) == 0 {
		return declaration
	}
	filtered := make([]string, 0, len(fields))
	removed := false
	for _, field := range fields {
		if !removed && field == "abstract" {
			removed = true
			continue
		}
		filtered = append(filtered, field)
	}
	if !removed {
		return declaration
	}

	lines := strings.Split(declaration, "\n")
	if len(lines) == 1 {
		return strings.Join(filtered, " ")
	}

	rebuilt := strings.Join(filtered, " ")
	if idx := strings.Index(rebuilt, "function "); idx >= 0 {
		prefix := rebuilt[:idx]
		suffix := rebuilt[idx:]
		if prefix == "" {
			return suffix
		}
		return strings.TrimSpace(prefix) + " " + suffix
	}
	return rebuilt
}

func dedentLines(lines []string) []string {
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(leadingWhitespace(line))
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if strings.TrimSpace(line) == "" || len(line) < minIndent {
			out[i] = line
			continue
		}
		out[i] = line[minIndent:]
	}
	return out
}

func (a *Analyzer) extractImplementMissingMethodDeclaration(member *symbols.Symbol) (string, bool) {
	if member == nil || member.URI == "" {
		return "", false
	}
	source := a.index.GetFileSource(member.URI)
	if source == "" {
		return "", false
	}
	result := parser.New().Parse(source)
	if result == nil {
		return "", false
	}

	method, ok := findImplementMissingMethodDef(result, member)
	if !ok {
		return "", false
	}

	startOffset, endOffset, ok := extractImplementMissingMethodOffsets(result, method)
	if !ok || startOffset < 0 || endOffset <= startOffset || endOffset > len(source) {
		return "", false
	}
	return source[startOffset:endOffset], true
}

func findImplementMissingMethodDef(result *parser.ParseResult, member *symbols.Symbol) (parser.MethodDef, bool) {
	matchMethod := func(methods []parser.MethodDef) (parser.MethodDef, bool) {
		for _, method := range methods {
			if method.Name == member.Name && method.Line == member.Range.Start.Line {
				return method, true
			}
		}
		return parser.MethodDef{}, false
	}

	for _, cls := range result.Classes {
		if cls.FullName == member.ParentFQN {
			return matchMethod(cls.Methods)
		}
	}
	for _, iface := range result.Interfaces {
		if iface.FullName == member.ParentFQN {
			return matchMethod(iface.Methods)
		}
	}
	for _, tr := range result.Traits {
		if tr.FullName == member.ParentFQN {
			return matchMethod(tr.Methods)
		}
	}
	for _, en := range result.Enums {
		if en.FullName == member.ParentFQN {
			return matchMethod(en.Methods)
		}
	}
	return parser.MethodDef{}, false
}

func extractImplementMissingMethodOffsets(result *parser.ParseResult, method parser.MethodDef) (int, int, bool) {
	functionIdx := -1
	for i := range result.Tokens {
		tok := result.Tokens[i]
		if tok.Line != method.Line || tok.Kind != parser.TokenFunction {
			continue
		}
		for j := i + 1; j < len(result.Tokens); j++ {
			if result.Tokens[j].Line < method.Line {
				continue
			}
			if result.Tokens[j].Kind == parser.TokenIdentifier && result.Tokens[j].Value == method.Name {
				functionIdx = i
				break
			}
			if result.Tokens[j].Kind == parser.TokenSemicolon || result.Tokens[j].Kind == parser.TokenOpenBrace {
				break
			}
		}
		if functionIdx >= 0 {
			break
		}
	}
	if functionIdx < 0 {
		return 0, 0, false
	}

	startIdx := functionIdx
	for startIdx > 0 {
		switch result.Tokens[startIdx-1].Kind {
		case parser.TokenPublic, parser.TokenProtected, parser.TokenPrivate, parser.TokenStatic, parser.TokenAbstract, parser.TokenFinal, parser.TokenReadonly:
			startIdx--
		default:
			goto foundStart
		}
	}
foundStart:

	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	endOffset := 0
	for i := functionIdx; i < len(result.Tokens); i++ {
		switch result.Tokens[i].Kind {
		case parser.TokenOpenParen:
			parenDepth++
		case parser.TokenCloseParen:
			if parenDepth > 0 {
				parenDepth--
			}
		case parser.TokenOpenBracket:
			bracketDepth++
		case parser.TokenCloseBracket:
			if bracketDepth > 0 {
				bracketDepth--
			}
		case parser.TokenOpenBrace:
			braceDepth++
		case parser.TokenCloseBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case parser.TokenSemicolon:
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				endOffset = result.Tokens[i].Offset + len(result.Tokens[i].Value)
				return result.Tokens[startIdx].Offset, endOffset, true
			}
		}
	}

	return 0, 0, false
}

func implementMissingMethodsInsertionEdit(uri, source string, target *implementMissingMethodsTarget, methods []string) *protocol.WorkspaceEdit {
	if len(methods) == 0 {
		return nil
	}

	lines := strings.Split(source, "\n")
	if target.endLine < 0 || target.endLine >= len(lines) {
		return nil
	}

	insertPrefix := ""
	if implementMissingMethodsNeedsLeadingBlank(lines, target.startLine, target.endLine) {
		insertPrefix = "\n"
	}

	return &protocol.WorkspaceEdit{
		Changes: map[string][]protocol.TextEdit{
			uri: {{
				Range: protocol.Range{
					Start: protocol.Position{Line: target.endLine, Character: 0},
					End:   protocol.Position{Line: target.endLine, Character: 0},
				},
				NewText: insertPrefix + strings.Join(methods, "\n\n") + "\n",
			}},
		},
	}
}

func implementMissingMethodsIndent(source string, line int) string {
	lines := strings.Split(source, "\n")
	if line < 0 || line >= len(lines) {
		return "    "
	}
	return leadingWhitespace(lines[line]) + "    "
}

func implementMissingMethodsNeedsLeadingBlank(lines []string, startLine, endLine int) bool {
	for line := endLine - 1; line > startLine; line-- {
		if strings.TrimSpace(lines[line]) == "" {
			return false
		}
		return true
	}
	return false
}

func implementMissingMethodsBuildFQN(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
}
