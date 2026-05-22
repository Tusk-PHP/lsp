package analyzer

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
)

func (a *Analyzer) generateGetterSetterCodeActions(uri, source string, result *parser.ParseResult, only []string, pos protocol.Position) []protocol.CodeAction {
	if !codeActionKindAllowed(only, "refactor") {
		return nil
	}
	if result == nil {
		return nil
	}

	target := findGetterSetterTarget(result, pos.Line)
	if target == nil {
		return nil
	}

	propName := strings.TrimPrefix(target.property.Name, "$")
	if propName == "" {
		return nil
	}

	methodBase := getterSetterMethodBase(propName)
	getterName := "get" + methodBase
	setterName := "set" + methodBase

	getterMissing := !getterSetterHasMethod(target.methods, getterName)
	setterEligible := getterSetterCanGenerateSetter(target.property)
	setterMissing := setterEligible && !getterSetterHasMethod(target.methods, setterName)

	if !getterMissing && !setterMissing {
		return nil
	}

	insertion := getterSetterInsertionEdit(source, uri, target.endLine, target.property, getterName, setterName, getterMissing, setterMissing)
	if insertion == nil {
		return nil
	}

	var actions []protocol.CodeAction
	if getterMissing && setterMissing {
		actions = append(actions, protocol.CodeAction{
			Title: "Generate getter and setter for " + target.property.Name,
			Kind:  "refactor",
			Edit:  insertion(true, true),
		})
	}
	if getterMissing {
		actions = append(actions, protocol.CodeAction{
			Title: "Generate getter for " + target.property.Name,
			Kind:  "refactor",
			Edit:  insertion(true, false),
		})
	}
	if setterMissing {
		actions = append(actions, protocol.CodeAction{
			Title: "Generate setter for " + target.property.Name,
			Kind:  "refactor",
			Edit:  insertion(false, true),
		})
	}

	return actions
}

type getterSetterTarget struct {
	property parser.PropertyDef
	methods  []parser.MethodDef
	endLine  int
}

func findGetterSetterTarget(result *parser.ParseResult, cursorLine int) *getterSetterTarget {
	for _, cls := range result.Classes {
		target := getterSetterTargetForProperties(cls.Properties, cls.Methods, cls.EndLine, cursorLine)
		if target != nil {
			return target
		}
	}
	for _, tr := range result.Traits {
		target := getterSetterTargetForProperties(tr.Properties, tr.Methods, tr.EndLine, cursorLine)
		if target != nil {
			return target
		}
	}
	return nil
}

func getterSetterTargetForProperties(properties []parser.PropertyDef, methods []parser.MethodDef, endLine, cursorLine int) *getterSetterTarget {
	var match *parser.PropertyDef
	for i := range properties {
		prop := &properties[i]
		if prop.Line != cursorLine {
			continue
		}
		if match != nil {
			return nil
		}
		match = prop
	}
	if match == nil {
		return nil
	}
	if match.IsStatic || len(match.Hooks) > 0 {
		return nil
	}
	return &getterSetterTarget{
		property: *match,
		methods:  methods,
		endLine:  endLine,
	}
}

func getterSetterHasMethod(methods []parser.MethodDef, name string) bool {
	for _, method := range methods {
		if strings.EqualFold(method.Name, name) {
			return true
		}
	}
	return false
}

func getterSetterCanGenerateSetter(prop parser.PropertyDef) bool {
	return !prop.IsReadonly && prop.SetVisibility == ""
}

func getterSetterMethodBase(name string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range name {
		if r == '_' || r == '-' {
			upperNext = true
			continue
		}
		if upperNext {
			b.WriteRune(unicode.ToUpper(r))
			upperNext = false
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func getterSetterInsertionEdit(source, uri string, endLine int, prop parser.PropertyDef, getterName, setterName string, getterMissing, setterMissing bool) func(bool, bool) *protocol.WorkspaceEdit {
	lines := strings.Split(source, "\n")
	if endLine < 0 || endLine >= len(lines) {
		return nil
	}

	methodIndent := getterSetterIndent(lines, prop.Line)
	insertPrefix := "\n"
	if endLine > 0 && strings.TrimSpace(lines[endLine-1]) == "" {
		insertPrefix = ""
	}

	return func(includeGetter, includeSetter bool) *protocol.WorkspaceEdit {
		methods := make([]string, 0, 2)
		if includeGetter && getterMissing {
			methods = append(methods, renderGetterMethod(methodIndent, prop, getterName))
		}
		if includeSetter && setterMissing {
			methods = append(methods, renderSetterMethod(methodIndent, prop, setterName))
		}
		if len(methods) == 0 {
			return nil
		}

		return &protocol.WorkspaceEdit{
			Changes: map[string][]protocol.TextEdit{
				uri: {{
					Range: protocol.Range{
						Start: protocol.Position{Line: endLine, Character: 0},
						End:   protocol.Position{Line: endLine, Character: 0},
					},
					NewText: insertPrefix + strings.Join(methods, "\n\n") + "\n",
				}},
			},
		}
	}
}

func getterSetterIndent(lines []string, line int) string {
	if line < 0 || line >= len(lines) {
		return "    "
	}
	indent := leadingWhitespace(lines[line])
	if indent == "" {
		return "    "
	}
	return indent
}

func renderGetterMethod(indent string, prop parser.PropertyDef, getterName string) string {
	var b strings.Builder
	returnType := getterSetterTypeSuffix(prop.Type)
	field := strings.TrimPrefix(prop.Name, "$")

	fmt.Fprintf(&b, "%spublic function %s()%s {", indent, getterName, returnType)
	fmt.Fprintf(&b, "\n%s    return $this->%s;", indent, field)
	fmt.Fprintf(&b, "\n%s}", indent)
	return b.String()
}

func renderSetterMethod(indent string, prop parser.PropertyDef, setterName string) string {
	var b strings.Builder
	field := strings.TrimPrefix(prop.Name, "$")
	paramType := getterSetterParamTypePrefix(prop.Type)

	fmt.Fprintf(&b, "%spublic function %s(%s$%s): void {", indent, setterName, paramType, field)
	fmt.Fprintf(&b, "\n%s    $this->%s = $%s;", indent, field, field)
	fmt.Fprintf(&b, "\n%s}", indent)
	return b.String()
}

func getterSetterTypeSuffix(typeName string) string {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return ""
	}
	return ": " + typeName
}

func getterSetterParamTypePrefix(typeName string) string {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return ""
	}
	return typeName + " "
}
