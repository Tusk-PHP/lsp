package analyzer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/types"
)

func (a *Analyzer) implementMissingMethodsCodeActions(uri, source string, result *parser.ParseResult, only []protocol.CodeActionKind, pos protocol.Position) []protocol.CodeAction {
	r := a.engine.Resolver()
	if !protocol.CodeActionRefactor.In(only) {
		return nil
	}
	if result == nil {
		return nil
	}

	target := findImplementMissingMethodsTarget(result, pos.Line)
	if target == nil || target.endLine <= target.startLine {
		return nil
	}

	methods := a.collectImplementMissingMethods(r, source, target)
	if len(methods) == 0 {
		return nil
	}

	edit := implementMissingMethodsInsertionEdit(uri, source, target, methods)
	if edit == nil {
		return nil
	}

	return []protocol.CodeAction{{
		Title: "Implement missing methods",
		Kind:  protocol.CodeActionRefactor,
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

func (a *Analyzer) collectImplementMissingMethods(r *resolve.Resolver, source string, target *implementMissingMethodsTarget) []string {
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
		// An interface's methods are listed with the class's members, but
		// they're only declared there, never implemented.
		if owner := a.index.Lookup(member.ParentFQN); owner != nil && owner.Kind == symbols.KindInterface {
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
		a.collectInterfaceMethodRequirements(r, r.ResolveClassName(ifaceName, targetFile), addRequirement, make(map[string]bool))
	}

	parentFQN := ""
	if target.extends != "" {
		parentFQN = r.ResolveClassName(target.extends, targetFile)
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
		text, ok := renderImplementMissingMethod(requirements[name].symbol, methodIndent, targetFile)
		if !ok {
			continue
		}
		rendered = append(rendered, text)
	}
	return rendered
}

func (a *Analyzer) collectInterfaceMethodRequirements(r *resolve.Resolver, ifaceFQN string, add func(*symbols.Symbol), seen map[string]bool) {
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
	for _, parentFQN := range iface.Implements {
		a.collectInterfaceMethodRequirements(r, parentFQN, add, seen)
	}
}

func classNameIn(fqn string, file *parser.FileNode) string {
	fqn = strings.TrimPrefix(fqn, "\\")
	short, namespace := fqn, ""
	if i := strings.LastIndex(fqn, "\\"); i >= 0 {
		short, namespace = fqn[i+1:], fqn[:i]
	}
	shadowed := false
	for _, use := range file.Uses {
		if use.Kind == "function" || use.Kind == "const" {
			continue
		}
		if strings.EqualFold(use.FullName, fqn) {
			return use.Alias
		}
		shadowed = shadowed || strings.EqualFold(use.Alias, short)
	}
	if !shadowed && strings.EqualFold(namespace, file.Namespace) {
		return short
	}
	return "\\" + fqn
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

func renderImplementMissingMethod(member *symbols.Symbol, indent string, file *parser.FileNode) (string, bool) {
	var b strings.Builder
	b.WriteString(indent)
	if member.Visibility == "" {
		b.WriteString("public")
	} else {
		b.WriteString(member.Visibility)
	}
	if member.IsStatic {
		b.WriteString(" static")
	}
	b.WriteString(" function ")
	b.WriteString(member.Name)
	b.WriteByte('(')
	for i, param := range member.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		if param.Type != "" {
			typ, ok := nativeTypeDeclaration(types.ParseTypeExpr(param.Type), file)
			if !ok {
				return "", false
			}
			b.WriteString(typ)
			b.WriteByte(' ')
		}
		if param.IsReference {
			b.WriteByte('&')
		}
		if param.IsVariadic {
			b.WriteString("...")
		}
		b.WriteString(param.Name)
		if param.DefaultValue != "" {
			b.WriteString(" = ")
			b.WriteString(param.DefaultValue)
		}
	}
	b.WriteByte(')')
	if member.ReturnType != "" {
		typ, ok := nativeTypeDeclaration(types.ParseTypeExpr(member.ReturnType), file)
		if !ok {
			return "", false
		}
		b.WriteString(": ")
		b.WriteString(typ)
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

func nativeTypeDeclaration(expr types.TypeExpr, file *parser.FileNode) (string, bool) {
	var out string
	switch expr.Kind {
	case types.TypeKindNamed:
		if expr.Name == "" {
			return "", false
		}
		out = expr.Name
		if !symbols.IsPHPBuiltinType(out) {
			out = classNameIn(out, file)
		}
	case types.TypeKindUnion, types.TypeKindIntersection:
		sep := "|"
		if expr.Kind == types.TypeKindIntersection {
			sep = "&"
		}
		parts := make([]string, len(expr.Types))
		for i, part := range expr.Types {
			rendered, ok := nativeTypeDeclaration(part, file)
			if !ok {
				return "", false
			}
			parts[i] = rendered
		}
		out = strings.Join(parts, sep)
	case types.TypeKindCallable:
		if expr.ParamTypes != nil || expr.ReturnType != nil {
			return "", false
		}
		out = expr.Name
	case types.TypeKindLiteral:
		switch strings.ToLower(expr.Literal) {
		case "null", "false", "true":
			out = expr.Literal
		default:
			return "", false
		}
	default:
		return "", false
	}
	if expr.Wrapped {
		out = "(" + out + ")"
	}
	if expr.Nullable {
		out = "?" + out
	}
	return out, true
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
