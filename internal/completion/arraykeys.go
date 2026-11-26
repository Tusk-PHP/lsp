package completion

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/types"
)

type arrayKeyContext struct {
	Var        *ast.Variable
	AccessKeys []string
	Partial    string
	Quote      string
}

func arrayKeyContextOf(dim sourcectx.StringDim) (*arrayKeyContext, bool) {
	v, ok := dim.Base.(*ast.Variable)
	if !ok {
		return nil, false
	}
	return &arrayKeyContext{Var: v, AccessKeys: dim.Keys, Partial: dim.Partial, Quote: dim.Quote}, true
}

func (p *Provider) completeArrayKeys(r *resolve.Resolver, file *parser.FileNode, ctx *arrayKeyContext) []protocol.CompletionItem {
	keys := types.FieldsAt(p.variableKeys(r, file, ctx.Var), ctx.AccessKeys)
	if len(keys) == 0 {
		return nil
	}

	q := "'"
	if ctx.Quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(ctx.Partial)
	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}
		detail := k.Type
		if k.Optional {
			detail += " (optional)"
		}

		insertText := k.Key
		if ctx.Quote == "" {
			insertText = q + k.Key + q
		}

		sortText := "0" + k.Key
		if k.Optional {
			sortText = "1" + k.Key
		}

		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     detail,
			InsertText: insertText,
			SortText:   sortText,
		})
	}
	return items
}

func (p *Provider) variableKeys(r *resolve.Resolver, file *parser.FileNode, v *ast.Variable) []types.ShapeField {
	if keys := shapeFields(r.NodeType(file, v)); len(keys) > 0 {
		return keys
	}
	if value, ok := r.AssignedValue(file, v); ok {
		if keys := p.frameworkKeys(r, file, value); len(keys) > 0 {
			return keys
		}
	}
	return offsetWrites(file, v)
}

func shapeFields(rt resolve.ResolvedType) []types.ShapeField {
	fields := make([]types.ShapeField, 0, len(rt.Fields))
	for _, f := range rt.Fields {
		fields = append(fields, types.ShapeField{Key: f.Key, Type: f.Type.FullString(), Optional: f.Optional})
	}
	return fields
}

func (p *Provider) frameworkKeys(r *resolve.Resolver, file *parser.FileNode, value ast.Expr) []types.ShapeField {
	if p.arrayResolver == nil {
		return nil
	}
	src := file.Source
	switch call := value.(type) {
	case *ast.Call:
		if p.framework != "laravel" || sourcectx.FunctionName(src, call) != configHelper {
			return nil
		}
		if len(call.Args.Args) == 0 {
			return p.arrayResolver.ConfigKeys("")
		}
		if path, ok := sourcectx.StringValue(src, call.Args.Args[0].Value); ok && path != "" {
			return p.arrayResolver.ConfigKeys(path)
		}
	case *ast.MethodCall:
		return p.arrayResolver.ResolveMethodReturnKeys(r.NodeType(file, call.Object).BaseFQN(), sourcectx.MethodName(src, call))
	}
	return nil
}

func offsetWrites(file *parser.FileNode, v *ast.Variable) []types.ShapeField {
	src := file.Source
	name := v.Bounds().Text(src)
	body := enclosingFunction(file.Tree, v.Start)
	var keys []types.ShapeField
	seen := make(map[string]bool)
	ast.Walk(body, func(n ast.Node) bool {
		if n != body && isFunctionLike(n) {
			return false
		}
		assign, ok := n.(*ast.Assign)
		if !ok || assign.Op != token.Equals {
			return true
		}
		dim, ok := assign.Target.(*ast.ArrayDimFetch)
		if !ok {
			return true
		}
		if target, ok := dim.Array.(*ast.Variable); !ok || target.Bounds().Text(src) != name {
			return true
		}
		if key, ok := sourcectx.StringValue(src, dim.Dim); ok && !seen[key] {
			seen[key] = true
			keys = append(keys, types.ShapeField{Key: key})
		}
		return true
	})
	return keys
}

func enclosingFunction(root ast.Node, off uint32) ast.Node {
	found := root
	ast.Walk(root, func(n ast.Node) bool {
		span := n.Bounds()
		if off < span.Start || off > span.End {
			return false
		}
		if isFunctionLike(n) {
			found = n
		}
		return true
	})
	return found
}

func isFunctionLike(n ast.Node) bool {
	switch n.(type) {
	case *ast.FunctionDecl, *ast.Method, *ast.Closure, *ast.ArrowFunction:
		return true
	}
	return false
}
