package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// enclosingReturnType is the declared return type of the method or function
// at pos, preferring its PHPDoc where that says more.
func (r *Resolver) enclosingReturnType(lines []string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	if file == nil {
		return ResolvedType{}
	}
	native, doc := "", ""
	owner := FindEnclosingClass(file, pos)
	if m := FindEnclosingMethod(file, pos); m != nil && (m.EndLine == 0 || pos.Line <= m.EndLine) {
		native, doc = m.ReturnType.Name, m.DocComment
	} else {
		owner = ""
		for i := range file.Functions {
			fn := &file.Functions[i]
			if pos.Line >= fn.StartLine && (fn.EndLine == 0 || pos.Line <= fn.EndLine) {
				native, doc = fn.ReturnType.Name, fn.DocComment
			}
		}
	}
	docType := ""
	if d := parser.ParseDocBlock(doc); d != nil {
		docType = d.Return.Type
	}
	raw := r.preferDocType(native, docType, nil, owner)
	if raw == "" {
		return ResolvedType{}
	}
	return r.resolveDocTypeIn(raw, owner, file)
}

// resolveForeachValueType looks backward for a foreach loop that binds varPrefix
// as its value or key variable and returns the value or key type of what it
// iterates. Handles both "foreach ($x as $v)" and "foreach ($x as $k => $v)".
func (r *Resolver) resolveForeachValueType(varPrefix string, lines []string, pos protocol.Position, file *parser.FileNode, firstLine int) ResolvedType {
	var source string
	for i := pos.Line; i >= firstLine && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		line := strings.TrimSpace(lines[i])
		if !strings.Contains(line, "foreach") || !strings.Contains(line, " as ") {
			continue
		}
		foreachIdx := strings.Index(line, "foreach")
		openParen := strings.IndexByte(line[foreachIdx:], '(')
		if openParen < 0 {
			continue
		}
		openParen += foreachIdx
		asIdx := strings.Index(line[openParen:], " as ")
		if asIdx < 0 {
			continue
		}
		asIdx += openParen
		afterAs := line[asIdx+4:]
		keyVar, valueVar := "", ""
		if arrowIdx := strings.Index(afterAs, "=>"); arrowIdx >= 0 {
			keyVar = strings.TrimSpace(afterAs[:arrowIdx])
			valueVar = strings.TrimSpace(afterAs[arrowIdx+2:])
		} else {
			valueVar = strings.TrimSpace(afterAs)
		}
		valueVar = strings.TrimPrefix(strings.TrimRight(valueVar, ") {:;\t "), "&")
		isKey := keyVar == varPrefix
		if valueVar != varPrefix && !isKey {
			continue
		}
		iterableExpr := strings.TrimSpace(line[openParen+1 : asIdx])
		if iterableExpr == "" {
			continue
		}

		var iterableType ResolvedType
		hasChain := strings.Contains(iterableExpr, "->") || strings.Contains(iterableExpr, "::")
		if source == "" && (hasChain || r.TypedChainResolver != nil) {
			source = r.JoinLines(lines)
		}
		switch {
		case r.TypedChainResolver != nil && hasChain:
			iterableType = r.TypedChainResolver(iterableExpr, source, pos, file)
		case r.ChainResolver != nil && hasChain:
			if t := r.ChainResolver(iterableExpr, source, pos, file); t != "" {
				iterableType = ResolvedType{FQN: t}
			}
		case strings.HasPrefix(iterableExpr, "$") && extractLeadingVariable(iterableExpr) == iterableExpr:
			iterableType = r.ResolveVariableTypeTyped(iterableExpr, lines, pos, file)
		default:
			iterableType = r.ExpressionType(iterableExpr, ExprContext{Lines: lines, Pos: pos, File: file})
			if iterableType.IsEmpty() {
				if funcName := extractFunctionCallName(iterableExpr); funcName != "" && r.Index != nil {
					iterableType = r.resolveFunctionCallReturnType(funcName, file)
				}
			}
		}
		if iterableType.IsEmpty() {
			continue
		}
		if k, v, ok := r.IterableTypes(iterableType); ok {
			if isKey {
				return k
			}
			return v
		}
		if isKey {
			continue
		}
		return arrayElementType(iterableType)
	}
	return ResolvedType{}
}
