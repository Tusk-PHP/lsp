package analyzer

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func (a *Analyzer) generateConstructorCodeAction(uri string, toks codeTokens, result *parser.ParseResult, only []protocol.CodeActionKind, pos protocol.Position) *protocol.CodeAction {
	if result == nil || !protocol.CodeActionRefactor.In(only) {
		return nil
	}

	classDef, ok := classDefAtLine(result.Classes, pos.Line)
	if !ok || hasConstructor(classDef.Methods) {
		return nil
	}

	props := constructorEligibleProperties(classDef.Properties)
	if len(props) == 0 {
		return nil
	}

	insertPos, ok := classBodyEnd(toks, classDef)
	if !ok {
		return nil
	}

	memberIndent, indentUnit := constructorIndent(toks, classDef)
	newText := renderGeneratedConstructor(classDef, props, memberIndent, indentUnit, insertPos.Line == classDef.Line)
	return &protocol.CodeAction{
		Title: "Generate Constructor",
		Kind:  protocol.CodeActionRefactor,
		Edit: &protocol.WorkspaceEdit{
			Changes: map[string][]protocol.TextEdit{
				uri: {{
					Range:   protocol.Range{Start: insertPos, End: insertPos},
					NewText: newText,
				}},
			},
		},
	}
}

func classDefAtLine(classes []parser.ClassDef, line int) (parser.ClassDef, bool) {
	for _, classDef := range classes {
		if classDef.Line == line {
			return classDef, true
		}
	}
	return parser.ClassDef{}, false
}

func hasConstructor(methods []parser.MethodDef) bool {
	for _, method := range methods {
		if strings.EqualFold(method.Name, "__construct") {
			return true
		}
	}
	return false
}

func constructorEligibleProperties(properties []parser.PropertyDef) []parser.PropertyDef {
	eligible := make([]parser.PropertyDef, 0, len(properties))
	for _, prop := range properties {
		if prop.IsStatic || prop.HasDefault {
			continue
		}
		eligible = append(eligible, prop)
	}
	return eligible
}

func classBodyEnd(toks codeTokens, classDef parser.ClassDef) (protocol.Position, bool) {
	for i, tok := range toks.all {
		if tok.Kind != token.Class || toks.position(tok.Start).Line != classDef.Line {
			continue
		}
		if open, ok := classBodyOpen(toks, i, classDef.Name); ok {
			end := lexer.MatchingClose(toks.all, open)
			return toks.position(toks.all[max(end, 0)].Start), end >= 0
		}
	}
	return protocol.Position{}, false
}

func classBodyOpen(toks codeTokens, class int, name string) (int, bool) {
	named := false
	for j := class + 1; ; j++ {
		switch toks.kind(j) {
		case token.Identifier:
			named = named || toks.text(j) == name
		case token.LeftBrace:
			return j, named
		case token.Semicolon, token.Invalid:
			return 0, false
		}
	}
}

func constructorIndent(toks codeTokens, classDef parser.ClassDef) (memberIndent, indentUnit string) {
	classIndent := leadingWhitespace(toks.line(classDef.Line))
	memberIndent = leadingWhitespace(toks.line(classDef.Properties[0].Line))
	if memberIndent == "" && len(classDef.Methods) > 0 {
		memberIndent = leadingWhitespace(toks.line(classDef.Methods[0].Line))
	}

	indentUnit = "    "
	if strings.HasPrefix(memberIndent, classIndent) && len(memberIndent) > len(classIndent) {
		indentUnit = memberIndent[len(classIndent):]
	}
	if memberIndent == "" {
		if strings.Contains(classIndent, "\t") {
			indentUnit = "\t"
		}
		memberIndent = classIndent + indentUnit
	}
	return memberIndent, indentUnit
}

func renderGeneratedConstructor(classDef parser.ClassDef, props []parser.PropertyDef, memberIndent, indentUnit string, closingOnClassLine bool) string {
	var b strings.Builder
	if closingOnClassLine || len(classDef.Properties)+len(classDef.Methods)+len(classDef.Constants)+len(classDef.Traits) > 0 {
		b.WriteByte('\n')
	}

	b.WriteString(memberIndent)
	b.WriteString("public function __construct(")
	for i, prop := range props {
		if i > 0 {
			b.WriteString(", ")
		}
		if prop.Type != "" {
			b.WriteString(prop.Type)
			b.WriteByte(' ')
		}
		b.WriteString(prop.Name)
	}
	b.WriteString(")\n")
	b.WriteString(memberIndent)
	b.WriteString("{\n")

	bodyIndent := memberIndent + indentUnit
	for _, prop := range props {
		name := strings.TrimPrefix(prop.Name, "$")
		b.WriteString(bodyIndent)
		b.WriteString("$this->")
		b.WriteString(name)
		b.WriteString(" = ")
		b.WriteString(prop.Name)
		b.WriteString(";\n")
	}

	b.WriteString(memberIndent)
	b.WriteString("}\n")
	return b.String()
}
