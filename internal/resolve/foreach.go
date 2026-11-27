package resolve

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// enclosingReturnType is the declared return type of the method or function
// at pos, preferring its PHPDoc where that says more.
func (r *Resolver) enclosingReturnType(lines []string, pos textpos.Position, file *parser.FileNode) ResolvedType {
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

// resolveForeachValueType finds the nearest foreach before pos that binds
// varPrefix as its value or key variable and returns the value or key type
// of what it iterates.
func (r *Resolver) resolveForeachValueType(varPrefix string, lines []string, pos textpos.Position, file *parser.FileNode, firstLine int) ResolvedType {
	t := r.syntaxTree(lines, file)
	off, ok := t.offset(pos)
	if !ok {
		return ResolvedType{}
	}
	ctx := ExprContext{Lines: lines, Pos: pos, File: file}
	for i := len(t.foreaches) - 1; i >= 0; i-- {
		f := t.foreaches[i]
		if f.Start > off {
			continue
		}
		line, _ := t.lines.Position(f.Start)
		if line < firstLine || line < pos.Line-200 {
			break
		}
		isKey := f.Key != nil && t.isVariable(f.Key, varPrefix)
		if !isKey && !t.isVariable(f.Value, varPrefix) {
			continue
		}
		iterableType := r.iterableType(t, f.Subject, ctx)
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

func (t *syntaxTree) isVariable(e ast.Expr, name string) bool {
	v, ok := e.(*ast.Variable)
	return ok && t.text(v) == name
}

func (r *Resolver) iterableType(t *syntaxTree, e ast.Expr, ctx ExprContext) ResolvedType {
	if isMemberChain(e) {
		return r.delegatedType(t, e, ctx)
	}
	if rt := r.typeOf(t, e, nil, ctx); !rt.IsEmpty() {
		return rt
	}
	call, ok := e.(*ast.Call)
	if !ok || r.Index == nil {
		return ResolvedType{}
	}
	name, ok := call.Callee.(*ast.ConstFetch)
	if !ok {
		return ResolvedType{}
	}
	return r.FunctionTypeForCall(r.typedFunction(t.text(name), ctx.File), ctx.File, nil)
}
