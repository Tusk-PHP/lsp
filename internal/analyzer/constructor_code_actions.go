package analyzer

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
)

func (a *Analyzer) generateConstructorCodeAction(uri, source string, result *parser.ParseResult, only []string, pos protocol.Position) *protocol.CodeAction {
	if result == nil || !importCodeActionKindAllowed(only, "refactor") {
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

	openTok, closeTok, ok := classBodyTokens(result, classDef)
	if !ok {
		return nil
	}

	lines := strings.Split(source, "\n")
	insertPos, memberIndent, indentUnit, ok := constructorInsertPosition(lines, classDef, openTok, closeTok)
	if !ok {
		return nil
	}

	newText := renderGeneratedConstructor(classDef, props, memberIndent, indentUnit, closeTok.Line == classDef.Line)
	return &protocol.CodeAction{
		Title: "Generate Constructor",
		Kind:  "refactor",
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

func classBodyTokens(result *parser.ParseResult, classDef parser.ClassDef) (parser.Token, parser.Token, bool) {
	tokens := result.Tokens
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind != parser.TokenClass || tokens[i].Line != classDef.Line {
			continue
		}

		nameMatched := false
		for j := i + 1; j < len(tokens); j++ {
			tok := tokens[j]
			if tok.Kind == parser.TokenIdentifier && tok.Value == classDef.Name {
				nameMatched = true
			}
			if tok.Kind == parser.TokenOpenBrace {
				if !nameMatched {
					break
				}
				depth := 1
				for k := j + 1; k < len(tokens); k++ {
					switch tokens[k].Kind {
					case parser.TokenOpenBrace:
						depth++
					case parser.TokenCloseBrace:
						depth--
						if depth == 0 {
							return tok, tokens[k], true
						}
					}
				}
				return parser.Token{}, parser.Token{}, false
			}
			if tok.Kind == parser.TokenSemicolon {
				break
			}
		}
	}
	return parser.Token{}, parser.Token{}, false
}

func constructorInsertPosition(lines []string, classDef parser.ClassDef, _ parser.Token, closeTok parser.Token) (protocol.Position, string, string, bool) {
	if closeTok.Line < 0 || closeTok.Line >= len(lines) || classDef.Line < 0 || classDef.Line >= len(lines) {
		return protocol.Position{}, "", "", false
	}

	classIndent := leadingWhitespace(lines[classDef.Line])
	memberIndent := ""
	for _, prop := range classDef.Properties {
		if prop.Line >= 0 && prop.Line < len(lines) {
			memberIndent = leadingWhitespace(lines[prop.Line])
			break
		}
	}
	if memberIndent == "" {
		for _, method := range classDef.Methods {
			if method.Line >= 0 && method.Line < len(lines) {
				memberIndent = leadingWhitespace(lines[method.Line])
				break
			}
		}
	}

	indentUnit := "    "
	if strings.HasPrefix(memberIndent, classIndent) && len(memberIndent) > len(classIndent) {
		indentUnit = memberIndent[len(classIndent):]
	}
	if memberIndent == "" {
		if strings.Contains(classIndent, "\t") {
			indentUnit = "\t"
		}
		memberIndent = classIndent + indentUnit
	}

	return protocol.Position{Line: closeTok.Line, Character: closeTok.Column}, memberIndent, indentUnit, true
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
